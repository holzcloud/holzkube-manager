import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { api, type Problem } from '@/api'
import { SessionExpiryWatcher, SudoDialog } from './SudoDialog'

/**
 * The most expensive behavioural claim this plan makes, proven here instead of
 * being deferred to a browser: a 428 opens the password prompt and the original
 * request is replayed unchanged afterwards, so the operator does not lose what
 * they typed.
 *
 * The client is faked at the only place it touches the outside world -- the
 * global fetch -- which is also what makes "unchanged body and headers"
 * checkable: the test compares the recorded arguments of the first call and the
 * replay call directly, rather than trusting a claim about them.
 *
 * No server runs.
 */

function problemResponse(status: number, code: string, detail: string, headers = {}): Response {
  const body: Problem = {
    type: `https://holzkube.dev/problems/${code.split('.')[0]}`,
    title: 'Re-authentication required',
    status,
    detail,
    instance: '/requests/deadbeef',
    code,
  }
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/problem+json', ...headers },
  })
}

function noContent(): Response {
  return new Response(null, { status: 204 })
}

type FetchCall = [string, RequestInit]

/** The recorded arguments of one fetch call, or a failure if it never happened. */
function callAt(fetchMock: { mock: { calls: unknown[][] } }, index: number): FetchCall {
  const call = fetchMock.mock.calls[index]
  if (call === undefined) {
    throw new Error(`expected a fetch call at index ${index}`)
  }
  return call as FetchCall
}

function pathsCalled(fetchMock: { mock: { calls: unknown[][] } }): string[] {
  return fetchMock.mock.calls.map((call) => String(call[0]))
}

/** Records every (input, init) pair the pipeline hands to fetch. */
function recordingFetch(...responses: Array<() => Response>) {
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

describe('SudoDialog', () => {
  it('opens on a 428, names the action, and replays the original request unchanged', async () => {
    const fetchMock = recordingFetch(
      () => problemResponse(428, 'sudo.required', 'This action is destructive.'),
      () => noContent(), // POST /api/v1/auth/sudo
      () => noContent(), // the replay
    )
    const user = userEvent.setup()

    render(<SudoDialog />)

    const pending = api.changePassword('old-password-1', 'new-password-1')

    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('Change the operator password')
    expect(dialog).toHaveTextContent('asks for your password again')

    await user.type(screen.getByLabelText('Password'), 'old-password-1')
    await user.click(screen.getByRole('button', { name: 'Confirm' }))

    await expect(pending).resolves.toBeUndefined()

    expect(fetchMock).toHaveBeenCalledTimes(3)

    const first = callAt(fetchMock, 0)
    const sudo = callAt(fetchMock, 1)
    const replay = callAt(fetchMock, 2)

    // Exactly one call to the sudo endpoint, and it is the middle one.
    expect(sudo[0]).toBe('/api/v1/auth/sudo')
    expect(pathsCalled(fetchMock).filter((path) => path === '/api/v1/auth/sudo')).toHaveLength(1)

    // The replay is the original request: same path, same body, same headers.
    expect(replay[0]).toBe(first[0])
    expect(replay[0]).toBe('/api/v1/account/password')
    expect(replay[1].body).toBe(first[1].body)
    expect(replay[1].body).toBe(
      JSON.stringify({ current_password: 'old-password-1', new_password: 'new-password-1' }),
    )
    expect(replay[1].headers).toEqual(first[1].headers)
    expect(replay[1].credentials).toBe('same-origin')

    // And the CSRF preconditions survived the replay, by value.
    const headers = replay[1].headers as Record<string, string>
    expect(headers['X-Holzkube-CSRF']).toBe('1')
    expect(headers['Content-Type']).toBe('application/json')

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })

  it('does not replay anything when the operator cancels', async () => {
    const fetchMock = recordingFetch(() =>
      problemResponse(428, 'sudo.required', 'This action is destructive.'),
    )
    const user = userEvent.setup()

    render(<SudoDialog />)

    // The rejection handler is attached immediately: the promise settles the
    // moment Cancel is clicked, and an unobserved rejection in between would be
    // reported as an unhandled error rather than as the assertion below.
    const pending = api.changePassword('old-password-1', 'new-password-1')
    const settled = pending.catch((error: unknown) => error)

    await screen.findByRole('dialog')
    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    // The caller learns the action did not happen, and learns why: the original
    // 428 comes back so the calling form can stay exactly as it was.
    await expect(settled).resolves.toMatchObject({ code: 'sudo.required' })

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(pathsCalled(fetchMock).filter((path) => path === '/api/v1/auth/sudo')).toHaveLength(0)
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })

  it('stays open and explains itself when the re-authentication password is wrong', async () => {
    const fetchMock = recordingFetch(
      () => problemResponse(428, 'sudo.required', 'This action is destructive.'),
      () => problemResponse(401, 'auth.unauthenticated', 'Username or password is not correct.'),
    )
    const user = userEvent.setup()

    render(<SudoDialog />)

    const pending = api.changePassword('old-password-1', 'new-password-1')
    void pending.catch(() => undefined)

    await screen.findByRole('dialog')
    await user.type(screen.getByLabelText('Password'), 'the-wrong-one')
    await user.click(screen.getByRole('button', { name: 'Confirm' }))

    expect(await screen.findByText('Username or password is not correct.')).toBeInTheDocument()
    // Still open: the pending action has not been thrown away.
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('shows the remaining wait, and never a lockout, when re-authentication is rate limited', async () => {
    recordingFetch(
      () => problemResponse(428, 'sudo.required', 'This action is destructive.'),
      () => problemResponse(429, 'ratelimit.delayed', 'Too many attempts.', { 'Retry-After': '8' }),
    )
    const user = userEvent.setup()

    render(<SudoDialog />)
    const pending = api.changePassword('old-password-1', 'new-password-1')
    void pending.catch(() => undefined)

    await screen.findByRole('dialog')
    await user.type(screen.getByLabelText('Password'), 'old-password-1')
    await user.click(screen.getByRole('button', { name: 'Confirm' }))

    const message = await screen.findByText(/Try again in 8 seconds/)
    expect(message).toBeInTheDocument()
    expect(message.textContent).not.toMatch(/lock|locked|disabled|suspend/i)

    // The action is still pending behind the dialog; abandon it so the test
    // does not leave a promise nobody will ever settle.
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
  })

  it('does not prompt for a 428 when nothing is mounted to answer it', async () => {
    const fetchMock = recordingFetch(() =>
      problemResponse(428, 'sudo.required', 'This action is destructive.'),
    )

    await expect(api.changePassword('old-password-1', 'new-password-1')).rejects.toMatchObject({
      code: 'sudo.required',
    })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})

describe('SessionExpiryWatcher', () => {
  it('turns a 401 during ordinary work into a login transition with an explanation', async () => {
    recordingFetch(() =>
      problemResponse(401, 'auth.unauthenticated', 'Your session is no longer valid.'),
    )
    const onExpired = vi.fn()

    render(<SessionExpiryWatcher onExpired={onExpired} />)

    await expect(api.audit()).rejects.toMatchObject({ code: 'auth.unauthenticated' })

    await waitFor(() => expect(onExpired).toHaveBeenCalledTimes(1))
    const problem = onExpired.mock.calls[0]?.[0] as Problem
    expect(problem.code).toBe('auth.unauthenticated')
    // An explanation, not a blank screen and not a bare status code.
    expect(problem.detail).toBe('Your session is no longer valid.')
  })

  it('leaves the session probe alone: a 401 from /auth/me is an answer, not an expiry', async () => {
    recordingFetch(() => problemResponse(401, 'auth.unauthenticated', 'No session.'))
    const onExpired = vi.fn()

    render(<SessionExpiryWatcher onExpired={onExpired} />)

    await expect(api.me()).rejects.toMatchObject({ code: 'auth.unauthenticated' })
    expect(onExpired).not.toHaveBeenCalled()
  })

  it('stops listening once unmounted', async () => {
    recordingFetch(() =>
      problemResponse(401, 'auth.unauthenticated', 'Your session is no longer valid.'),
    )
    const onExpired = vi.fn()

    const view = render(<SessionExpiryWatcher onExpired={onExpired} />)
    view.unmount()

    await expect(api.audit()).rejects.toMatchObject({ code: 'auth.unauthenticated' })
    expect(onExpired).not.toHaveBeenCalled()
  })
})
