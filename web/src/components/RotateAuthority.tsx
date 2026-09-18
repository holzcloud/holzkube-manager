import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound } from 'lucide-react'
import { useId, useState } from 'react'
import { api } from '@/api'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

/**
 * Rotating a cluster's Talos certificate authority (V2-OPS-02).
 *
 * This is the most dangerous thing the product can do, and the panel is built
 * around saying so rather than around the click. Renewing this installation's
 * certificate touches no node; this changes what every node TRUSTS, in four
 * passes over the whole cluster, and a node that misses the first pass cannot
 * be repaired afterwards — reaching it would need the credential it no longer
 * accepts.
 *
 * Three things follow from that, and each is here on purpose:
 *
 * It loads nothing until asked. The preview is a read against every node's
 * record, and a cluster card that fetched it on render would be asking about an
 * operation nobody is doing.
 *
 * The four passes and the warnings come from the server. They are statements
 * about what this build does and what it has never proven, and a second copy
 * in the browser would be a second thing to drift.
 *
 * The cluster's name has to be typed, and the server checks it too. A dialog
 * protects against a misclick; the typed phrase is what makes the operator
 * name the cluster they mean.
 */
export function RotateAuthority({ clusterID }: { clusterID: string }) {
  const [open, setOpen] = useState(false)
  const [typed, setTyped] = useState('')
  const queryClient = useQueryClient()
  const typedID = useId()

  const preview = useQuery({
    queryKey: ['authority', clusterID],
    queryFn: () => api.authority.preview(clusterID),
    enabled: open,
  })

  const rotate = useMutation({
    mutationFn: async () => {
      const confirmation = await api.authority.confirm(clusterID, typed)
      return api.authority.rotate(clusterID, confirmation.token)
    },
    onSuccess: () => {
      setTyped('')
      void queryClient.invalidateQueries({ queryKey: ['jobs'] })
      void queryClient.invalidateQueries({ queryKey: ['authority', clusterID] })
    },
  })

  if (!open) {
    return (
      <Button type="button" size="sm" variant="outline" onClick={() => setOpen(true)}>
        <KeyRound aria-hidden="true" className="size-4" />
        Rotate this cluster’s certificate authority…
      </Button>
    )
  }

  const plan = preview.data
  const phrase = plan?.confirm_phrase ?? ''
  const matches = typed.trim() === phrase && phrase !== ''
  const blocked = plan?.locked === true

  return (
    <section aria-label="Rotate this cluster’s certificate authority" className="space-y-3">
      <h3 className="font-medium text-sm">Rotate this cluster’s certificate authority</h3>

      {preview.isPending && <p className="text-muted-foreground text-xs">Reading the cluster…</p>}
      {preview.error ? <Problem error={preview.error} /> : null}

      {plan && (
        <>
          {plan.in_progress && (
            <p className="text-amber-700 text-xs dark:text-amber-300">
              A rotation was started on this cluster and did not finish. Starting it again continues
              that one rather than beginning a second.
            </p>
          )}

          <ol className="list-decimal space-y-1 pl-5 text-xs">
            {plan.passes.map((pass) => (
              <li key={pass}>{pass}</li>
            ))}
          </ol>

          <div className="text-xs">
            <p className="text-muted-foreground">
              {plan.nodes.length} node{plan.nodes.length === 1 ? '' : 's'} will be written, and
              every one of them has to answer:
            </p>
            <ul className="mt-1 space-y-0.5">
              {plan.nodes.map((node) => (
                <li key={node.id} className="font-mono text-xs">
                  {node.hostname === '' ? node.id : node.hostname} · {node.role}
                </li>
              ))}
            </ul>
          </div>

          <ul className="space-y-1 text-xs">
            {plan.warnings.map((warning) => (
              <li key={warning} className="text-amber-700 dark:text-amber-300">
                {warning}
              </li>
            ))}
          </ul>

          <div className="space-y-1">
            <label className="block text-xs" htmlFor={typedID}>
              Type <span className="font-mono">{phrase}</span> to confirm
            </label>
            <Input
              id={typedID}
              value={typed}
              autoComplete="off"
              onChange={(event) => setTyped(event.target.value)}
            />
          </div>

          {/* The reason sits BESIDE the button and not in a title attribute: a
              tooltip is not a reason on a phone, which has no hover. */}
          {blocked && (
            <p className="text-xs text-muted-foreground">
              This cluster is adopted read-only, so nothing may change it. Unlock it first.
            </p>
          )}
          {!blocked && !matches && (
            <p className="text-xs text-muted-foreground">
              The cluster’s name has to match exactly before this can start.
            </p>
          )}

          <div className="flex gap-2">
            <Button
              type="button"
              size="sm"
              variant="destructive"
              disabled={!matches || blocked || rotate.isPending}
              onClick={() => rotate.mutate()}
            >
              {rotate.isPending ? 'Starting…' : 'Rotate the authority'}
            </Button>
            <Button type="button" size="sm" variant="ghost" onClick={() => setOpen(false)}>
              Cancel
            </Button>
          </div>
        </>
      )}

      {rotate.error ? <Problem error={rotate.error} /> : null}
      {rotate.isSuccess && (
        <p role="status" className="text-xs text-muted-foreground">
          Started. It runs as a job with {rotate.data.job.steps.length} steps — watch it on the Jobs
          screen, which says which pass it is on.
        </p>
      )}
    </section>
  )
}
