import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { api, type Workload } from '@/api'
import { DataTable } from '@/components/DataTable'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

/**
 * Everything that runs, whatever kind it is (2026-09-19).
 *
 * The screen used to list Deployments and nothing else, which describes a
 * cluster nobody runs: the storage layer is a DaemonSet, the database is a
 * StatefulSet, the backup is a CronJob. An operator whose Longhorn was broken
 * looked at a workload list with Longhorn absent from it — worse than an empty
 * screen, because an empty screen does not imply the thing is not there.
 *
 * # What is NOT offered is the part to get right
 *
 * The server says per row whether it can be scaled or rolled, and this shows
 * only what it says. A DaemonSet has no replica count — its size is how many
 * nodes match — so no field appears; a Job has no pod template, so no roll
 * button does. Offering them and letting the API server refuse would teach that
 * the buttons here are suggestions.
 *
 * And the state comes from the server as a SENTENCE per kind, rather than being
 * assembled here from two numbers: "0 of 0 ready" is what a CronJob between runs
 * looks like to arithmetic, and it reports a schedule as an outage.
 */
export function Workloads({ clusterID, namespace }: { clusterID: string; namespace: string }) {
  const workloads = useQuery({
    queryKey: ['kubernetes', 'workloads', clusterID, namespace],
    queryFn: () => api.kubernetes.workloads(clusterID, namespace),
    refetchInterval: 15_000,
  })

  if (workloads.error) return <Problem error={workloads.error} />
  if (!workloads.data) return <p className="text-muted-foreground text-sm">Reading…</p>

  return (
    <DataTable
      label="Workloads"
      rows={workloads.data}
      keyOf={(w) => `${w.kind}/${w.namespace}/${w.name}`}
      empty={
        <>
          The API server answered, and nothing runs
          {namespace === '' ? ' in this cluster' : ` in ${namespace}`}.
        </>
      }
      columns={[
        {
          key: 'kind',
          label: 'Kind',
          render: (w) => <span className="text-xs">{w.kind}</span>,
        },
        {
          key: 'name',
          label: 'Name',
          role: 'identity',
          render: (w) => (
            <span className="break-all font-mono text-xs">
              {w.namespace}/{w.name}
            </span>
          ),
        },
        {
          key: 'state',
          label: 'State',
          render: (w) => (
            <span
              className={
                w.suspended || (w.desired > 0 && w.ready < w.desired)
                  ? 'text-amber-700 dark:text-amber-300'
                  : undefined
              }
            >
              {w.summary}
            </span>
          ),
        },
        {
          key: 'schedule',
          label: 'Schedule',
          role: 'detail',
          render: (w) => (w.schedule === '' ? '—' : <code className="text-xs">{w.schedule}</code>),
        },
        {
          key: 'image',
          label: 'Image',
          role: 'detail',
          render: (w) => <span className="break-all font-mono text-xs">{w.image || '—'}</span>,
        },
        {
          key: 'actions',
          label: 'Actions',
          role: 'actions',
          render: (w) => <WorkloadActionsFor clusterID={clusterID} workload={w} />,
        },
      ]}
    />
  )
}

function WorkloadActionsFor({ clusterID, workload }: { clusterID: string; workload: Workload }) {
  const queryClient = useQueryClient()
  const [replicas, setReplicas] = useState(String(workload.desired))

  const invalidate = () => void queryClient.invalidateQueries({ queryKey: ['kubernetes'] })

  const stop = useMutation({
    mutationFn: () =>
      api.kubernetes.stopWorkload(clusterID, workload.kind, workload.namespace, workload.name),
    onSuccess: invalidate,
  })
  const start = useMutation({
    mutationFn: () =>
      api.kubernetes.startWorkload(clusterID, workload.kind, workload.namespace, workload.name),
    onSuccess: invalidate,
  })

  const scale = useMutation({
    mutationFn: (next: number) =>
      api.kubernetes.scaleWorkload(
        clusterID,
        workload.kind,
        workload.namespace,
        workload.name,
        next,
      ),
    onSuccess: invalidate,
  })
  const roll = useMutation({
    mutationFn: () =>
      api.kubernetes.restartWorkload(clusterID, workload.kind, workload.namespace, workload.name),
    onSuccess: invalidate,
  })

  const parsed = Number.parseInt(replicas, 10)
  const valid = Number.isInteger(parsed) && parsed >= 0
  const changed = valid && parsed !== workload.desired

  // Whether there is a stop, and whether it is already stopped, both come from
  // the server -- the same reason `scalable` does. A DaemonSet's size is how
  // many nodes match, so it has no count to set to zero, and offering the button
  // and letting the API server refuse would teach that the buttons here are
  // suggestions.
  const { stoppable, stopped } = workload

  const pair = stoppable ? (
    <Button
      type="button"
      size="sm"
      variant={stopped ? 'outline' : 'destructive'}
      className="max-md:h-11"
      disabled={stop.isPending || start.isPending}
      onClick={() => (stopped ? start.mutate() : stop.mutate())}
    >
      {stop.isPending || start.isPending
        ? stopped
          ? 'Starting…'
          : 'Stopping…'
        : stopped
          ? 'Start'
          : 'Stop'}
    </Button>
  ) : null

  const pairProblem = (
    <>
      {stop.error ? <Problem error={stop.error} /> : null}
      {start.error ? <Problem error={start.error} /> : null}
    </>
  )

  if (!workload.scalable && !workload.rollable) {
    return (
      <div className="space-y-1">
        {pair}
        <p className="text-muted-foreground text-xs">
          {workload.kind === 'CronJob'
            ? stopped
              ? 'Suspended: it keeps its schedule and runs nothing. Starting it lets the schedule run again.'
              : 'A CronJob runs on its schedule, so there is nothing to scale or roll. Stopping one suspends it — it keeps its schedule and runs nothing.'
            : 'A Job runs to completion, so there is nothing to scale or roll. Stopping one suspends it.'}
        </p>
        {pairProblem}
      </div>
    )
  }

  return (
    <div className="space-y-1">
      <div className="flex flex-wrap items-center gap-2 max-md:flex-col max-md:items-stretch">
        {workload.scalable && (
          <>
            <Input
              className="w-20 max-md:h-11 max-md:w-full"
              aria-label={`Replicas for ${workload.name}`}
              inputMode="numeric"
              value={replicas}
              onChange={(event) => setReplicas(event.target.value)}
            />
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="max-md:h-11"
              disabled={!changed || scale.isPending}
              onClick={() => scale.mutate(parsed)}
            >
              {scale.isPending ? 'Scaling…' : 'Scale'}
            </Button>
          </>
        )}
        {workload.rollable && (
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="max-md:h-11"
            disabled={roll.isPending}
            onClick={() => roll.mutate()}
          >
            {roll.isPending ? 'Rolling…' : 'Roll pods'}
          </Button>
        )}
        {pair}
      </div>

      {/* What a start would bring back, BEFORE anybody presses it. Scaling to
          zero throws the count away, so the number was written onto the object
          when it was stopped; saying it here is how nobody is surprised by it. */}
      {stopped && workload.scalable && (
        <p className="text-muted-foreground text-xs">
          {workload.would_start_with > 0
            ? `Stopped. Starting brings back the ${workload.would_start_with} it was running.`
            : 'At zero, and nothing recorded what it was running — something else scaled it down — so starting it runs one.'}
        </p>
      )}

      {/* A DaemonSet has no stop, and the reason is not a tooltip: a phone has
          no hover. */}
      {!stoppable && (
        <p className="text-muted-foreground text-xs">
          A DaemonSet runs on every matching node, so there is no count to set to zero. Cordon or
          drain the nodes instead.
        </p>
      )}

      {/* Why a DaemonSet has no field, rather than a disabled one with a
          tooltip: a phone has no hover, so a title attribute is not a reason. */}
      {!workload.scalable && workload.rollable && (
        <p className="text-muted-foreground text-xs">
          A DaemonSet runs on every matching node, so its size is the cluster's shape rather than a
          number to set here.
        </p>
      )}
      {workload.scalable && !valid && (
        <p className="text-muted-foreground text-xs">
          A replica count is a whole number, zero or more.
        </p>
      )}

      {scale.error ? <Problem error={scale.error} /> : null}
      {roll.error ? <Problem error={roll.error} /> : null}
      {pairProblem}
    </div>
  )
}
