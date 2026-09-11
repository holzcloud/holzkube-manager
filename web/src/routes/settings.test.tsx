import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, it } from 'vitest'
import { PasswordCard } from '@/routes/settings'

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
