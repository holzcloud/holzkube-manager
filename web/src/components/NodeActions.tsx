import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Power, RotateCcw, Trash2 } from 'lucide-react'
import type { ReactNode } from 'react'
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
 * Reboot, shut down, reset.
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
export function NodeActions({ machine }: { machine: Machine }) {
  const [resetOpen, setResetOpen] = useState(false)

  return (
    <div className="flex flex-wrap gap-2">
      <SimpleAction
        machine={machine}
        kind="reboot"
        label="Reboot"
        icon={<RotateCcw aria-hidden="true" className="size-4" />}
        description="The node reboots and comes back on its own. Workloads on it stop until it does."
      />
      <SimpleAction
        machine={machine}
        kind="shutdown"
        label="Shut down"
        icon={<Power aria-hidden="true" className="size-4" />}
        description="The node powers off and does not come back until somebody turns it on."
      />
      <Button
        variant="outline"
        size="sm"
        className="text-destructive"
        onClick={() => setResetOpen(true)}
      >
        <Trash2 aria-hidden="true" className="size-4" /> Reset
      </Button>

      <ResetDialog machine={machine} open={resetOpen} onOpenChange={setResetOpen} />
    </div>
  )
}

/**
 * Reboot and shutdown: one confirmation, no typing.
 *
 * Asking somebody to type a hostname before every reboot is how they learn to
 * paste it without reading -- and then the typing means nothing on the one
 * screen where it matters.
 */
function SimpleAction({
  machine,
  kind,
  label,
  icon,
  description,
}: {
  machine: Machine
  kind: 'reboot' | 'shutdown'
  label: string
  icon: ReactNode
  description: string
}) {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [failure, setFailure] = useState('')

  const run = useMutation({
    mutationFn: async () => {
      const { token } = await api.machines.confirm(machine.id, `node.${kind}`, {}, '')
      return api.machines.action(machine.id, kind, token, {}, machine.cluster)
    },
    onSuccess: () => {
      setOpen(false)
      setFailure('')
      void queryClient.invalidateQueries({ queryKey: ['jobs'] })
    },
    onError: (e: Error) => setFailure(e.message),
  })

  return (
    <>
      <Button variant="outline" size="sm" onClick={() => setOpen(true)}>
        {icon} {label}
      </Button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {label} {machine.hostname.value || machine.id.slice(0, 8)}?
            </DialogTitle>
            <DialogDescription>{description}</DialogDescription>
          </DialogHeader>

          {failure !== '' && <p className="text-sm text-destructive">{failure}</p>}

          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button disabled={run.isPending} onClick={() => run.mutate()}>
              {label}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
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
