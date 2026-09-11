import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createRoute } from '@tanstack/react-router'
import { AlertTriangle, Copy } from 'lucide-react'
import { useEffect, useState } from 'react'
import { api, type ConfigPreview, type Machine } from '@/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
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
 * The configuration screen: look at it, patch it, see what would change, apply
 * it.
 *
 * Everything shown here arrives already redacted from the server. There is no
 * "show secrets" toggle and there is no endpoint behind one: a machine
 * configuration carries the cluster's certificate-authority private key, so
 * "look at a node's config" and "hand the cluster over" would be the same
 * feature.
 *
 * The screen's shape follows the one decision an operator actually has to
 * make, which is not "what do I change" but "what mode do I apply it in". So
 * the diff and the mode arrive together, from one call, and the mode is
 * computed from the changed paths rather than offered as a free choice with a
 * default.
 */
export function ConfigPage() {
  const [machine, setMachine] = useState('')

  const machines = useQuery({
    queryKey: ['machines'],
    queryFn: () => api.machines.list(),
  })

  const chosen = machine || machines.data?.[0]?.id || ''

  return (
    <section className="space-y-5">
      <div>
        <h1 className="font-heading text-xl font-semibold tracking-tight">Config</h1>
        <p className="max-w-prose text-sm text-muted-foreground">
          A node's machine configuration, with its secrets removed on the server before it reaches
          this page.
        </p>
      </div>

      {machines.isSuccess && machines.data.length === 0 && (
        <p className="text-sm text-muted-foreground">No machines in the inventory yet.</p>
      )}

      {machines.isSuccess && machines.data.length > 0 && (
        <div className="max-w-sm space-y-1">
          <Label htmlFor="config-machine">Node</Label>
          <Select value={chosen} onValueChange={setMachine}>
            <SelectTrigger id="config-machine">
              <SelectValue placeholder="Choose a node" />
            </SelectTrigger>
            <SelectContent>
              {machines.data.map((m) => (
                <SelectItem key={m.id} value={m.id}>
                  {m.hostname.value || m.id.slice(0, 8)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      {chosen !== '' && (
        <NodeConfig machine={machines.data?.find((m) => m.id === chosen)} machineID={chosen} />
      )}

      <PatchLibrary />
    </section>
  )
}

function NodeConfig({ machine, machineID }: { machine?: Machine; machineID: string }) {
  const [tab, setTab] = useState<'rendered' | 'raw'>('rendered')
  const [patch, setPatch] = useState('')
  const [preview, setPreview] = useState<ConfigPreview | null>(null)
  const [failure, setFailure] = useState('')

  const view = useQuery({
    queryKey: ['config', machineID],
    queryFn: () => api.config.get(machineID),
  })

  const plan = useMutation({
    mutationFn: () => api.config.plan(machineID, [patch]),
    onSuccess: (p) => {
      setPreview(p)
      setFailure('')
    },
    onError: (e: Error) => {
      setPreview(null)
      setFailure(e.message)
    },
  })

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-2">
          <CardTitle className="text-base">Current configuration</CardTitle>
          <div className="flex items-center gap-2">
            {view.data?.staged_pending && (
              <Badge
                variant="outline"
                className="border-amber-600/40 text-amber-700 dark:text-amber-300"
                title="A configuration is waiting for this node's next boot, as far as this process knows. A second staged apply would replace it."
              >
                staged change pending
              </Badge>
            )}
            <Button
              size="sm"
              variant={tab === 'rendered' ? 'secondary' : 'outline'}
              onClick={() => setTab('rendered')}
            >
              Rendered
            </Button>
            <Button
              size="sm"
              variant={tab === 'raw' ? 'secondary' : 'outline'}
              onClick={() => setTab('raw')}
            >
              Raw
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {view.isLoading && <p className="text-sm text-muted-foreground">Reading the node</p>}
          {view.error && (
            <p className="text-sm text-destructive">
              The configuration could not be read: {(view.error as Error).message}
            </p>
          )}
          {view.data && (
            <pre className="max-h-96 overflow-auto rounded-md bg-muted/30 p-3 font-mono text-xs">
              {tab === 'raw' ? view.data.raw : view.data.rendered}
            </pre>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Change it</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="max-w-prose text-sm text-muted-foreground">
            A strategic merge patch. It addresses entries by key rather than by index, which is what
            makes the same patch mean the same thing on two nodes whose lists are different lengths.
            JSON Patch is refused for exactly that reason.
          </p>

          <textarea
            value={patch}
            rows={8}
            spellCheck={false}
            placeholder={'machine:\n  network:\n    hostname: node-1\n'}
            onChange={(e) => setPatch(e.target.value)}
            className="w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs"
          />

          {failure !== '' && <p className="text-sm text-destructive">{failure}</p>}

          <Button disabled={patch.trim() === '' || plan.isPending} onClick={() => plan.mutate()}>
            See what would change
          </Button>
        </CardContent>
      </Card>

      {preview && (
        <PreviewCard
          preview={preview}
          machineID={machineID}
          cluster={machine?.cluster ?? ''}
          patch={patch}
        />
      )}
    </div>
  )
}

/**
 * The preview, which is the screen this whole phase exists for.
 *
 * Four things are called out rather than left in the diff, because each is a
 * mistake somebody makes and does not notice:
 *
 * - a list that grew, with both lengths;
 * - a duplicate, which is nearly always a patch applied twice;
 * - a `.machine.install` change, which applies, reports success and does
 *   nothing until the next install;
 * - a network change, which gets the rollback timer and its countdown.
 */
function PreviewCard({
  preview,
  machineID,
  cluster,
  patch,
}: {
  preview: ConfigPreview
  machineID: string
  cluster: string
  patch: string
}) {
  const queryClient = useQueryClient()
  const [mode, setMode] = useState(preview.verdict.mode)
  const [applied, setApplied] = useState('')
  const [failure, setFailure] = useState('')

  const apply = useMutation({
    mutationFn: () => api.config.apply(machineID, [patch], mode, cluster),
    onSuccess: (r) => {
      setApplied(`Applied in ${r.mode} mode. ${r.details}`)
      setFailure('')
      void queryClient.invalidateQueries({ queryKey: ['config', machineID] })
    },
    onError: (e: Error) => setFailure(e.message),
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">What would change</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4 text-sm">
        {preview.verdict.sentences.map((s) => (
          <p key={s} className="max-w-prose">
            {s}
          </p>
        ))}

        {!preview.idempotent && (
          <p className="flex items-start gap-2 rounded-md border border-amber-600/40 bg-amber-600/10 px-3 py-2 text-amber-700 dark:text-amber-300">
            <AlertTriangle aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
            <span>
              Applying this twice does not give the same result as applying it once. That is almost
              always a patch that appends to a list rather than setting it.
            </span>
          </p>
        )}

        {preview.diff.duplicates && (
          <p className="flex items-start gap-2 rounded-md border border-amber-600/40 bg-amber-600/10 px-3 py-2 text-amber-700 dark:text-amber-300">
            <Copy aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
            <span>
              A list would end up with the same value twice. Check whether this patch has already
              been applied to this node.
            </span>
          </p>
        )}

        {!preview.valid && (
          <div className="rounded-md border border-red-600/40 bg-red-600/10 px-3 py-2 text-red-700 dark:text-red-300">
            <p className="font-medium">Talos would refuse this configuration.</p>
            <ul className="mt-1 list-inside list-disc">
              {preview.validation.map((v) => (
                <li key={v}>{v}</li>
              ))}
            </ul>
          </div>
        )}

        <ul className="space-y-1">
          {preview.diff.changes.map((c) => (
            <li key={c.path} className="font-mono text-xs">
              <span className="mr-2 text-muted-foreground">{c.kind}</span>
              <span className="font-medium">{c.path}</span>
              {c.kind === 'list-changed' ? (
                <span className="block text-muted-foreground">
                  {c.len_before} entries become {c.len_after}: {c.after}
                  {c.duplicates.length > 0 && (
                    <span className="block text-amber-700 dark:text-amber-300">
                      duplicated: {c.duplicates.join(', ')}
                    </span>
                  )}
                </span>
              ) : (
                <span className="block text-muted-foreground">
                  {c.before !== '' && <span className="line-through">{c.before}</span>}{' '}
                  {c.after !== '' && <span>{c.after}</span>}
                </span>
              )}
            </li>
          ))}
        </ul>

        <div className="flex flex-wrap items-end gap-3">
          <div className="w-48 space-y-1">
            <Label htmlFor="apply-mode">Apply mode</Label>
            <Select value={mode} onValueChange={(v) => setMode(v as typeof mode)}>
              <SelectTrigger id="apply-mode">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="no-reboot">no-reboot</SelectItem>
                <SelectItem value="try">try (rollback timer)</SelectItem>
                <SelectItem value="staged">staged (next boot)</SelectItem>
                <SelectItem value="reboot">reboot</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {mode === 'try' && preview.verdict.try_seconds > 0 && (
            <TryCountdown seconds={preview.verdict.try_seconds} running={apply.isPending} />
          )}

          <Button disabled={apply.isPending} onClick={() => apply.mutate()}>
            Apply
          </Button>
        </div>

        {applied !== '' && <p className="text-emerald-700 dark:text-emerald-300">{applied}</p>}
        {failure !== '' && <p className="text-destructive">{failure}</p>}
      </CardContent>
    </Card>
  )
}

/**
 * The visible countdown CFG-09 asks for.
 *
 * It counts the node's own timer, not an approximation of it: the number comes
 * from the server, which takes it from the same constant the apply uses. A
 * countdown that ran on its own clock would be telling the operator how long
 * is left according to nobody.
 */
function TryCountdown({ seconds, running }: { seconds: number; running: boolean }) {
  const [left, setLeft] = useState(seconds)

  useEffect(() => {
    if (!running) {
      setLeft(seconds)
      return
    }
    const t = setInterval(() => setLeft((n) => (n > 0 ? n - 1 : 0)), 1000)
    return () => clearInterval(t)
  }, [running, seconds])

  return (
    <p className="text-sm text-muted-foreground">
      {running
        ? `Rolls back in ${left}s unless the node stays reachable.`
        : `The node will undo this after ${seconds}s if it becomes unreachable.`}
    </p>
  )
}

/** The stored patches, newest version of each first, superseded ones marked
 * rather than hidden. */
function PatchLibrary() {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [body, setBody] = useState('')
  const [failure, setFailure] = useState('')

  const patches = useQuery({ queryKey: ['patches'], queryFn: () => api.patches.list() })

  const create = useMutation({
    mutationFn: () => api.patches.create({ name, body }),
    onSuccess: () => {
      setName('')
      setBody('')
      setFailure('')
      void queryClient.invalidateQueries({ queryKey: ['patches'] })
    },
    onError: (e: Error) => setFailure(e.message),
  })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Saved patches</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4 text-sm">
        <p className="max-w-prose text-muted-foreground">
          Editing a patch writes a new version and keeps the old one. Nothing here is deleted,
          because "what exactly was applied to this node in March" only has an answer if the thing
          applied still exists.
        </p>

        {(patches.data ?? []).length === 0 && (
          <p className="text-muted-foreground">No saved patches yet.</p>
        )}

        <ul className="space-y-2">
          {(patches.data ?? []).map((p) => (
            <li key={p.id} className="rounded-md border border-border p-2">
              <div className="flex items-center gap-2">
                <span className="font-medium">{p.name}</span>
                <Badge variant="outline">v{p.version}</Badge>
                {p.superseded && (
                  <Badge variant="outline" className="text-muted-foreground">
                    superseded
                  </Badge>
                )}
              </div>
              <pre className="mt-1 overflow-auto font-mono text-xs text-muted-foreground">
                {p.body}
              </pre>
            </li>
          ))}
        </ul>

        <div className="grid gap-3 sm:grid-cols-[12rem_1fr]">
          <div className="space-y-1">
            <Label htmlFor="patch-name">Name</Label>
            <Input id="patch-name" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="space-y-1">
            <Label htmlFor="patch-body">Body</Label>
            <textarea
              id="patch-body"
              value={body}
              rows={4}
              spellCheck={false}
              onChange={(e) => setBody(e.target.value)}
              className="w-full rounded-md border border-input bg-transparent p-2 font-mono text-xs"
            />
          </div>
        </div>

        {failure !== '' && <p className="text-destructive">{failure}</p>}

        <Button
          variant="secondary"
          disabled={name === '' || body.trim() === '' || create.isPending}
          onClick={() => create.mutate()}
        >
          Save patch
        </Button>
      </CardContent>
    </Card>
  )
}

export const configRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/config',
  component: ConfigPage,
})
