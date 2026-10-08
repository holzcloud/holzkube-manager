import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, render, waitFor } from '@testing-library/react'
import { StrictMode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, onSessionExpired, onSudoRequired, type SudoChallenge } from '@/api'
import { PROBLEM_BASE_URI } from '@/test/problem-fixtures'
import { SudoDialog } from './SudoDialog'

vi.mock('@/api', async (original) => {
  const real = await original<typeof import('@/api')>()
  return { ...real, onSudoRequired: vi.fn(real.onSudoRequired) }
})

afterEach(() => vi.unstubAllGlobals())

function challenge(settle: (granted: boolean) => void): SudoChallenge {
  return { action: 'Do a thing', settle } as SudoChallenge
}

describe('SudoDialog settling', () => {
  it('settles a displaced challenge exactly once, even when React runs updaters twice', async () => {
    render(
      <StrictMode>
        <QueryClientProvider client={new QueryClient()}>
          <SudoDialog />
        </QueryClientProvider>
      </StrictMode>,
    )
    const calls = vi.mocked(onSudoRequired).mock.calls
    const handler = calls[calls.length - 1]?.[0]
    expect(handler).toBeTypeOf('function')

    const first = vi.fn()
    const second = vi.fn()
    act(() => handler?.(challenge(first)))
    act(() => handler?.(challenge(second)))

    expect(first).toHaveBeenCalledTimes(1)
    expect(first).toHaveBeenCalledWith(false)
    expect(second).not.toHaveBeenCalled()
  })
})

describe('session expiry', () => {
  it('runs the handler once when several requests are answered 401 together', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          new Response(
            JSON.stringify({
              type: `${PROBLEM_BASE_URI}auth`,
              title: 'Sign in',
              status: 401,
              code: 'auth.unauthenticated',
            }),
            { status: 401, headers: { 'Content-Type': 'application/problem+json' } },
          ),
        ),
      ),
    )
    const handler = vi.fn()
    onSessionExpired(handler)

    await Promise.allSettled([api.audit(), api.audit(), api.audit()])

    await waitFor(() => expect(handler).toHaveBeenCalledTimes(1))
    onSessionExpired(null)
  })
})
