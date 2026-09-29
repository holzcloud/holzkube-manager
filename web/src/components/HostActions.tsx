import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Download } from 'lucide-react'
import { type FormEvent, useEffect, useRef, useState } from 'react'
import { api, type Host, type HostAction, type HostOrder } from '@/api'
import { Problem } from '@/components/Problem'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

/**
 * The host actions on /host (Phase 13, HACT-01..05).
 *
 * holzkube-manager carries none of them out itself. Pressing one asks the
 * server for a confirmation token -- only given to somebody who typed this
 * machine's hostname, which the server compares with uname(2) -- and then
 * presents it to the action route, which wants an open sudo window (a 428
 * opens the password prompt through the shared interceptor and replays the
 * request) and places a one-line order for the root-owned helper. What the
 * helper made of it comes back in the host answer the page already polls, and
 * HostOrderStatus says it.
 */

/** Each action's label, as its button, its status box and its dialog name it. */
export const HOST_ACTION_LABEL: Record<HostAction, string> = {
  update: 'Check for updates and install',
  'restart-service': 'Restart service',
  reboot: 'Restart host',
  poweroff: 'Shut down host',
}

export function HostActions({
  host,
  onPlaced,
}: {
  host: Host
  onPlaced: (order: HostOrder) => void
}) {
  const [pending, setPending] = useState<HostAction | null>(null)
  const hostname = host.device.hostname.readable ? host.device.hostname.value : ''

  return (
    <div className="flex flex-col items-end gap-1 max-md:w-full max-md:items-stretch">
      {/* A fieldset is a group to assistive technology without a role
          attribute; the reset classes undo its browser frame and its
          min-content width, which would push the phone grid past the edge. */}
      <fieldset
        aria-label="Host actions"
        className="m-0 flex min-w-0 flex-wrap gap-2 border-0 p-0 max-md:grid max-md:grid-cols-2"
      >
        <Button
          variant="outline"
          size="sm"
          className="max-md:h-auto max-md:min-h-11 max-md:min-w-0 max-md:py-2 max-md:whitespace-normal"
          disabled={hostname === ''}
          onClick={() => setPending('update')}
        >
          <Download aria-hidden="true" className="size-4" />
          {HOST_ACTION_LABEL.update}
        </Button>
      </fieldset>
      {pending !== null && (
        <HostActionDialog
          action={pending}
          hostname={hostname}
          onClose={() => setPending(null)}
          onPlaced={(order) => {
            setPending(null)
            onPlaced(order)
          }}
        />
      )}
    </div>
  )
}

function HostActionDialog({
  action,
  hostname,
  onClose,
  onPlaced,
}: {
  action: HostAction
  hostname: string
  onClose: () => void
  onPlaced: (order: HostOrder) => void
}) {
  const queryClient = useQueryClient()
  const [typed, setTyped] = useState('')

  const run = useMutation({
    mutationFn: async () => {
      const { token } = await api.hostActions.confirm(action, typed)
      return api.hostActions.place(action, token)
    },
    onSuccess: ({ order }) => {
      void queryClient.invalidateQueries({ queryKey: ['host'] })
      onPlaced(order)
    },
  })

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (typed === hostname && !run.isPending) {
      run.mutate()
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent
        className="sm:max-w-md"
        // The trigger is off while the order is under way; focus goes to the
        // status box instead (HostOrderStatus takes it).
        onCloseAutoFocus={(e) => e.preventDefault()}
      >
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>
              {HOST_ACTION_LABEL[action]} on {hostname}?
            </DialogTitle>
            <DialogDescription>
              Runs the same update the hourly timer runs: it looks for a newer release and, if there
              is one, installs it and restarts holzkube-manager. If there is none, nothing changes.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4 text-sm">
            <p className="rounded-md border border-slate-500/40 bg-slate-500/10 px-3 py-2 text-slate-700 dark:text-slate-300">
              If a newer release is installed, the page loses its connection while holzkube-manager
              restarts. It keeps asking and picks up again when holzkube-manager is back.
            </p>
            <div className="space-y-1 text-xs text-muted-foreground">
              <p>
                holzkube-manager does not do this itself: it leaves an order for the
                holzkube-manager-host helper, which carries it out as root.
              </p>
              <p>
                Next, holzkube-manager asks you to confirm it is you, unless you did in the last
                five minutes.
              </p>
            </div>
            <div className="space-y-1">
              <Label htmlFor="host-action-confirm">
                Type <span className="font-mono">{hostname}</span> to confirm
              </Label>
              <Input
                id="host-action-confirm"
                value={typed}
                autoComplete="off"
                autoCapitalize="none"
                autoCorrect="off"
                spellCheck={false}
                onChange={(e) => setTyped(e.target.value)}
              />
            </div>
            {run.error ? <Problem error={run.error} /> : null}
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Keep running
            </Button>
            <Button
              type="submit"
              variant="default"
              disabled={typed !== hostname || hostname === '' || run.isPending}
            >
              <Download aria-hidden="true" className="size-4" />
              {run.isPending ? 'Working…' : HOST_ACTION_LABEL[action]}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

/** Where an order stands, as the page tells it. */
export type OrderPhase = 'placed' | 'picked-up' | 'started' | 'rejected' | 'failed'

/**
 * The phase of the order this page placed (`held`), from the host answer: the
 * helper's result for that id when there is one; otherwise the order's state
 * as the daemon reports it -- or, before the daemon has reported the order at
 * all, as the placement answered it.
 */
export function orderPhase(held: HostOrder, host: Host): OrderPhase {
  const result = host.actions.result
  if (result.readable && result.value.id === held.id) {
    return result.value.outcome
  }
  const reported = host.actions.order
  const state = reported !== null && reported.id === held.id ? reported.state : held.state
  return state === 'pending' ? 'placed' : 'picked-up'
}

const STARTED: Record<HostAction, string> = {
  update:
    'started. If a newer release exists it is installed and holzkube-manager restarts; the outcome appears here and under Update check.',
  'restart-service': 'started. holzkube-manager is restarting.',
  reboot: 'started. The host is restarting.',
  poweroff: 'started. The host is shutting down.',
}

const SLATE = 'border-slate-500/40 bg-slate-500/10 text-slate-700 dark:text-slate-300'
const RED = 'border-red-600/40 bg-red-600/10 text-red-700 dark:text-red-300'

function phaseSentence(phase: OrderPhase, action: HostAction) {
  switch (phase) {
    case 'placed':
      return 'order placed. Waiting for the helper to pick it up.'
    case 'picked-up':
      return 'the helper picked up the order.'
    case 'started':
      return STARTED[action]
    case 'rejected':
      return (
        <>
          the helper rejected the order, so nothing was done.{' '}
          <code className="font-mono text-xs">journalctl -u holzkube-manager-host</code> says why.
        </>
      )
    case 'failed':
      return (
        <>
          the helper could not carry it out.{' '}
          <code className="font-mono text-xs">journalctl -u holzkube-manager-host</code> says why.
        </>
      )
  }
}

/**
 * The order status box: first in the page's notice stack while this page holds
 * an order it placed. It takes focus when a new order arrives, because the
 * dialog that placed it has closed and its trigger is off.
 */
export function HostOrderStatus({ host, held }: { host: Host; held: HostOrder }) {
  const phase = orderPhase(held, host)
  const box = useRef<HTMLDivElement>(null)

  // Once per order: a new id is a new order to follow; the 3-s polls of the
  // same order must not pull focus back every time.
  const id = held.id
  useEffect(() => {
    if (id !== '') {
      box.current?.focus()
    }
  }, [id])

  return (
    <div
      ref={box}
      role="status"
      tabIndex={-1}
      className={`rounded-md border px-3 py-2 text-sm ${
        phase === 'rejected' || phase === 'failed' ? RED : SLATE
      }`}
    >
      <p>
        <span className="font-semibold">{HOST_ACTION_LABEL[held.action]}</span> —{' '}
        {phaseSentence(phase, held.action)}
      </p>
      <p className="mt-1 text-xs tabular-nums">
        Order <span className="font-mono">{held.id}</span> · placed{' '}
        {new Date(held.placed_at).toLocaleTimeString()}
      </p>
    </div>
  )
}
