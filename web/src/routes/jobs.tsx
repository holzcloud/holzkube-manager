import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createRoute } from '@tanstack/react-router'
import { AlertTriangle } from 'lucide-react'
import { api, type Job, type JobState } from '@/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import { authenticatedRoute } from '@/routes/__root'

/**
 * Jobs: what is running, what finished, and what is waiting for a person.
 *
 * The parked state gets the most space on this page, and that is proportionate
 * rather than decorative. Every other state is information; parked is a
 * request. It means holzkube-manager was interrupted inside a step it cannot check
 * afterwards -- a reset, in practice -- so it does not know whether the side
 * effect happened, and it refuses to guess. Both guesses are wrong in a way
 * that matters: retrying wipes a second time, giving up reports a machine as
 * intact that may not be.
 */

const JOB_STATE: Record<JobState, { label: string; className: string }> = {
  pending: { label: 'waiting', className: 'border-border bg-muted text-muted-foreground' },
  running: {
    label: 'running',
    className: 'border-sky-600/40 bg-sky-600/15 text-sky-700 dark:text-sky-300',
  },
  parked: {
    label: 'needs you',
    className: 'border-amber-600/40 bg-amber-600/15 text-amber-700 dark:text-amber-300',
  },
  succeeded: {
    label: 'done',
    className: 'border-emerald-600/40 bg-emerald-600/15 text-emerald-700 dark:text-emerald-300',
  },
  failed: {
    label: 'failed',
    className: 'border-red-600/40 bg-red-600/15 text-red-700 dark:text-red-300',
  },
  cancelled: { label: 'cancelled', className: 'border-border bg-muted text-muted-foreground' },
}

export function JobsPage() {
  const { data, isLoading, error } = useQuery({
    queryKey: ['jobs'],
    queryFn: () => api.jobs.list(),
    // Quick while something is running, because a job is the one screen
    // somebody watches. Phase 5's stream carries the same progress; this poll
    // is what keeps the list correct when no panel is open on any of them.
    refetchInterval: 3000,
  })

  const jobs = data ?? []
  const parked = jobs.filter((j) => j.state === 'parked')
  const rest = jobs.filter((j) => j.state !== 'parked')

  return (
    <section className="space-y-5">
      <div>
        <h1 className="font-heading text-xl font-semibold tracking-tight">Jobs</h1>
        <p className="max-w-prose text-sm text-muted-foreground">
          Long-running operations. They survive a restart of holzkube-manager: one is either
          continued, or handed to you because it cannot be continued safely.
        </p>
      </div>

      {isLoading && <p className="text-sm text-muted-foreground">Loading jobs</p>}
      {error && (
        <p className="text-sm text-destructive">
          The job list could not be read: {(error as Error).message}
        </p>
      )}

      {parked.length > 0 && (
        <div className="space-y-3">
          <h2 className="font-heading text-base font-semibold">Waiting for a decision</h2>
          {parked.map((j) => (
            <JobCard key={j.id} job={j} />
          ))}
        </div>
      )}

      {rest.length === 0 && parked.length === 0 && !isLoading && (
        <p className="text-sm text-muted-foreground">Nothing has been run yet.</p>
      )}

      <div className="space-y-3">
        {rest.map((j) => (
          <JobCard key={j.id} job={j} />
        ))}
      </div>
    </section>
  )
}

function JobCard({ job }: { job: Job }) {
  const queryClient = useQueryClient()

  const cancel = useMutation({
    mutationFn: () => api.jobs.cancel(job.id),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: ['jobs'] }),
  })

  const running = job.state === 'running' || job.state === 'pending'
  const presentation = JOB_STATE[job.state]

  return (
    <Card className={job.state === 'parked' ? 'border-amber-600/40' : undefined}>
      <CardHeader className="flex flex-row items-start justify-between gap-2">
        <div>
          <CardTitle className="text-base">{job.kind}</CardTitle>
          <p className="font-mono text-xs text-muted-foreground">
            {job.machine.slice(0, 8)} {new Date(job.created_at).toLocaleString()}
            {job.actor !== '' && ` by ${job.actor}`}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Badge variant="outline" className={presentation.className}>
            {presentation.label}
          </Badge>
          {running && (
            <Button
              size="sm"
              variant="outline"
              disabled={cancel.isPending || job.cancel_requested}
              onClick={() => cancel.mutate()}
              title="Stops at the next step boundary. What has already run is not undone."
            >
              {job.cancel_requested ? 'stopping' : 'Cancel'}
            </Button>
          )}
        </div>
      </CardHeader>

      <CardContent className="space-y-3 text-sm">
        {job.state === 'parked' && (
          <div className="flex items-start gap-2 rounded-md border border-amber-600/40 bg-amber-600/10 px-3 py-2 text-amber-700 dark:text-amber-300">
            <AlertTriangle aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
            <div>
              <p className="font-medium">This job needs you to decide.</p>
              <p>{job.parked_reason}</p>
            </div>
          </div>
        )}

        <ol className="space-y-1">
          {job.steps.map((step, i) => (
            <li key={step.name} className="flex items-baseline gap-2">
              <span
                className={cn(
                  'w-16 shrink-0 text-xs',
                  step.state === 'done' && 'text-emerald-700 dark:text-emerald-300',
                  step.state === 'failed' && 'text-red-700 dark:text-red-300',
                  step.state === 'running' && 'text-sky-700 dark:text-sky-300',
                  step.state === 'skipped' && 'text-muted-foreground',
                  step.state === 'pending' && 'text-muted-foreground',
                )}
              >
                {step.state}
              </span>
              <span className={i === job.current ? 'font-medium' : undefined}>
                {step.name}
                {!step.verifiable && (
                  <span
                    className="ml-2 text-xs text-muted-foreground"
                    title="This step cannot be checked after the fact. If holzkube-manager is interrupted while it runs, the job is handed to you rather than retried."
                  >
                    (not checkable)
                  </span>
                )}
                {step.detail !== '' && (
                  <span className="block text-xs text-muted-foreground">{step.detail}</span>
                )}
              </span>
            </li>
          ))}
        </ol>

        {Object.keys(job.params).length > 0 && (
          <dl className="grid grid-cols-[8rem_1fr] gap-x-3 text-xs text-muted-foreground">
            {Object.entries(job.params).map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="font-mono">{k}</dt>
                <dd className="font-mono">{v}</dd>
              </div>
            ))}
          </dl>
        )}
      </CardContent>
    </Card>
  )
}

export const jobsRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: '/jobs',
  component: JobsPage,
})
