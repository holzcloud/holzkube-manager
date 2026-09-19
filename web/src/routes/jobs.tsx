import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createRoute } from '@tanstack/react-router'
import { AlertTriangle } from 'lucide-react'
import { api, type Job, type JobState } from '@/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import { formatDuration, medianSeconds, secondsBetween, useElapsedSeconds } from '@/lib/waiting'
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
            <JobCard key={j.id} job={j} typical={typicalSecondsFor(j.kind, jobs)} />
          ))}
        </div>
      )}

      {rest.length === 0 && parked.length === 0 && !isLoading && (
        <p className="text-sm text-muted-foreground">Nothing has been run yet.</p>
      )}

      <div className="space-y-3">
        {rest.map((j) => (
          <JobCard key={j.id} job={j} typical={typicalSecondsFor(j.kind, jobs)} />
        ))}
      </div>
    </section>
  )
}

/**
 * How long a job of this kind usually takes here, from the ones that finished.
 *
 * Measured and never guessed, which is the operator's decision of 2026-09-17
 * and the whole point of ledger entry 62: a constant somebody thought plausible
 * is an assertion with nothing behind it. Only jobs that reached `done` count --
 * a failed one stopped early and a parked one waited on a person, and neither
 * says anything about how long the work takes.
 */
export function typicalSecondsFor(kind: string, jobs: readonly Job[]): number | null {
  const samples: number[] = []
  for (const job of jobs) {
    if (job.kind !== kind || job.state !== 'succeeded') {
      continue
    }
    const seconds = secondsBetween(job.started_at, job.finished_at)
    if (seconds !== null) {
      samples.push(seconds)
    }
  }
  return medianSeconds(samples)
}

function JobCard({ job, typical }: { job: Job; typical: number | null }) {
  const queryClient = useQueryClient()

  const cancel = useMutation({
    mutationFn: () => api.jobs.cancel(job.id),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: ['jobs'] }),
  })

  const running = job.state === 'running' || job.state === 'pending'
  const presentation = JOB_STATE[job.state]

  // How long this one has been going, and how long its kind usually takes.
  // PITFALLS.md:164 item 5 asks for both, and for a reason that is specific:
  // an operator who cannot tell a slow run from a stuck one clicks the button
  // again, and for bootstrap the second click is a split brain.
  const elapsed = useElapsedSeconds(job.started_at, running)
  const step = job.steps[job.current]
  const stepElapsed = useElapsedSeconds(step?.started_at, running && step?.state === 'running')

  return (
    <Card className={job.state === 'parked' ? 'border-amber-600/40' : undefined}>
      <CardHeader className="flex flex-row items-start justify-between gap-2">
        <div>
          <CardTitle className="text-base">{job.kind}</CardTitle>
          <p className="font-mono text-xs text-muted-foreground">
            {job.machine.slice(0, 8)} {new Date(job.created_at).toLocaleString()}
            {job.actor !== '' && ` by ${job.actor}`}
          </p>

          {/* The elapsed time and what it should be compared against. The
              expected figure is the MEDIAN OF JOBS OF THIS KIND THAT HAVE
              ACTUALLY FINISHED on this installation -- not a constant somebody
              thought plausible, which is what ledger entry 62 is about. Until
              one has finished it says so, because "no experience yet" is a
              true statement and a made-up number is not. */}
          {running && elapsed !== null && (
            <p className="text-xs text-muted-foreground">
              Running for{' '}
              <span className="font-medium text-foreground">{formatDuration(elapsed)}</span>
              {typical !== null ? (
                <>
                  {' · '}
                  {elapsed > typical * 2
                    ? `well past the usual ${formatDuration(typical)} for a ${job.kind}`
                    : `a ${job.kind} here usually takes ${formatDuration(typical)}`}
                </>
              ) : (
                ' · no finished job of this kind to compare against yet'
              )}
            </p>
          )}
        </div>
        <div className="flex items-center gap-2">
          <Badge variant="outline" className={presentation.className}>
            {presentation.label}
          </Badge>
          {running && (
            <div className="flex flex-col items-end gap-1">
              <Button
                size="sm"
                variant="outline"
                disabled={cancel.isPending || job.cancel_requested}
                onClick={() => cancel.mutate()}
                title="Stops at the next step boundary. What has already run is not undone."
              >
                {job.cancel_requested ? 'stopping' : 'Cancel'}
              </Button>
              {/* The reason BESIDE the button and not only in its tooltip.
                  PITFALLS.md: "A disabled button with a reason prevents the
                  click that a spinner invites" -- and a reason only a hover
                  reveals is no reason at all on a phone, which has no hover. */}
              {job.cancel_requested && (
                <span className="text-xs text-muted-foreground">
                  already asked to stop{elapsed !== null && `, ${formatDuration(elapsed)} in`}
                </span>
              )}
            </div>
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
                {/* The sub-step's own clock. The job's total says whether the
                    whole thing is slow; this says WHICH part is, which is the
                    difference between "it is taking a while" and "it has been
                    on 'wait for the node to come back' for nine minutes". */}
                {i === job.current && step.state === 'running' && stepElapsed !== null && (
                  <span className="block text-xs text-muted-foreground">
                    on this step for {formatDuration(stepElapsed)}
                  </span>
                )}
              </span>
            </li>
          ))}
        </ol>

        {Object.keys(job.params).length > 0 && (
          <dl className="grid grid-cols-1 md:grid-cols-[8rem_minmax(0,1fr)] gap-x-3 text-xs text-muted-foreground">
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
