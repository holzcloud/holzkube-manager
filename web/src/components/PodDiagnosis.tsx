import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { api, type KubeContainer } from '@/api'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

/**
 * Why a pod is broken (2026-09-19).
 *
 * Everything on the Kubernetes screen before this said WHAT was wrong — a pod
 * in CrashLoopBackOff, one of two containers ready. None of it said why, and an
 * operator who gets that far still reaches for kubectl. This is the panel that
 * answers the question instead.
 *
 * Three things, in the order somebody needs them:
 *
 *   1. Which container, and how it ended. An exit code of 137 is a memory
 *      limit; 1 is a throw; ImagePullBackOff never started. Different days.
 *   2. Its log — and by default the log of the run that ALREADY ENDED, when
 *      there is one. A pod in CrashLoopBackOff has printed nothing in its
 *      current container, so a log view that showed only that one would answer
 *      every crash loop with an empty box.
 *   3. The events, because "Pending" is never explained by the pod. It is
 *      explained by "0/2 nodes are available: insufficient cpu", which exists
 *      only here.
 */
export function PodDiagnosis({
  clusterID,
  namespace,
  pod,
  onClose,
}: {
  clusterID: string
  namespace: string
  pod: string
  onClose: () => void
}) {
  const containers = useQuery({
    queryKey: ['kubernetes', 'containers', clusterID, namespace, pod],
    queryFn: () => api.kubernetes.containers(clusterID, namespace, pod),
  })

  const events = useQuery({
    queryKey: ['kubernetes', 'pod-events', clusterID, namespace, pod],
    queryFn: () => api.kubernetes.podEvents(clusterID, namespace, pod),
  })

  const [chosen, setChosen] = useState<string | null>(null)
  const list = containers.data ?? []
  const container = list.find((c) => c.name === chosen) ?? list[0]

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90dvh] overflow-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle className="break-all font-mono text-base">{pod}</DialogTitle>
          <DialogDescription>{namespace}</DialogDescription>
        </DialogHeader>

        {containers.error ? <Problem error={containers.error} /> : null}

        {list.length > 1 && (
          <div className="flex flex-wrap gap-2">
            {list.map((c) => (
              <Button
                key={c.name}
                type="button"
                size="sm"
                variant={c.name === container?.name ? 'default' : 'outline'}
                className="max-md:h-11"
                onClick={() => setChosen(c.name)}
              >
                {c.name}
                {c.init && <span className="ml-1 text-xs opacity-70">(init)</span>}
              </Button>
            ))}
          </div>
        )}

        {container && <ContainerFacts container={container} />}

        {container && (
          <LogPanel clusterID={clusterID} namespace={namespace} pod={pod} container={container} />
        )}

        <YamlPanel clusterID={clusterID} namespace={namespace} pod={pod} />

        <section className="space-y-2">
          <h3 className="font-medium text-sm">What the cluster reported</h3>
          {events.error ? <Problem error={events.error} /> : null}
          {events.data && events.data.events.length === 0 && (
            <p className="text-muted-foreground text-sm">{events.data.notice}</p>
          )}
          {events.data && events.data.events.length > 0 && (
            <>
              <ul className="space-y-1 text-sm">
                {events.data.events.map((event) => (
                  <li key={`${event.reason}-${event.last_seen}-${event.message}`}>
                    <span
                      className={
                        event.type === 'Warning'
                          ? 'font-medium text-amber-700 dark:text-amber-300'
                          : 'font-medium'
                      }
                    >
                      {event.reason}
                    </span>
                    {event.count > 1 && (
                      <span className="text-muted-foreground"> ×{event.count}</span>
                    )}{' '}
                    <span className="break-words">{event.message}</span>
                  </li>
                ))}
              </ul>
              {/* Said even when there ARE events: the ones older than the
                  cluster's retention are gone, and a list that looked complete
                  would be a claim nobody can support. */}
              <p className="text-muted-foreground text-xs">{events.data.notice}</p>
            </>
          )}
        </section>
      </DialogContent>
    </Dialog>
  )
}

function ContainerFacts({ container }: { container: KubeContainer }) {
  return (
    <section className="space-y-1 text-sm">
      {container.explanation !== '' && (
        <p className="font-medium" role="status">
          {container.explanation}
        </p>
      )}
      <dl className="grid grid-cols-1 gap-x-3 gap-y-1 md:grid-cols-[8rem_minmax(0,1fr)]">
        <dt className="text-muted-foreground">Image</dt>
        <dd className="break-all font-mono text-xs">{container.image || '—'}</dd>

        <dt className="text-muted-foreground">State</dt>
        <dd>
          {container.state}
          {container.reason !== '' && ` (${container.reason})`}
        </dd>

        <dt className="text-muted-foreground">Restarts</dt>
        <dd className="tabular-nums">{container.restarts}</dd>

        {container.last_state !== '' && (
          <>
            <dt className="text-muted-foreground">Previous run</dt>
            <dd>
              exited {container.last_exit_code}
              {container.last_reason !== '' && ` (${container.last_reason})`}
            </dd>
          </>
        )}

        <dt className="text-muted-foreground">Requests</dt>
        {/* An empty one is worth showing rather than hiding: a container that
            asked for nothing is one the scheduler places blind and the kubelet
            evicts first. */}
        <dd>
          {container.cpu_request === '' && container.memory_request === ''
            ? 'none — the scheduler places this blind'
            : `${container.cpu_request || '—'} CPU, ${container.memory_request || '—'} memory`}
        </dd>

        <dt className="text-muted-foreground">Limits</dt>
        <dd>
          {container.cpu_limit === '' && container.memory_limit === ''
            ? 'none'
            : `${container.cpu_limit || '—'} CPU, ${container.memory_limit || '—'} memory`}
        </dd>
      </dl>

      {container.message !== '' && (
        <p className="break-words text-muted-foreground text-xs">{container.message}</p>
      )}
    </section>
  )
}

function LogPanel({
  clusterID,
  namespace,
  pod,
  container,
}: {
  clusterID: string
  namespace: string
  pod: string
  container: KubeContainer
}) {
  // The previous run by default when there is one, because that is the log the
  // panel was opened for. A screen that defaulted to the running container
  // would show an empty box for exactly the case somebody is investigating.
  const [previous, setPrevious] = useState(container.has_previous)

  const log = useQuery({
    queryKey: ['kubernetes', 'log', clusterID, namespace, pod, container.name, previous],
    queryFn: () =>
      api.kubernetes.logs(clusterID, namespace, pod, { container: container.name, previous }),
  })

  return (
    <section className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="font-medium text-sm">Log</h3>
        {container.has_previous && (
          <div className="flex gap-2">
            <Button
              type="button"
              size="sm"
              variant={previous ? 'default' : 'outline'}
              className="max-md:h-11"
              onClick={() => setPrevious(true)}
            >
              The run that crashed
            </Button>
            <Button
              type="button"
              size="sm"
              variant={previous ? 'outline' : 'default'}
              className="max-md:h-11"
              onClick={() => setPrevious(false)}
            >
              Now
            </Button>
          </div>
        )}
      </div>

      {log.error ? <Problem error={log.error} /> : null}
      {log.isPending && <p className="text-muted-foreground text-sm">Reading…</p>}

      {log.data && (
        <>
          {log.data.truncated && (
            <p className="text-muted-foreground text-xs">
              Only the end is shown: the beginning was cut, because the last lines are the ones that
              explain a failure.
            </p>
          )}
          {log.data.lines.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              This container has printed nothing{previous ? ' in its previous run' : ' yet'}.
            </p>
          ) : (
            // Text in a pre. A log is a workload's own output and is never
            // rendered as markup, for the same reason the service proxy's
            // answer is not.
            <pre className="max-h-80 overflow-auto rounded-lg border bg-muted/40 p-2.5 font-mono text-xs whitespace-pre-wrap">
              {log.data.lines.join('\n')}
            </pre>
          )}
        </>
      )}
    </section>
  )
}

/**
 * The whole object, on request.
 *
 * Behind a button rather than open, because it is long and it is the last
 * resort: the panel above answers most questions, and this answers the one it
 * did not. Fetched only when asked, so opening a pod does not pull a document
 * nobody reads.
 */
function YamlPanel({
  clusterID,
  namespace,
  pod,
}: {
  clusterID: string
  namespace: string
  pod: string
}) {
  const [open, setOpen] = useState(false)

  const object = useQuery({
    queryKey: ['kubernetes', 'object', clusterID, namespace, pod],
    queryFn: () =>
      api.kubernetes.object(clusterID, {
        api_version: 'v1',
        kind: 'Pod',
        namespace,
        name: pod,
      }),
    enabled: open,
  })

  return (
    <section className="space-y-2">
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="max-md:h-11"
        onClick={() => setOpen((was) => !was)}
      >
        {open ? 'Hide the object' : 'Show the whole object'}
      </Button>

      {open && object.error ? <Problem error={object.error} /> : null}
      {open && object.isPending && <p className="text-muted-foreground text-sm">Reading…</p>}
      {open && object.data && (
        <>
          {object.data.notice !== '' && (
            <p className="text-muted-foreground text-xs">{object.data.notice}</p>
          )}
          {object.data.truncated && (
            <p className="text-muted-foreground text-xs">
              Cut: this object is longer than the screen shows.
            </p>
          )}
          <pre className="max-h-80 overflow-auto rounded-lg border bg-muted/40 p-2.5 font-mono text-xs whitespace-pre-wrap">
            {object.data.yaml}
          </pre>
        </>
      )}
    </section>
  )
}
