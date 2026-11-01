import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/api'
import { readSeenVersion, writeSeenVersion } from '@/lib/releaseNotes'
import { AboutCard } from '@/routes/settings'
import { WhatsNewOnUpdate } from './WhatsNew'

const RELEASES = [
  {
    version: 'v0.8.0',
    series: 'v0.8',
    date: '2026-10-15',
    changes: [{ icon: '🧭', text: 'Configuration drift. A node is compared with its baseline.' }],
  },
  {
    version: 'v0.7.1',
    series: 'v0.7',
    date: '2026-10-10',
    changes: [{ icon: '🔧', text: 'A fix.' }],
  },
]

function mount(ui: React.ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>)
}

beforeEach(() => {
  window.localStorage.clear()
  vi.spyOn(api, 'version').mockResolvedValue({ version: 'v0.8.0', releases: RELEASES })
})

afterEach(() => {
  vi.restoreAllMocks()
  window.localStorage.clear()
})

describe('the notes after an update', () => {
  it('open by themselves the first time a browser meets a newer version, once', async () => {
    writeSeenVersion('v0.7.1')
    const first = mount(<WhatsNewOnUpdate />)

    expect(await screen.findByRole('dialog')).toHaveTextContent('Configuration drift')
    expect(readSeenVersion()).toBe('v0.8.0')
    first.unmount()

    // The next visit, same version: nothing opens.
    mount(<WhatsNewOnUpdate />)
    await waitFor(() => expect(api.version).toHaveBeenCalledTimes(2))
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('stay closed on a browser that has never seen a version, and remember it', async () => {
    mount(<WhatsNewOnUpdate />)
    await waitFor(() => expect(readSeenVersion()).toBe('v0.8.0'))
    expect(screen.queryByRole('dialog')).toBeNull()
  })
})

describe('Settings, About', () => {
  it('shows the version and opens the notes on request', async () => {
    mount(<AboutCard />)

    const button = await screen.findByRole('button', { name: /what's new/i })
    expect(screen.getByText('v0.8.0')).toBeInTheDocument()
    button.click()

    expect(await screen.findByRole('dialog')).toHaveTextContent('Configuration drift')
    expect(screen.getAllByRole('tab')).toHaveLength(2)
  })
})
