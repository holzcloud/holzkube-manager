import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Eraser, Unplug } from 'lucide-react'
import { useEffect, useState } from 'react'
import { api, type Job, type Machine } from '@/api'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { messageFor } from '@/lib/problem'
import { cn } from '@/lib/utils'

/**
 * Reboot, shut down, reset, and remove from the cluster.
 *
 * Every one of them goes through the server twice: once to get a confirmation
 * token for exactly this action with exactly these parameters, and once to
 * submit it. The dialog below is therefore not the gate -- the server is. What
 * the dialog does is make sure the operator is looking at the same action the
 * token will authorise, which is why it shows the effective flags rather than
 * a summary.
 *
 * The reset dialog is deliberately the longest thing on this page. `talosctl
 * reset` defaults to wiping every disk and leaving the machine powered off,
 * and an operator who knows that tool will assume the same here unless told
 * otherwise. So nothing is pre-selected to match it, and the difference is
 * stated on the screen.
 */
export function NodeActions({
  machine,
  onRemoved,
}: {
  machine: Machine
  /**
   * Called once the operator closes the removal dialog after the removal job
   * has succeeded. The record is gone by then, so the screen this component is
   * on is about a machine that no longer exists — but the dialog is also the
   * only place the cordon-and-drain notice appears, so navigating away the
   * instant the job finishes would take it off the screen before it was read.
   * The caller decides where to go, after the reading. It is not called for a
   * job that failed or is parked: the record is still there.
   */
  onRemoved?: () => void
}) {
  const [resetOpen, setResetOpen] = useState(false)
  const [removeOpen, setRemoveOpen] = useState(false)

  return (
    <div className="flex gap-2 max-md:contents">
      {/* Reboot and shut down live in the power menu since 2026-09-26, as
          restart and stop -- two buttons for one act drifted apart once. */}

      {/*
        Only for a node that is in a cluster. A machine in maintenance mode has
        nothing to be removed from, and a button that answers "this machine
        belongs to no cluster" is a button that teaches the operator to ignore
        what it says.
      */}
      {machine.cluster !== '' && (
        <Button
          variant="outline"
          size="sm"
          className="text-destructive max-md:min-w-11"
          onClick={() => setRemoveOpen(true)}
          title="Remove from cluster"
          aria-label="Remove from cluster"
        >
          <Unplug aria-hidden="true" className="size-4" />{' '}
          <span className="max-md:sr-only">Remove from cluster</span>
        </Button>
      )}

      <Button
        variant="outline"
        size="sm"
        className="text-destructive max-md:min-w-11"
        onClick={() => setResetOpen(true)}
        title="Reset"
        aria-label="Reset"
      >
        <Eraser aria-hidden="true" className="size-4" />{' '}
        <span className="max-md:sr-only">Reset</span>
      </Button>

      <ResetDialog machine={machine} open={resetOpen} onOpenChange={setResetOpen} />
      <RemoveFromClusterDialog
        machine={machine}
        open={removeOpen}
        onOpenChange={setRemoveOpen}
        onRemoved={onRemoved}
      />
    </div>
  )
}

/**
 * Taking a node out of its cluster for good (UPG-13).
 *
 * The same shape as Reset: the server issues a token bound to this action,
 * this machine and this cluster, and the dialog's job is to make sure the
 * operator is looking at the action the token will authorise.
 *
 * It is a job. Submitting answers 202 straight away — or 409 when the cluster
 * cannot spare the voter, which is checked before any job exists — and the
 * dialog then follows the job through its steps (leave etcd, wait, wipe,
 * forget) until it is done, failed or parked. Closing the dialog does not stop
 * it: the job lives on the daemon and on the Jobs page.
 *
 * Three things are said here and nowhere else. The first is that
 * holzkube-manager cannot cordon or drain -- it speaks the Talos machine API
 * and not the Kubernetes one -- so anything still scheduled on the node stops
 * when it does. The server sends that sentence back with the job as well, and
 * it is shown again afterwards on purpose: an operator who read it on the way
 * in has stopped reading by the time it is true.
 *
 * The second is that the node's system disk is wiped. It is not wiped further:
 * data disks stay, and the machine comes back in maintenance mode, ready to be
 * provisioned again.
 *
 * The third is where a job that did not finish leaves things. A failure before
 * the wipe leaves the node as it was; a parked job means the wipe may or may
 * not have run, and a person has to look.
 */
function RemoveFromClusterDialog({
  machine,
  open,
  onOpenChange,
  onRemoved,
}: {
  machine: Machine
  open: boolean
  onOpenChange: (v: boolean) => void
  onRemoved?: () => void
}) {
  const queryClient = useQueryClient()
  const [typed, setTyped] = useState('')
  const [failure, setFailure] = useState('')
  const [accepted, setAccepted] = useState<{ job: string; notice: string } | null>(null)

  // The hostname, for the reason the reset dialog gives: it is the thing the
  // operator can check against the machine in front of them. A node that has
  // not reported one falls back to its id, which is still specific to this
  // machine -- unlike a generic word, which confirms only that somebody can
  // type.
  const phrase = machine.hostname.value || machine.id

  const run = useMutation({
    mutationFn: async () => {
      const params = { cluster: machine.cluster }
      const { token } = await api.machines.confirm(
        machine.id,
        'node.remove-from-cluster',
        params,
        typed,
      )
      return api.machines.removeFromCluster(machine.id, machine.cluster, token)
    },
    onSuccess: (result) => {
      setFailure('')
      setAccepted({ job: result.job.id, notice: result.notice })
      void queryClient.invalidateQueries({ queryKey: ['jobs'] })
    },
    onError: (e: Error) => setFailure(e.message),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-destructive">
            <AlertTriangle aria-hidden="true" className="size-5" />
            Remove {phrase} from its cluster
          </DialogTitle>
          <DialogDescription>
            The node leaves etcd, forfeiting leadership first if it holds it, waits for etcd to
            settle, has its system disk wiped, and its record here is forgotten. This is not a full
            reset: its other disks are left alone, and the machine comes back in maintenance mode,
            ready to be provisioned again.
          </DialogDescription>
        </DialogHeader>

        {accepted !== null ? (
          <RemovalProgress
            jobID={accepted.job}
            notice={accepted.notice}
            phrase={phrase}
            onClose={(removed) => {
              onOpenChange(false)
              if (removed) {
                onRemoved?.()
              }
            }}
          />
        ) : (
          <div className="space-y-4 text-sm">
            <p className="rounded-md border border-amber-600/40 bg-amber-600/10 px-3 py-2 text-amber-700 dark:text-amber-300">
              holzkube-manager speaks the Talos machine API and not the Kubernetes API, so it cannot
              cordon or drain this node. Anything still scheduled on it stops when it does. Run{' '}
              <code>kubectl drain &lt;node&gt;</code> first if those workloads need to move rather
              than restart.
            </p>

            {machine.etcd_member.value && (
              <p className="rounded-md border border-red-600/40 bg-red-600/10 px-3 py-2 text-red-700 dark:text-red-300">
                This node is an etcd member. Removing it lowers the cluster's voting count, and a
                cluster that drops below a quorum stops accepting writes until enough members are
                back.
              </p>
            )}

            <div className="space-y-1">
              <Label htmlFor="remove-confirm">
                Type <span className="font-mono">{phrase}</span> to confirm
              </Label>
              <Input
                id="remove-confirm"
                value={typed}
                autoComplete="off"
                onChange={(e) => setTyped(e.target.value)}
              />
            </div>

            {failure !== '' && <p className="text-destructive">{failure}</p>}

            <div className="flex justify-end gap-2">
              <Button variant="ghost" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button
                variant="destructive"
                disabled={typed !== phrase || run.isPending}
                onClick={() => run.mutate()}
              >
                Remove from cluster
              </Button>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

const REMOVAL_POLL_MS = 2000

function jobIsOver(job: Job | undefined): boolean {
  return (
    job !== undefined &&
    (job.state === 'succeeded' ||
      job.state === 'failed' ||
      job.state === 'cancelled' ||
      job.state === 'parked')
  )
}

/**
 * Follows the removal job: its steps while it runs, then what it ended as.
 *
 * Polled rather than streamed, as the Jobs page is: this is one job for a
 * minute, and a poll that survives a dropped connection is worth more here than
 * a stream that needs reconnecting. A poll that fails is said, not hidden —
 * a progress panel that quietly stops moving is the thing that makes an
 * operator click the button again.
 */
function RemovalProgress({
  jobID,
  notice,
  phrase,
  onClose,
}: {
  jobID: string
  notice: string
  phrase: string
  onClose: (removed: boolean) => void
}) {
  const queryClient = useQueryClient()

  const poll = useQuery({
    queryKey: ['job', jobID],
    queryFn: () => api.jobs.get(jobID),
    refetchInterval: (query) => (jobIsOver(query.state.data) ? false : REMOVAL_POLL_MS),
  })

  const job = poll.data
  const done = job?.state === 'succeeded'
  const over = jobIsOver(job)

  // The record is gone once the job has succeeded; the lists have to say so.
  useEffect(() => {
    if (job !== undefined && over) {
      void queryClient.invalidateQueries({ queryKey: ['jobs'] })
      void queryClient.invalidateQueries({ queryKey: ['machines'] })
      void queryClient.invalidateQueries({ queryKey: ['clusters'] })
    }
  }, [job, over, queryClient])

  const failedStep = job?.steps.find((s) => s.state === 'failed')

  return (
    <div className="space-y-4 text-sm">
      {job === undefined ? (
        <p className="text-muted-foreground">
          {poll.error ? `The job could not be read: ${messageFor(poll.error)}` : 'Starting…'}
        </p>
      ) : (
        <>
          <ol className="space-y-1" aria-label="Removal steps">
            {job.steps.map((step) => (
              <li key={step.name} className="flex items-baseline gap-2">
                <span
                  className={cn(
                    'w-16 shrink-0 text-xs',
                    step.state === 'done' && 'text-emerald-700 dark:text-emerald-300',
                    step.state === 'failed' && 'text-red-700 dark:text-red-300',
                    step.state === 'running' && 'text-sky-700 dark:text-sky-300',
                    (step.state === 'skipped' || step.state === 'pending') &&
                      'text-muted-foreground',
                  )}
                >
                  {step.state}
                </span>
                <span>
                  {step.name}
                  {step.detail !== '' && (
                    <span className="block text-xs text-muted-foreground">{step.detail}</span>
                  )}
                </span>
              </li>
            ))}
          </ol>

          {job.state === 'failed' && (
            <p className="rounded-md border border-red-600/40 bg-red-600/10 px-3 py-2 text-red-700 dark:text-red-300">
              The removal stopped at “{failedStep?.name ?? 'a step'}”.{' '}
              {failedStep?.name === 'wipe the node' || failedStep?.name === 'forget the node'
                ? 'The node has already left etcd and may be wiped. Look at it before trying again.'
                : 'Nothing was wiped and the node is still in the inventory.'}
            </p>
          )}

          {job.state === 'parked' && (
            <p className="rounded-md border border-amber-600/40 bg-amber-600/10 px-3 py-2 text-amber-700 dark:text-amber-300">
              <span className="font-medium">This removal needs you to decide.</span>{' '}
              {job.parked_reason} It is on the Jobs page.
            </p>
          )}

          {job.state === 'cancelled' && (
            <p className="rounded-md border px-3 py-2 text-muted-foreground">
              The removal was cancelled at a step boundary. What already ran is not undone.
            </p>
          )}

          {done && (
            <p className="rounded-md border border-emerald-600/40 bg-emerald-600/10 px-3 py-2 text-emerald-700 dark:text-emerald-300">
              {phrase} has been removed.
            </p>
          )}

          {!over && poll.error && (
            <p className="text-destructive">
              The job's progress could not be read: {messageFor(poll.error)}. It is still running on
              the daemon.
            </p>
          )}
        </>
      )}

      {/* Shown from the start, not only at the end: it is true from the first
          step, and an operator who watches the progress should not have to wait
          for the last one to be told. */}
      <p className="rounded-md border border-amber-600/40 bg-amber-600/10 px-3 py-2 text-amber-700 dark:text-amber-300">
        {notice}
      </p>

      <div className="flex justify-end">
        <Button onClick={() => onClose(done)}>{over ? 'Done' : 'Close and keep going'}</Button>
      </div>
    </div>
  )
}

/**
 * The reset dialog (JOB-07).
 *
 * It shows the disks, the scope choice, both flags and what they mean, and it
 * requires the hostname to be typed -- the machine's own name, because that is
 * the thing the operator can check against the machine in front of them. A
 * generic "DELETE" confirms that somebody can type, not that they know which
 * machine this is.
 */
function ResetDialog({
  machine,
  open,
  onOpenChange,
}: {
  machine: Machine
  open: boolean
  onOpenChange: (v: boolean) => void
}) {
  const queryClient = useQueryClient()
  const [mode, setMode] = useState('')
  const [graceful, setGraceful] = useState(true)
  const [reboot, setReboot] = useState(true)
  const [disks, setDisks] = useState<string[]>([])
  const [typed, setTyped] = useState('')
  const [failure, setFailure] = useState('')

  const preview = useQuery({
    queryKey: ['reset-preview', machine.id],
    queryFn: () => api.machines.resetPreview(machine.id),
    enabled: open,
  })

  const chosenMode = mode || preview.data?.defaults.mode || ''
  const needsDisks = preview.data?.modes.find((m) => m.mode === chosenMode)?.needs_disks ?? false

  const params: Record<string, string> = {
    wipe_mode: chosenMode,
    graceful: String(graceful),
    reboot: String(reboot),
  }
  if (needsDisks) {
    params.user_disks = disks.join(',')
  }

  const run = useMutation({
    mutationFn: async () => {
      const { token } = await api.machines.confirm(machine.id, 'node.reset', params, typed)
      return api.machines.action(machine.id, 'reset', token, params, machine.cluster)
    },
    onSuccess: () => {
      onOpenChange(false)
      setFailure('')
      void queryClient.invalidateQueries({ queryKey: ['jobs'] })
    },
    onError: (e: Error) => setFailure(e.message),
  })

  const ready =
    chosenMode !== '' &&
    typed === (preview.data?.confirm_phrase ?? ' ') &&
    (!needsDisks || disks.length > 0)

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-destructive">
            <AlertTriangle aria-hidden="true" className="size-5" />
            Reset {preview.data?.hostname || machine.id.slice(0, 8)}
          </DialogTitle>
          <DialogDescription>
            This wipes disks on a real machine. Nothing here is pre-selected to match{' '}
            <code>talosctl</code>.
          </DialogDescription>
        </DialogHeader>

        {preview.isLoading && <p className="text-sm text-muted-foreground">Reading the machine</p>}

        {preview.data && (
          <div className="space-y-4 text-sm">
            <p className="rounded-md border border-amber-600/40 bg-amber-600/10 px-3 py-2 text-amber-700 dark:text-amber-300">
              {preview.data.talos_default_warning}
            </p>

            <div>
              <p className="mb-1 font-medium">Disks on this machine</p>
              {preview.data.disks.length === 0 ? (
                <p className="text-muted-foreground">
                  None reported. The machine has not answered recently, so refresh it before
                  resetting.
                </p>
              ) : (
                <ul className="font-mono text-xs">
                  {preview.data.disks.map((d) => {
                    const text = (
                      <span>
                        {d.device} {d.pretty_size || d.size} {d.model && `(${d.model})`}
                      </span>
                    )
                    return (
                      <li key={d.device}>
                        {needsDisks ? (
                          /* One label per row, around its own checkbox and its
                             text, so on a phone the whole 44px row is the tap
                             target and a tap toggles only this disk. The label
                             carries the row's flex classes, so above md
                             checkbox and text sit where they sat. The checkbox
                             keeps its own name: a tap is not a reset -- the
                             typed phrase and the server's token still are. */
                          <label className="flex items-center gap-2 max-md:min-h-11">
                            <input
                              type="checkbox"
                              className="max-md:size-6"
                              aria-label={`Wipe ${d.device}`}
                              checked={disks.includes(d.device)}
                              onChange={(e) =>
                                setDisks((prev) =>
                                  e.target.checked
                                    ? [...prev, d.device]
                                    : prev.filter((x) => x !== d.device),
                                )
                              }
                            />
                            {text}
                          </label>
                        ) : (
                          <div className="flex items-center gap-2">{text}</div>
                        )}
                      </li>
                    )
                  })}
                </ul>
              )}
            </div>

            <fieldset className="space-y-2">
              <legend className="mb-1 font-medium">What to wipe</legend>
              {preview.data.modes.map((m) => (
                <label key={m.mode} className="flex items-start gap-2">
                  <input
                    type="radio"
                    name="wipe-mode"
                    className="mt-1"
                    checked={chosenMode === m.mode}
                    onChange={() => setMode(m.mode)}
                  />
                  <span>
                    <span className="font-medium">{m.label}</span>
                    <span className="block text-muted-foreground">{m.description}</span>
                  </span>
                </label>
              ))}
            </fieldset>

            <fieldset className="space-y-2">
              <legend className="mb-1 font-medium">Effective flags</legend>
              <label className="flex items-start gap-2 max-md:min-h-11">
                <input
                  type="checkbox"
                  className="mt-1 max-md:mt-3 max-md:size-6"
                  checked={graceful}
                  onChange={(e) => setGraceful(e.target.checked)}
                />
                <span>
                  Leave etcd first
                  <span className="block text-muted-foreground">
                    {graceful
                      ? 'The node leaves the etcd cluster cleanly before wiping.'
                      : 'The node is wiped while still an etcd member. On a three-node control plane this can cost quorum.'}
                  </span>
                </span>
              </label>
              <label className="flex items-start gap-2 max-md:min-h-11">
                <input
                  type="checkbox"
                  className="mt-1 max-md:mt-3 max-md:size-6"
                  checked={reboot}
                  onChange={(e) => setReboot(e.target.checked)}
                />
                <span>
                  Reboot afterwards
                  <span className="block text-muted-foreground">
                    {reboot
                      ? 'The machine comes back up, in maintenance mode if its system disk was wiped.'
                      : 'The machine halts. Somebody has to walk over and turn it on.'}
                  </span>
                </span>
              </label>
            </fieldset>

            <div className="space-y-1">
              <Label htmlFor="reset-confirm">
                Type <span className="font-mono">{preview.data.confirm_phrase}</span> to confirm
              </Label>
              <Input
                id="reset-confirm"
                value={typed}
                autoComplete="off"
                onChange={(e) => setTyped(e.target.value)}
              />
            </div>

            {failure !== '' && <p className="text-destructive">{failure}</p>}

            <div className="flex justify-end gap-2">
              <Button variant="ghost" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button
                variant="destructive"
                disabled={!ready || run.isPending}
                onClick={() => run.mutate()}
              >
                Reset this machine
              </Button>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
