import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import { RunCommand } from '@/components/RunCommand'

/**
 * Running one command in a container.
 *
 * The claim: what is typed becomes a LIST of arguments, not a string somebody
 * else splits. That is the one place this could quietly become a shell.
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

describe('running a command', () => {
  it('sends a list of arguments rather than a string', async () => {
    const exec = vi.spyOn(api.kubernetes, 'exec').mockResolvedValue({
      namespace: 'default',
      pod: 'api-1',
      container: 'api',
      command: ['cat', '/etc/hosts'],
      stdout: '127.0.0.1 localhost',
      stderr: '',
      truncated: false,
      identity: 'holz@holzcloud.ch',
    })

    wrap(<RunCommand clusterID="c-1" namespace="default" pod="api-1" container="api" />)
    await userEvent.type(screen.getByLabelText(/program and arguments/i), 'cat /etc/hosts')
    await userEvent.click(screen.getByRole('button', { name: 'Run' }))

    expect(exec).toHaveBeenCalledWith('c-1', 'default', 'api-1', 'api', ['cat', '/etc/hosts'])
    expect(await screen.findByText(/127\.0\.0\.1 localhost/)).toBeInTheDocument()
  })

  it('says who the cluster saw run it', async () => {
    vi.spyOn(api.kubernetes, 'exec').mockResolvedValue({
      namespace: 'default',
      pod: 'api-1',
      container: 'api',
      command: ['id'],
      stdout: 'uid=0',
      stderr: '',
      truncated: false,
      identity: 'holz@holzcloud.ch',
    })

    wrap(<RunCommand clusterID="c-1" namespace="default" pod="api-1" container="api" />)
    await userEvent.type(screen.getByLabelText(/program and arguments/i), 'id')
    await userEvent.click(screen.getByRole('button', { name: 'Run' }))

    // Attributed to the person, not to the product.
    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent(/ran as holz@holzcloud\.ch/i),
    )
  })

  it('shows the server’s refusal of a shell rather than hiding it', async () => {
    vi.spyOn(api.kubernetes, 'exec').mockRejectedValue(
      new Error(
        'sh -c hides the real command inside a string, and this product archives what was run',
      ),
    )

    wrap(<RunCommand clusterID="c-1" namespace="default" pod="api-1" container="api" />)
    await userEvent.type(screen.getByLabelText(/program and arguments/i), 'sh -c whoami')
    await userEvent.click(screen.getByRole('button', { name: 'Run' }))

    await waitFor(() => expect(screen.getByText(/hides the real command/i)).toBeInTheDocument())
  })

  it('says it is not a terminal, so nobody reads it as a broken one', () => {
    wrap(<RunCommand clusterID="c-1" namespace="default" pod="api-1" container="api" />)

    expect(screen.getByText(/not a terminal/i)).toBeInTheDocument()
    expect(screen.getByText(/is refused/i)).toBeInTheDocument()
  })
})
