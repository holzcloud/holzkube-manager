import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createRoute } from '@tanstack/react-router'
import { useCallback, useEffect, useRef, useState } from 'react'
import {
  api,
  type CreatedSchematic,
  type MetaValue,
  ProblemError,
  type Schematic,
  type SchematicInput,
} from '@/api'
import {
  LiveSchematicWarnings,
  predictWarnings,
  SchematicWarnings,
} from '@/components/SchematicWarnings'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { authenticatedRoute } from '@/routes/__root'

/**
 * The Images screen: assembling an Image Factory schematic, and reading back
 * what it actually produces.
 *
 * Three things here are requirements rather than design choices.
 *
 * The extension picker is a list of checkboxes over the catalog fetched for the
 * selected Talos version, and there is deliberately **no text field for an
 * extension name** — that is what FACT-01's "kein Freitextfeld" means. An
 * extension name typed by hand is a name the Factory accepts, assigns an
 * ordinary id to, and refuses only when an image is finally requested.
 *
 * The version selector defaults to the newest *stable* version and never to the
 * last element of the upstream list. The upstream list is served ascending and
 * ends in the current alpha, beta and rc tags, so "newest" and "last" are
 * different answers and only one of them is safe. Prereleases are behind an
 * explicit opt-in; a version in the curated broken table is shown disabled with
 * the reason it is listed, because a greyed-out control with no stated cause is
 * one an operator cannot judge.
 *
 * Kernel arguments and META entries are free-form by nature — they are
 * operator-authored values, not names from a catalog — and typing one raises
 * the installer/initramfs warning *live*, before the schematic is created.
 */

export type MetaRow = { key: number; value: string }

/**
 * Whether a value carries a character holzkube's canonical serialiser refuses.
 *
 * The original rule lives in `internal/imagefactory/schematicid.go`'s
 * `representable`: not valid UTF-8, any rune below U+0020, or U+007F. This is
 * the control-character half of it, transcribed. A lone surrogate — the invalid
 * UTF-8 half — is unreachable from a normal input event and is left to the
 * server's 400.
 *
 * The server's 400 is the backstop here, not the fallback. Refusing at the input
 * is what tells an operator *which row* is wrong while they are still looking at
 * it; the server answering afterwards is the guarantee that a value which slips
 * past this never reaches the Factory.
 *
 * It reports rather than strips. An operator who pasted a value from somewhere
 * is better served by being told than by having their input quietly rewritten
 * into something they did not type (T-02-67).
 */
/**
 * The row-level message. It names the character class and never the value, for
 * the same reason the server's problem body does not: kernel arguments and META
 * values can carry secrets (T-02-64).
 */
const CONTROL_CHARACTER_MESSAGE =
  'This contains a control character. The Image Factory schematic cannot carry one, so remove it before creating.'

export function hasControlCharacter(value: string): boolean {
  for (const character of value) {
    const code = character.codePointAt(0) ?? 0
    if (code < 0x20 || code === 0x7f) {
      return true
    }
  }
  return false
}

const ARCHITECTURES = ['amd64', 'arm64'] as const
export type Architecture = (typeof ARCHITECTURES)[number]

function ImagesView() {
  const queryClient = useQueryClient()

  const [name, setName] = useState('')
  const [chosenVersion, setChosenVersion] = useState<string | null>(null)
  const [showPrerelease, setShowPrerelease] = useState(false)
  const [arch, setArch] = useRememberedArch()
  const [secureBoot, setSecureBoot] = useState(false)
  const [extensions, setExtensions] = useState<string[]>([])
  const [kernelArgs, setKernelArgs] = useState<string[]>([])
  const [meta, setMeta] = useState<MetaRow[]>([])
  const [dropped, setDropped] = useState<string[]>([])
  const [created, setCreated] = useState<CreatedSchematic | null>(null)
  const [selected, setSelected] = useState<string | null>(null)

  const versions = useQuery({
    queryKey: ['factory', 'versions'],
    queryFn: () => api.factory.versions(),
  })

  // The default is derived rather than written into state by an effect: the
  // newest stable version is a fact about the server's answer, and copying it
  // into state would leave a window in which the control shows something the
  // server did not say.
  //
  // The empty string is treated as "nothing chosen" and not as a choice. The
  // picker is mounted before the version list has arrived, and a Select that
  // reports its own empty starting value back as a selection would otherwise
  // pin the screen to no version at all -- with the list on screen, the default
  // never applied, and no error anywhere.
  const version =
    chosenVersion !== null && chosenVersion !== ''
      ? chosenVersion
      : (versions.data?.newest_stable ?? '')

  const catalog = useQuery({
    queryKey: ['factory', 'extensions', version],
    queryFn: () => api.factory.extensions(version),
    enabled: version !== '',
  })

  const catalogData = catalog.data

  // An extension valid at one Talos version may not exist at another. Carrying
  // a selection across a version change silently produces a schematic that is
  // un-buildable at exactly the moment it matters, so the absent ones are
  // dropped and reported by name rather than sent to the Factory.
  useEffect(() => {
    if (catalogData === undefined) {
      return
    }
    const available = new Set(catalogData.extensions.map((extension) => extension.name))
    const absent = extensions.filter((selected) => !available.has(selected))
    if (absent.length === 0) {
      return
    }
    setExtensions((previous) => previous.filter((selected) => available.has(selected)))
    setDropped(absent)
  }, [catalogData, extensions])

  const create = useMutation({
    mutationFn: (input: SchematicInput) => api.schematics.create(input),
    onSuccess: async (result) => {
      setCreated(result)
      await queryClient.invalidateQueries({ queryKey: ['schematics'] })
    },
  })

  // A create error belongs to the request it came from. G-02-6 watched one stay
  // on screen through subsequent unrelated interactions, so an operator editing
  // the form was reading a sentence about a submission they had already
  // replaced.
  //
  // The fingerprint is exactly the fields the request is built from, and the
  // mutation's own state is deliberately not among them: an effect that
  // depended on it would be its own cause. The reset itself goes through a ref
  // for the same reason -- the mutation object is re-created on every state
  // change it makes.
  const requestFingerprint = JSON.stringify([
    name,
    version,
    arch,
    extensions,
    kernelArgs,
    meta,
    secureBoot,
  ])
  const submittedFingerprint = useRef(requestFingerprint)
  const resetCreate = useRef(create.reset)
  resetCreate.current = create.reset
  useEffect(() => {
    if (submittedFingerprint.current === requestFingerprint) {
      return
    }
    submittedFingerprint.current = requestFingerprint
    resetCreate.current()
  }, [requestFingerprint])

  const toggleExtension = useCallback((extensionName: string) => {
    setExtensions((previous) =>
      previous.includes(extensionName)
        ? previous.filter((each) => each !== extensionName)
        : [...previous, extensionName],
    )
  }, [])

  // Computed once here and passed down, rather than duplicated inside both row
  // components: the rule has one home, and "is anything offending" is one
  // question rather than two that could disagree.
  const kernelArgErrors = kernelArgs.map((value) =>
    hasControlCharacter(value) ? CONTROL_CHARACTER_MESSAGE : null,
  )
  const metaErrors = meta.map((row) =>
    hasControlCharacter(row.value) ? CONTROL_CHARACTER_MESSAGE : null,
  )
  const hasUnusableValue =
    kernelArgErrors.some((each) => each !== null) || metaErrors.some((each) => each !== null)

  const submit = useCallback(() => {
    create.mutate({
      name,
      talos_version: version,
      arch,
      // Only names that came out of the fetched catalog can be here: the
      // control offers nothing else, and the server rejects anything else.
      extensions,
      kernel_args: kernelArgs.map((arg) => arg.trim()).filter((arg) => arg !== ''),
      meta: meta
        .filter((row) => row.value.trim() !== '')
        .map((row): MetaValue => ({ key: row.key, value: row.value })),
      secureboot: secureBoot,
    })
  }, [create, name, version, arch, extensions, kernelArgs, meta, secureBoot])

  const brokenReason = versions.data?.broken[version]

  return (
    <div className="space-y-6">
      <div>
        <h1 className="font-heading text-2xl font-semibold tracking-tight">Images</h1>
        <p className="text-sm text-muted-foreground">
          Assemble an Image Factory schematic and read the exact URLs it produces.
        </p>
      </div>

      <form
        className="space-y-6"
        onSubmit={(event) => {
          event.preventDefault()
          submit()
        }}
      >
        <div className="flex flex-wrap items-end gap-3">
          <div className="flex flex-col gap-1">
            <Label htmlFor="schematic-name">Name</Label>
            <Input
              id="schematic-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="workers with intel microcode"
              className="w-72"
            />
          </div>

          <div className="flex flex-col gap-1">
            <Label htmlFor="schematic-version">Talos version</Label>
            <Select
              value={version}
              onValueChange={(value) => {
                // The report below belongs to the change that produced it. A
                // list of names dropped two versions ago, still on screen after
                // a version where nothing was dropped, is worse than no report.
                setDropped([])
                setChosenVersion(value)
              }}
            >
              <SelectTrigger id="schematic-version" className="h-8 w-56">
                <SelectValue placeholder="Select a version" />
              </SelectTrigger>
              <SelectContent>
                {(versions.data?.stable ?? []).map((entry) => (
                  <VersionOption
                    key={entry}
                    version={entry}
                    reason={versions.data?.broken[entry]}
                  />
                ))}
                {showPrerelease &&
                  (versions.data?.prerelease ?? []).map((entry) => (
                    <VersionOption
                      key={entry}
                      version={entry}
                      reason={versions.data?.broken[entry]}
                      prerelease
                    />
                  ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-col gap-1">
            <Label htmlFor="schematic-arch">Architecture</Label>
            <Select value={arch} onValueChange={(value) => setArch(value as Architecture)}>
              <SelectTrigger id="schematic-arch" className="h-8 w-32">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {ARCHITECTURES.map((entry) => (
                  <SelectItem key={entry} value={entry}>
                    {entry}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <label className="flex items-center gap-2 pb-1 text-sm">
            <input
              type="checkbox"
              checked={showPrerelease}
              onChange={(event) => setShowPrerelease(event.target.checked)}
            />
            Show pre-release versions
          </label>

          <label className="flex items-center gap-2 pb-1 text-sm">
            <input
              type="checkbox"
              checked={secureBoot}
              onChange={(event) => setSecureBoot(event.target.checked)}
            />
            SecureBoot
          </label>
        </div>

        {brokenReason !== undefined && (
          <p role="alert" className="text-sm text-destructive">
            {version} is listed as broken: {brokenReason}
          </p>
        )}

        <section className="space-y-2">
          <h2 className="text-sm font-medium">System extensions</h2>
          <p className="text-xs text-muted-foreground">
            The official catalog for {version === '' ? 'the selected version' : version}. The list
            is version-scoped: an extension that exists at one Talos version need not exist at
            another, which is why there is no free-text field here.
          </p>

          {dropped.length > 0 && (
            <p role="alert" className="text-sm text-destructive">
              Removed from the selection because the catalog for {version} does not list them:{' '}
              {dropped.join(', ')}.
            </p>
          )}

          {catalog.isPending && version !== '' && (
            <p className="text-sm text-muted-foreground">Loading the catalog…</p>
          )}

          {catalog.isError && (
            <p role="alert" className="text-sm text-destructive">
              The extension catalog for {version} could not be fetched. Nothing is offered rather
              than a list from another version — one of those validates successfully and produces an
              image that will not build.
            </p>
          )}

          {catalog.isSuccess && (
            <fieldset className="grid gap-1" aria-label="System extensions">
              {catalog.data.extensions.map((extension) => (
                <label key={extension.name} className="flex items-start gap-2 text-sm">
                  <input
                    type="checkbox"
                    className="mt-1"
                    checked={extensions.includes(extension.name)}
                    onChange={() => toggleExtension(extension.name)}
                  />
                  <span>
                    <span className="font-mono text-xs">{extension.name}</span>
                    {extension.description !== '' && (
                      <span className="block text-xs text-muted-foreground">
                        {extension.description}
                      </span>
                    )}
                  </span>
                </label>
              ))}
            </fieldset>
          )}
        </section>

        <RepeatableRows
          title="Extra kernel arguments"
          addLabel="Add kernel argument"
          values={kernelArgs}
          onChange={setKernelArgs}
          placeholder="console=ttyS0"
          inputLabel="Kernel argument"
          errors={kernelArgErrors}
        />

        <MetaRows meta={meta} onChange={setMeta} errors={metaErrors} />

        <LiveSchematicWarnings kernelArgs={kernelArgs} meta={meta} />

        {create.isError && (
          <p role="alert" aria-label="Create failed" className="text-sm text-destructive">
            {create.error instanceof Error
              ? create.error.message
              : 'The schematic was not created.'}
          </p>
        )}

        <Button
          type="submit"
          disabled={create.isPending || name === '' || version === '' || hasUnusableValue}
        >
          {create.isPending ? 'Creating…' : 'Create schematic'}
        </Button>
      </form>

      {created !== null && (
        <CreatedPanel created={created} onOpen={() => setSelected(created.id)} />
      )}

      <SavedSchematics onOpen={setSelected} />

      {/*
        The remembered architecture goes down as a starting value and nothing
        comes back up. See AssetPanel for why that direction is the whole
        point.
      */}
      <SchematicDetail id={selected} archSeed={arch} onClose={() => setSelected(null)} />
    </div>
  )
}

/**
 * One version in the picker.
 *
 * A version in the curated broken table is offered as a disabled item carrying
 * the reason it is listed. Hiding it would leave an operator wondering where a
 * version went; greying it out without a reason would leave them with a dead
 * control and no way to judge whether the reason still applies to them.
 */
function VersionOption({
  version,
  reason,
  prerelease,
}: {
  version: string
  reason?: string
  prerelease?: boolean
}) {
  const broken = reason !== undefined
  return (
    <SelectItem value={version} disabled={broken}>
      <span className="flex flex-col">
        <span>
          {version}
          {prerelease === true && ' (pre-release)'}
        </span>
        {broken && <span className="text-xs text-muted-foreground">Broken: {reason}</span>}
      </span>
    </SelectItem>
  )
}

/** A repeatable free-text row with an add and a remove control. */
function RepeatableRows({
  title,
  addLabel,
  values,
  onChange,
  placeholder,
  inputLabel,
  errors,
}: {
  title: string
  addLabel: string
  values: string[]
  onChange: (next: string[]) => void
  placeholder: string
  inputLabel: string
  /** Per-row message, or null for a row with nothing wrong. Computed by the
   * caller so the rule that produces it has exactly one home. */
  errors?: (string | null)[]
}) {
  return (
    <section className="space-y-2">
      <h2 className="text-sm font-medium">{title}</h2>
      {values.map((value, index) => (
        // The index is the identity here: these rows have no key of their own
        // and reordering is not offered, so nothing can go stale.
        // biome-ignore lint/suspicious/noArrayIndexKey: rows are positional
        <div key={index} className="space-y-1">
          <div className="flex items-center gap-2">
            <Input
              aria-label={`${inputLabel} ${index + 1}`}
              value={value}
              placeholder={placeholder}
              className="w-96"
              onChange={(event) => {
                const next = [...values]
                next[index] = event.target.value
                onChange(next)
              }}
            />
            <Button
              type="button"
              variant="ghost"
              onClick={() => onChange(values.filter((_, each) => each !== index))}
            >
              Remove
            </Button>
          </div>
          <RowProblem label={`${inputLabel} ${index + 1}`} message={errors?.[index] ?? null} />
        </div>
      ))}
      <Button type="button" variant="secondary" onClick={() => onChange([...values, ''])}>
        {addLabel}
      </Button>
    </section>
  )
}

/**
 * A row-level refusal, in the register the rest of the screen uses for one: an
 * alert next to the row it belongs to, named after that row so a screen reader
 * and a test both address the same thing.
 */
function RowProblem({ label, message }: { label: string; message: string | null }) {
  if (message === null) {
    return null
  }
  return (
    <p role="alert" aria-label={`${label} is not usable`} className="text-sm text-destructive">
      {message}
    </p>
  )
}

function MetaRows({
  meta,
  onChange,
  errors,
}: {
  meta: MetaRow[]
  onChange: (next: MetaRow[]) => void
  errors?: (string | null)[]
}) {
  return (
    <section className="space-y-2">
      <h2 className="text-sm font-medium">META values</h2>
      {meta.map((row, index) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: rows are positional
        <div key={index} className="space-y-1">
          <div className="flex items-center gap-2">
            <Input
              aria-label={`META key ${index + 1}`}
              type="number"
              min={0}
              max={255}
              value={row.key}
              className="w-24"
              onChange={(event) => {
                const next = [...meta]
                next[index] = { ...row, key: Number(event.target.value) }
                onChange(next)
              }}
            />
            <Input
              aria-label={`META value ${index + 1}`}
              value={row.value}
              className="w-96"
              onChange={(event) => {
                const next = [...meta]
                next[index] = { ...row, value: event.target.value }
                onChange(next)
              }}
            />
            <Button
              type="button"
              variant="ghost"
              onClick={() => onChange(meta.filter((_, each) => each !== index))}
            >
              Remove
            </Button>
          </div>
          <RowProblem label={`META value ${index + 1}`} message={errors?.[index] ?? null} />
        </div>
      ))}
      <Button
        type="button"
        variant="secondary"
        onClick={() => onChange([...meta, { key: 0, value: '' }])}
      >
        Add META value
      </Button>
    </section>
  )
}

/**
 * The create result.
 *
 * "Created" and "usable" are two different facts and this panel keeps them
 * apart. The Factory assigns an id to a schematic naming an extension that does
 * not exist; the refusal arrives only when an image is requested. A panel that
 * merged the two would be the exact lie the probe exists to prevent.
 */
function CreatedPanel({ created, onOpen }: { created: CreatedSchematic; onOpen: () => void }) {
  return (
    <section className="space-y-3 rounded-md border border-border p-4">
      <div className="flex flex-wrap items-center gap-3">
        <p className="text-sm font-medium">Schematic created.</p>
        <UsabilityBadge
          usable={created.usable}
          probedAt={created.probed_at}
          reason={created.probe_reason}
        />
      </div>
      <p className="break-all font-mono text-xs">{created.id}</p>
      <SchematicWarnings warnings={created.warnings} />
      <Button type="button" variant="secondary" onClick={onOpen}>
        Open this schematic
      </Button>
    </section>
  )
}

/** The zero time means the probe never answered, which is not a refusal. */
export function isProbed(probedAt: string): boolean {
  return probedAt !== '' && !probedAt.startsWith('0001-01-01')
}

/**
 * Three states, never two.
 *
 * "Usable", "the Factory refused" and "nobody asked" are three different
 * situations with three different repairs, and collapsing the last two would
 * recreate the exact lie FACT-02 exists to prevent: an unverified schematic
 * shown as a broken one sends an operator to fix something nothing has found
 * fault with, and shown as a good one sends them to install from it.
 */
export function UsabilityBadge({
  usable,
  probedAt,
  reason,
  arch,
}: {
  usable: boolean
  probedAt: string
  reason?: string
  /**
   * The architecture the verdict is about (model.Schematic.arch). Empty or
   * absent means a record written before the field existed, and such a record
   * renders exactly the verdict below with nothing added: the architecture a
   * past probe used is not recoverable, and a qualifier nobody measured would
   * be the same lie G-02-8 is about, in a new place.
   */
  arch?: string
}) {
  const verdict = <UsabilityVerdict usable={usable} probedAt={probedAt} reason={reason} />
  if (arch === undefined || arch === '') {
    return verdict
  }
  return (
    <span className="flex flex-wrap items-center gap-2">
      {verdict}
      <span className="text-xs text-muted-foreground">architecture: {arch}</span>
    </span>
  )
}

/**
 * The verdict itself, unqualified.
 *
 * The architecture is rendered beside this by UsabilityBadge rather than spliced
 * into any of the three sentences, for three reasons. Three sentences would
 * otherwise become six. ProbeReason already names the architecture inside the
 * refusal text, so the refused branch would say it twice. And
 * 02-DECISION-probe-budget.md Option 1 would rewrite these sentences again -- a
 * qualifier that composes with whatever they say survives that rewrite
 * untouched, which is also why plan 02-13 runs after 02-12 rather than editing
 * the same copy twice.
 */
function UsabilityVerdict({
  usable,
  probedAt,
  reason,
}: {
  usable: boolean
  probedAt: string
  reason?: string
}) {
  if (usable) {
    return <Badge variant="secondary">Usable — the build probe confirmed it</Badge>
  }
  if (!isProbed(probedAt)) {
    // G-02-1. This used to read "the build probe did not run", which is a claim
    // the record cannot support: on the measured common case the probe ran for a
    // full thirty seconds and gave up, and the record looks identical either
    // way. The badge now asserts only what is true in both cases -- there is no
    // verdict -- and the muted line states the disjunction rather than picking
    // one.
    //
    // This is a copy change and only a copy change. No new field, no third
    // state, no change to the probed-or-not predicate above, and no change to
    // what the server stores.
    // 02-DECISION-probe-budget.md owns the state question and is still open;
    // this item is on its unconditional list precisely because it needs none of
    // that. If closing it ever seems to need a third probe state, stop — the
    // boundary has been crossed and the decision has to land first.
    return (
      <span className="flex flex-col gap-1">
        <Badge variant="outline">Not verified — the build probe has no verdict</Badge>
        <span className="text-xs text-muted-foreground">
          The probe either did not run or did not answer in time. The schematic may still be
          buildable.
        </span>
      </span>
    )
  }
  return (
    <span className="flex flex-col gap-1">
      <Badge variant="destructive">Not usable — the Factory refused to build it</Badge>
      {reason !== undefined && reason !== '' && (
        <span className="text-xs text-muted-foreground">{reason}</span>
      )}
    </span>
  )
}

/**
 * The architecture the operator last chose to *build for*.
 *
 * This belongs to the creation form alone. The asset panel keeps its own, so
 * that inspecting a saved schematic at another architecture cannot change what
 * the next schematic is created and probed against.
 *
 * There is no sensible default. holzkube is developed on arm64 and targets
 * amd64, so a hardcoded one is a bug that only ever appears on someone else's
 * machine -- and asking again on every visit is a control an operator has
 * already answered. The last answer is the least wrong starting point, and it
 * is a preference rather than a secret: nothing security-relevant is stored.
 */
export const ARCH_STORAGE_KEY = 'holzkube.images.arch'

function isArchitecture(value: unknown): value is Architecture {
  return value === 'amd64' || value === 'arm64'
}

function useRememberedArch(): [Architecture, (next: Architecture) => void] {
  const [arch, setArchState] = useState<Architecture>(() => {
    try {
      const raw = localStorage.getItem(ARCH_STORAGE_KEY)
      return isArchitecture(raw) ? raw : 'amd64'
    } catch {
      // A browser with storage disabled still gets a working screen; it just
      // cannot remember the choice across a reload.
      return 'amd64'
    }
  })

  const setArch = useCallback((next: Architecture) => {
    setArchState(next)
    try {
      localStorage.setItem(ARCH_STORAGE_KEY, next)
    } catch {
      // Same as above: remembering is a convenience, not a requirement.
    }
  }, [])

  return [arch, setArch]
}

/**
 * The saved schematics, in the dense-table register D-13 says later phases
 * inherit from the audit screen.
 *
 * The usability column is the reason this table is worth having at all. A list
 * that showed only names and versions would let an operator pick the one
 * schematic in it that will not build.
 */
function SavedSchematics({ onOpen }: { onOpen: (id: string) => void }) {
  const saved = useQuery({
    queryKey: ['schematics'],
    queryFn: () => api.schematics.list(),
  })

  return (
    <section className="space-y-2">
      <h2 className="font-heading text-lg font-semibold tracking-tight">Saved schematics</h2>

      {saved.isPending && <p className="text-sm text-muted-foreground">Loading…</p>}

      {saved.isError && (
        <p role="alert" className="text-sm text-destructive">
          The saved schematics could not be loaded.
        </p>
      )}

      {saved.isSuccess && saved.data.length === 0 && (
        <p className="text-sm text-muted-foreground">
          No schematics yet. The Image Factory offers no way to list them back, so a schematic that
          is not saved here is a reference nothing can recover.
        </p>
      )}

      {saved.isSuccess && saved.data.length > 0 && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Talos version</TableHead>
              <TableHead>Extensions</TableHead>
              <TableHead>Created</TableHead>
              <TableHead>Usability</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {saved.data.map((record) => (
              <TableRow
                key={record.id}
                onClick={() => onOpen(record.id)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter' || event.key === ' ') {
                    event.preventDefault()
                    onOpen(record.id)
                  }
                }}
                tabIndex={0}
                role="button"
                aria-label={`Schematic ${record.name}`}
                className="cursor-pointer"
              >
                <TableCell>{record.name}</TableCell>
                <TableCell className="tabular-nums">{record.talos_version}</TableCell>
                <TableCell className="tabular-nums">{record.extensions.length}</TableCell>
                <TableCell className="tabular-nums">{record.created_at}</TableCell>
                <TableCell>
                  <UsabilityBadge
                    usable={record.usable}
                    probedAt={record.probed_at}
                    reason={record.probe_reason}
                  />
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </section>
  )
}

/**
 * One saved schematic in full.
 *
 * The id is shown complete and unabbreviated: it is the value an operator
 * pastes into a machine config, and a truncated one is worse than none because
 * it looks copyable.
 */
function SchematicDetail({
  id,
  archSeed,
  onClose,
}: {
  id: string | null
  archSeed: Architecture
  onClose: () => void
}) {
  const queryClient = useQueryClient()

  const record = useQuery({
    queryKey: ['schematics', id],
    queryFn: () => api.schematics.get(id ?? ''),
    enabled: id !== null,
  })

  const remove = useMutation({
    mutationFn: () => api.schematics.remove(id ?? ''),
    onSuccess: async () => {
      onClose()
      await queryClient.invalidateQueries({ queryKey: ['schematics'] })
    },
  })

  // A detail fetch that answered `notfound.schematic` is the one failure that
  // says something about the *list*: the row just clicked refers to a record
  // the server no longer has, so the list is asked again and the stale row
  // leaves.
  //
  // No other code does this. A transport failure or a 500 says nothing about
  // whether the record still exists, and dropping the row on one would remove
  // the operator's only recoverable reference to an id -- the Image Factory
  // will not enumerate schematics -- at exactly the moment the server is
  // unwell.
  //
  // The guard is a ref and the invalidation is `exact`, which together make a
  // loop impossible: `exact` leaves this dialog's own ['schematics', id] query
  // untouched, so nothing this effect causes appears in its dependencies, and
  // the ref bounds it to once per failed id regardless.
  const invalidatedFor = useRef<string | null>(null)
  const failure = record.error
  useEffect(() => {
    if (id === null || !(failure instanceof ProblemError)) {
      return
    }
    if (failure.code !== 'notfound.schematic' || invalidatedFor.current === id) {
      return
    }
    invalidatedFor.current = id
    void queryClient.invalidateQueries({ queryKey: ['schematics'], exact: true })
  }, [id, failure, queryClient])

  return (
    <Dialog
      open={id !== null}
      onOpenChange={(open) => {
        if (!open) {
          onClose()
        }
      }}
    >
      <DialogContent className="max-h-[85vh] max-w-3xl overflow-y-auto">
        {record.isSuccess && (
          <SchematicDetailBody
            record={record.data}
            archSeed={archSeed}
            onDelete={() => remove.mutate()}
            deleting={remove.isPending}
            deleteError={remove.isError}
          />
        )}
        {record.isError && id !== null && <SchematicDetailUnavailable error={record.error} />}
        {record.isPending && id !== null && (
          <DialogHeader>
            <DialogTitle>Schematic</DialogTitle>
            <DialogDescription>Loading…</DialogDescription>
          </DialogHeader>
        )}
      </DialogContent>
    </Dialog>
  )
}

/**
 * Why a schematic cannot be shown.
 *
 * Two reasons, told apart by the problem `code` and never by the message text,
 * because they carry different repairs. A record that is gone is finished
 * business; a store or transport failure is a retry. Conflating them either
 * sends an operator hunting for a schematic that is still there, or tells them
 * one was deleted when only the connection to the store failed.
 *
 * The alert is inside the dialog's own header so the modal keeps an accessible
 * name. The failure this replaces opened a dialog whose entire text content was
 * the word "Close".
 */
function SchematicDetailUnavailable({ error }: { error: unknown }) {
  const gone = error instanceof ProblemError && error.code === 'notfound.schematic'

  return (
    <>
      <DialogHeader>
        <DialogTitle>
          {gone ? 'This schematic is gone' : 'This schematic could not be loaded'}
        </DialogTitle>
        <DialogDescription>
          {gone
            ? 'The server has no record with this id.'
            : 'The server did not answer with the record. It may still exist.'}
        </DialogDescription>
      </DialogHeader>

      <p role="alert" className="text-sm text-destructive">
        {gone
          ? 'This schematic is no longer stored. It was most likely removed in another tab or by another session, and it has been dropped from the saved list.'
          : 'The schematic could not be loaded. Nothing here says it is gone — the store or the connection to it failed — so the row has been left in the saved list.'}
      </p>

      {gone && (
        <p className="text-sm text-muted-foreground">
          The id is only recoverable from a stored record: the Image Factory will not list
          schematics back. If this one is still needed, it has to be created again.
        </p>
      )}
    </>
  )
}

function SchematicDetailBody({
  record,
  archSeed,
  onDelete,
  deleting,
  deleteError,
}: {
  record: Schematic
  archSeed: Architecture
  onDelete: () => void
  deleting: boolean
  deleteError: boolean
}) {
  return (
    <>
      <DialogHeader>
        <DialogTitle>{record.name}</DialogTitle>
        <DialogDescription>
          Authored against {record.talos_version}. The id below is the SHA-256 of the Factory's own
          canonical document, and it is what a machine config refers to.
        </DialogDescription>
      </DialogHeader>

      <div className="flex flex-wrap items-center gap-3">
        <UsabilityBadge
          usable={record.usable}
          probedAt={record.probed_at}
          reason={record.probe_reason}
          arch={record.arch}
        />
      </div>

      <div>
        <h3 className="mb-1 text-sm font-medium">Schematic ID</h3>
        <div className="flex items-start gap-2">
          <p className="break-all font-mono text-xs">{record.id}</p>
          <CopyButton label="Schematic ID" value={record.id} />
        </div>
      </div>

      <SchematicWarnings warnings={predictWarnings(record.kernel_args, record.meta)} />

      <AssetPanel record={record} archSeed={archSeed} />

      <div>
        <h3 className="mb-1 text-sm font-medium">Factory-canonical document</h3>
        <pre className="overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs">
          {record.canonical}
        </pre>
      </div>

      {deleteError && (
        <p role="alert" className="text-sm text-destructive">
          The schematic was not deleted.
        </p>
      )}

      {/*
        No confirmation of this screen's own. DELETE is Destructive, so the
        server answers 428 without an open sudo window and the existing sudo
        dialog replays the request. A second "are you sure?" in front of it
        would train the operator to click past the first one.
      */}
      <Button type="button" variant="destructive" disabled={deleting} onClick={onDelete}>
        {deleting ? 'Deleting…' : 'Delete schematic'}
      </Button>
    </>
  )
}

/**
 * The asset references for a chosen architecture.
 *
 * The installer reference sits next to the ISO URL on purpose. An ISO built
 * from one schematic and an installer taken from another is the documented
 * drift (PITFALLS P9): the machine boots with every extension and then installs
 * a system without them, and nothing reports it.
 */
function AssetPanel({ record, archSeed }: { record: Schematic; archSeed: Architecture }) {
  // The architecture lives here, beside SecureBoot, and is seeded from the
  // remembered value rather than bound to it.
  //
  // The next control added to this panel will face the same temptation, so:
  // the remembered architecture is a preference about what the operator
  // *builds*, and reading somebody else's saved schematic at another
  // architecture is a *question*. Letting the answer to a question rewrite a
  // preference is how an operator ends up probing amd64 hardware against an
  // arm64 image without ever having chosen it -- and the record carries no
  // architecture, so nothing downstream can detect the mismatch.
  //
  // That last clause is the boundary of this change. Persisting the probed
  // architecture on the record and rendering the verdict as arch-qualified is
  // planned in phase 02 plan 13, after plan 12 has rewritten the verdict
  // sentence an architecture would qualify. It is sequenced, not dropped.
  //
  // No reset effect is needed: Radix unmounts dialog content on close, so
  // reopening a schematic re-seeds this from the remembered value. The
  // regression test in images.test.tsx asserts exactly that.
  const [arch, setArch] = useState<Architecture>(archSeed)
  const [secureBoot, setSecureBoot] = useState(false)

  const assets = useQuery({
    queryKey: ['schematics', record.id, 'assets', arch, secureBoot],
    queryFn: () => api.schematics.assets(record.id, { arch, secureboot: secureBoot }),
  })

  return (
    <section className="space-y-3">
      <h3 className="text-sm font-medium">Assets</h3>

      <div className="flex flex-wrap items-end gap-3">
        <div className="flex flex-col gap-1">
          <Label htmlFor="asset-arch">Architecture</Label>
          <Select value={arch} onValueChange={(value) => setArch(value as Architecture)}>
            <SelectTrigger id="asset-arch" className="h-8 w-32">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {ARCHITECTURES.map((entry) => (
                <SelectItem key={entry} value={entry}>
                  {entry}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <label className="flex items-center gap-2 pb-1 text-sm">
          <input
            type="checkbox"
            checked={secureBoot}
            onChange={(event) => setSecureBoot(event.target.checked)}
          />
          SecureBoot
        </label>
      </div>

      {assets.isPending && <p className="text-sm text-muted-foreground">Resolving…</p>}

      {assets.isError && (
        <p role="alert" className="text-sm text-destructive">
          The asset references could not be resolved. The installer reference in particular is
          resolved against the registry and never assembled, so nothing is shown rather than a
          plausible string — a guessed installer is the failure this avoids.
        </p>
      )}

      {assets.isSuccess && (
        <>
          {/*
           * Above the grid, not below it. An operator who has already copied the
           * reference has stopped reading, and this is the sentence that says
           * the reference they just copied is provisional.
           *
           * The heading names the reference rather than the schematic: this
           * warning is about how one repository name was obtained on this
           * request, and the component's default heading would claim something
           * about the ISO and the installed system that is not being claimed.
           */}
          <SchematicWarnings
            warnings={assets.data.warnings}
            label="Installer reference warnings"
            heading="The installer reference below is usable but was not fully proven."
          />
          <div className="grid gap-2">
            <AssetRow label="ISO" value={assets.data.iso} />
            <AssetRow label="Installer" value={assets.data.installer} />
            <AssetRow label="PXE" value={assets.data.pxe} />
            <AssetRow label="Disk image" value={assets.data.disk_image} />
            <AssetRow label="Kernel cmdline" value={assets.data.cmdline} />
          </div>
          <p className="text-xs text-muted-foreground">
            The ISO and the installer must share this schematic. Booting from this ISO and
            installing with a different installer produces a machine that boots with these
            extensions and runs without them, and nothing reports the difference.
          </p>
        </>
      )}
    </section>
  )
}

/**
 * One labelled reference with its copy control.
 *
 * A named group rather than a dt/dd pair: the row carries an accessible name so
 * a screen reader and a test can both address "the ISO reference" as one thing.
 * A dd is name-prohibited, so labelling one would be markup that validates and
 * does not work.
 */
function AssetRow({ label, value }: { label: string; value: string }) {
  return (
    <fieldset
      aria-label={`${label} reference`}
      className="grid grid-cols-[minmax(0,8rem)_1fr_auto] items-start gap-2"
    >
      <span className="text-sm text-muted-foreground">{label}</span>
      <ReferenceValue value={value} />
      <CopyButton label={label} value={value} />
    </fieldset>
  )
}

/**
 * A reference that may wrap, but never inside a token.
 *
 * This carried `break-all`, which permits a break at any character — so
 * `metal-installer` could wrap into something a reader takes for `installer`,
 * and during this phase's UAT a DOM-scraping verifier did exactly that and
 * reported a real gap as refuted. The repository name is the one part of the
 * string where a misreading changes its meaning: the two names are different
 * images, and the reference is consumed by the upgrade RPC.
 *
 * So breaks are offered only at path separators. `<wbr />` marks the permitted
 * points and `break-normal` refuses the rest, which means a 64-character
 * schematic id — one segment longer than any row — still overflows rather than
 * breaking. That is acceptable and unavoidable; a fifteen-character repository
 * name never can, which is the property that matters.
 *
 * The element's text content stays the complete reference, because `<wbr />`
 * contributes nothing to it. The Copy control and every `toHaveTextContent`
 * assertion keep working on the exact string.
 */
function ReferenceValue({ value }: { value: string }) {
  const segments = value.split('/')

  return (
    <span data-reference className="min-w-0 overflow-x-auto break-normal font-mono text-xs">
      {segments.map((segment, index) => (
        // The index is the key because a reference can repeat a segment
        // (`installer` appears in both the host path and the repository name on
        // some references) and the list is never reordered.
        // biome-ignore lint/suspicious/noArrayIndexKey: positional by nature
        <span key={index}>
          {index > 0 && '/'}
          {segment}
          {index < segments.length - 1 && <wbr />}
        </span>
      ))}
    </span>
  )
}

/** Copies one reference. Nothing here is secret; it is a URL an operator needs. */
function CopyButton({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false)

  return (
    <Button
      type="button"
      variant="ghost"
      aria-label={`Copy ${label}`}
      onClick={() => {
        void navigator.clipboard?.writeText(value).then(
          () => setCopied(true),
          () => setCopied(false),
        )
      }}
    >
      {copied ? 'Copied' : 'Copy'}
    </Button>
  )
}

export const imagesRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/images',
  component: ImagesView,
})

export { ImagesView }
