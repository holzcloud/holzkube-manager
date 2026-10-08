import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import { SESSION_QUERY_KEY, useSession } from '@/hooks/useSession'
import { ProblemError } from '@/lib/problem'
import { problemType } from '@/test/problem-fixtures'

/**
 * Only a real 401 ends the session. A refetch that fails because the server
 * blinked must not turn an operator who is signed in into one who is not, which
 * the shell would answer with a redirect to /login.
 */

const me = {
  id: 'u1',
  username: 'holz',
  dry_run: false,
  role: 'admin',
  sso: false,
} as Awaited<ReturnType<typeof api.me>>

function wrapper(client: QueryClient) {
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
}

function stubStatus() {
  vi.spyOn(api, 'status').mockResolvedValue({
    setup_required: false,
  } as Awaited<ReturnType<typeof api.status>>)
}

afterEach(() => vi.restoreAllMocks())

describe('useSession', () => {
  it('stays authenticated when a refetch of the identity fails without a 401', async () => {
    stubStatus()
    const meSpy = vi.spyOn(api, 'me').mockResolvedValueOnce(me)
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(() => useSession(), { wrapper: wrapper(client) })
    await waitFor(() => expect(result.current.authenticated).toBe(true))

    meSpy.mockRejectedValueOnce(new TypeError('Failed to fetch'))
    await client.refetchQueries({ queryKey: SESSION_QUERY_KEY })

    await waitFor(() => expect(client.getQueryState(SESSION_QUERY_KEY)?.status).toBe('error'))
    expect(result.current.authenticated).toBe(true)
    expect(result.current.me?.username).toBe('holz')
    expect(result.current.unreachable).toBe(false)
  })

  it('ends the session on a real 401', async () => {
    stubStatus()
    const meSpy = vi.spyOn(api, 'me').mockResolvedValueOnce(me)
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(() => useSession(), { wrapper: wrapper(client) })
    await waitFor(() => expect(result.current.authenticated).toBe(true))

    meSpy.mockRejectedValueOnce(
      new ProblemError({
        type: problemType('auth'),
        title: 'Sign in',
        status: 401,
        code: 'auth.required',
      }),
    )
    await client.refetchQueries({ queryKey: SESSION_QUERY_KEY })

    await waitFor(() => expect(result.current.authenticated).toBe(false))
    expect(result.current.unreachable).toBe(false)
  })

  it('reports an unreachable server, not a logged-out one, when no identity is held', async () => {
    stubStatus()
    vi.spyOn(api, 'me').mockRejectedValue(new TypeError('Failed to fetch'))
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(() => useSession(), { wrapper: wrapper(client) })

    await waitFor(() => expect(result.current.unreachable).toBe(true))
    expect(result.current.authenticated).toBe(false)
  })
})
