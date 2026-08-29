import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createRoute } from '@tanstack/react-router'
import { useCallback, useEffect, useState } from 'react'
import { api, type CreatedSchematic, type MetaValue, type SchematicInput } from '@/api'
import { LiveSchematicWarnings, SchematicWarnings } from '@/components/SchematicWarnings'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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

const ARCHITECTURES = ['amd64', 'arm64'] as const
export type Architecture = (typeof ARCHITECTURES)[number]

function ImagesView() {
  const queryClient = useQueryClient()

  const [name, setName] = useState('')
  const [chosenVersion, setChosenVersion] = useState<string | null>(null)
  const [showPrerelease, setShowPrerelease] = useState(false)
  const [arch, setArch] = useState<Architecture>('amd64')
  const [secureBoot, setSecureBoot] = useState(false)
  const [extensions, setExtensions] = useState<string[]>([])
  const [kernelArgs, setKernelArgs] = useState<string[]>([])
  const [meta, setMeta] = useState<MetaRow[]>([])
  const [dropped, setDropped] = useState<string[]>([])
  const [created, setCreated] = useState<CreatedSchematic | null>(null)

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

  const toggleExtension = useCallback((extensionName: string) => {
    setExtensions((previous) =>
      previous.includes(extensionName)
        ? previous.filter((each) => each !== extensionName)
        : [...previous, extensionName],
    )
  }, [])

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
        />

        <MetaRows meta={meta} onChange={setMeta} />

        <LiveSchematicWarnings kernelArgs={kernelArgs} meta={meta} />

        {create.isError && (
          <p role="alert" className="text-sm text-destructive">
            {create.error instanceof Error
              ? create.error.message
              : 'The schematic was not created.'}
          </p>
        )}

        <Button type="submit" disabled={create.isPending || name === '' || version === ''}>
          {create.isPending ? 'Creating…' : 'Create schematic'}
        </Button>
      </form>

      {created !== null && <CreatedPanel created={created} />}
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
}: {
  title: string
  addLabel: string
  values: string[]
  onChange: (next: string[]) => void
  placeholder: string
  inputLabel: string
}) {
  return (
    <section className="space-y-2">
      <h2 className="text-sm font-medium">{title}</h2>
      {values.map((value, index) => (
        // The index is the identity here: these rows have no key of their own
        // and reordering is not offered, so nothing can go stale.
        // biome-ignore lint/suspicious/noArrayIndexKey: rows are positional
        <div key={index} className="flex items-center gap-2">
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
      ))}
      <Button type="button" variant="secondary" onClick={() => onChange([...values, ''])}>
        {addLabel}
      </Button>
    </section>
  )
}

function MetaRows({ meta, onChange }: { meta: MetaRow[]; onChange: (next: MetaRow[]) => void }) {
  return (
    <section className="space-y-2">
      <h2 className="text-sm font-medium">META values</h2>
      {meta.map((row, index) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: rows are positional
        <div key={index} className="flex items-center gap-2">
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
function CreatedPanel({ created }: { created: CreatedSchematic }) {
  return (
    <section className="space-y-3 rounded-md border border-border p-4">
      <div className="flex flex-wrap items-center gap-3">
        <p className="text-sm font-medium">Schematic created.</p>
        <UsabilityBadge usable={created.usable} probedAt={created.probed_at} />
      </div>
      <p className="break-all font-mono text-xs">{created.id}</p>
      <SchematicWarnings warnings={created.warnings} />
    </section>
  )
}

/** The zero time means the probe never answered, which is not a refusal. */
export function isProbed(probedAt: string): boolean {
  return probedAt !== '' && !probedAt.startsWith('0001-01-01')
}

export function UsabilityBadge({ usable, probedAt }: { usable: boolean; probedAt: string }) {
  if (usable) {
    return <Badge variant="secondary">Usable — the build probe confirmed it</Badge>
  }
  if (!isProbed(probedAt)) {
    return <Badge variant="outline">Not verified — the build probe did not run</Badge>
  }
  return <Badge variant="destructive">Not usable — the Factory refused to build it</Badge>
}

export const imagesRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/images',
  component: ImagesView,
})

export { ImagesView }
