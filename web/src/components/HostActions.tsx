import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  AlertTriangle,
  Download,
  type LucideIcon,
  PowerOff,
  RotateCcw,
  RotateCw,
} from 'lucide-react'
import {
  type FormEvent,
  Fragment,
  type ReactNode,
  type RefObject,
  useEffect,
  useRef,
  useState,
} from 'react'
import {
  api,
  type Host,
  type HostAction,
  type HostHelperMissing,
  type HostOrder,
  roleAtLeast,
} from '@/api'
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
import { cn } from '@/lib/utils'

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
const EMERALD = 'border-emerald-600/40 bg-emerald-600/10 text-emerald-700 dark:text-emerald-300'
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
  underWay: 'The last host action is still under way; wait for it to finish.',
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
    // Started is not done: a restart or shutdown the helper has begun takes the
    // host or the service away within seconds, and a second order placed in
    // those seconds would outlive the process that placed it.
    if (order.phase === 'started') {
      return order.action === 'update' ? REASON.updateRunning : REASON.underWay
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
  // The button that opened the dialog: a cancel hands focus back to it.
  const opener = useRef<HTMLButtonElement | null>(null)
  // The group's reason line: where a cancel hands focus when the opener was
  // turned off while the dialog was open (14-REVIEW WR-01).
  const reasonLine = useRef<HTMLParagraphElement | null>(null)
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
              {/* The machine-wide pair stands 16 px apart from the service pair:
                  a zero-width spacer between the row's two gap-2s, 8 + 0 + 8.
                  A w-4 spacer drew 32 (13-UI-REVIEW). Hidden on the phone grid,
                  where it would take a cell. */}
              {action === 'reboot' && <span aria-hidden="true" className="w-0 max-md:hidden" />}
              <Button
                variant="outline"
                size="sm"
                // Every class a whitespace-bounded literal: Tailwind reads the
                // source as text, and a class written flush against an
                // interpolation (`...whitespace-normal${`) is a token it cannot
                // read, so no rule was generated and the label ran past its
                // button at 390 px (G-13-3).
                className={cn(
                  'max-md:h-auto max-md:min-h-11 max-md:min-w-0 max-md:py-2 max-md:whitespace-normal',
                  spec.destructive && 'text-destructive',
                )}
                disabled={reason !== null}
                onClick={(e) => {
                  opener.current = e.currentTarget
                  setPending(action)
                }}
              >
                <Icon aria-hidden="true" className="size-4" />
                {HOST_ACTION_LABEL[action]}
              </Button>
            </Fragment>
          )
        })}
      </fieldset>
      {reason !== null && (
        // tabIndex -1: not a tab stop, but focusable from script, for the
        // cancel that finds its opener turned off.
        <p
          ref={reasonLine}
          id="host-actions-reason"
          tabIndex={-1}
          className="text-xs text-muted-foreground md:text-right"
        >
          {reason}
        </p>
      )}
      {pending !== null && (
        <HostActionDialog
          action={pending}
          hostname={hostname}
          reason={reason}
          opener={opener}
          reasonLine={reasonLine}
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
  reason,
  opener,
  reasonLine,
  onClose,
  onPlaced,
}: {
  action: HostAction
  hostname: string
  /**
   * The group's reason, followed while the dialog is open: a failed poll,
   * another order or a role change arriving now turns the confirm off, because
   * the route would refuse it. The server stays the lock (409/403).
   */
  reason: string | null
  /** The button that opened the dialog, which a cancel gives focus back to. */
  opener: RefObject<HTMLButtonElement | null>
  /**
   * The group's reason line, which a cancel gives focus to instead when the
   * opener was turned off meanwhile: focus() on a disabled button does nothing.
   */
  reasonLine: RefObject<HTMLParagraphElement | null>
  onClose: () => void
  onPlaced: (order: HostOrder) => void
}) {
  const queryClient = useQueryClient()
  const [typed, setTyped] = useState('')
  // Set once this dialog placed an order, before it hands the order on.
  const placed = useRef(false)
  const spec = ACTION[action]
  const Icon = spec.icon

  const run = useMutation({
    mutationFn: async () => {
      const { token } = await api.hostActions.confirm(action, typed)
      return api.hostActions.place(action, token)
    },
    onSuccess: ({ order }) => {
      void queryClient.invalidateQueries({ queryKey: ['host'] })
      placed.current = true
      onPlaced(order)
    },
  })

  // Blanks around the name do not count: the server compares the trimmed
  // text (docs/api-contract.md), and a name pasted with a trailing space must
  // not leave the button off with no reason given.
  const matches = typed.trim() === hostname && hostname !== ''

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (matches && reason === null && !run.isPending) {
      run.mutate()
    }
  }

  return (
    // No way out while the order is on its way (14-REVIEW CR-01): the
    // mutation's onSuccess runs even after the dialog has gone, so a close in
    // those seconds would say "Keep running" and place the order anyway.
    // Escape, a tap outside and the close X all arrive here and are ignored
    // until the confirm and the placement have answered.
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !run.isPending) {
          onClose()
        }
      }}
    >
      <DialogContent
        className="sm:max-w-md"
        // After an order was placed the trigger is off while it is under way,
        // and focus goes to the status box instead (HostOrderStatus takes it).
        // A cancel -- Keep running, Escape, the close X, none of which closes
        // while an order is on its way -- placed nothing: the trigger gets
        // focus back. Radix would only do that for a DialogTrigger, which these
        // buttons are not, and left alone it drops focus on the page body next
        // to the destructive pair. A reason that arrived while the dialog was
        // open (a failed poll, another order, a role change) has turned the
        // trigger off as well, and a disabled button takes no focus: then the
        // line that says why is where focus goes.
        onCloseAutoFocus={(e) => {
          e.preventDefault()
          if (placed.current) {
            return
          }
          const trigger = opener.current
          if (trigger !== null && !trigger.disabled) {
            trigger.focus()
          } else {
            reasonLine.current?.focus()
          }
        }}
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
                onChange={(e) => {
                  setTyped(e.target.value)
                  // An old Problem does not stay under a corrected name.
                  if (run.isError) {
                    run.reset()
                  }
                }}
              />
            </div>
            {run.error ? <Problem error={run.error} /> : null}
          </div>

          {reason !== null && (
            <p id="host-action-dialog-reason" className="text-xs text-muted-foreground">
              {reason}
            </p>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose} disabled={run.isPending}>
              Keep running
            </Button>
            <Button
              type="submit"
              variant={spec.destructive ? 'destructive' : 'default'}
              disabled={!matches || reason !== null || run.isPending}
              aria-describedby={reason === null ? undefined : 'host-action-dialog-reason'}
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

/**
 * Where an order stands, as the page tells it (UI-SPEC "The order status box").
 * Every phase is read off server fields -- the order's state, the helper's
 * result for its id, the process start, the boot time, the update status --
 * so the page never says an action is done that the host did not report.
 */
export type OrderPhase =
  | 'placed'
  | 'picked-up'
  | 'started'
  | 'rejected'
  | 'failed'
  | 'not-picked-up'
  | 'waiting'
  | 'back'
  | 'update-finished'
  | 'no-answer'

/** The phases after which nothing more will happen to the order. */
const FINAL: ReadonlySet<OrderPhase> = new Set([
  'rejected',
  'failed',
  'not-picked-up',
  'back',
  'update-finished',
  'no-answer',
])

/**
 * How long after its placement an order may go without the helper's result
 * before the page stops waiting for one. The daemon withdraws an order nobody
 * took within 10 s, and the helper records "started" before it acts, so a
 * minute without either is not an answer still on its way: the helper took the
 * order and recorded nothing (killed, its state directory full, its result
 * unreadable), and nothing will change without somebody looking.
 */
export const RESULT_WITHIN_MS = 60 * 1000

/**
 * How long a started order may go without the host saying it is done -- the
 * update status newer than the order, the process or the machine started
 * after it -- before the page stops waiting. An update that downloads and
 * installs a release is the longest of the four; a Pi reboots in a minute.
 */
export const STARTED_WITHIN_MS = 15 * 60 * 1000

export function isFinal(phase: OrderPhase): boolean {
  return FINAL.has(phase)
}

type UpdateStatus = Extract<Host['service']['update'], { readable: true }>['value']

/**
 * When the machine last booted, by the server's own clock: the reading's time
 * minus the uptime it read. Null when the uptime could not be read.
 */
function bootTime(host: Host): number | null {
  const up = host.device.uptime_seconds
  return up.readable ? Date.parse(host.observed_at) - up.value * 1000 : null
}

/**
 * The phase of `order` in the host answer `host`; `pollFailed` is whether the
 * latest poll failed, so that `host` is the last reading before it.
 *
 * - no result for this id yet: the order's state as the daemon reports it (or,
 *   before it has, as the placement answered it) -- pending is placed,
 *   withdrawn is not picked up, picked-up is picked up, or waiting while the
 *   host does not answer; and no answer once RESULT_WITHIN_MS have passed
 *   without the daemon reporting the file still there;
 * - the helper's result for this id: rejected or failed as it says; started
 *   becomes update finished once the update status is newer than the order,
 *   back once the process started (restart service, update) or the machine
 *   booted (restart host, shut down host) after it, waiting while the host
 *   does not answer, and started until then.
 *
 * "Update finished" is asked first: an update that installed a release also
 * restarted the process, and its own sentence says which it was.
 */
export function orderPhase(order: HostOrder, host: Host, pollFailed = false): OrderPhase {
  const result = host.actions.result
  const reported = host.actions.order
  const elapsed = Date.parse(host.observed_at) - Date.parse(order.placed_at)
  if (!result.readable || result.value.id !== order.id) {
    const known = reported !== null && reported.id === order.id
    const state = known ? reported.state : order.state
    if (state === 'withdrawn') {
      return 'not-picked-up'
    }
    // The daemon says the file is there right now: that is the truth, however
    // long it has been.
    if (state === 'pending' && (known || pollFailed)) {
      return 'placed'
    }
    if (pollFailed) {
      return 'waiting'
    }
    // Neither picked up and answered nor withdrawn, and nobody will say more:
    // stop waiting, so that the buttons come back and the box can be dismissed.
    if (elapsed > RESULT_WITHIN_MS) {
      return 'no-answer'
    }
    return state === 'pending' ? 'placed' : 'picked-up'
  }
  if (result.value.outcome !== 'started') {
    return result.value.outcome
  }
  const placed = Date.parse(order.placed_at)
  const update = host.service.update
  if (
    order.action === 'update' &&
    update.readable &&
    Date.parse(update.value.checked_at) > placed
  ) {
    return 'update-finished'
  }
  if (pollFailed) {
    return 'waiting'
  }
  if (order.action === 'restart-service' || order.action === 'update') {
    if (Date.parse(host.service.started_at) > placed) {
      return 'back'
    }
  } else {
    const boot = bootTime(host)
    if (boot !== null && boot > placed) {
      return 'back'
    }
  }
  if (elapsed > STARTED_WITHIN_MS) {
    return 'no-answer'
  }
  return 'started'
}

/** How long an order the page did not place is still worth a status box. */
export const ORDER_SHOWN_FOR_MS = 15 * 60 * 1000

/** The order the status box follows, and where the page learned of it. */
export type Followed = {
  order: HostOrder
  /**
   * `held`: this page placed it. `order`: the daemon's last order. `result`:
   * only the helper's record is left (the daemon restarted since) -- its time
   * is when the helper recorded it, not when the order was placed.
   */
  from: 'held' | 'order' | 'result'
}

/**
 * Which order the status box follows: the one this page placed, until
 * dismissed; else the daemon's last order; else the helper's last result that
 * names an order -- those two only within 15 minutes of the reading, so a
 * reload the next day is not greeted by yesterday's reboot. A dismissed id
 * stays dismissed, whichever way it comes back.
 */
export function followedOrder(
  held: HostOrder | null,
  host: Host,
  dismissed: ReadonlySet<string>,
): Followed | null {
  if (held !== null && !dismissed.has(held.id)) {
    return { order: held, from: 'held' }
  }
  const now = Date.parse(host.observed_at)
  const recent = (at: string) => now - Date.parse(at) <= ORDER_SHOWN_FOR_MS
  const reported = host.actions.order
  if (reported !== null && !dismissed.has(reported.id) && recent(reported.placed_at)) {
    return { order: reported, from: 'order' }
  }
  const result = host.actions.result
  if (result.readable) {
    const { id, action, at } = result.value
    if (id !== '' && action !== '' && !dismissed.has(id) && recent(at)) {
      return { order: { id, action, placed_at: at, state: 'picked-up' }, from: 'result' }
    }
  }
  return null
}

/**
 * The update status as one sentence -- the Service card's "Update check" and
 * the status box's "update finished" both say this one.
 */
export function outcomeSentence(u: UpdateStatus): string {
  switch (u.outcome) {
    case 'current':
      return 'Up to date.'
    case 'available':
      return u.latest !== null ? `${u.latest} is available.` : 'A newer release is available.'
    case 'updated':
      return u.installed !== null ? `Updated to ${u.installed}.` : 'Updated.'
    case 'rolled-back':
      return u.latest !== null && u.installed !== null
        ? `The update to ${u.latest} was rolled back; ${u.installed} is installed.`
        : 'The update was rolled back.'
    case 'failed':
      return u.installed !== null
        ? `The last update run failed; ${u.installed} is still installed.`
        : 'The last update run failed.'
  }
}

const STARTED: Record<HostAction, string> = {
  update:
    'started. If a newer release exists it is installed and holzkube-manager restarts; the outcome appears here and under Update check.',
  'restart-service': 'started. holzkube-manager is restarting.',
  reboot: 'started. The host is restarting.',
  poweroff: 'started. The host is shutting down.',
}

const JOURNAL = <code className="font-mono text-xs">journalctl -u holzkube-manager-host</code>
const UPDATE_JOURNAL = (
  <code className="font-mono text-xs">journalctl -u holzkube-manager-update</code>
)

/** A started order the host never reported done: what did not happen. */
const NOT_DONE: Record<HostAction, ReactNode> = {
  update: (
    <>
      started, but no finished update was reported within 15 min. {UPDATE_JOURNAL} says what
      happened.
    </>
  ),
  'restart-service': (
    <>
      started, but holzkube-manager has not restarted within 15 min. {JOURNAL} says what happened.
    </>
  ),
  reboot: <>started, but the host has not restarted within 15 min. {JOURNAL} says what happened.</>,
  poweroff: <>started, but the host is still running after 15 min. {JOURNAL} says what happened.</>,
}

function timeOf(ms: number): string {
  return new Date(ms).toLocaleTimeString()
}

function phaseSentence(phase: OrderPhase, order: HostOrder, host: Host): ReactNode {
  const result = host.actions.result
  switch (phase) {
    case 'placed':
      return 'order placed. Waiting for the helper to pick it up.'
    case 'picked-up':
      return 'the helper picked up the order.'
    case 'started':
      return STARTED[order.action]
    case 'waiting':
      // The box keeps saying what was last known; the waiting notice below
      // says that the page is waiting.
      return result.readable && result.value.id === order.id
        ? STARTED[order.action]
        : 'the helper picked up the order.'
    case 'rejected':
      return <>the helper rejected the order, so nothing was done. {JOURNAL} says why.</>
    case 'failed':
      return <>the helper could not carry it out. {JOURNAL} says why.</>
    case 'not-picked-up':
      return (
        <>
          the helper did not pick up the order within 10 s, so holzkube-manager withdrew it. Nothing
          was done. Check that the helper is running:{' '}
          <code className="font-mono text-xs">systemctl status holzkube-manager-host.path</code>
        </>
      )
    case 'update-finished': {
      const u = host.service.update
      return u.readable ? `finished. ${outcomeSentence(u.value)}` : 'finished.'
    }
    case 'no-answer':
      return result.readable && result.value.id === order.id ? (
        NOT_DONE[order.action]
      ) : (
        <>
          no answer: the helper recorded nothing for this order within 1 min. {JOURNAL} says what
          happened.
        </>
      )
    case 'back': {
      const version = host.service.version
      const boot = bootTime(host)
      switch (order.action) {
        case 'restart-service':
          return `done. holzkube-manager is back, running ${version} since ${timeOf(
            Date.parse(host.service.started_at),
          )}.`
        case 'update':
          return `done. holzkube-manager is back, running ${version}.`
        // With the boot time unknown the clause is left out, never "up since .".
        case 'reboot':
          return boot === null
            ? 'done. The host restarted and holzkube-manager is back.'
            : `done. The host restarted and holzkube-manager is back; up since ${timeOf(boot)}.`
        case 'poweroff':
          return boot === null
            ? 'the host was switched on again and holzkube-manager is back.'
            : `the host was switched on again and holzkube-manager is back; up since ${timeOf(boot)}.`
      }
    }
  }
}

/** The box's colour set: slate under way, emerald done, red when nothing (good) happened. */
function phaseColour(phase: OrderPhase, host: Host): string {
  switch (phase) {
    case 'back':
      return EMERALD
    case 'update-finished': {
      const u = host.service.update
      return u.readable && (u.value.outcome === 'current' || u.value.outcome === 'updated')
        ? EMERALD
        : RED
    }
    case 'rejected':
    case 'failed':
    case 'not-picked-up':
    case 'no-answer':
      return RED
    default:
      return SLATE
  }
}

/**
 * The order status box: first in the page's notice stack while there is an
 * order to follow. Only the phase sentence changes, so the polite status role
 * announces each transition once. It takes focus once, when this page has
 * just placed the order -- the dialog that placed it has closed and its
 * trigger is off -- and never on a later poll or for an order it only found.
 */
export function HostOrderStatus({
  host,
  followed,
  phase,
  takeFocus,
  onDismiss,
}: {
  host: Host
  followed: Followed
  phase: OrderPhase
  takeFocus: boolean
  onDismiss: () => void
}) {
  const box = useRef<HTMLDivElement>(null)
  const { order } = followed

  const id = order.id
  useEffect(() => {
    if (takeFocus && id !== '') {
      box.current?.focus()
    }
  }, [id, takeFocus])

  return (
    <div
      ref={box}
      role="status"
      tabIndex={-1}
      className={`rounded-md border px-3 py-2 text-sm ${phaseColour(phase, host)}`}
    >
      <p>
        <span className="font-semibold">{HOST_ACTION_LABEL[order.action]}</span> —{' '}
        {phaseSentence(phase, order, host)}
      </p>
      <p className="mt-1 text-xs tabular-nums">
        Order <span className="font-mono">{order.id}</span> ·{' '}
        {followed.from === 'result' ? 'recorded' : 'placed'}{' '}
        {new Date(order.placed_at).toLocaleTimeString()}
      </p>
      {isFinal(phase) && (
        <Button variant="ghost" size="sm" className="mt-2" onClick={onDismiss}>
          Dismiss status
        </Button>
      )}
    </div>
  )
}

const DEPLOY = (file: string) => <code className="font-mono text-xs">{file}</code>

/** What each missing piece is and where it comes from (UI-SPEC "Missing" rows). */
const MISSING_SENTENCE: Record<HostHelperMissing['item'], ReactNode> = {
  script: <>the helper script, from {DEPLOY('deploy/holzkube-manager-host.sh')}</>,
  'path-unit': (
    <>
      the unit that watches for orders, from {DEPLOY('deploy/holzkube-manager-host.path')} (with{' '}
      {DEPLOY('holzkube-manager-host.service')} beside it)
    </>
  ),
  'not-enabled': <>{DEPLOY('holzkube-manager-host.path')} is installed but not enabled</>,
}

/**
 * The helper is not installed (HACT-07, D-12): what is missing, in the
 * server's order and only what is missing, and the commands that install it --
 * the server's copy, which the install guide is held to byte for byte. The
 * commands are selectable text and there is no copy button: the clipboard is
 * refused outside a secure origin, which is where this product often runs.
 * No button, no dismiss: it goes when a poll says the helper is installed.
 */
export function HostHelperNotice({
  missing,
  commands,
}: {
  missing: HostHelperMissing[]
  commands: string[]
}) {
  return (
    <div className={`rounded-md border px-3 py-2 text-sm ${SLATE}`}>
      <p className="font-semibold">Host actions need the helper, which is not installed</p>
      <p className="mt-1">
        holzkube-manager never restarts or shuts down this machine itself. A small root-owned helper
        does, and it knows exactly four orders. Until it is installed, the four buttons above stay
        off.
      </p>
      <ul className="mt-2 space-y-1">
        {missing.map((m) => (
          <li key={m.item}>
            <span className="font-mono break-all">{m.path}</span> — {MISSING_SENTENCE[m.item]}
          </li>
        ))}
      </ul>
      {commands.length > 0 && (
        <pre className="mt-2 overflow-x-auto rounded-sm bg-muted px-2 py-1 font-mono text-xs">
          {commands.join('\n')}
        </pre>
      )}
      <p className="mt-2 text-xs">
        The files are in {DEPLOY('deploy/')} in the release archive;{' '}
        {DEPLOY('deploy/HOST-HELPER.md')} explains each step. The service's own unit keeps every
        line of its hardening.
      </p>
    </div>
  )
}
