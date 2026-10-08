import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import { JobsPage } from '@/routes/jobs'
import { NodesPage } from '@/routes/nodes'

/**
 * A query can reject with anything. `(error as Error).message` on a value that
 * is not an Error printed nothing after the colon; the shared helper always
 * produces a sentence.
 */

function renderPage(page: React.ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{page}</QueryClientProvider>)
}

describe('list screens that cannot read their list', () => {
  it('says something when the node list rejects with a value that is not an Error', async () => {
    vi.spyOn(api.machines, 'list').mockRejectedValue({ not: 'an error' })
    renderPage(<NodesPage />)
    expect(
      await screen.findByText(/The node list could not be read: Something went wrong\./),
    ).toBeInTheDocument()
  })

  it('says something when the job list rejects with a value that is not an Error', async () => {
    vi.spyOn(api.jobs, 'list').mockRejectedValue({ not: 'an error' })
    renderPage(<JobsPage />)
    expect(
      await screen.findByText(/The job list could not be read: Something went wrong\./),
    ).toBeInTheDocument()
  })
})
