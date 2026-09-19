import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, type ClusterResource } from '@/api'
import { ClusterResources } from '@/components/ClusterResources'

/**
 * The objects beside the workloads.
 *
 * Two claims: the ones that are the REASON something is stuck come first, and
 * removing one needs its name typed. The second matters because the rows look
 * alike and deleting is the only thing here that doing again does not undo.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

const healthy: ClusterResource = {
  kind: 'ConfigMap',
  namespace: 'default',
  name: 'website-settings',
  summary: '2 keys',
  detail: 'locale, theme',
  healthy: true,
  created_at: '',
}

const stuck: ClusterResource = {
  kind: 'PersistentVolumeClaim',
  namespace: 'db',
  name: 'data-postgres-0',
  summary: 'Pending, 20Gi requested',
  detail: 'class longhorn',
  healthy: false,
  created_at: '',
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the objects beside the workloads', () => {
  it('puts what is stuck first, because that is why somebody is here', async () => {
    // The names are chosen so that health-first and alphabetical DISAGREE. With
    // the real names they happened to coincide, and this test passed against a
    // sort by name -- which is the ordering it exists to rule out.
    vi.spyOn(api.kubernetes, 'resources').mockResolvedValue([
      { ...healthy, name: 'aaa-config' },
      { ...stuck, name: 'zzz-claim' },
    ])

    wrap(<ClusterResources clusterID="c-1" namespace="" />)
    await screen.findByText(/zzz-claim/)

    // At desk width DataTable renders a table, so the rows are table rows; the
    // first is the header.
    const rows = screen.getAllByRole('row')
    // A list sorted by name buries the finding among forty ConfigMaps.
    expect(rows[1]).toHaveTextContent('zzz-claim')
  })

  it('will not remove anything until the name is typed', async () => {
    vi.spyOn(api.kubernetes, 'resources').mockResolvedValue([healthy])
    const remove = vi.spyOn(api.kubernetes, 'deleteObject').mockResolvedValue(undefined)

    wrap(<ClusterResources clusterID="c-1" namespace="" />)
    await screen.findByText(/website-settings/)
    await userEvent.click(screen.getByRole('button', { name: 'Remove' }))

    expect(screen.getByRole('button', { name: 'Remove it' })).toBeDisabled()

    await userEvent.type(screen.getByLabelText(/type website-settings/i), 'website-setting')
    expect(screen.getByRole('button', { name: 'Remove it' })).toBeDisabled()
    expect(remove).not.toHaveBeenCalled()

    await userEvent.type(screen.getByLabelText(/type website-settings/i), 's')
    expect(screen.getByRole('button', { name: 'Remove it' })).toBeEnabled()
  })

  it('removes exactly the object whose name was typed', async () => {
    vi.spyOn(api.kubernetes, 'resources').mockResolvedValue([healthy])
    const remove = vi.spyOn(api.kubernetes, 'deleteObject').mockResolvedValue(undefined)

    wrap(<ClusterResources clusterID="c-1" namespace="" />)
    await screen.findByText(/website-settings/)
    await userEvent.click(screen.getByRole('button', { name: 'Remove' }))
    await userEvent.type(screen.getByLabelText(/type website-settings/i), 'website-settings')
    await userEvent.click(screen.getByRole('button', { name: 'Remove it' }))

    await waitFor(() =>
      expect(remove).toHaveBeenCalledWith('c-1', {
        api_version: 'v1',
        kind: 'ConfigMap',
        namespace: 'default',
        name: 'website-settings',
      }),
    )
  })
})
