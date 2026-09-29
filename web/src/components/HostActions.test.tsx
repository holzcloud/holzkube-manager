import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type Host, type HostAction, type HostOrder, hostSchema } from '@/api'
import {
  type FollowedOrder,
  HostActions,
  HostOrderStatus,
  orderPhase,
} from '@/components/HostActions'
import { SudoDialog } from '@/components/SudoDialog'
import { SESSION_QUERY_KEY } from '@/hooks/useSession'
import { PROBLEM_BASE_URI } from '@/test/problem-fixtures'
import demo from '../../fixtures/demo.json'

/**
 * The host actions on /host (Phase 13): the four buttons, the one reason they
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
  install_commands: [],
}

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

/** The four header buttons, in document order. */
function headerButtons(): HTMLElement[] {
  return within(screen.getByRole('group', { name: 'Host actions' })).getAllByRole('button')
}

function reasonLine(): HTMLElement | null {
  return document.getElementById('host-actions-reason')
}

/** All four rendered, all four off, and the one line says why. */
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
}

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('HostActions: the four buttons and the one reason they are off', () => {
  it('offers all four to an operator with the helper installed, harmless first, and says nothing', () => {
    actions()

    const buttons = headerButtons()
    expect(buttons.map((b) => b.textContent)).toEqual(LABELS)
    for (const b of buttons) {
      expect(b).toBeEnabled()
    }
    // The machine-wide pair is red, the service pair is not.
    expect(buttons.map((b) => b.classList.contains('text-destructive'))).toEqual([
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

  it('for a reader: all four still shown, all off, and the role named (F21)', () => {
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

  it.each(['placed', 'picked-up'] as const)(
    'with an order %s and not answered: all off',
    (phase) => {
      actions({ order: { action: 'reboot', phase } })
      expectOffBecause(
        'An order is waiting for the helper. The next one can be placed when it is answered.',
      )
    },
  )

  it('once the helper answered an order that is not an update, offers them again', () => {
    actions({ order: { action: 'restart-service', phase: 'rejected' } })
    for (const b of headerButtons()) {
      expect(b).toBeEnabled()
    }
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

describe('HostOrderStatus', () => {
  it('says the order waits for the helper while the file is there', () => {
    const host = hostWith({ order: held, result: noResult })
    wrap(<HostOrderStatus host={host} held={held} />)

    const box = screen.getByRole('status')
    expect(box).toHaveTextContent(
      'Check for updates and install — order placed. Waiting for the helper to pick it up.',
    )
    expect(box).toHaveTextContent(`Order ${ID} · placed`)
  })

  it("says what the helper did once its result names this order's id", () => {
    const host = hostWith({
      order: { ...held, state: 'picked-up' },
      result: {
        readable: true,
        value: { id: ID, action: 'update', outcome: 'started', at: '2026-09-28T10:00:06Z' },
      },
    })
    wrap(<HostOrderStatus host={host} held={held} />)

    expect(screen.getByRole('status')).toHaveTextContent(
      'Check for updates and install — started. If a newer release exists it is installed and ' +
        'holzkube-manager restarts; the outcome appears here and under Update check.',
    )
  })

  it("never takes another order's result for this one", () => {
    const other = {
      readable: true,
      value: {
        id: '0000000000000000',
        action: 'update',
        outcome: 'rejected',
        at: '2026-09-28T10:00:06Z',
      },
    }
    expect(
      orderPhase(held, hostWith({ order: { ...held, state: 'picked-up' }, result: other })),
    ).toBe('picked-up')
    // An answer from before the daemon reported the order: as it was placed.
    expect(orderPhase(held, hostWith({ order: null, result: noResult }))).toBe('placed')
  })
})
