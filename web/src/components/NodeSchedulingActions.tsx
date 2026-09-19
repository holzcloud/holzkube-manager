import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { api } from '@/api'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'

/**
 * Cordon, uncordon and drain, per node (milestone v1.17, slice 3).
 *
 * This closes a gap the product has been papering over with a sentence: taking
 * a node out of a cluster and upgrading one both tell the operator to cordon
 * and drain by hand first, because until now there was no Kubernetes client to
 * do it with.
 *
 * The two drain flags are checkboxes and not defaults, and that is the whole
 * design of this panel. A pod nothing owns is gone for good once it is
 * evicted; a pod with local storage loses that storage when it moves. `kubectl`
 * refuses both without being told, and so does this — so the screen has to be
 * the place where an operator says yes, rather than a place where the product
 * decided for them.
 *
 * A drain comes back as a job. It waits out each pod's termination grace period
 * and can take minutes, so what this panel reports is that the job started and
 * where to watch it.
 */
export function NodeSchedulingActions({
  clusterID,
  node,
  unschedulable,
}: {
  clusterID: string
  node: string
  unschedulable: boolean
}) {
  const [open, setOpen] = useState(false)
  const [force, setForce] = useState(false)
  const [deleteLocalData, setDeleteLocalData] = useState(false)
  const queryClient = useQueryClient()
  const forceID = useId()
  const localID = useId()

  const cordon = useMutation({
    mutationFn: (next: boolean) => api.kubernetes.cordon(clusterID, node, next),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['kubernetes'] }),
  })

  const drain = useMutation({
    mutationFn: () =>
      api.kubernetes.drain(clusterID, node, { force, delete_local_data: deleteLocalData }),
    onSuccess: () => {
      setOpen(false)
      void queryClient.invalidateQueries({ queryKey: ['kubernetes'] })
      void queryClient.invalidateQueries({ queryKey: ['jobs'] })
    },
  })

  return (
    <div className="space-y-1">
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={cordon.isPending}
          onClick={() => cordon.mutate(!unschedulable)}
        >
          {unschedulable ? 'Uncordon' : 'Cordon'}
        </Button>
        <Button type="button" size="sm" variant="outline" onClick={() => setOpen(!open)}>
          Drain…
        </Button>
      </div>

      {cordon.error ? <Problem error={cordon.error} /> : null}

      {open && (
        <div className="space-y-2 rounded-md border p-2">
          <p className="text-xs">
            Draining cordons {node} and then evicts its pods. Pods a DaemonSet owns are left in
            place — they would be recreated on this node immediately — and static pods belong to the
            kubelet, which the API server cannot evict.
          </p>

          <label className="flex items-start gap-2 py-1 text-xs max-md:min-h-11" htmlFor={forceID}>
            <input
              id={forceID}
              type="checkbox"
              className="mt-0.5 max-md:mt-2.5 max-md:size-6"
              checked={force}
              onChange={(event) => setForce(event.target.checked)}
            />
            <span>
              Evict pods no controller owns. Nothing will recreate them elsewhere: evicting one
              means losing it.
            </span>
          </label>

          <label className="flex items-start gap-2 py-1 text-xs max-md:min-h-11" htmlFor={localID}>
            <input
              id={localID}
              type="checkbox"
              className="mt-0.5 max-md:mt-2.5 max-md:size-6"
              checked={deleteLocalData}
              onChange={(event) => setDeleteLocalData(event.target.checked)}
            />
            <span>Evict pods with local storage. An emptyDir goes with the pod when it moves.</span>
          </label>

          <p className="text-muted-foreground text-xs">
            Without these, a drain that meets such a pod stops and names it. The node stays cordoned
            either way, so nothing new is scheduled onto it while you decide.
          </p>

          <div className="flex gap-2">
            <Button
              type="button"
              size="sm"
              variant="destructive"
              disabled={drain.isPending}
              onClick={() => drain.mutate()}
            >
              {drain.isPending ? 'Starting…' : 'Drain this node'}
            </Button>
            <Button type="button" size="sm" variant="ghost" onClick={() => setOpen(false)}>
              Cancel
            </Button>
          </div>

          {drain.error ? <Problem error={drain.error} /> : null}
        </div>
      )}

      {drain.isSuccess && (
        <p role="status" className="text-xs text-muted-foreground">
          Draining. It runs as a job — the Jobs screen says what moved and what stayed, with the
          reason for each pod that stayed.
        </p>
      )}
    </div>
  )
}
