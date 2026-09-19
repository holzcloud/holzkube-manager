import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { api, type ManifestPlan } from '@/api'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'

/**
 * Applying a manifest (milestone v1.17, slice 5).
 *
 * The plan is not a courtesy and it is not optional here: **Apply** is disabled
 * until a plan exists for exactly the text in the box. Editing the text throws
 * the plan away, because a plan for a document somebody has since changed is
 * worse than no plan — it is a reassuring description of something else.
 *
 * What the plan is worth saying twice: `create` and `update` are decided by
 * asking the cluster, not by reading the manifest. The manifest looks identical
 * either way, which is exactly why an operator cannot tell from the text which
 * of their objects already exist.
 *
 * Conflicts are shown and not retried. A conflict means another field manager —
 * usually a controller — owns a field this apply sets; forcing takes the value
 * away from whatever is managing it, which will set it back. That is a decision,
 * so it is on the screen rather than in a retry.
 */
export function ApplyManifest({ clusterID }: { clusterID: string }) {
  const [manifest, setManifest] = useState('')
  const [plan, setPlan] = useState<{ for: string; plan: ManifestPlan } | null>(null)
  const boxID = useId()
  const queryClient = useQueryClient()

  const planned = plan !== null && plan.for === manifest

  const planIt = useMutation({
    mutationFn: () => api.kubernetes.planManifest(clusterID, manifest),
    onSuccess: (result) => setPlan({ for: manifest, plan: result }),
  })

  const applyIt = useMutation({
    mutationFn: () => api.kubernetes.applyManifest(clusterID, manifest),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['kubernetes'] }),
  })

  const change = (next: string) => {
    setManifest(next)
    // The plan belonged to the previous text. Keeping it would mean offering an
    // apply described by a document that is no longer in the box.
    setPlan(null)
    applyIt.reset()
  }

  return (
    <section className="space-y-3">
      <div className="space-y-1">
        <label htmlFor={boxID} className="font-medium text-sm">
          Manifest
        </label>
        <textarea
          id={boxID}
          value={manifest}
          onChange={(event) => change(event.target.value)}
          rows={10}
          spellCheck={false}
          placeholder={
            'apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: settings\n  namespace: default'
          }
          className="w-full rounded-lg border border-input bg-transparent p-2.5 font-mono text-sm outline-none transition-colors focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
        />
        <p className="text-muted-foreground text-xs">
          Several documents separated by <code>---</code> are fine. Nothing is deleted: removing an
          object is a separate operation.
        </p>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={manifest.trim() === '' || planIt.isPending}
          onClick={() => planIt.mutate()}
        >
          {planIt.isPending ? 'Asking the cluster…' : 'Plan'}
        </Button>
        <Button
          type="button"
          disabled={!planned || applyIt.isPending}
          onClick={() => applyIt.mutate()}
        >
          {applyIt.isPending ? 'Applying…' : 'Apply'}
        </Button>
      </div>

      {!planned && manifest.trim() !== '' && !planIt.isPending && (
        <p className="text-muted-foreground text-xs">
          Plan it first. A plan asks the cluster which of these objects already exist, and it
          belongs to the text it was made for — editing the manifest throws it away.
        </p>
      )}

      {planIt.error ? <Problem error={planIt.error} /> : null}

      {planned && (
        <div className="space-y-2">
          <h3 className="font-medium text-sm">What applying this would do</h3>
          <ul className="space-y-1 text-sm">
            {plan.plan.objects.map((object) => (
              <li key={`${object.api_version}/${object.kind}/${object.namespace}/${object.name}`}>
                <span className="font-medium">
                  {object.action === 'update' ? 'Update' : 'Create'}
                </span>{' '}
                {object.kind} <code>{object.name}</code>
                {object.namespaced ? (
                  <>
                    {' '}
                    in <code>{object.namespace}</code>
                  </>
                ) : (
                  <span className="text-muted-foreground"> (cluster-wide)</span>
                )}
              </li>
            ))}
          </ul>

          {plan.plan.warnings.length > 0 && (
            <ul
              aria-label="Warnings"
              className="space-y-1 text-amber-700 text-xs dark:text-amber-500"
            >
              {plan.plan.warnings.map((warning) => (
                <li key={warning}>{warning}</li>
              ))}
            </ul>
          )}

          {plan.plan.objects.length === 0 && (
            <p className="text-muted-foreground text-sm">
              Nothing in this manifest can be applied to this cluster.
            </p>
          )}
        </div>
      )}

      {applyIt.error ? <Problem error={applyIt.error} /> : null}

      {applyIt.data && (
        <div className="space-y-2" role="status">
          <p className="text-sm">
            {applyIt.data.fully_applied
              ? `Applied ${applyIt.data.applied.length} object${applyIt.data.applied.length === 1 ? '' : 's'}.`
              : `Applied ${applyIt.data.applied.length}, and ${applyIt.data.failed.length} did not apply.`}
          </p>
          {applyIt.data.failed.length > 0 && (
            <ul className="space-y-1 text-sm">
              {applyIt.data.failed.map((failure) => (
                <li
                  key={`${failure.object.kind}/${failure.object.namespace}/${failure.object.name}`}
                >
                  <span className="font-medium">
                    {failure.object.kind} {failure.object.name}
                  </span>
                  : {failure.reason}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </section>
  )
}
