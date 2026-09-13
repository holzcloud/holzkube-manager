import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { User } from '@/api'
import { AccountTable } from '@/routes/accounts'

/**
 * The accounts table, and specifically the two things it must not offer.
 *
 * The server refuses both — demoting the last admin, and an admin demoting
 * itself — so nothing here is what keeps them from happening. What this
 * screen is for is saying so before the click rather than after it: an error
 * message that arrives on a button press is a worse teacher than a disabled
 * control with the reason next to it, and the reasons are different in the two
 * cases because the remedies are.
 */

function account(over: Partial<User> = {}): User {
  return {
    id: 'u1',
    username: 'somebody',
    role: 'operator',
    created_at: '2026-09-12T00:00:00Z',
    kind: 'person',
    token_issued_at: '',
    last_used_at: '',
    linked_identity: false,
    self: false,
    ...over,
  }
}

function wrap(users: User[]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <AccountTable users={users} onChanged={vi.fn()} />
    </QueryClientProvider>,
  )
}

describe('the accounts table', () => {
  it('will not let the only admin be removed, and says why', () => {
    wrap([account({ id: 'a1', username: 'only-admin', role: 'admin' }), account()])

    const row = screen.getByText('only-admin').closest('tr')
    if (row === null) {
      throw new Error('the admin has no row')
    }

    const remove = within(row).getByRole('button', { name: 'Remove' })
    expect(remove).toBeDisabled()
    expect(remove.getAttribute('title')).toMatch(/only admin/i)

    // And the role cannot be changed away either, with the reason on screen
    // rather than only in a tooltip.
    expect(within(row).getByRole('combobox', { name: /Role of only-admin/i })).toBeDisabled()
    expect(within(row).getByText(/Promote another account first/i)).toBeInTheDocument()
  })

  it('lets an admin be removed once there is a second one', () => {
    wrap([
      account({ id: 'a1', username: 'first-admin', role: 'admin' }),
      account({ id: 'a2', username: 'second-admin', role: 'admin' }),
    ])

    const row = screen.getByText('first-admin').closest('tr')
    if (row === null) {
      throw new Error('the admin has no row')
    }
    expect(within(row).getByRole('button', { name: 'Remove' })).toBeEnabled()
  })

  /**
   * Two admins, so the last-admin rule is not what is being tested. What is
   * left is the other rule, and it has a different remedy: another admin can
   * do this, and this account cannot.
   */
  it('will not let an admin demote itself, and names the remedy', () => {
    wrap([
      account({ id: 'a1', username: 'me', role: 'admin', self: true }),
      account({ id: 'a2', username: 'them', role: 'admin' }),
    ])

    const row = screen.getByText('me').closest('tr')
    if (row === null) {
      throw new Error('the account has no row')
    }
    expect(within(row).getByRole('combobox', { name: /Role of me/i })).toBeDisabled()
    expect(within(row).getByText(/Another admin can/i)).toBeInTheDocument()

    // Removing itself is a different question and is allowed: somebody leaving
    // should not have to ask a colleague to do it.
    expect(within(row).getByRole('button', { name: 'Remove' })).toBeEnabled()
  })

  it('offers a password reset without asking for the old one', async () => {
    const user = userEvent.setup()
    wrap([account({ username: 'somebody' })])

    await user.click(screen.getByRole('button', { name: /Reset password/i }))

    const field = await screen.findByLabelText(/New password for somebody/i)
    expect(field).toHaveAttribute('type', 'password')

    // Nothing anywhere asks for a current password: the point of a reset is
    // that nobody has it.
    expect(screen.queryByLabelText(/current password/i)).toBeNull()

    const set = screen.getByRole('button', { name: 'Set' })
    expect(set).toBeDisabled()
    await user.type(field, 'a-long-enough-passphrase')
    await waitFor(() => expect(set).toBeEnabled())
  })

  /**
   * A service account has no password, so offering a reset for one is offering
   * to change something that does not exist. What it has instead is a token,
   * and the only thing anybody can do to a token is replace it.
   */
  it('offers a service account a rotation rather than a password reset', () => {
    wrap([account({ username: 'ci-bot', kind: 'service', token_issued_at: '2026-09-12T00:00:00Z' })])

    const row = screen.getByText('ci-bot').closest('tr')
    if (row === null) {
      throw new Error('the service account has no row')
    }

    expect(within(row).getByRole('button', { name: /Rotate token/i })).toBeInTheDocument()
    expect(within(row).queryByRole('button', { name: /Reset password/i })).toBeNull()

    // And a token nothing has used yet says so, rather than showing the zero
    // time as a date in the year 1.
    expect(within(row).getByText(/never used/i)).toBeInTheDocument()
  })

  it('says which accounts sign in through the identity provider', () => {
    wrap([
      account({ id: 'u1', username: 'local-only' }),
      account({ id: 'u2', username: 'linked', linked_identity: true }),
    ])

    const linked = screen.getByText('linked').closest('tr')
    const local = screen.getByText('local-only').closest('tr')
    if (linked === null || local === null) {
      throw new Error('an account has no row')
    }
    expect(within(linked).getByText(/single sign-on/i)).toBeInTheDocument()
    expect(within(local).queryByText(/single sign-on/i)).toBeNull()
  })
})
