import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  AlertTriangle,
  Download,
  type LucideIcon,
  PowerOff,
  RotateCcw,
  RotateCw,
} from 'lucide-react'
import { type FormEvent, Fragment, type ReactNode, useEffect, useRef, useState } from 'react'
import { api, type Host, type HostAction, type HostOrder, roleAtLeast } from '@/api'
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
 *
 * Everything the block decides -- whether the buttons are on, and if not, why
 * -- is drawn from that one answer and the session's role. The server is the
 * lock (the routes refuse a reader, a container, a missing helper and a second
 * order); the page only never offers what the route will refuse.
 */

/** Each action's label, as its button, its status box and its dialog name it. */
export const HOST_ACTION_LABEL: Record<HostAction, string> = {
  update: 'Check for updates and install',
  'restart-service': 'Restart service',
  reboot: 'Restart host',
  poweroff: 'Shut down host',
}

const SLATE = 'border-slate-500/40 bg-slate-500/10 text-slate-700 dark:text-slate-300'
const RED = 'border-red-600/40 bg-red-600/10 text-red-700 dark:text-red-300'

/** The hostname as the dialogs show it: one word a phone may break anywhere. */
function Hostname({ name }: { name: string }) {
  return <span className="font-mono break-all">{name}</span>
}

type ActionSpec = {
  icon: LucideIcon
  /** The dialog's question, with the hostname in it. */
  title: (hostname: string) => ReactNode
  description: string
  /** What happens to this page, in the box under the description. */
  consequence: string
  consequenceClass: string
  /**
   * The machine-wide pair: red trigger text, a red title with the triangle and
   * a destructive confirm. The other two take down only this service, which
   * comes back by itself (PowerMenu's forced / unforced split).
   */
  destructive: boolean
}

/** The four actions, in the order the header shows them: harmless first. */
export const HOST_ACTIONS: HostAction[] = ['update', 'restart-service', 'reboot', 'poweroff']

const ACTION: Record<HostAction, ActionSpec> = {
  update: {
    icon: Download,
    title: (h) => (
      <>
        Check for updates and install on <Hostname name={h} />?
      </>
    ),
    description:
      'Runs the same update the hourly timer runs: it looks for a newer release and, if there is one, installs it and restarts holzkube-manager. If there is none, nothing changes.',
    consequence:
      'If a newer release is installed, the page loses its connection while holzkube-manager restarts. It keeps asking and picks up again when holzkube-manager is back.',
    consequenceClass: SLATE,
    destructive: false,
  },
  'restart-service': {
    icon: RotateCw,
    title: (h) => (
      <>
        Restart holzkube-manager on <Hostname name={h} />?
      </>
    ),
    description:
      'Restarts the holzkube-manager service. The machine and everything else on it keep running.',
    consequence:
      'The page will lose its connection while holzkube-manager restarts. It keeps asking and picks up again when it is back, usually within seconds.',
    consequenceClass: SLATE,
    destructive: false,
  },
  reboot: {
    icon: RotateCcw,
    title: (h) => (
      <>
        Restart <Hostname name={h} />?
      </>
    ),
    description:
      'Restarts the whole machine, and everything running on it, not only holzkube-manager.',
    consequence:
      'The page will lose its connection while the host restarts. It keeps asking and picks up again when holzkube-manager is back.',
    consequenceClass: SLATE,
    destructive: true,
  },
  poweroff: {
    icon: PowerOff,
    title: (h) => (
      <>
        Shut down <Hostname name={h} />?
      </>
    ),
    description: 'Switches the whole machine off, and everything running on it.',
    consequence:
      'Nothing here can switch it back on. Somebody has to power the machine on by hand; until then holzkube-manager, and this page, are gone.',
    // The one action nothing on this page can undo: NodeActions' etcd red.
    consequenceClass: RED,
    destructive: true,
  },
}

/** The order the page follows, as far as the buttons care: which, and where it stands. */
export type FollowedOrder = { action: HostAction; phase: OrderPhase }

export const REASON = {
  container: 'Host actions are only available with the systemd installation.',
  helper:
    'Host actions need the holzkube-manager-host helper, which is not installed. The note below says what to install.',
  reader: 'Host actions need the operator role. You are signed in as a reader.',
  hostname: 'The hostname could not be read, so there is nothing to type to confirm a host action.',
  notAnswering: 'holzkube-manager is not answering; host actions return when it does.',
  updateRunning: 'An update is running; wait for it to finish.',
  pending: 'An order is waiting for the helper. The next one can be placed when it is answered.',
} as const

/**
 * Why the four buttons are off, or null when they are on. The first reason
 * that applies wins; every one of them applies to all four buttons, so there
 * is one line for the group rather than one per button (UI-SPEC, checker
 * resolution 1).
 *
 * `available` is the server's own verdict (container, helper installed); the
 * helper reason stands for it once the container has been ruled out, because
 * the routes refuse exactly when it is false and the page must not offer more.
 */
export function disabledReason(
  host: Host,
  role: string | undefined,
  pollFailed: boolean,
  order: FollowedOrder | null,
): string | null {
  if (host.container) {
    return REASON.container
  }
  if (!host.actions.available) {
    return REASON.helper
  }
  if (role === undefined || !roleAtLeast(role, 'operator')) {
    return REASON.reader
  }
  if (!host.device.hostname.readable || host.device.hostname.value === '') {
    return REASON.hostname
  }
  if (pollFailed) {
    return REASON.notAnswering
  }
  if (order !== null) {
    if (order.action === 'update' && order.phase === 'started') {
      return REASON.updateRunning
    }
    if (order.phase === 'placed' || order.phase === 'picked-up') {
      return REASON.pending
    }
  }
  return null
}

export function HostActions({
  host,
  sessionRole,
  pollFailed,
  order,
  onPlaced,
}: {
  host: Host
  /** The session's role; undefined until it is known, which offers nothing. */
  sessionRole: string | undefined
  /** The latest poll failed: the reading on screen is old (stale or waiting). */
  pollFailed: boolean
  /** The order the status box follows, if any. */
  order: FollowedOrder | null
  onPlaced: (order: HostOrder) => void
}) {
  const [pending, setPending] = useState<HostAction | null>(null)
  const hostname = host.device.hostname.readable ? host.device.hostname.value : ''
  const reason = disabledReason(host, sessionRole, pollFailed, order)

  return (
    <div className="flex flex-col items-end gap-1 max-md:w-full max-md:items-stretch">
      {/* A fieldset is a group to assistive technology without a role
          attribute; the reset classes undo its browser frame and its
          min-content width, which would push the phone grid past the edge. */}
      <fieldset
        aria-label="Host actions"
        aria-describedby={reason === null ? undefined : 'host-actions-reason'}
        className="m-0 flex min-w-0 flex-wrap gap-2 border-0 p-0 max-md:grid max-md:grid-cols-2"
      >
        {HOST_ACTIONS.map((action) => {
          const spec = ACTION[action]
          const Icon = spec.icon
          return (
            <Fragment key={action}>
              {/* The machine-wide pair stands apart from the service pair. */}
              {action === 'reboot' && <span aria-hidden="true" className="w-4 max-md:hidden" />}
              <Button
                variant="outline"
                size="sm"
                className={`max-md:h-auto max-md:min-h-11 max-md:min-w-0 max-md:py-2 max-md:whitespace-normal${
                  spec.destructive ? ' text-destructive' : ''
                }`}
                disabled={reason !== null}
                onClick={() => setPending(action)}
              >
                <Icon aria-hidden="true" className="size-4" />
                {HOST_ACTION_LABEL[action]}
              </Button>
            </Fragment>
          )
        })}
      </fieldset>
      {reason !== null && (
        <p id="host-actions-reason" className="text-xs text-muted-foreground md:text-right">
          {reason}
        </p>
      )}
      {pending !== null && (
        <HostActionDialog
          action={pending}
          hostname={hostname}
          onClose={() => setPending(null)}
          onPlaced={(placed) => {
            setPending(null)
            onPlaced(placed)
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
  const spec = ACTION[action]
  const Icon = spec.icon

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
    if (typed === hostname && hostname !== '' && !run.isPending) {
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
            {spec.destructive ? (
              <DialogTitle className="flex items-center gap-2 text-destructive">
                <AlertTriangle aria-hidden="true" className="size-5" />
                <span className="min-w-0">{spec.title(hostname)}</span>
              </DialogTitle>
            ) : (
              <DialogTitle>{spec.title(hostname)}</DialogTitle>
            )}
            <DialogDescription>{spec.description}</DialogDescription>
          </DialogHeader>

          <div className="space-y-4 text-sm">
            <p className={`rounded-md border px-3 py-2 ${spec.consequenceClass}`}>
              {spec.consequence}
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
                <span className="min-w-0">
                  Type <Hostname name={hostname} /> to confirm
                </span>
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
              variant={spec.destructive ? 'destructive' : 'default'}
              disabled={typed !== hostname || hostname === '' || run.isPending}
            >
              <Icon aria-hidden="true" className="size-4" />
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
