import { useQuery } from '@tanstack/react-query'
import { api } from '@/api'
import { Problem } from '@/components/Problem'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

/**
 * Why nothing will schedule on a node (2026-09-19).
 *
 * The node list says Ready and cordoned; neither answers "why is this pod
 * Pending". Three things do, and they are all here:
 *
 *   - A taint keeps pods off, and NoExecute removes the ones already there.
 *     From the pod's side all of this shows as "0/2 nodes are available", which
 *     names no node.
 *   - A condition is the kubelet's own verdict, and the pressures are bad when
 *     TRUE — the inversion of Ready.
 *   - What the scheduler has left, which is allocatable minus what pods
 *     REQUESTED. Not what they use. A node at 5% CPU can have no room, and that
 *     sentence is on the screen rather than implied.
 */
export function NodeWhy({
  clusterID,
  node,
  onClose,
}: {
  clusterID: string
  node: string
  onClose: () => void
}) {
  const detail = useQuery({
    queryKey: ['kubernetes', 'node', clusterID, node],
    queryFn: () => api.kubernetes.nodeDetail(clusterID, node),
  })

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90dvh] overflow-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="break-all font-mono text-base">{node}</DialogTitle>
          <DialogDescription>What the scheduler sees</DialogDescription>
        </DialogHeader>

        {detail.error ? <Problem error={detail.error} /> : null}
        {detail.isPending && <p className="text-muted-foreground text-sm">Reading…</p>}

        {detail.data && (
          <>
            <section className="space-y-1">
              <h3 className="font-medium text-sm">Room left</h3>
              <dl className="grid grid-cols-1 gap-x-3 gap-y-1 text-sm md:grid-cols-[10rem_minmax(0,1fr)]">
                <dt className="text-muted-foreground">CPU</dt>
                <dd>
                  {detail.data.cpu_requested || '0'} requested of{' '}
                  {detail.data.cpu_allocatable || '—'}
                </dd>
                <dt className="text-muted-foreground">Memory</dt>
                <dd>
                  {detail.data.memory_requested || '0'} requested of{' '}
                  {detail.data.memory_allocatable || '—'}
                </dd>
                <dt className="text-muted-foreground">Pods</dt>
                <dd>
                  {detail.data.pods_running} of {detail.data.pod_capacity || '—'}
                </dd>
              </dl>
              {/* The misreading this panel exists to prevent, said rather than
                  left to be inferred from the word "requested". */}
              <p className="text-muted-foreground text-xs">{detail.data.notice}</p>
            </section>

            <section className="space-y-1">
              <h3 className="font-medium text-sm">Taints</h3>
              {detail.data.taints.length === 0 ? (
                <p className="text-muted-foreground text-sm">
                  None. Nothing here keeps pods off this node.
                </p>
              ) : (
                <ul className="space-y-1 text-sm">
                  {detail.data.taints.map((taint) => {
                    const explains = detail.data.explaining_taints.some((t) => t.key === taint.key)
                    return (
                      <li key={`${taint.key}-${taint.effect}`}>
                        <span
                          className={
                            explains
                              ? 'font-mono text-amber-700 text-xs dark:text-amber-300'
                              : 'font-mono text-xs'
                          }
                        >
                          {taint.key}
                          {taint.value !== '' && `=${taint.value}`}:{taint.effect}
                        </span>
                        <span className="text-muted-foreground"> — {taint.explanation}</span>
                        {!explains && (
                          <span className="text-muted-foreground text-xs"> (ordinary)</span>
                        )}
                      </li>
                    )
                  })}
                </ul>
              )}
            </section>

            <section className="space-y-1">
              <h3 className="font-medium text-sm">What the kubelet reports</h3>
              <ul className="space-y-1 text-sm">
                {detail.data.conditions.map((condition) => (
                  <li key={condition.type}>
                    <span
                      className={
                        condition.bad ? 'font-medium text-amber-700 dark:text-amber-300' : undefined
                      }
                    >
                      {condition.type}: {condition.status}
                    </span>
                    {condition.reason !== '' && (
                      <span className="text-muted-foreground"> — {condition.reason}</span>
                    )}
                  </li>
                ))}
              </ul>
            </section>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
