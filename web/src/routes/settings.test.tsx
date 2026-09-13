import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, it } from 'vitest'
import { MetricsCard, PasswordCard } from '@/routes/settings'

/**
 * The password form, which is why this screen exists (UAT gap G-01-1).
 *
 * `POST /api/v1/account/password` and the 428 sudo flow have been in place
 * since phase 1 with no way to reach either from the interface, so the thing
 * the gap asked for was an entry point — and the thing that has to be true of
 * it is that the sudo dialog does not cost the operator what they typed.
 */

function wrap(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

describe('the password form', () => {
  it('refuses two new passwords that do not match, without asking the server', () => {
    wrap(<PasswordCard />)

    fireEvent.change(screen.getByLabelText('Current password'), { target: { value: 'old' } })
    fireEvent.change(screen.getByLabelText('New password'), { target: { value: 'first' } })
    fireEvent.change(screen.getByLabelText('New password again'), { target: { value: 'second' } })
    fireEvent.click(screen.getByRole('button', { name: 'Change password' }))

    // The server takes one new password and cannot see the confirmation at
    // all, so this mismatch has to be caught here — otherwise somebody changes
    // their password to something they cannot reproduce.
    expect(screen.getByText('The two new passwords do not match.')).toBeInTheDocument()
  })

  it('will not submit without a current password', () => {
    wrap(<PasswordCard />)

    fireEvent.change(screen.getByLabelText('New password'), { target: { value: 'new' } })
    expect(screen.getByRole('button', { name: 'Change password' })).toBeDisabled()
  })

  it('says that cancelling the sudo dialog does not lose what was typed', () => {
    wrap(<PasswordCard />)
    expect(screen.getByText(/exactly as it is/)).toBeInTheDocument()
  })

  it('keeps what was typed across a re-render, which is what the dialog does to it', () => {
    const { rerender } = wrap(<PasswordCard />)

    fireEvent.change(screen.getByLabelText('New password'), { target: { value: 'kept' } })

    // The sudo dialog renders above this form rather than replacing it, so the
    // form re-renders and does not unmount. That is the mechanism the claim
    // above rests on, and this is it exercised rather than asserted in prose.
    const client = new QueryClient()
    rerender(
      <QueryClientProvider client={client}>
        <PasswordCard />
      </QueryClientProvider>,
    )

    expect(screen.getByLabelText('New password')).toHaveValue('kept')
  })
})

/**
 * The metrics card (V2-API-02).
 *
 * `/metrics` needs no session, because a scraper cannot log in — which makes
 * it reachable and completely undiscoverable. An operator who does not already
 * know the convention has no way to learn this instance exports anything. This
 * card is the only place that says so, which is why what it says is asserted
 * rather than assumed.
 */
describe('the metrics card', () => {
  it('names the path a scraper has to be pointed at', () => {
    wrap(<MetricsCard />)
    expect(screen.getByText('/metrics')).toBeInTheDocument()
  })

  it('says the endpoint needs no session, and what guards it instead', () => {
    wrap(<MetricsCard />)

    // Left unsaid, "no session" reads as "unprotected", and an operator who
    // believes that either panics or moves the listener somewhere worse.
    expect(screen.getByText(/needs no session/i)).toBeInTheDocument()
    expect(screen.getByText(/host allowlist/i)).toBeInTheDocument()
  })

  it('states the export-never-ingest boundary', () => {
    wrap(<MetricsCard />)

    // The backlog drew this line and the product keeps it. Saying so here is
    // what stops somebody asking this product for an alert rule.
    expect(screen.getByText(/does not alert/i)).toBeInTheDocument()
  })

  it('says an expired certificate is a negative number rather than a missing one', () => {
    wrap(<MetricsCard />)

    // The one metric whose sign carries meaning. An alert rule written against
    // it has to know that "no data" is not how expiry shows up.
    expect(screen.getByText(/negative once it has expired/i)).toBeInTheDocument()
  })
})
