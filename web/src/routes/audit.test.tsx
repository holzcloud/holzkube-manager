import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AuditPage, AuditRecord } from '@/api'
import { AuditView } from './audit'

/**
 * D-13 as executed tests: the table, the two filters, the detail dialog and the
 * cursor. The client is faked at the global fetch; no server runs.
 *
 * The paging cases carry the weight. `next_cursor` is number-or-null by
 * contract and `0` is never sent, so a truthiness check would be right only by
 * the accident of that choice -- one of the cases below feeds a `0` through
 * precisely to prove the code branches on `!== null` instead.
 */

vi.mock('@/routes/__root', () => ({
  authenticatedRoute: { addChildren: () => undefined },
}))

function record(seq: number, overrides: Partial<AuditRecord> = {}): AuditRecord {
  return {
    seq,
    ts: `2026-08-28T10:0${seq}:00Z`,
    actor: 'holz',
    session: 'abcdef0123456789abcdef',
    src_ip: '127.0.0.1',
    cluster_id: '',
    machine_id: '',
    action: 'auth.login',
    params: { request_id: `req-${seq}`, password: '<redacted>', username: 'holz' },
    job_id: '',
    outcome: 'success',
    prev_hash: `prev-hash-${seq}`,
    hash: `hash-${seq}`,
    ...overrides,
  }
}

/** Records every requested URL and answers with the queued pages in order. */
function stubAudit(...pages: AuditPage[]) {
  let call = 0
  const fetchMock = vi.fn(async () => {
    const body = pages[call] ?? pages[pages.length - 1]
    call += 1
    return new Response(JSON.stringify(body), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function urlsRequested(fetchMock: { mock: { calls: unknown[][] } }): string[] {
  return fetchMock.mock.calls.map((call) => String(call[0]))
}

function renderAudit() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return render(
    <QueryClientProvider client={client}>
      <AuditView />
    </QueryClientProvider>,
  )
}

describe('AuditView', () => {
  beforeEach(() => {
    vi.stubGlobal('scrollTo', vi.fn())
  })

  it('renders the records in the order the server delivered them, newest first', async () => {
    stubAudit({ items: [record(9), record(8), record(7)], next_cursor: null })

    renderAudit()

    const rows = await screen.findAllByRole('button', { name: /^Record \d+:/ })
    expect(rows.map((row) => row.getAttribute('aria-label'))).toEqual([
      'Record 9: auth.login',
      'Record 8: auth.login',
      'Record 7: auth.login',
    ])
  })

  it('sends exactly the contract parameters when a date range and an action are set', async () => {
    const fetchMock = stubAudit({ items: [], next_cursor: null })
    const user = userEvent.setup()

    renderAudit()
    await waitFor(() => expect(fetchMock).toHaveBeenCalled())

    await user.type(screen.getByLabelText('From'), '2026-08-01T00:00')
    await user.type(screen.getByLabelText('To'), '2026-08-28T23:59')
    await user.click(screen.getByRole('combobox', { name: 'Action' }))
    await user.click(await screen.findByRole('option', { name: 'auth.login' }))
    await user.click(screen.getByRole('button', { name: 'Apply filters' }))

    await waitFor(() => expect(fetchMock.mock.calls.length).toBeGreaterThan(1))

    const url = new URL(urlsRequested(fetchMock).at(-1) as string, 'https://127.0.0.1:8443')
    expect(url.pathname).toBe('/api/v1/audit')
    expect([...url.searchParams.keys()].sort()).toEqual(['action', 'from', 'to'])
    expect(url.searchParams.get('action')).toBe('auth.login')
    expect(url.searchParams.get('from')).toBe(new Date('2026-08-01T00:00').toISOString())
    expect(url.searchParams.get('to')).toBe(new Date('2026-08-28T23:59').toISOString())
  })

  it('explains an empty result instead of showing an empty table', async () => {
    stubAudit({ items: [], next_cursor: null })

    renderAudit()

    expect(await screen.findByText(/No audit records match these filters/)).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
  })

  it('opens a detail dialog showing seq, the hashes and the redacted parameters', async () => {
    stubAudit({ items: [record(4)], next_cursor: null })
    const user = userEvent.setup()

    renderAudit()
    await user.click(await screen.findByRole('button', { name: 'Record 4: auth.login' }))

    const dialog = await screen.findByRole('dialog')
    const details = within(dialog)

    expect(dialog).toHaveTextContent('Record 4')
    expect(details.getByText('prev-hash-4')).toBeInTheDocument()
    expect(details.getByText('hash-4')).toBeInTheDocument()
    expect(details.getByText('req-4')).toBeInTheDocument()
    expect(details.getByText('127.0.0.1')).toBeInTheDocument()
    // Truncated, not the full token.
    expect(details.getByText('abcdef012345…')).toBeInTheDocument()

    // A redacted value reads as redacted, not as an empty field.
    expect(details.getByText(/redacted — recorded as sent/)).toBeInTheDocument()
    expect(details.getByText('"holz"')).toBeInTheDocument()
  })

  it('marks an intent that never got an outcome', async () => {
    stubAudit({
      items: [record(6, { action: 'account.password', outcome: 'attempt' }), record(5)],
      next_cursor: null,
    })

    renderAudit()

    const row = await screen.findByRole('button', { name: 'Record 6: account.password' })
    expect(within(row).getByText('attempt — no outcome')).toBeInTheDocument()
  })

  it('offers paging for a numeric next_cursor and calls back with exactly that cursor', async () => {
    const fetchMock = stubAudit(
      { items: [record(9), record(8)], next_cursor: 8 },
      { items: [record(7)], next_cursor: null },
    )
    const user = userEvent.setup()

    renderAudit()

    await user.click(await screen.findByRole('button', { name: 'Load older records' }))

    await waitFor(() => expect(fetchMock.mock.calls.length).toBe(2))
    const url = new URL(urlsRequested(fetchMock)[1] as string, 'https://127.0.0.1:8443')
    expect(url.searchParams.get('cursor')).toBe('8')

    // The second page is appended, not swapped in.
    expect(await screen.findByRole('button', { name: 'Record 7: auth.login' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Record 9: auth.login' })).toBeInTheDocument()

    // And it is exhausted now.
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Load older records' })).not.toBeInTheDocument(),
    )
  })

  it('offers no paging control when next_cursor is null', async () => {
    stubAudit({ items: [record(9)], next_cursor: null })

    renderAudit()

    await screen.findByRole('button', { name: 'Record 9: auth.login' })
    expect(screen.queryByRole('button', { name: 'Load older records' })).not.toBeInTheDocument()
  })

  it('would offer paging for a cursor of 0, proving the check is !== null and not truthiness', async () => {
    // The server never sends 0 and the contract says so. This case exists as a
    // regression guard: a falsy check would read 0 as exhaustion and would be
    // correct only by the accident of which sentinel was chosen.
    stubAudit({ items: [record(9)], next_cursor: 0 })

    renderAudit()

    await screen.findByRole('button', { name: 'Record 9: auth.login' })
    expect(await screen.findByRole('button', { name: 'Load older records' })).toBeInTheDocument()
  })

  it('starts a filtered query from the beginning rather than from the old cursor', async () => {
    const fetchMock = stubAudit(
      { items: [record(9)], next_cursor: 9 },
      { items: [record(8)], next_cursor: null },
      { items: [record(3)], next_cursor: null },
    )
    const user = userEvent.setup()

    renderAudit()

    await user.click(await screen.findByRole('button', { name: 'Load older records' }))
    await waitFor(() => expect(fetchMock.mock.calls.length).toBe(2))

    await user.click(screen.getByRole('combobox', { name: 'Action' }))
    await user.click(await screen.findByRole('option', { name: 'auth.logout' }))
    await user.click(screen.getByRole('button', { name: 'Apply filters' }))

    await waitFor(() => expect(fetchMock.mock.calls.length).toBe(3))
    const url = new URL(urlsRequested(fetchMock)[2] as string, 'https://127.0.0.1:8443')
    expect(url.searchParams.get('cursor')).toBeNull()
    expect(url.searchParams.get('action')).toBe('auth.logout')
  })
})
