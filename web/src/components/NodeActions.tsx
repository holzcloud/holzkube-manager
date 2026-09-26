import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Eraser, Unplug } from 'lucide-react'
import { useState } from 'react'
import { api, type Machine } from '@/api'
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
   * Called once the operator closes the removal dialog after a successful
   * removal. The record is gone by then, so the screen this component is on is
   * about a machine that no longer exists — but the dialog is also the only
   * place the cordon-and-drain notice appears, so navigating away the instant
   * the call returns would take it off the screen before it was read. The
   * caller decides where to go, after the reading.
   */
  onRemoved?: () => void
}) {
  const [resetOpen, setResetOpen] = useState(false)
  const [removeOpen, setRemoveOpen] = useState(false)

  return (
    <div className="flex gap-2 max-md:contents">
      {/* Reboot and shut down live in the power menu since 2026-09-26, as
          restart and stop -- two buttons for one act drifted apart once. */}
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
 * The route has existed since phase 9 with nothing to click, which made the
 * whole of that phase's etcd work reachable only with curl. This is the same
 * shape as Reset: the server issues a token bound to this action, this machine
 * and this cluster, and the dialog's job is to make sure the operator is
 * looking at the action the token will authorise.
 *
 * Two things are said here and nowhere else. The first is that holzkube-manager
 * cannot cordon or drain -- it speaks the Talos machine API and not the
 * Kubernetes one -- so anything still scheduled on the node stops when it does.
 * The server sends that sentence back with the result as well, and it is shown
 * again afterwards on purpose: an operator who read it on the way in has
 * stopped reading by the time it is true.
 *
 * The second is that this is not a reset. The machine keeps its disks and its
 * configuration; what it loses is its membership and its record here. Somebody
 * expecting a wipe would otherwise leave a machine on the network still holding
 * the cluster's secrets.
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
  const [notice, setNotice] = useState('')

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
      setNotice(result.notice)
      void queryClient.invalidateQueries({ queryKey: ['machines'] })
      void queryClient.invalidateQueries({ queryKey: ['clusters'] })
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
            The node leaves etcd, forfeiting leadership first if it holds it, and its record here is
            forgotten. Its disks and its configuration are untouched — this is not a reset, and the
            machine keeps the cluster's secrets until somebody wipes it.
          </DialogDescription>
        </DialogHeader>

        {notice !== '' ? (
          <div className="space-y-4 text-sm">
            <p className="rounded-md border border-emerald-600/40 bg-emerald-600/10 px-3 py-2 text-emerald-700 dark:text-emerald-300">
              {phrase} has been removed.
            </p>
            <p className="rounded-md border border-amber-600/40 bg-amber-600/10 px-3 py-2 text-amber-700 dark:text-amber-300">
              {notice}
            </p>
            <div className="flex justify-end">
              <Button
                onClick={() => {
                  onOpenChange(false)
                  onRemoved?.()
                }}
              >
                Close
              </Button>
            </div>
          </div>
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
                  {preview.data.disks.map((d) => (
                    <li key={d.device} className="flex items-center gap-2">
                      {needsDisks && (
                        <input
                          type="checkbox"
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
                      )}
                      <span>
                        {d.device} {d.pretty_size || d.size} {d.model && `(${d.model})`}
                      </span>
                    </li>
                  ))}
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
              <label className="flex items-start gap-2">
                <input
                  type="checkbox"
                  className="mt-1"
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
              <label className="flex items-start gap-2">
                <input
                  type="checkbox"
                  className="mt-1"
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
