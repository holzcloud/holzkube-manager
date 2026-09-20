import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api, type Sweepable } from '@/api'
import { DataTable } from '@/components/DataTable'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'

/**
 * Clearing out what is finished (2026-09-20).
 *
 * # The plan is the feature
 *
 * The operator asked for "a button that deletes old things I no longer need".
 * The button is the easy half. Nothing here is recoverable, and "it removed more
 * than I expected" is the failure to prevent — so the button does not delete.
 * It asks the server what it WOULD remove, shows the list with a reason on every
 * row, and only a second press on that list removes anything.
 *
 * The list is passed back to the server rather than recomputed there. Between a
 * plan and an apply somebody's CronJob can run, and a sweep that recomputed
 * would remove things nobody saw in the list they approved.
 *
 * # Images are not in the list, and the notice says so
 *
 * The Talos machine API can LIST the images on a node and has no delete. Image
 * removal is the kubelet's own garbage collection, which runs when the disk
 * crosses a threshold. A button here claiming to delete images would do nothing
 * at all, so instead the server's notice says who does it.
 */
export function TidyUp({ clusterID, namespace }: { clusterID: string; namespace: string }) {
  // Nothing is asked for until somebody asks. A plan is three list calls against
  // the API server, and this panel sits on a screen that refetches itself.
  const [planning, setPlanning] = useState(false)
  const [swept, setSwept] = useState<string | null>(null)
  const queryClient = useQueryClient()

  const plan = useQuery({
    queryKey: ['kubernetes', 'sweep', clusterID, namespace],
    queryFn: () => api.kubernetes.sweepPlan(clusterID, namespace),
    enabled: planning && clusterID !== '',
    // Never on a timer: a list somebody is reading before pressing delete must
    // not change under them between reading it and pressing.
    refetchOnWindowFocus: false,
    staleTime: Number.POSITIVE_INFINITY,
  })

  const sweep = useMutation({
    mutationFn: (items: Sweepable[]) => api.kubernetes.sweep(clusterID, items),
    onSuccess: (result) => {
      setSwept(
        result.failed.length === 0
          ? `Removed ${result.removed}.`
          : `Removed ${result.removed}; ${result.failed.length} could not be removed.`,
      )
      setPlanning(false)
      void queryClient.invalidateQueries({ queryKey: ['kubernetes'] })
    },
  })

  if (!planning) {
    return (
      <div className="space-y-2">
        <Button
          type="button"
          variant="outline"
          className="max-md:h-11 max-md:w-full"
          onClick={() => {
            setSwept(null)
            setPlanning(true)
          }}
        >
          Look for things to clear out
        </Button>
        {swept === null ? (
          <p className="text-muted-foreground text-sm">
            Finished pods, finished Jobs nothing owns, and rollouts that have been replaced. This
            shows the list before anything is removed.
          </p>
        ) : (
          <p className="text-sm">{swept}</p>
        )}
        {sweep.error ? <Problem error={sweep.error} /> : null}
      </div>
    )
  }

  if (plan.error) {
    return (
      <div className="space-y-2">
        <Problem error={plan.error} />
        <Button type="button" variant="ghost" size="sm" onClick={() => setPlanning(false)}>
          Close
        </Button>
      </div>
    )
  }
  if (!plan.data) return <p className="text-muted-foreground text-sm">Looking…</p>

  const items = plan.data.items

  return (
    <div className="space-y-3">
      <DataTable
        label="What would be removed"
        phone="rows"
        rows={items}
        keyOf={(item) => `${item.kind}/${item.namespace}/${item.name}`}
        empty={
          <>
            Nothing has finished and been left behind
            {namespace === '' ? ' in this cluster' : ` in ${namespace}`}.
          </>
        }
        columns={[
          {
            key: 'name',
            label: 'Object',
            role: 'identity',
            render: (item) => (
              <span className="break-all font-mono text-xs">
                {item.kind} {item.namespace}/{item.name}
              </span>
            ),
          },
          { key: 'age', label: 'Age', render: (item) => item.age },
          {
            key: 'reason',
            label: 'Why',
            // Never a detail: a list of names with no reasons is a list nobody
            // can check before pressing the button.
            render: (item) => <span className="text-xs">{item.reason}</span>,
          },
        ]}
      />

      <div className="flex flex-wrap gap-2 max-md:flex-col">
        <Button
          type="button"
          variant="destructive"
          className="max-md:h-11"
          disabled={items.length === 0 || sweep.isPending}
          onClick={() => sweep.mutate(items)}
        >
          {sweep.isPending
            ? 'Removing…'
            : `Remove ${items.length === 1 ? 'this one' : `these ${items.length}`}`}
        </Button>
        <Button
          type="button"
          variant="outline"
          className="max-md:h-11"
          onClick={() => setPlanning(false)}
        >
          Leave it
        </Button>
      </div>

      {/* What is NOT in the list — images especially, which no button can
          delete. An empty plan must not read as "nothing to clean up anywhere". */}
      <p className="text-muted-foreground text-xs">{plan.data.notice}</p>
      {sweep.error ? <Problem error={sweep.error} /> : null}
    </div>
  )
}
