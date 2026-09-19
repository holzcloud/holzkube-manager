import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import { DeploymentActions, RestartPodButton } from '@/components/WorkloadActions'

/**
 * Restarting a pod, scaling a deployment, rolling its pods.
 *
 * The two deployment actions look similar and are not, which is what these
 * tests pin: scaling changes how many there are, and a rollout restart replaces
 * the ones there are under the deployment's own strategy. A screen that mixed
 * them up would take a workload down while claiming to restart it.
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

describe('a deployment’s actions', () => {
  it('will not scale to the number it already runs, and says why', async () => {
    const scale = vi.spyOn(api.kubernetes, 'scale').mockResolvedValue(undefined)

    wrap(<DeploymentActions clusterID="c-1" namespace="default" deployment="api" desired={3} />)

    expect(screen.getByRole('button', { name: 'Scale' })).toBeDisabled()
    expect(screen.getByText(/already runs 3/i)).toBeInTheDocument()
    expect(scale).not.toHaveBeenCalled()
  })

  it('scales to what was typed, including zero', async () => {
    const scale = vi.spyOn(api.kubernetes, 'scale').mockResolvedValue(undefined)

    wrap(<DeploymentActions clusterID="c-1" namespace="default" deployment="api" desired={3} />)

    const field = screen.getByLabelText(/replicas for api/i)
    await userEvent.clear(field)
    await userEvent.type(field, '0')
    await userEvent.click(screen.getByRole('button', { name: 'Scale' }))

    // Zero is a real answer: it is how a workload is switched off, and a field
    // that refused it would make switching one off impossible from here.
    expect(scale).toHaveBeenCalledWith('c-1', 'default', 'api', 0)
  })

  it('refuses something that is not a count, with the reason beside the button', async () => {
    wrap(<DeploymentActions clusterID="c-1" namespace="default" deployment="api" desired={3} />)

    const field = screen.getByLabelText(/replicas for api/i)
    await userEvent.clear(field)
    await userEvent.type(field, '-2')

    expect(screen.getByRole('button', { name: 'Scale' })).toBeDisabled()
    expect(screen.getByText(/whole number, zero or more/i)).toBeInTheDocument()
  })

  it('rolls the pods through the deployment rather than deleting them', async () => {
    const roll = vi.spyOn(api.kubernetes, 'rolloutRestart').mockResolvedValue(undefined)
    const restart = vi.spyOn(api.kubernetes, 'restartPod')

    wrap(<DeploymentActions clusterID="c-1" namespace="default" deployment="api" desired={3} />)
    await userEvent.click(screen.getByRole('button', { name: 'Roll pods' }))

    expect(roll).toHaveBeenCalledWith('c-1', 'default', 'api')
    // Not by deleting the pods one at a time, which is the operation that takes
    // the workload down.
    expect(restart).not.toHaveBeenCalled()

    await waitFor(() => expect(screen.getByRole('status')).toBeInTheDocument())
    expect(screen.getByRole('status')).toHaveTextContent(/stays up/i)
  })
})
