import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type KubeContainer } from '@/api'
import { PodDiagnosis } from '@/components/PodDiagnosis'

/**
 * Why a pod is broken.
 *
 * The claim these pin is the one the whole panel exists for: a pod in
 * CrashLoopBackOff has printed nothing in its CURRENT container, so a log view
 * that opened on that one would answer the case somebody came for with an empty
 * box. It has to open on the run that crashed.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const crashing: KubeContainer = {
  name: 'api',
  image: 'ghcr.io/holzcloud/api:2026.09.17',
  init: false,
  ready: false,
  started: false,
  restarts: 14,
  state: 'waiting',
  reason: 'CrashLoopBackOff',
  message: '',
  last_state: 'terminated',
  last_reason: 'OOMKilled',
  last_exit_code: 137,
  last_finished_at: '2026-09-19T12:00:00Z',
  has_previous: true,
  cpu_request: '',
  memory_request: '',
  cpu_limit: '500m',
  memory_limit: '256Mi',
  explanation: 'The previous run was killed for using more memory than its limit allowed.',
}

const noEvents = { events: [], notice: 'A cluster forgets its events after about an hour.' }

afterEach(() => {
  vi.restoreAllMocks()
})

describe('diagnosing a pod', () => {
  it('opens on the log of the run that crashed, not the one starting up', async () => {
    vi.spyOn(api.kubernetes, 'containers').mockResolvedValue([crashing])
    vi.spyOn(api.kubernetes, 'podEvents').mockResolvedValue(noEvents)
    const logs = vi.spyOn(api.kubernetes, 'logs').mockResolvedValue({
      namespace: 'default',
      name: 'api-7c9',
      container: 'api',
      previous: true,
      lines: ['fatal: out of memory'],
      truncated: false,
    })

    wrap(<PodDiagnosis clusterID="c-1" namespace="default" pod="api-7c9" onClose={() => {}} />)

    // previous: true on the FIRST read. Anything else shows an empty box for
    // exactly the pod somebody opened this for.
    await waitFor(() =>
      expect(logs).toHaveBeenCalledWith('c-1', 'default', 'api-7c9', {
        container: 'api',
        previous: true,
      }),
    )
    expect(await screen.findByText(/out of memory/)).toBeInTheDocument()
  })

  it('says what is wrong in a sentence, rather than only a reason code', async () => {
    vi.spyOn(api.kubernetes, 'containers').mockResolvedValue([crashing])
    vi.spyOn(api.kubernetes, 'podEvents').mockResolvedValue(noEvents)
    vi.spyOn(api.kubernetes, 'logs').mockResolvedValue({
      namespace: 'default',
      name: 'api-7c9',
      container: 'api',
      previous: true,
      lines: [],
      truncated: false,
    })

    wrap(<PodDiagnosis clusterID="c-1" namespace="default" pod="api-7c9" onClose={() => {}} />)

    expect(await screen.findByText(/more memory than its limit/i)).toBeInTheDocument()
    // And the numbers behind it: 137 alone is something you have to already
    // know to decode.
    expect(screen.getByText(/exited 137/)).toBeInTheDocument()
  })

  it('says a container asked for nothing, because that is a finding', async () => {
    vi.spyOn(api.kubernetes, 'containers').mockResolvedValue([crashing])
    vi.spyOn(api.kubernetes, 'podEvents').mockResolvedValue(noEvents)
    vi.spyOn(api.kubernetes, 'logs').mockResolvedValue({
      namespace: 'default',
      name: 'api-7c9',
      container: 'api',
      previous: true,
      lines: [],
      truncated: false,
    })

    wrap(<PodDiagnosis clusterID="c-1" namespace="default" pod="api-7c9" onClose={() => {}} />)

    expect(await screen.findByText(/places this blind/i)).toBeInTheDocument()
  })

  it('does not offer the crashed log when there was no previous run', async () => {
    vi.spyOn(api.kubernetes, 'containers').mockResolvedValue([
      {
        ...crashing,
        has_previous: false,
        last_state: '',
        reason: '',
        state: 'running',
        ready: true,
      },
    ])
    vi.spyOn(api.kubernetes, 'podEvents').mockResolvedValue(noEvents)
    const logs = vi.spyOn(api.kubernetes, 'logs').mockResolvedValue({
      namespace: 'default',
      name: 'api-7c9',
      container: 'api',
      previous: false,
      lines: ['serving'],
      truncated: false,
    })

    wrap(<PodDiagnosis clusterID="c-1" namespace="default" pod="api-7c9" onClose={() => {}} />)

    await waitFor(() =>
      expect(logs).toHaveBeenCalledWith('c-1', 'default', 'api-7c9', {
        container: 'api',
        previous: false,
      }),
    )
    // A button that would answer "no such log" is not offered.
    expect(screen.queryByRole('button', { name: /the run that crashed/i })).toBeNull()
  })

  it('says why an empty event list is not a claim that nothing happened', async () => {
    vi.spyOn(api.kubernetes, 'containers').mockResolvedValue([crashing])
    vi.spyOn(api.kubernetes, 'podEvents').mockResolvedValue(noEvents)
    vi.spyOn(api.kubernetes, 'logs').mockResolvedValue({
      namespace: 'default',
      name: 'api-7c9',
      container: 'api',
      previous: true,
      lines: [],
      truncated: false,
    })

    wrap(<PodDiagnosis clusterID="c-1" namespace="default" pod="api-7c9" onClose={() => {}} />)

    expect(await screen.findByText(/forgets its events/i)).toBeInTheDocument()
  })

  it('fetches the whole object only when it is asked for', async () => {
    vi.spyOn(api.kubernetes, 'containers').mockResolvedValue([crashing])
    vi.spyOn(api.kubernetes, 'podEvents').mockResolvedValue(noEvents)
    vi.spyOn(api.kubernetes, 'logs').mockResolvedValue({
      namespace: 'default',
      name: 'api-7c9',
      container: 'api',
      previous: true,
      lines: [],
      truncated: false,
    })
    const object = vi.spyOn(api.kubernetes, 'object').mockResolvedValue({
      api_version: 'v1',
      kind: 'Pod',
      namespace: 'default',
      name: 'api-7c9',
      yaml: 'kind: Pod\n',
      truncated: false,
      notice: 'Left out, because they are bookkeeping rather than configuration: managedFields.',
    })

    wrap(<PodDiagnosis clusterID="c-1" namespace="default" pod="api-7c9" onClose={() => {}} />)
    await screen.findByText(/more memory/i)

    expect(object).not.toHaveBeenCalled()
    await userEvent.click(screen.getByRole('button', { name: /show the whole object/i }))

    await waitFor(() => expect(object).toHaveBeenCalled())
    // The removal is announced, because an object silently missing fields is
    // how somebody concludes a field is not set when it is.
    expect(await screen.findByText(/bookkeeping rather than configuration/i)).toBeInTheDocument()
  })
})
