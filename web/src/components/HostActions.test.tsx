import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type Host, type HostAction, type HostOrder, hostSchema } from '@/api'
import {
  type FollowedOrder,
  followedOrder,
  HostActions,
  HostOrderStatus,
  orderPhase,
} from '@/components/HostActions'
import { SudoDialog } from '@/components/SudoDialog'
import { SESSION_QUERY_KEY } from '@/hooks/useSession'
import { ProblemError } from '@/lib/problem'
import { PROBLEM_BASE_URI } from '@/test/problem-fixtures'
import demo from '../../fixtures/demo.json'

/**
 * The host actions on /host (Phase 13): the five buttons, the one reason they
 * are off, the dialog with the typed hostname, and the status box that follows
 * the order through the host answer. The server is the gate (the routes refuse
 * a reader, a container, a missing helper, a second order, and a typed text
 * that is not uname(2)'s); what these hold is that the page never offers what
 * the route will refuse, asks it the right question, and says what came back.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const noResult = {
  readable: false,
  reason: { code: 'host-action.no-result', message: 'Nothing recorded.' },
}

/** The helper installed completely, nothing placed yet. */
const installed = {
  order: null,
  result: noResult,
  available: true,
  missing: [],
  outdated: [],
  install_commands: [],
}

/** A helper installed before the check existed: the four older orders work. */
const olderScript = [{ item: 'script-outdated', path: '/usr/local/sbin/holzkube-manager-host' }]
const noCheckUnit = [
  { item: 'check-unit', path: '/etc/systemd/system/holzkube-manager-update-check.service' },
]
const CHECK_NEEDS_NEWER =
  'Check for updates needs a newer holzkube-manager-host helper. The note below says how to reinstall it.'

/**
 * The demo host, renamed to the documentation hostname, with the helper
 * installed unless the test says otherwise.
 */
function hostWith(actions: Record<string, unknown> = {}, top: Record<string, unknown> = {}): Host {
  const base = structuredClone((demo as Record<string, unknown>)['/api/v1/host']) as Record<
    string,
    unknown
  >
  const device = base.device as Record<string, unknown>
  device.hostname = { readable: true, value: 'example-host' }
  base.actions = { ...installed, ...actions }
  return hostSchema.parse({ ...base, ...top })
}

const ID = '3f9c2a7b1d4e8f60'

const held: HostOrder = {
  id: ID,
  action: 'update',
  placed_at: '2026-09-28T10:00:05Z',
  state: 'pending',
}

const LABELS = [
  'Check for updates',
  'Check for updates and install',
  'Restart service',
  'Restart host',
  'Shut down host',
]

function actions(
  opts: {
    host?: Host
    role?: string | undefined
    pollFailed?: boolean
    order?: FollowedOrder | null
    onPlaced?: (order: HostOrder) => void
  } = {},
) {
  const { host = hostWith(), pollFailed = false, order = null, onPlaced = () => undefined } = opts
  // An operator unless the test names a role -- including none at all.
  const role = 'role' in opts ? opts.role : 'operator'
  return wrap(
    <HostActions
      host={host}
      sessionRole={role}
      pollFailed={pollFailed}
      order={order}
      onPlaced={onPlaced}
    />,
  )
}

/** The five header buttons, in document order. */
function headerButtons(): HTMLElement[] {
  return within(screen.getByRole('group', { name: 'Host actions' })).getAllByRole('button')
}

function reasonLine(): HTMLElement | null {
  return document.getElementById('host-actions-reason')
}

/** All five rendered, all five off, and the one line says why. */
function expectOffBecause(reason: string) {
  const buttons = headerButtons()
  expect(buttons.map((b) => b.textContent)).toEqual(LABELS)
  for (const b of buttons) {
    expect(b).toBeDisabled()
  }
  expect(reasonLine()).toHaveTextContent(reason)
  expect(screen.getByRole('group', { name: 'Host actions' })).toHaveAttribute(
    'aria-describedby',
    'host-actions-reason',
  )
  // The group's reason is said once, by the group, not again by each button.
  for (const b of buttons) {
    expect(b).not.toHaveAttribute('aria-describedby')
  }
}

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('HostActions: the five buttons and the one reason they are off', () => {
  it('offers all five to an operator with the helper installed, harmless first, and says nothing', () => {
    actions()

    const buttons = headerButtons()
    expect(buttons.map((b) => b.textContent)).toEqual(LABELS)
    for (const b of buttons) {
      expect(b).toBeEnabled()
    }
    // The machine-wide pair is red, the check and the service pair are not.
    expect(buttons.map((b) => b.classList.contains('text-destructive'))).toEqual([
      false,
      false,
      false,
      true,
      true,
    ])
    expect(reasonLine()).toBeNull()
    expect(screen.getByRole('group', { name: 'Host actions' })).not.toHaveAttribute(
      'aria-describedby',
    )
  })

  it('in a container: all off, the systemd installation is where they are', () => {
    actions({ host: hostWith({ available: false }, { container: true }) })
    expectOffBecause('Host actions are only available with the systemd installation.')
  })

  it('with the helper missing: all off, and the note below says what to install', () => {
    actions({
      host: hostWith({
        available: false,
        missing: [{ item: 'script', path: '/usr/local/sbin/holzkube-manager-host' }],
      }),
    })
    expectOffBecause(
      'Host actions need the holzkube-manager-host helper, which is not installed. The note below says what to install.',
    )
  })

  it('for a reader: all five still shown, all off, and the role named (F21)', () => {
    actions({ role: 'reader' })
    expectOffBecause('Host actions need the operator role. You are signed in as a reader.')
  })

  it('before the role is known: offers nothing', () => {
    actions({ role: undefined })
    expectOffBecause('Host actions need the operator role. You are signed in as a reader.')
  })

  it('offers them to an admin, who carries the operator role', () => {
    actions({ role: 'admin' })
    for (const b of headerButtons()) {
      expect(b).toBeEnabled()
    }
  })

  it('with the hostname not readable: all off, there is nothing to type', () => {
    const host = hostWith()
    host.device.hostname = {
      readable: false,
      reason: { code: 'read-failed', message: 'uname(2) failed.' },
    }
    actions({ host })
    expectOffBecause(
      'The hostname could not be read, so there is nothing to type to confirm a host action.',
    )
  })

  it('while holzkube-manager does not answer: all off until it does', () => {
    actions({ pollFailed: true })
    expectOffBecause('holzkube-manager is not answering; host actions return when it does.')
  })

  it('while an update runs: all off until it finishes', () => {
    actions({ order: { action: 'update', phase: 'started' } })
    expectOffBecause('An update is running; wait for it to finish.')
  })

  // A second order during a check would be an update running the update script
  // beside it, or a restart cutting it off.
  it('while a check runs: all five off until it finishes', () => {
    actions({ order: { action: 'check-update', phase: 'started' } })
    expectOffBecause('An update check is running; wait for it to finish.')
  })

  // 13-REVIEW-2 WR-01, V-01: the routes refuse every order while the helper
  // waits for a check (409 conflict.host-helper-busy), whoever placed it. The
  // server says so in actions.busy, from the helper's own record, and the page
  // reads that and nothing else -- not the update status, which the hourly run
  // writes while a check runs too.
  describe('a check the page does not follow (another tab or client placed it)', () => {
    const checkAt = '2026-09-28T10:04:35Z'
    const checkStarted = { order: null, result: resultFor(ID, 'check-update', 'started', checkAt) }

    it('while the server says the helper is busy: all five off, as the routes refuse', () => {
      actions({ host: later({ ...checkStarted, busy: true }) })
      expectOffBecause('An update check is running; wait for it to finish.')
    })

    it('with an update status newer than the check: still off while busy', () => {
      const newer = { ...sameSecond('current'), checked_at: '2026-09-28T10:04:50Z' }
      actions({ host: later({ ...checkStarted, busy: true }, { update: newer }) })
      expectOffBecause('An update check is running; wait for it to finish.')
    })

    it.each([
      ['the server says it is over', checkStarted],
      [
        'the helper recorded it done',
        { order: null, result: resultFor(ID, 'check-update', 'done', checkAt) },
      ],
    ] as const)('once %s: offers them again', (_, actionsNow) => {
      actions({ host: later({ ...actionsNow, busy: false }) })
      expect(reasonLine()).toBeNull()
      for (const b of headerButtons()) {
        expect(b).toBeEnabled()
      }
    })

    it('a daemon that does not say: not busy', () => {
      expect(later({ order: null }).actions.busy).toBe(false)
    })
  })

  it('once a check finished, offers them again', () => {
    actions({ order: { action: 'check-update', phase: 'update-finished' } })
    expect(reasonLine()).toBeNull()
    for (const b of headerButtons()) {
      expect(b).toBeEnabled()
    }
  })

  // WR-02: a restart or shutdown under way takes the host or the service away
  // within seconds; a second order placed in them would outlive its process.
  it.each(['reboot', 'poweroff', 'restart-service'] as const)(
    'while a %s the helper started is not back: all off',
    (action) => {
      actions({ order: { action, phase: 'started' } })
      expectOffBecause('The last host action is still under way; wait for it to finish.')
    },
  )

  it.each(['placed', 'picked-up'] as const)(
    'with an order %s and not answered: all off',
    (phase) => {
      actions({ order: { action: 'reboot', phase } })
      expectOffBecause(
        'An order is waiting for the helper. The next one can be placed when it is answered.',
      )
    },
  )

  it('once the page stopped waiting for an answer, offers them again (WR-04)', () => {
    actions({ order: { action: 'reboot', phase: 'no-answer' } })
    expect(reasonLine()).toBeNull()
    for (const b of headerButtons()) {
      expect(b).toBeEnabled()
    }
  })

  it('once the helper answered an order that is not an update, offers them again', () => {
    actions({ order: { action: 'restart-service', phase: 'rejected' } })
    for (const b of headerButtons()) {
      expect(b).toBeEnabled()
    }
  })

  // D-15, D-16: an older helper carries out the four older orders and refuses
  // the check, so only the check is off, with its own sentence.
  it.each([
    ['its script does not know the check', olderScript],
    ['the check unit is not installed', noCheckUnit],
  ] as const)('with an older helper (%s): only the check is off, and says why', (_, outdated) => {
    actions({ host: hostWith({ outdated }) })

    const buttons = headerButtons()
    expect(buttons.map((b) => b.textContent)).toEqual(LABELS)
    expect(buttons.map((b) => b.hasAttribute('disabled'))).toEqual([
      true,
      false,
      false,
      false,
      false,
    ])
    expect(reasonLine()?.textContent).toBe(CHECK_NEEDS_NEWER)
    // 13-REVIEW-2 V-27: the line is the check's, so it describes the check
    // button, not the group -- a screen reader in Restart host must not hear
    // that it needs a newer helper -- and the four that are on carry nothing.
    expect(screen.getByRole('group', { name: 'Host actions' })).not.toHaveAttribute(
      'aria-describedby',
    )
    expect(buttons.map((b) => b.getAttribute('aria-describedby'))).toEqual([
      'host-actions-reason',
      null,
      null,
      null,
      null,
    ])
    expect(buttons[0]).toHaveAccessibleDescription(CHECK_NEEDS_NEWER)
    for (const b of buttons.slice(1)) {
      expect(b).toHaveAccessibleDescription('')
    }
  })

  it('with nothing outdated: all five on, and no line', () => {
    actions({ host: hostWith({ outdated: [] }) })
    for (const b of headerButtons()) {
      expect(b).toBeEnabled()
    }
    expect(reasonLine()).toBeNull()
  })

  // A reason for the whole group wins over the check's own: all five off, and
  // the line says the group's reason.
  it.each([
    [
      'in a container',
      { host: hostWith({ available: false, outdated: olderScript }, { container: true }) },
      'Host actions are only available with the systemd installation.',
    ],
    [
      'with the helper missing',
      {
        host: hostWith({
          available: false,
          missing: [{ item: 'script', path: '/usr/local/sbin/holzkube-manager-host' }],
          outdated: olderScript,
        }),
      },
      'Host actions need the holzkube-manager-host helper, which is not installed. The note below says what to install.',
    ],
    [
      'for a reader',
      { host: hostWith({ outdated: olderScript }), role: 'reader' },
      'Host actions need the operator role. You are signed in as a reader.',
    ],
    [
      'while holzkube-manager does not answer',
      { host: hostWith({ outdated: olderScript }), pollFailed: true },
      'holzkube-manager is not answering; host actions return when it does.',
    ],
    [
      'while an order is under way',
      {
        host: hostWith({ outdated: olderScript }),
        order: { action: 'reboot', phase: 'started' } as FollowedOrder,
      },
      'The last host action is still under way; wait for it to finish.',
    ],
  ] as const)('%s with an older helper: the group reason wins, all five off', (_, opts, why) => {
    actions(opts)
    expectOffBecause(why)
  })

  it('with the hostname not readable and an older helper: the group reason wins', () => {
    const host = hostWith({ outdated: olderScript })
    host.device.hostname = {
      readable: false,
      reason: { code: 'read-failed', message: 'uname(2) failed.' },
    }
    actions({ host })
    expectOffBecause(
      'The hostname could not be read, so there is nothing to type to confirm a host action.',
    )
  })

  it('parses an answer without outdated (a daemon before 13-14) as nothing outdated', () => {
    const host = hostWith()
    const raw = structuredClone((demo as Record<string, unknown>)['/api/v1/host']) as Record<
      string,
      unknown
    >
    const { outdated: _dropped, ...older } = raw.actions as Record<string, unknown>
    expect(hostSchema.parse({ ...raw, actions: older }).actions.outdated).toEqual([])
    expect(host.actions.outdated).toEqual([])
  })

  it('says the first reason that applies, and only that one', () => {
    // A reader, in a container, with no hostname and a failed poll: the
    // container is what nobody can change from this page.
    const host = hostWith({ available: false }, { container: true })
    host.device.hostname = {
      readable: false,
      reason: { code: 'read-failed', message: 'uname(2) failed.' },
    }
    actions({ host, role: 'reader', pollFailed: true })
    expect(reasonLine()?.textContent).toBe(
      'Host actions are only available with the systemd installation.',
    )
  })
})

type DialogCase = {
  action: HostAction
  label: string
  title: string
  description: string
  box: string
  red: boolean
  destructive: boolean
}

const DIALOGS: DialogCase[] = [
  {
    action: 'check-update',
    label: 'Check for updates',
    title: 'Check for updates on example-host?',
    description:
      'Looks up the newest release and compares it with the version installed here. Nothing is downloaded or installed, and holzkube-manager keeps running.',
    box: 'The page stays connected. The answer appears here and under Update check, usually within seconds.',
    red: false,
    destructive: false,
  },
  {
    action: 'update',
    label: 'Check for updates and install',
    title: 'Check for updates and install on example-host?',
    description:
      'Runs the same update the hourly timer runs: it looks for a newer release and, if there is one, installs it and restarts holzkube-manager. If there is none, nothing changes.',
    box: 'If a newer release is installed, the page loses its connection while holzkube-manager restarts. It keeps asking and picks up again when holzkube-manager is back.',
    red: false,
    destructive: false,
  },
  {
    action: 'restart-service',
    label: 'Restart service',
    title: 'Restart holzkube-manager on example-host?',
    description:
      'Restarts the holzkube-manager service. The machine and everything else on it keep running.',
    box: 'The page will lose its connection while holzkube-manager restarts. It keeps asking and picks up again when it is back, usually within seconds.',
    red: false,
    destructive: false,
  },
  {
    action: 'reboot',
    label: 'Restart host',
    title: 'Restart example-host?',
    description:
      'Restarts the whole machine, and everything running on it, not only holzkube-manager.',
    box: 'The page will lose its connection while the host restarts. It keeps asking and picks up again when holzkube-manager is back.',
    red: false,
    destructive: true,
  },
  {
    action: 'poweroff',
    label: 'Shut down host',
    title: 'Shut down example-host?',
    description: 'Switches the whole machine off, and everything running on it.',
    box: 'Nothing here can switch it back on. Somebody has to power the machine on by hand; until then holzkube-manager, and this page, are gone.',
    red: true,
    destructive: true,
  },
]

describe('HostActions: one dialog per action', () => {
  it.each(DIALOGS)(
    '$label: title, description, box, the two lines, the typed field, the confirm',
    async (c) => {
      actions()
      await userEvent.click(screen.getByRole('button', { name: c.label }))

      const dialog = screen.getByRole('dialog')
      const title = within(dialog).getByRole('heading', { name: c.title })
      // The machine-wide pair: red title with the triangle.
      expect(title.classList.contains('text-destructive')).toBe(c.destructive)
      expect(title.querySelector('svg') !== null).toBe(c.destructive)
      // The hostname is a word a phone may break anywhere.
      expect(within(title).getByText('example-host')).toHaveClass('font-mono', 'break-all')
      expect(dialog).toHaveTextContent(c.description)

      const box = within(dialog).getByText(c.box)
      expect(box).toHaveClass(c.red ? 'border-red-600/40' : 'border-slate-500/40')
      expect(
        within(dialog).getByText(/leaves an order for the holzkube-manager-host helper/),
      ).toBeInTheDocument()
      expect(within(dialog).getByText(/asks you to confirm it is you/)).toBeInTheDocument()

      const field = within(dialog).getByLabelText('Type example-host to confirm')
      expect(field).toHaveAttribute('autocapitalize', 'none')
      expect(field).toHaveAttribute('autocorrect', 'off')
      expect(field).toHaveAttribute('spellcheck', 'false')
      expect(field).toHaveAttribute('autocomplete', 'off')

      const confirm = within(dialog).getByRole('button', { name: c.label })
      expect(confirm).toHaveAttribute('data-variant', c.destructive ? 'destructive' : 'default')
      expect(confirm).toBeDisabled()
      // The way out does not say "Cancel": nothing is cancelled, the host keeps
      // running.
      expect(within(dialog).getByRole('button', { name: 'Keep running' })).toBeInTheDocument()
      expect(within(dialog).queryByRole('button', { name: 'Cancel' })).toBeNull()
    },
  )

  it('keeps the confirm button off until the hostname is typed exactly', async () => {
    actions()
    await userEvent.click(screen.getByRole('button', { name: 'Restart host' }))
    const dialog = screen.getByRole('dialog')
    const confirm = within(dialog).getByRole('button', { name: 'Restart host' })
    const field = within(dialog).getByLabelText(/to confirm/)

    // What a phone keyboard makes of it: not the hostname.
    await userEvent.type(field, 'Example-host')
    expect(confirm).toBeDisabled()
    // One character short, and one too many.
    await userEvent.clear(field)
    await userEvent.type(field, 'example-hos')
    expect(confirm).toBeDisabled()
    await userEvent.type(field, 'tt')
    expect(confirm).toBeDisabled()

    await userEvent.clear(field)
    await userEvent.type(field, 'example-host')
    expect(confirm).toBeEnabled()
  })

  // IN-03: the server compares the text trimmed of surrounding blanks; the
  // button must not stay off, with no reason given, for a name pasted with a
  // trailing space.
  it('takes the hostname with blanks around it, as the server does', async () => {
    const confirmRoute = vi
      .spyOn(api.hostActions, 'confirm')
      .mockResolvedValue({ token: 'token-1', expires: '2026-09-28T10:10:05Z' })
    vi.spyOn(api.hostActions, 'place').mockResolvedValue({ order: held })
    actions()
    await userEvent.click(screen.getByRole('button', { name: 'Restart host' }))
    const dialog = screen.getByRole('dialog')
    const confirm = within(dialog).getByRole('button', { name: 'Restart host' })
    await userEvent.type(within(dialog).getByLabelText(/to confirm/), ' example-host ')
    expect(confirm).toBeEnabled()
    await userEvent.click(confirm)
    await vi.waitFor(() => expect(confirmRoute).toHaveBeenCalledWith('reboot', ' example-host '))
  })

  it.each(DIALOGS)(
    '$label: asks for a token with the typed hostname, places the order with it, and hands it on',
    async (c) => {
      const order: HostOrder = { ...held, action: c.action }
      const confirm = vi
        .spyOn(api.hostActions, 'confirm')
        .mockResolvedValue({ token: 'token-1', expires: '2026-09-28T10:10:05Z' })
      const place = vi.spyOn(api.hostActions, 'place').mockResolvedValue({ order })
      const onPlaced = vi.fn()

      actions({ onPlaced })
      await userEvent.click(screen.getByRole('button', { name: c.label }))
      await userEvent.type(screen.getByLabelText(/to confirm/), 'example-host')
      await userEvent.click(
        within(screen.getByRole('dialog')).getByRole('button', { name: c.label }),
      )

      await vi.waitFor(() => expect(onPlaced).toHaveBeenCalledWith(order))
      expect(confirm).toHaveBeenCalledWith(c.action, 'example-host')
      expect(place).toHaveBeenCalledWith(c.action, 'token-1')
      expect(screen.queryByRole('dialog')).toBeNull()
    },
  )

  it.each([
    ['the confirm route', 'confirm', 'does not match the hostname'],
    [
      'the action route',
      'place',
      'Another host action is still waiting for the helper. Wait for it to be answered, then try again.',
    ],
  ] as const)(
    'keeps the dialog open with the typed text and the problem when %s refuses',
    async (_, route, message) => {
      if (route === 'confirm') {
        vi.spyOn(api.hostActions, 'confirm').mockRejectedValue(new Error(message))
      } else {
        vi.spyOn(api.hostActions, 'confirm').mockResolvedValue({
          token: 'token-1',
          expires: '2026-09-28T10:10:05Z',
        })
        vi.spyOn(api.hostActions, 'place').mockRejectedValue(new Error(message))
      }
      const onPlaced = vi.fn()

      actions({ onPlaced })
      await userEvent.click(screen.getByRole('button', { name: 'Shut down host' }))
      await userEvent.type(screen.getByLabelText(/to confirm/), 'example-host')
      await userEvent.click(
        within(screen.getByRole('dialog')).getByRole('button', { name: 'Shut down host' }),
      )

      const dialog = await screen.findByRole('dialog')
      expect(await within(dialog).findByText(message)).toHaveClass('text-destructive')
      expect(within(dialog).getByLabelText(/to confirm/)).toHaveValue('example-host')
      expect(onPlaced).not.toHaveBeenCalled()
    },
  )

  it('Keep running closes the dialog and places nothing', async () => {
    const confirm = vi.spyOn(api.hostActions, 'confirm')
    actions()
    await userEvent.click(screen.getByRole('button', { name: 'Restart service' }))
    await userEvent.click(screen.getByRole('button', { name: 'Keep running' }))
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(confirm).not.toHaveBeenCalled()
  })

  // 14-REVIEW CR-01: once the confirm is pressed the order is on its way, and a
  // mutation's onSuccess runs even after the dialog has gone. A close in those
  // seconds would say "Keep running" and restart the host anyway, so there is
  // no close: the dialog stays, says Working…, and ends as the order does.
  // The check's dialog is held the same way (13-12): its own dialog, its own
  // order on the way.
  it.each(
    (
      [
        [
          'Keep running',
          () => userEvent.click(screen.getByRole('button', { name: 'Keep running' })),
        ],
        ['Escape', () => userEvent.keyboard('{Escape}')],
        [
          'the close X',
          () =>
            userEvent.click(
              within(screen.getByRole('dialog')).getByRole('button', { name: 'Close' }),
            ),
        ],
      ] as const
    ).flatMap(([how, attempt]) =>
      (
        [
          ['reboot', 'Restart host'],
          ['check-update', 'Check for updates'],
        ] as const
      ).map(([action, label]) => [how, label, attempt, action] as const),
    ),
  )(
    'while the order is being placed, %s does not close the %s dialog',
    async (_, label, attempt, action) => {
      let answer: (value: { token: string; expires: string }) => void = () => undefined
      vi.spyOn(api.hostActions, 'confirm').mockReturnValue(
        new Promise((resolve) => {
          answer = resolve
        }),
      )
      const order: HostOrder = { ...held, action }
      const place = vi.spyOn(api.hostActions, 'place').mockResolvedValue({ order })
      const onPlaced = vi.fn()
      actions({ onPlaced })
      await userEvent.click(screen.getByRole('button', { name: label }))
      await userEvent.type(screen.getByLabelText(/to confirm/), 'example-host')
      await userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: label }))
      const dialog = screen.getByRole('dialog')
      await within(dialog).findByRole('button', { name: /Working/ })

      await attempt()
      // Past the tick in which Radix would have unmounted it.
      await new Promise((resolve) => setTimeout(resolve, 50))
      expect(screen.queryByRole('dialog')).toBe(dialog)
      expect(within(dialog).getByRole('button', { name: 'Keep running' })).toBeDisabled()
      expect(place).not.toHaveBeenCalled()

      // The order goes through in front of the operator, not behind a closed dialog.
      answer({ token: 'token-1', expires: '2026-09-28T10:10:05Z' })
      await vi.waitFor(() => expect(onPlaced).toHaveBeenCalledWith(order))
      expect(place).toHaveBeenCalledWith(action, 'token-1')
    },
  )

  // 13-UI-REVIEW fix 1: a cancel is not an order. The trigger is still on, and
  // a keyboard or screen-reader user left on the page body sits next to the
  // destructive pair with no idea where they are.
  it.each([
    ['Keep running', () => userEvent.click(screen.getByRole('button', { name: 'Keep running' }))],
    ['Escape', () => userEvent.keyboard('{Escape}')],
    [
      'the close X',
      () =>
        userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Close' })),
    ],
  ] as const)('focus returns to the button that opened it on cancel with %s', async (_, cancel) => {
    actions()
    const trigger = screen.getByRole('button', { name: 'Restart host' })
    await userEvent.click(trigger)
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    await cancel()
    expect(screen.queryByRole('dialog')).toBeNull()
    // Radix hands focus back after a tick.
    await vi.waitFor(() => expect(trigger).toHaveFocus())
  })

  // 14-REVIEW WR-01: a reason that arrives while the dialog is open turns the
  // trigger off too, and focus() on a disabled button does nothing -- the
  // operator was left on the page body. The group's reason line says why the
  // buttons went off, so that is where a cancel hands focus.
  it.each([
    ['Keep running', () => userEvent.click(screen.getByRole('button', { name: 'Keep running' }))],
    ['Escape', () => userEvent.keyboard('{Escape}')],
    [
      'the close X',
      () =>
        userEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Close' })),
    ],
  ] as const)(
    'with the trigger turned off meanwhile, %s hands focus to the reason line',
    async (_, cancel) => {
      const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
      const view = (pollFailed: boolean) => (
        <QueryClientProvider client={client}>
          <HostActions
            host={hostWith()}
            sessionRole="operator"
            pollFailed={pollFailed}
            order={null}
            onPlaced={() => undefined}
          />
        </QueryClientProvider>
      )
      const { rerender } = render(view(false))
      const trigger = screen.getByRole('button', { name: 'Restart host' })
      await userEvent.click(trigger)
      rerender(view(true))
      expect(trigger).toBeDisabled()

      await cancel()
      expect(screen.queryByRole('dialog')).toBeNull()
      const line = reasonLine()
      expect(line).toHaveTextContent(
        'holzkube-manager is not answering; host actions return when it does.',
      )
      await vi.waitFor(() => expect(line).toHaveFocus())
      expect(document.activeElement).not.toBe(document.body)
    },
  )

  // WR-01 for the check: an older helper learnt while the check's dialog is
  // open turns its trigger off; the dialog says why and a cancel hands focus to
  // the line that says it.
  it('an older helper learnt while the check dialog is open: the confirm is off, and a cancel focuses the reason', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const view = (outdated: ReadonlyArray<unknown>) => (
      <QueryClientProvider client={client}>
        <HostActions
          host={hostWith({ outdated })}
          sessionRole="operator"
          pollFailed={false}
          order={null}
          onPlaced={() => undefined}
        />
      </QueryClientProvider>
    )
    const { rerender } = render(view([]))
    const trigger = screen.getByRole('button', { name: 'Check for updates' })
    await userEvent.click(trigger)
    await userEvent.type(screen.getByLabelText(/to confirm/), 'example-host')
    rerender(view(olderScript))
    expect(trigger).toBeDisabled()
    const dialog = screen.getByRole('dialog')
    within(dialog).getByText(CHECK_NEEDS_NEWER)
    expect(within(dialog).getByRole('button', { name: /^Check for updates/ })).toBeDisabled()

    await userEvent.click(screen.getByRole('button', { name: 'Keep running' }))
    expect(screen.queryByRole('dialog')).toBeNull()
    const line = reasonLine()
    expect(line).toHaveTextContent(CHECK_NEEDS_NEWER)
    await vi.waitFor(() => expect(line).toHaveFocus())
  })

  // The check's reason is the check's alone: a restart's dialog stays usable.
  it('with an older helper the other dialogs are not told the check reason', async () => {
    actions({ host: hostWith({ outdated: olderScript }) })
    await userEvent.click(screen.getByRole('button', { name: 'Restart host' }))
    await userEvent.type(screen.getByLabelText(/to confirm/), 'example-host')
    const dialog = screen.getByRole('dialog')
    expect(within(dialog).queryByText(CHECK_NEEDS_NEWER)).toBeNull()
    expect(within(dialog).getByRole('button', { name: /^Restart host/ })).toBeEnabled()
  })

  // The other half of the rule: after an order the status box takes focus, so
  // the dialog must not hand it back -- even when the trigger is still on
  // (here the parent never follows the order, so nothing turns it off).
  it('focus does not return to the button after an order was placed', async () => {
    vi.spyOn(api.hostActions, 'confirm').mockResolvedValue({
      token: 'token-1',
      expires: '2026-09-28T10:10:05Z',
    })
    vi.spyOn(api.hostActions, 'place').mockResolvedValue({ order: { ...held, action: 'reboot' } })
    const onPlaced = vi.fn()
    actions({ onPlaced })
    const trigger = screen.getByRole('button', { name: 'Restart host' })
    await userEvent.click(trigger)
    await userEvent.type(screen.getByLabelText(/to confirm/), 'example-host')
    await userEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: 'Restart host' }),
    )
    await vi.waitFor(() => expect(onPlaced).toHaveBeenCalled())
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(trigger).toBeEnabled()
    // Past the tick after which Radix would have handed focus back.
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(trigger).not.toHaveFocus()
  })

  // 13-UI-REVIEW fix 2: the page learns a reason while the dialog is open (a
  // failed poll, another order, a role change). The route would refuse the
  // confirm, so the dialog must not offer it.
  it('the reason in an open dialog: shown above the footer, and the confirm is off until it is gone', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const view = (pollFailed: boolean) => (
      <QueryClientProvider client={client}>
        <HostActions
          host={hostWith()}
          sessionRole="operator"
          pollFailed={pollFailed}
          order={null}
          onPlaced={() => undefined}
        />
      </QueryClientProvider>
    )
    const { rerender } = render(view(false))
    await userEvent.click(screen.getByRole('button', { name: 'Restart host' }))
    await userEvent.type(screen.getByLabelText(/to confirm/), 'example-host')
    const confirmIn = () =>
      within(screen.getByRole('dialog')).getByRole('button', { name: /^Restart host/ })
    expect(confirmIn()).toBeEnabled()

    rerender(view(true))
    const dialog = screen.getByRole('dialog')
    const said = within(dialog).getByText(
      'holzkube-manager is not answering; host actions return when it does.',
    )
    expect(confirmIn()).toBeDisabled()
    expect(confirmIn()).toHaveAttribute('aria-describedby', said.id)
    expect(said.id).not.toBe('')
    // Above the footer: the sentence comes before Keep running in the dialog.
    const keep = within(dialog).getByRole('button', { name: 'Keep running' })
    expect(said.compareDocumentPosition(keep) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

    rerender(view(false))
    expect(
      within(screen.getByRole('dialog')).queryByText(
        'holzkube-manager is not answering; host actions return when it does.',
      ),
    ).toBeNull()
    expect(confirmIn()).toBeEnabled()
    expect(confirmIn()).not.toHaveAttribute('aria-describedby')
  })

  it('the Problem clears as soon as the typed text changes', async () => {
    vi.spyOn(api.hostActions, 'confirm').mockRejectedValue(
      new ProblemError({
        type: `${PROBLEM_BASE_URI}validation`,
        title: 'Validation failed',
        status: 400,
        detail: "Type this machine's hostname exactly to confirm.",
        code: 'validation.failed',
      }),
    )
    actions()
    await userEvent.click(screen.getByRole('button', { name: 'Restart host' }))
    const field = screen.getByLabelText(/to confirm/)
    await userEvent.type(field, 'example-host')
    await userEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: 'Restart host' }),
    )
    const dialog = screen.getByRole('dialog')
    expect(
      await within(dialog).findByText("Type this machine's hostname exactly to confirm."),
    ).toHaveClass('text-destructive')

    await userEvent.type(field, 'x')
    expect(
      within(dialog).queryByText("Type this machine's hostname exactly to confirm."),
    ).toBeNull()
  })
})

/**
 * The password step, through the real client and the real SudoDialog: the
 * action route answers 428, the shell's dialog opens on top of the host dialog,
 * and the request is replayed -- or, cancelled, the host dialog is left as the
 * operator left it. Only fetch is faked.
 */
describe('HostActions: the password step', () => {
  const json = (status: number, body: unknown) => () =>
    new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    })
  const sudoRequired = () =>
    new Response(
      JSON.stringify({
        type: `${PROBLEM_BASE_URI}sudo`,
        title: 'Re-authentication required',
        status: 428,
        detail: 'This action is destructive.',
        instance: '/requests/deadbeef',
        code: 'sudo.required',
      }),
      { status: 428, headers: { 'Content-Type': 'application/problem+json' } },
    )
  const token = json(200, {
    token: 'token-1',
    expires: '2026-09-28T10:10:05Z',
    action: 'host.reboot',
  })

  function fetchAnswering(...responses: Array<() => Response>) {
    let call = 0
    const fetchMock = vi.fn(async () => {
      const next = responses[call]
      call += 1
      if (next === undefined) {
        throw new Error(`unexpected fetch call #${call}`)
      }
      return next()
    })
    vi.stubGlobal('fetch', fetchMock)
    return fetchMock
  }

  function withSudo(onPlaced: (order: HostOrder) => void) {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    // The identity the shell already holds: a password session.
    client.setQueryData(SESSION_QUERY_KEY, {
      id: 'u1',
      username: 'operator',
      dry_run: false,
      role: 'operator',
      sso: false,
    })
    return render(
      <QueryClientProvider client={client}>
        <HostActions
          host={hostWith()}
          sessionRole="operator"
          pollFailed={false}
          order={null}
          onPlaced={onPlaced}
        />
        <SudoDialog />
      </QueryClientProvider>,
    )
  }

  async function typeAndConfirm() {
    await userEvent.click(screen.getByRole('button', { name: 'Restart host' }))
    await userEvent.type(screen.getByLabelText(/to confirm/), 'example-host')
    await userEvent.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: 'Restart host' }),
    )
  }

  it('a 428 opens the password prompt on top, and the order is placed on the replay', async () => {
    const order: HostOrder = { ...held, action: 'reboot' }
    const fetchMock = fetchAnswering(
      token,
      sudoRequired,
      () => new Response(null, { status: 204 }), // POST /api/v1/auth/sudo
      json(202, { order }),
    )
    const onPlaced = vi.fn()
    withSudo(onPlaced)
    await typeAndConfirm()

    const password = await screen.findByLabelText('Password')
    // Both dialogs are open: the host dialog is not closed for the prompt.
    expect(screen.getAllByRole('dialog', { hidden: true })).toHaveLength(2)
    await userEvent.type(password, 'a-password')
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }))

    await vi.waitFor(() => expect(onPlaced).toHaveBeenCalledWith(order))
    const paths = fetchMock.mock.calls.map((c) => String((c as unknown[])[0]))
    expect(paths).toEqual([
      '/api/v1/host/confirm',
      '/api/v1/host/actions/reboot',
      '/api/v1/auth/sudo',
      '/api/v1/host/actions/reboot',
    ])
  })

  it('cancelling the password prompt leaves the host dialog open with its typed text', async () => {
    const fetchMock = fetchAnswering(token, sudoRequired)
    const onPlaced = vi.fn()
    withSudo(onPlaced)
    await typeAndConfirm()

    await screen.findByLabelText('Password')
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    await vi.waitFor(() => expect(screen.queryByLabelText('Password')).toBeNull())
    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByLabelText(/to confirm/)).toHaveValue('example-host')
    expect(within(dialog).getByRole('button', { name: 'Restart host' })).toBeEnabled()
    expect(onPlaced).not.toHaveBeenCalled()
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})

/** A result for the order `id`, as the helper records it. */
const resultFor = (
  id: string,
  action: HostAction,
  outcome: string,
  at = '2026-09-28T10:00:06Z',
) => ({
  readable: true,
  value: { id, action, outcome, at },
})

/**
 * A host answer at a later moment: read at `observed`, the process started at
 * `started`, the machine up for `uptime` seconds, and the update status as
 * given (not recorded unless named).
 */
function later(
  actions: Record<string, unknown>,
  {
    observed = '2026-09-28T10:05:00Z',
    started = '2026-09-28T08:00:00Z',
    uptime = 26090,
    update,
  }: {
    observed?: string
    started?: string
    uptime?: number
    update?: Record<string, unknown>
  } = {},
): Host {
  const host = hostWith(actions, { observed_at: observed })
  host.service.started_at = started
  host.device.uptime_seconds = { readable: true, value: uptime }
  if (update !== undefined) {
    host.service.update = hostSchema.shape.service.shape.update.parse({
      readable: true,
      value: update,
    })
  }
  return host
}

/** Readings 25 s and 15 min 1 s after the order below was placed. */
const SOON = '2026-09-28T10:00:30Z'
const LATE = '2026-09-28T10:15:06Z'

const order = (action: HostAction, state: HostOrder['state'] = 'picked-up'): HostOrder => ({
  id: ID,
  action,
  placed_at: '2026-09-28T10:00:05Z',
  state,
})

/** An order placed 734 ms into 10:00:05, as the daemon's clock writes it. */
const subSecond = (action: HostAction): HostOrder => ({
  ...order(action),
  placed_at: '2026-09-28T10:00:05.734Z',
})

/** The update status recorded in that same whole second. */
const sameSecond = (outcome: string) => ({
  checked_at: '2026-09-28T10:00:05Z',
  installed: 'v0.1.0',
  latest: outcome === 'current' ? 'v0.1.0' : 'v9.9.9',
  outcome,
})

describe('orderPhase: every phase from server fields', () => {
  type Row = [string, HostOrder, Host, boolean, string]
  const rows: Row[] = [
    [
      'the file is there',
      order('reboot', 'pending'),
      later({ order: order('reboot', 'pending') }),
      false,
      'placed',
    ],
    [
      'not reported yet: as placed',
      order('reboot', 'pending'),
      later({ order: null }, { observed: SOON }),
      false,
      'placed',
    ],
    [
      'the file is gone, nothing recorded',
      order('reboot', 'pending'),
      later({ order: order('reboot') }, { observed: SOON }),
      false,
      'picked-up',
    ],
    // WR-04: a picked-up order nobody answers must not lock the page for ever.
    [
      'the file is gone, nothing recorded for a minute',
      order('reboot', 'pending'),
      later({ order: order('reboot') }),
      false,
      'no-answer',
    ],
    [
      'never reported, a minute on (the daemon restarted)',
      order('reboot', 'pending'),
      later({ order: null }),
      false,
      'no-answer',
    ],
    [
      'the daemon says the file is still there: placed, however long',
      order('reboot', 'pending'),
      later({ order: order('reboot', 'pending') }),
      false,
      'placed',
    ],
    [
      'the result unreadable, a minute on',
      order('update'),
      later({ order: order('update'), result: noResult }),
      false,
      'no-answer',
    ],
    [
      'withdrawn after 10 s',
      order('reboot', 'pending'),
      later({ order: order('reboot', 'withdrawn') }),
      false,
      'not-picked-up',
    ],
    [
      'picked up, then no answer',
      order('reboot'),
      later({ order: order('reboot') }),
      true,
      'waiting',
    ],
    [
      'placed, then no answer: still placed',
      order('reboot', 'pending'),
      later({ order: order('reboot', 'pending') }),
      true,
      'placed',
    ],
    [
      'rejected',
      order('reboot'),
      later({ order: order('reboot'), result: resultFor(ID, 'reboot', 'rejected') }),
      false,
      'rejected',
    ],
    [
      'failed',
      order('poweroff'),
      later({ order: order('poweroff'), result: resultFor(ID, 'poweroff', 'failed') }),
      false,
      'failed',
    ],
    // How a check usually fails: the update script records failed, the unit
    // fails, and the helper records failed for the order. A failed record is
    // the check's end and it is failed -- not finished, as done is, and not
    // finished by an update status recorded while it ran, whatever that says:
    // the helper's word comes first (13-REVIEW-2 V-08). Which sentence the
    // box then says is held by the HostOrderStatus rows (WR-02, V-07).
    [
      'check failed, as the helper records it',
      order('check-update'),
      later({ order: order('check-update'), result: resultFor(ID, 'check-update', 'failed') }),
      false,
      'failed',
    ],
    [
      'check failed, the update status failed too',
      order('check-update'),
      later(
        { order: order('check-update'), result: resultFor(ID, 'check-update', 'failed') },
        { update: { ...sameSecond('failed'), checked_at: '2026-09-28T10:00:06Z' } },
      ),
      false,
      'failed',
    ],
    [
      'check failed, an hourly run during it said up to date',
      order('check-update'),
      later(
        { order: order('check-update'), result: resultFor(ID, 'check-update', 'failed') },
        { update: { ...sameSecond('current'), checked_at: '2026-09-28T10:00:06Z' } },
      ),
      false,
      'failed',
    ],
    [
      'check failed, then no answer: failed, never waiting',
      order('check-update'),
      later({ order: order('check-update'), result: resultFor(ID, 'check-update', 'failed') }),
      true,
      'failed',
    ],
    // 13-REVIEW-2 V-01: the helper records the end of a check it waited for.
    [
      'check done, as the helper records it',
      order('check-update'),
      later(
        { order: order('check-update'), result: resultFor(ID, 'check-update', 'done') },
        { observed: SOON },
      ),
      false,
      'update-finished',
    ],
    [
      'check done, then no answer: finished, never waiting',
      order('check-update'),
      later(
        { order: order('check-update'), result: resultFor(ID, 'check-update', 'done') },
        { observed: SOON },
      ),
      true,
      'update-finished',
    ],
    [
      "another order's result",
      order('reboot'),
      later(
        {
          order: order('reboot'),
          result: resultFor('0000000000000000', 'reboot', 'rejected'),
        },
        { observed: SOON },
      ),
      false,
      'picked-up',
    ],
    [
      'restart service started, same process',
      order('restart-service'),
      later({
        order: order('restart-service'),
        result: resultFor(ID, 'restart-service', 'started'),
      }),
      false,
      'started',
    ],
    [
      'restart service started, no answer',
      order('restart-service'),
      later({
        order: order('restart-service'),
        result: resultFor(ID, 'restart-service', 'started'),
      }),
      true,
      'waiting',
    ],
    [
      'restart service, a process started after the order',
      order('restart-service'),
      later(
        { order: null, result: resultFor(ID, 'restart-service', 'started') },
        { started: '2026-09-28T10:00:20Z' },
      ),
      false,
      'back',
    ],
    [
      'update started, nothing newer',
      order('update'),
      later({ order: order('update'), result: resultFor(ID, 'update', 'started') }),
      false,
      'started',
    ],
    [
      'update, a process started after the order, no status',
      order('update'),
      later(
        { order: null, result: resultFor(ID, 'update', 'started') },
        { started: '2026-09-28T10:00:40Z' },
      ),
      false,
      'back',
    ],
    [
      'update, the status newer than the order: finished, before back',
      order('update'),
      later(
        { order: null, result: resultFor(ID, 'update', 'started') },
        {
          started: '2026-09-28T10:00:40Z',
          update: {
            checked_at: '2026-09-28T10:00:45Z',
            installed: 'v0.2.0',
            latest: 'v0.2.0',
            outcome: 'updated',
          },
        },
      ),
      false,
      'update-finished',
    ],
    [
      'update, a status older than the order does not finish it',
      order('update'),
      later(
        { order: order('update'), result: resultFor(ID, 'update', 'started') },
        {
          update: {
            checked_at: '2026-09-28T09:00:00Z',
            installed: 'v0.1.0',
            latest: 'v0.1.0',
            outcome: 'current',
          },
        },
      ),
      false,
      'started',
    ],
    [
      'restart host, booted before the order',
      order('reboot'),
      later({ order: order('reboot'), result: resultFor(ID, 'reboot', 'started') }),
      false,
      'started',
    ],
    [
      'restart host, no answer',
      order('reboot'),
      later({ order: order('reboot'), result: resultFor(ID, 'reboot', 'started') }),
      true,
      'waiting',
    ],
    // Read at 10:05:00 after 60 s up: booted 10:04:00, after the order.
    [
      'restart host, booted after the order',
      order('reboot'),
      later({ order: null, result: resultFor(ID, 'reboot', 'started') }, { uptime: 60 }),
      false,
      'back',
    ],
    [
      'shut down host, switched on again',
      order('poweroff'),
      later({ order: null, result: resultFor(ID, 'poweroff', 'started') }, { uptime: 60 }),
      false,
      'back',
    ],
    [
      'shut down host, no answer',
      order('poweroff'),
      later({ order: order('poweroff'), result: resultFor(ID, 'poweroff', 'started') }),
      true,
      'waiting',
    ],
    // WR-04: started, and 15 minutes later still not done.
    [
      'update started, 15 min without a newer status',
      order('update'),
      later(
        { order: order('update'), result: resultFor(ID, 'update', 'started') },
        { observed: LATE },
      ),
      false,
      'no-answer',
    ],
    [
      'restart host started, 15 min and not rebooted',
      order('reboot'),
      later(
        { order: order('reboot'), result: resultFor(ID, 'reboot', 'started') },
        { observed: LATE },
      ),
      false,
      'no-answer',
    ],
    [
      'restart host started, 15 min, no answer: still waiting',
      order('reboot'),
      later(
        { order: order('reboot'), result: resultFor(ID, 'reboot', 'started') },
        { observed: LATE },
      ),
      true,
      'waiting',
    ],
    // Decision 6 of 13-12: checked_at has whole seconds, placed_at does not.
    // A run that ends in the second it was placed in is finished, not
    // waiting 15 min for "no answer".
    [
      'update, the status in the same second as the placement: finished',
      subSecond('update'),
      later(
        { order: subSecond('update'), result: resultFor(ID, 'update', 'started') },
        { update: sameSecond('current') },
      ),
      false,
      'update-finished',
    ],
    // 13-REVIEW-2 V-01: a check is over when the helper records it, not when
    // an update status appears -- the hourly run may write one meanwhile.
    [
      'check started, an update status since: still started',
      subSecond('check-update'),
      later(
        { order: subSecond('check-update'), result: resultFor(ID, 'check-update', 'started') },
        { observed: SOON, update: sameSecond('available') },
      ),
      false,
      'started',
    ],
    [
      'check, a status the second before the placement does not finish it',
      subSecond('check-update'),
      later(
        { order: subSecond('check-update'), result: resultFor(ID, 'check-update', 'started') },
        {
          observed: SOON,
          update: { ...sameSecond('available'), checked_at: '2026-09-28T10:00:04Z' },
        },
      ),
      false,
      'started',
    ],
    [
      'check started, nothing newer',
      order('check-update'),
      later(
        { order: order('check-update'), result: resultFor(ID, 'check-update', 'started') },
        { observed: SOON },
      ),
      false,
      'started',
    ],
    // A check restarts nothing: a process started after it is not its answer.
    [
      'check started, a process started after the order: still started, never back',
      order('check-update'),
      later(
        { order: null, result: resultFor(ID, 'check-update', 'started') },
        { observed: SOON, started: '2026-09-28T10:00:20Z' },
      ),
      false,
      'started',
    ],
    [
      'check started, 2 min 59 s without a newer status: still started',
      order('check-update'),
      later(
        { order: order('check-update'), result: resultFor(ID, 'check-update', 'started') },
        { observed: '2026-09-28T10:03:04Z' },
      ),
      false,
      'started',
    ],
    [
      'check started, 3 min without a newer status: no answer',
      order('check-update'),
      later(
        { order: order('check-update'), result: resultFor(ID, 'check-update', 'started') },
        { observed: '2026-09-28T10:03:06Z' },
      ),
      false,
      'no-answer',
    ],
    // A check takes nothing away: a failed poll during it is a stale reading,
    // never "Waiting for holzkube-manager to come back".
    [
      'check picked up, then no answer: still picked up, never waiting',
      order('check-update'),
      later({ order: order('check-update') }, { observed: SOON }),
      true,
      'picked-up',
    ],
    [
      'check started, then no answer: still started, never waiting',
      order('check-update'),
      later(
        { order: order('check-update'), result: resultFor(ID, 'check-update', 'started') },
        { observed: SOON },
      ),
      true,
      'started',
    ],
    [
      'shut down host, switched on again a day later: back, not no answer',
      order('poweroff'),
      later(
        { order: null, result: resultFor(ID, 'poweroff', 'started') },
        { observed: '2026-09-29T10:05:00Z', uptime: 60 },
      ),
      false,
      'back',
    ],
  ]
  it.each(rows)('%s', (_, o, host, pollFailed, want) => {
    expect(orderPhase(o, host, pollFailed)).toBe(want)
  })

  it('never calls a host back whose uptime could not be read', () => {
    const host = later({ order: null, result: resultFor(ID, 'reboot', 'started') })
    host.device.uptime_seconds = { readable: false, reason: { code: 'read-failed', message: 'x' } }
    expect(orderPhase(order('reboot'), host)).toBe('started')
  })
})

describe('followedOrder: which order the box follows', () => {
  const none: ReadonlySet<string> = new Set()

  it('the one this page placed, first', () => {
    const other = { ...order('reboot', 'pending'), id: '1111111111111111' }
    expect(followedOrder(held, later({ order: other }), none)).toEqual({
      order: held,
      from: 'held',
    })
  })

  it("else the daemon's order, within 15 minutes of the reading", () => {
    expect(followedOrder(null, later({ order: order('reboot') }), none)).toEqual({
      order: order('reboot'),
      from: 'order',
    })
    // Read 16 minutes after it was placed: yesterday's news.
    expect(
      followedOrder(
        null,
        later({ order: order('reboot') }, { observed: '2026-09-28T10:16:06Z' }),
        none,
      ),
    ).toBeNull()
  })

  it("else the helper's result that names an order, within 15 minutes", () => {
    expect(
      followedOrder(null, later({ order: null, result: resultFor(ID, 'reboot', 'started') }), none),
    ).toEqual({
      order: { id: ID, action: 'reboot', placed_at: '2026-09-28T10:00:06Z', state: 'picked-up' },
      from: 'result',
    })
    const unnamed = {
      readable: true,
      value: { id: '', action: '', outcome: 'rejected', at: '2026-09-28T10:00:06Z' },
    }
    expect(followedOrder(null, later({ order: null, result: unnamed }), none)).toBeNull()
  })

  it('never one that was dismissed, whichever way it comes back', () => {
    const gone = new Set([ID])
    expect(followedOrder(held, later({ order: order('update') }), gone)).toBeNull()
    expect(
      followedOrder(null, later({ order: null, result: resultFor(ID, 'update', 'failed') }), gone),
    ).toBeNull()
  })
})

const updatedShape = {
  checked_at: '2026-09-28T10:00:45Z',
  installed: 'v0.1.0',
  latest: 'v0.2.0',
  outcome: 'updated',
}

const SLATE = 'border-slate-500/40'
const EMERALD = 'border-emerald-600/40'
const RED = 'border-red-600/40'

function status(
  host: Host,
  o: HostOrder,
  phase: ReturnType<typeof orderPhase>,
  extra: Partial<{
    from: 'held' | 'order' | 'result'
    takeFocus: boolean
    onDismiss: () => void
  }> = {},
) {
  return wrap(
    <HostOrderStatus
      host={host}
      followed={{ order: o, from: extra.from ?? 'held' }}
      phase={phase}
      takeFocus={extra.takeFocus ?? false}
      onDismiss={extra.onDismiss ?? (() => undefined)}
    />,
  )
}

describe('HostOrderStatus', () => {
  type Row = [string, HostOrder, Host, ReturnType<typeof orderPhase>, string, string, boolean]
  const updated = {
    checked_at: '2026-09-28T10:00:45Z',
    installed: 'v0.2.0',
    latest: 'v0.2.0',
    outcome: 'updated',
  }
  const failedUpdate = {
    checked_at: '2026-09-28T10:00:45Z',
    installed: 'v0.1.0',
    latest: 'v0.2.0',
    outcome: 'failed',
  }
  const checked = (outcome: string) => ({
    checked_at: '2026-09-28T10:00:07Z',
    installed: 'v0.1.0',
    latest: outcome === 'current' ? 'v0.1.0' : 'v9.9.9',
    outcome,
  })
  const rows: Row[] = [
    [
      'placed',
      order('update', 'pending'),
      later({}),
      'placed',
      'Check for updates and install — order placed. Waiting for the helper to pick it up.',
      SLATE,
      false,
    ],
    [
      'picked up',
      order('reboot'),
      later({}),
      'picked-up',
      'Restart host — the helper picked up the order.',
      SLATE,
      false,
    ],
    [
      'started, update',
      order('update'),
      later({}),
      'started',
      'Check for updates and install — started. If a newer release exists it is installed and holzkube-manager restarts; the outcome appears here and under Update check.',
      SLATE,
      false,
    ],
    [
      'started, restart service',
      order('restart-service'),
      later({}),
      'started',
      'Restart service — started. holzkube-manager is restarting.',
      SLATE,
      false,
    ],
    [
      'started, restart host',
      order('reboot'),
      later({}),
      'started',
      'Restart host — started. The host is restarting.',
      SLATE,
      false,
    ],
    [
      'started, shut down host',
      order('poweroff'),
      later({}),
      'started',
      'Shut down host — started. The host is shutting down.',
      SLATE,
      false,
    ],
    [
      'waiting keeps the last sentence',
      order('reboot'),
      later({ result: resultFor(ID, 'reboot', 'started') }),
      'waiting',
      'Restart host — started. The host is restarting.',
      SLATE,
      false,
    ],
    [
      'rejected',
      order('reboot'),
      later({}),
      'rejected',
      'Restart host — the helper rejected the order, so nothing was done. journalctl -u holzkube-manager-host says why.',
      RED,
      true,
    ],
    [
      'failed',
      order('poweroff'),
      later({}),
      'failed',
      'Shut down host — the helper could not carry it out. journalctl -u holzkube-manager-host says why.',
      RED,
      true,
    ],
    [
      'check failed, as the helper records it',
      order('check-update'),
      later({ result: resultFor(ID, 'check-update', 'failed') }, { update: checked('failed') }),
      'failed',
      'Check for updates — the check failed, and nothing was installed. journalctl -u holzkube-manager-update-check says why.',
      RED,
      true,
    ],
    [
      'not picked up',
      order('reboot', 'withdrawn'),
      later({}),
      'not-picked-up',
      'Restart host — the helper did not pick up the order within 10 s, so holzkube-manager withdrew it. Nothing was done. Check that the helper is running: systemctl status holzkube-manager-host.path',
      RED,
      true,
    ],
    // 13-REVIEW-2 V-01: the helper was waiting for a check, not missing.
    [
      'not picked up while the helper waited for a check',
      order('reboot', 'withdrawn'),
      later({
        result: resultFor('c0ffee00c0ffee11', 'check-update', 'started', '2026-09-28T10:00:01Z'),
      }),
      'not-picked-up',
      'Restart host — the helper did not pick up the order within 10 s, so holzkube-manager withdrew it. Nothing was done. The helper was busy with an update check, which holds every other order until it ends; place the order again once it has.',
      RED,
      true,
    ],
    [
      'not picked up, a check that ended after the placement',
      order('reboot', 'withdrawn'),
      later({
        result: resultFor('c0ffee00c0ffee11', 'check-update', 'done', '2026-09-28T10:00:09Z'),
      }),
      'not-picked-up',
      'Restart host — the helper did not pick up the order within 10 s, so holzkube-manager withdrew it. Nothing was done. The helper was busy with an update check, which holds every other order until it ends; place the order again once it has.',
      RED,
      true,
    ],
    [
      'not picked up, a check that ended before the placement',
      order('reboot', 'withdrawn'),
      later({
        result: resultFor('c0ffee00c0ffee11', 'check-update', 'done', '2026-09-28T10:00:04Z'),
      }),
      'not-picked-up',
      'Restart host — the helper did not pick up the order within 10 s, so holzkube-manager withdrew it. Nothing was done. Check that the helper is running: systemctl status holzkube-manager-host.path',
      RED,
      true,
    ],
    [
      'not picked up, a check started before the last boot',
      order('reboot', 'withdrawn'),
      later(
        {
          result: resultFor('c0ffee00c0ffee11', 'check-update', 'started', '2026-09-28T09:00:00Z'),
        },
        { uptime: 60 },
      ),
      'not-picked-up',
      'Restart host — the helper did not pick up the order within 10 s, so holzkube-manager withdrew it. Nothing was done. Check that the helper is running: systemctl status holzkube-manager-host.path',
      RED,
      true,
    ],
    [
      'back, restart service',
      order('restart-service'),
      later({}, { started: '2026-09-28T10:00:20Z' }),
      'back',
      `Restart service — done. holzkube-manager is back, running 0.1.0 since ${new Date('2026-09-28T10:00:20Z').toLocaleTimeString()}.`,
      EMERALD,
      true,
    ],
    [
      'back, update',
      order('update'),
      later({}, { started: '2026-09-28T10:00:40Z' }),
      'back',
      'Check for updates and install — done. holzkube-manager is back, running 0.1.0.',
      EMERALD,
      true,
    ],
    [
      'back, restart host',
      order('reboot'),
      later({}, { uptime: 60 }),
      'back',
      `Restart host — done. The host restarted and holzkube-manager is back; up since ${new Date('2026-09-28T10:04:00Z').toLocaleTimeString()}.`,
      EMERALD,
      true,
    ],
    [
      'back, shut down host',
      order('poweroff'),
      later({}, { uptime: 60 }),
      'back',
      `Shut down host — the host was switched on again and holzkube-manager is back; up since ${new Date('2026-09-28T10:04:00Z').toLocaleTimeString()}.`,
      EMERALD,
      true,
    ],
    [
      'update finished, updated',
      order('update'),
      later({}, { update: updated }),
      'update-finished',
      'Check for updates and install — finished. Updated to v0.2.0.',
      EMERALD,
      true,
    ],
    [
      'no answer, nothing recorded',
      order('reboot'),
      later({}),
      'no-answer',
      'Restart host — no answer: the helper recorded nothing for this order within 1 min. journalctl -u holzkube-manager-host says what happened.',
      RED,
      true,
    ],
    [
      'no answer, update started',
      order('update'),
      later({ result: resultFor(ID, 'update', 'started') }, { observed: LATE }),
      'no-answer',
      'Check for updates and install — started, but no finished update was reported within 15 min. journalctl -u holzkube-manager-update says what happened.',
      RED,
      true,
    ],
    [
      'no answer, restart service started',
      order('restart-service'),
      later({ result: resultFor(ID, 'restart-service', 'started') }, { observed: LATE }),
      'no-answer',
      'Restart service — started, but holzkube-manager has not restarted within 15 min. journalctl -u holzkube-manager-host says what happened.',
      RED,
      true,
    ],
    [
      'no answer, restart host started',
      order('reboot'),
      later({ result: resultFor(ID, 'reboot', 'started') }, { observed: LATE }),
      'no-answer',
      'Restart host — started, but the host has not restarted within 15 min. journalctl -u holzkube-manager-host says what happened.',
      RED,
      true,
    ],
    [
      'no answer, shut down host started',
      order('poweroff'),
      later({ result: resultFor(ID, 'poweroff', 'started') }, { observed: LATE }),
      'no-answer',
      'Shut down host — started, but the host is still running after 15 min. journalctl -u holzkube-manager-host says what happened.',
      RED,
      true,
    ],
    [
      'started, check',
      order('check-update'),
      later({}),
      'started',
      'Check for updates — started. Looking for a newer release; nothing is installed.',
      SLATE,
      false,
    ],
    [
      'check finished, a newer release available',
      order('check-update'),
      later({}, { update: checked('available') }),
      'update-finished',
      'Check for updates — finished. v9.9.9 is available; v0.1.0 is installed. Nothing was installed: Check for updates and install, or the hourly update, installs it.',
      EMERALD,
      true,
    ],
    [
      'check finished, up to date',
      order('check-update'),
      later({}, { update: checked('current') }),
      'update-finished',
      'Check for updates — finished. v0.1.0 is installed and is the newest release; there is nothing to install.',
      EMERALD,
      true,
    ],
    [
      'check finished, the check failed',
      order('check-update'),
      later({}, { update: checked('failed') }),
      'update-finished',
      'Check for updates — the check failed, and nothing was installed. journalctl -u holzkube-manager-update-check says why.',
      RED,
      true,
    ],
    // The hourly timer ran between the placement and the answer: its outcome,
    // said as the Update check row says it.
    [
      'check finished by an hourly update in between',
      order('check-update'),
      later({}, { update: updated }),
      'update-finished',
      'Check for updates — finished. Updated to v0.2.0.',
      EMERALD,
      true,
    ],
    // 13-REVIEW-2 V-01: a check the helper recorded done. Its answer is the
    // update status the update script wrote just before -- also when the page
    // knows the order only from that record, whose time is the check's end.
    [
      'check done, a newer release available',
      order('check-update'),
      later(
        { result: resultFor(ID, 'check-update', 'done', '2026-09-28T10:00:08Z') },
        { update: checked('available') },
      ),
      'update-finished',
      'Check for updates — finished. v9.9.9 is available; v0.1.0 is installed. Nothing was installed: Check for updates and install, or the hourly update, installs it.',
      EMERALD,
      true,
    ],
    [
      'check done, known only from the record, its answer a second before it',
      { ...order('check-update'), placed_at: '2026-09-28T10:00:08Z' },
      later(
        { result: resultFor(ID, 'check-update', 'done', '2026-09-28T10:00:08Z') },
        { update: checked('current') },
      ),
      'update-finished',
      'Check for updates — finished. v0.1.0 is installed and is the newest release; there is nothing to install.',
      EMERALD,
      true,
    ],
    [
      'check done, the update status not recorded',
      order('check-update'),
      later({ result: resultFor(ID, 'check-update', 'done', '2026-09-28T10:00:08Z') }),
      'update-finished',
      'Check for updates — finished, but what it found is not in the update status holzkube-manager reads. journalctl -u holzkube-manager-update-check says what it found.',
      SLATE,
      true,
    ],
    [
      'check done, the update status from an earlier run',
      order('check-update'),
      later(
        { result: resultFor(ID, 'check-update', 'done', '2026-09-28T10:00:08Z') },
        { update: { ...checked('updated'), checked_at: '2026-09-28T09:00:00Z' } },
      ),
      'update-finished',
      'Check for updates — finished, but what it found is not in the update status holzkube-manager reads. journalctl -u holzkube-manager-update-check says what it found.',
      SLATE,
      true,
    ],
    [
      'no answer, check started',
      order('check-update'),
      later({ result: resultFor(ID, 'check-update', 'started') }, { observed: LATE }),
      'no-answer',
      'Check for updates — started, but no update check was reported within 3 min. journalctl -u holzkube-manager-update-check says what happened.',
      RED,
      true,
    ],
    [
      'update finished, failed',
      order('update'),
      later({}, { update: failedUpdate }),
      'update-finished',
      'Check for updates and install — finished. The last update run failed; v0.1.0 is still installed.',
      RED,
      true,
    ],
  ]

  it.each(rows)(
    '%s: its sentence, its colour, and Dismiss status only when final',
    (_, o, host, phase, sentence, colour, final) => {
      status(host, o, phase)
      const box = screen.getByRole('status')
      expect(box.querySelector('p')?.textContent).toBe(sentence)
      expect(box).toHaveClass(colour)
      expect(box).toHaveTextContent(
        `Order ${ID} · placed ${new Date(o.placed_at).toLocaleTimeString()}`,
      )
      expect(within(box).queryByRole('button', { name: 'Dismiss status' }) !== null).toBe(final)
    },
  )

  it('says a command in a sentence as code', () => {
    status(later({}), order('reboot', 'withdrawn'), 'not-picked-up')
    expect(screen.getByText('systemctl status holzkube-manager-host.path').tagName).toBe('CODE')
  })

  it("says the check unit's journal as code", () => {
    status(
      later({}, { update: { ...updatedShape, outcome: 'failed' } }),
      order('check-update'),
      'update-finished',
    )
    expect(screen.getByText('journalctl -u holzkube-manager-update-check').tagName).toBe('CODE')
  })

  // 13-REVIEW-2 WR-02: the phase taken from the readings, not handed in -- the
  // way a failed check reaches the box -- names the check unit's journal, not
  // the helper's, whose only line is that the order failed.
  it("a failed check, read as the page reads it, points at the check unit's journal", () => {
    const host = later(
      { order: order('check-update'), result: resultFor(ID, 'check-update', 'failed') },
      { update: { ...updatedShape, checked_at: '2026-09-28T10:00:07Z', outcome: 'failed' } },
    )
    const o = order('check-update')
    status(host, o, orderPhase(o, host))
    const box = screen.getByRole('status')
    expect(box).toHaveTextContent('the check failed, and nothing was installed')
    expect(screen.getByText('journalctl -u holzkube-manager-update-check').tagName).toBe('CODE')
    expect(screen.queryByText('journalctl -u holzkube-manager-host')).toBeNull()
  })

  // 13-REVIEW-2 V-07: one failed check, one sentence. Every reading below is a
  // poll of the same check, the phase taken as the page takes it; whichever
  // way the failure arrives, the box says it the same way, and the readings
  // before the helper's end say started, not a second "failed" sentence.
  describe('one failed check is said one way', () => {
    const checkFailed =
      'Check for updates — the check failed, and nothing was installed. journalctl -u holzkube-manager-update-check says why.'
    const failedStatus = (at: string) => ({ ...updatedShape, checked_at: at, outcome: 'failed' })
    const sentence = (host: Host, o: HostOrder = order('check-update')) => {
      const { unmount } = status(host, o, orderPhase(o, host))
      const box = screen.getByRole('status')
      const said = { text: box.querySelector('p')?.textContent, red: box.classList.contains(RED) }
      unmount()
      return said
    }

    const arrivals: [string, Host, HostOrder | undefined][] = [
      [
        'the helper recorded the order failed',
        later({ order: order('check-update'), result: resultFor(ID, 'check-update', 'failed') }),
        undefined,
      ],
      [
        'the helper recorded failed, the check its failed status first',
        later(
          { order: order('check-update'), result: resultFor(ID, 'check-update', 'failed') },
          { update: failedStatus('2026-09-28T10:00:07Z') },
        ),
        undefined,
      ],
      [
        'the check ended (done) and its own status says failed',
        later(
          {
            order: order('check-update'),
            result: resultFor(ID, 'check-update', 'done', '2026-09-28T10:00:08Z'),
          },
          { update: failedStatus('2026-09-28T10:00:07Z') },
        ),
        undefined,
      ],
      [
        'known only from the done record, its failed status a second before it',
        later(
          { result: resultFor(ID, 'check-update', 'done', '2026-09-28T10:00:08Z') },
          { update: failedStatus('2026-09-28T10:00:07Z') },
        ),
        { ...order('check-update'), placed_at: '2026-09-28T10:00:08Z' },
      ],
    ]
    it.each(arrivals)('%s: the one sentence, red', (_, host, o) => {
      expect(sentence(host, o)).toEqual({ text: checkFailed, red: true })
    })

    it('the status written, the helper not yet done: still started', () => {
      const host = later(
        {
          order: order('check-update'),
          result: resultFor(ID, 'check-update', 'started', '2026-09-28T10:00:06Z'),
        },
        { observed: SOON, update: failedStatus('2026-09-28T10:00:07Z') },
      )
      expect(sentence(host)).toEqual({
        text: 'Check for updates — started. Looking for a newer release; nothing is installed.',
        red: false,
      })
    })

    // The helper's done record is the check's end: an update status recorded
    // after it is a later run's, and its failure is not the check's.
    it('a check done, then a later hourly run failed: not called a failed check', () => {
      const host = later(
        {
          order: order('check-update'),
          result: resultFor(ID, 'check-update', 'done', '2026-09-28T10:00:08Z'),
        },
        { update: failedStatus('2026-09-28T10:01:00Z') },
      )
      const said = sentence(host)
      expect(said.text).not.toContain('failed')
      expect(said).toEqual({
        text: 'Check for updates — finished, but what it found is not in the update status holzkube-manager reads. journalctl -u holzkube-manager-update-check says what it found.',
        red: false,
      })
    })
  })

  it('says when the helper recorded an order it only knows from the result', () => {
    status(later({}), order('reboot'), 'started', { from: 'result' })
    // 14-UI-SPEC copy: "recorded {time}" instead of "placed {time}".
    expect(screen.getByRole('status')).toHaveTextContent(
      `Order ${ID} · recorded ${new Date('2026-09-28T10:00:05Z').toLocaleTimeString()}`,
    )
    expect(screen.getByRole('status')).not.toHaveTextContent('placed')
  })

  // 13-UI-REVIEW fix 3: orderPhase never reaches 'back' for these two without
  // a boot time, so the render path is driven with the phase given directly.
  // The sentence leaves the clause out rather than saying "up since .".
  it.each([
    ['reboot', 'Restart host — done. The host restarted and holzkube-manager is back.'],
    ['poweroff', 'Shut down host — the host was switched on again and holzkube-manager is back.'],
  ] as const)('%s back with the boot time unknown: no "up since" at all', (action, sentence) => {
    const host = later({})
    host.device.uptime_seconds = { readable: false, reason: { code: 'read-failed', message: 'x' } }
    status(host, order(action), 'back')
    const box = screen.getByRole('status')
    expect(box.querySelector('p')?.textContent).toBe(sentence)
    expect(box).not.toHaveTextContent('up since')
  })

  it('Dismiss status hands the dismissal on', async () => {
    const onDismiss = vi.fn()
    status(later({}), order('reboot'), 'rejected', { onDismiss })
    await userEvent.click(screen.getByRole('button', { name: 'Dismiss status' }))
    expect(onDismiss).toHaveBeenCalledOnce()
  })

  it('takes focus only when told to: an order this page just placed', () => {
    const { unmount } = status(later({}), order('reboot'), 'placed', { takeFocus: false })
    expect(screen.getByRole('status')).not.toHaveFocus()
    unmount()
    status(later({}), order('reboot'), 'placed', { takeFocus: true })
    expect(screen.getByRole('status')).toHaveFocus()
  })
})
