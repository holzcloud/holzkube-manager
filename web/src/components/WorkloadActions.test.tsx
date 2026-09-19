import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import { RestartPodButton } from '@/components/WorkloadActions'

/**
 * Restarting a pod.
 *
 * Scaling and rolling moved to Workloads when the screen stopped being about
 * Deployments alone; their tests moved with them. What stays here is the
 * refusal: "restart" is the word an operator uses and Kubernetes has no such
 * verb.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('restarting a pod', () => {
  it('asks the server, which is what refuses a pod nothing owns', async () => {
    const restart = vi.spyOn(api.kubernetes, 'restartPod').mockResolvedValue(undefined)

    wrap(<RestartPodButton clusterID="c-1" namespace="default" pod="api-7c9" />)
    await userEvent.click(screen.getByRole('button', { name: 'Restart' }))

    expect(restart).toHaveBeenCalledWith('c-1', 'default', 'api-7c9')
  })

  it('shows the refusal rather than swallowing it', async () => {
    vi.spyOn(api.kubernetes, 'restartPod').mockRejectedValue(
      new Error('no controller owns this pod, so deleting it is not a restart'),
    )

    wrap(<RestartPodButton clusterID="c-1" namespace="default" pod="debug" />)
    await userEvent.click(screen.getByRole('button', { name: 'Restart' }))

    await waitFor(() => expect(screen.getByText(/not a restart/i)).toBeInTheDocument())
  })
})
