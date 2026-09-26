import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  Ban,
  CirclePlay,
  CircleStop,
  OctagonX,
  Power as PowerIcon,
  RotateCcw,
  RotateCw,
  Zap,
} from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { api, type PowerAction, type PowerTarget } from '@/api'
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

/**
 * Power, for a cluster, a node or an app (2026-09-26).
 *
 * One menu with the same seven words wherever it appears, because the operator
 * asked for exactly that: stop, force-stop, start, disable, enable, restart,
 * force-restart -- for the whole cluster, for one node, for one service.
 *
 * The server decides what is possible. Every action is listed every time, and
 * one that cannot run now is shown greyed with the server's reason under it,
 * so "why can I not start this" is answered where the question is asked
 * instead of by a missing button.
 *
 * Every action asks once before it runs. The forced ones additionally go
 * through the password prompt the server demands (HTTP 428, handled by the
 * shared interceptor) -- the operator's choice on 2026-09-26 was that password
 * and nothing more.
 */

const LABEL: Record<PowerAction, string> = {
  stop: 'Stop',
  'force-stop': 'Force stop',
  start: 'Start',
  disable: 'Disable',
  enable: 'Enable',
  restart: 'Restart',
  'force-restart': 'Force restart',
}

const ICON: Record<PowerAction, ReactNode> = {
  stop: <CircleStop aria-hidden="true" />,
  'force-stop': <OctagonX aria-hidden="true" />,
  start: <CirclePlay aria-hidden="true" />,
  disable: <Ban aria-hidden="true" />,
  enable: <PowerIcon aria-hidden="true" />,
  restart: <RotateCw aria-hidden="true" />,
  'force-restart': <Zap aria-hidden="true" />,
}

const FORCED: ReadonlySet<PowerAction> = new Set(['force-stop', 'force-restart'])

/** What an action does to this kind of target, said before it is done. */
export function describe(target: PowerTarget, action: PowerAction): string {
  const what =
    target.kind === 'cluster'
      ? `the whole cluster ${target.name}`
      : target.kind === 'node'
        ? `the node ${target.name}`
        : `${target.name}`
  switch (target.kind) {
    case 'cluster':
      return {
        stop: `Stops ${what}: workers first, each emptied before it shuts down, then the control planes one by one.`,
        'force-stop': `Shuts down every node of ${what} at once. Nothing is moved first; everything running stops mid-flight.`,
        start: `Wakes every node of ${what} by Wake-on-LAN, control planes first, and lets them take work again once they answer. Disabled nodes stay off.`,
        disable: `Stops ${what} and keeps it off: a start refuses until you enable it again.`,
        enable: `Allows ${what} to run again, then starts it.`,
        restart: `Restarts ${what} one node at a time — each emptied, rebooted and back before the next — and stops at the first node that does not come back healthy.`,
        'force-restart': `Reboots every node of ${what} at once. The cluster is down until they are all back.`,
      }[action]
    case 'node':
      return {
        stop: `Moves the pods off ${what}, then shuts it down.`,
        'force-stop': `Shuts down ${what} now. Its pods are not moved first and stop mid-flight.`,
        start: `Wakes ${what} by Wake-on-LAN if it is off, and lets it take pods again once it answers.`,
        disable: `Stops ${what} and keeps it off: a cluster start skips it, and if it is switched on anyway it gets no pods, until you enable it.`,
        enable: `Allows ${what} to run again, then starts it.`,
        restart: `Moves the pods off ${what}, reboots it, and lets it take pods again once it is back.`,
        'force-restart': `Reboots ${what} now, without moving its pods first.`,
      }[action]
    case 'app':
      return {
        stop: `Stops ${what}. How many copies it ran is remembered, so a start brings back the same number.`,
        'force-stop': `Stops ${what} and kills its pods immediately, without the usual time to shut down cleanly.`,
        start: `Starts ${what} again with the number of copies it had before it was stopped.`,
        disable: `Stops ${what} and keeps it off: start refuses until you enable it again.`,
        enable: `Allows ${what} to run again, then starts it.`,
        restart: `Replaces the pods of ${what} one by one, the way a rollout does, so it stays reachable.`,
        'force-restart': `Kills every pod of ${what} at once; they come back fresh. It is unreachable until they do.`,
      }[action]
  }
}

export function PowerMenu({
  target,
  size = 'sm',
}: {
  target: PowerTarget
  size?: 'sm' | 'default'
}) {
  const queryClient = useQueryClient()
  const [pending, setPending] = useState<PowerAction | null>(null)
  const [done, setDone] = useState<{ message: string; jobID?: string } | null>(null)

  const power = useQuery({
    queryKey: ['power', target],
    queryFn: () => api.power.get(target),
    refetchInterval: 10_000,
    retry: false,
  })

  const run = useMutation({
    mutationFn: (action: PowerAction) => api.power.run(target, action),
    onSuccess: (result) => {
      setPending(null)
      setDone(
        'job' in result
          ? { message: 'Started. Its progress is on the Jobs page.', jobID: result.job.id }
          : { message: result.message || 'Done.' },
      )
      void queryClient.invalidateQueries({ queryKey: ['power'] })
      void queryClient.invalidateQueries({ queryKey: ['kubernetes'] })
      void queryClient.invalidateQueries({ queryKey: ['machines'] })
    },
  })

  const actions = power.data?.actions ?? []
  const state = power.data?.state

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="outline"
            size={size}
            aria-label={`Power: ${target.name}`}
            title="Power"
            className="max-md:min-w-11"
          >
            <PowerIcon aria-hidden="true" className="size-4" />{' '}
            <span className="max-md:sr-only">Power</span>
            {state && state !== 'running' && (
              <span className="text-muted-foreground text-xs max-md:sr-only">· {state}</span>
            )}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-80">
          {power.error ? (
            <div className="p-2">
              <Problem error={power.error} />
            </div>
          ) : actions.length === 0 ? (
            <p className="p-2 text-muted-foreground text-sm">Asking what is possible…</p>
          ) : (
            actions.map((a, i) => {
              const action = a.action as PowerAction
              if (!(action in LABEL)) return null
              return (
                <div key={a.action}>
                  {(action === 'disable' || action === 'restart') && i > 0 && (
                    <DropdownMenuSeparator />
                  )}
                  <DropdownMenuItem
                    disabled={!a.available}
                    variant={FORCED.has(action) ? 'destructive' : 'default'}
                    onSelect={() => setPending(action)}
                    className="items-start"
                  >
                    <span className="mt-0.5">{ICON[action]}</span>
                    <span className="min-w-0">
                      <span className="block">{LABEL[action]}</span>
                      {!a.available && a.reason !== '' && (
                        <span className="block text-muted-foreground text-xs">{a.reason}</span>
                      )}
                      {a.available && a.sudo && (
                        <span className="block text-muted-foreground text-xs">
                          asks for your password
                        </span>
                      )}
                    </span>
                  </DropdownMenuItem>
                </div>
              )
            })
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={pending !== null} onOpenChange={(open) => !open && setPending(null)}>
        <DialogContent className="sm:max-w-md">
          {pending !== null && (
            <>
              <DialogHeader>
                <DialogTitle className={FORCED.has(pending) ? 'text-destructive' : undefined}>
                  {LABEL[pending]} {target.name}?
                </DialogTitle>
                <DialogDescription>{describe(target, pending)}</DialogDescription>
              </DialogHeader>
              {FORCED.has(pending) && (
                <p className="text-muted-foreground text-sm">
                  Forced actions ask for your password next.
                </p>
              )}
              {run.error ? <Problem error={run.error} /> : null}
              <DialogFooter>
                <Button variant="outline" onClick={() => setPending(null)}>
                  Cancel
                </Button>
                <Button
                  variant={FORCED.has(pending) ? 'destructive' : 'default'}
                  disabled={run.isPending}
                  onClick={() => run.mutate(pending)}
                >
                  {pending === 'restart' || pending === 'force-restart' ? (
                    <RotateCcw aria-hidden="true" className="size-4" />
                  ) : (
                    ICON[pending]
                  )}
                  {run.isPending ? 'Working…' : LABEL[pending]}
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={done !== null} onOpenChange={(open) => !open && setDone(null)}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{target.name}</DialogTitle>
            <DialogDescription>{done?.message}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            {done?.jobID !== undefined && (
              <Button asChild variant="outline">
                <Link to="/jobs">Open Jobs</Link>
              </Button>
            )}
            <Button onClick={() => setDone(null)}>Close</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
