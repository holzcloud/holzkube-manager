import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { api, type Me, type User } from '@/api'
import { SESSION_QUERY_KEY } from '@/hooks/useSession'
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
    linked_provider: '',
    self: false,
    ...over,
  }
}

function wrap(
  users: User[],
  onChanged: () => void = vi.fn(),
  options: { me?: Me; onOwnSessionEnded?: () => void } = {},
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  if (options.me !== undefined) {
    client.setQueryData(SESSION_QUERY_KEY, options.me)
  }
  return render(
    <QueryClientProvider client={client}>
      <AccountTable
        users={users}
        onChanged={onChanged}
        onOwnSessionEnded={options.onOwnSessionEnded ?? vi.fn()}
      />
    </QueryClientProvider>,
  )
}

function me(sso: boolean): Me {
  return { id: 'u1', username: 'somebody', role: 'admin', dry_run: false, sso }
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
    wrap([
      account({ username: 'ci-bot', kind: 'service', token_issued_at: '2026-09-12T00:00:00Z' }),
    ])

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
    // The Sign-in text, specifically: the linked row also holds the unlink
    // button, whose label says single sign-on too.
    expect(within(linked).getByText(/password and single sign-on/i)).toBeInTheDocument()
    expect(within(local).queryByText(/single sign-on/i)).toBeNull()
  })

  it('names the provider and unlinks through a confirmation', async () => {
    const user = userEvent.setup()
    const onChanged = vi.fn()
    const unlink = vi
      .spyOn(api.users, 'unlinkIdentity')
      .mockResolvedValue(account({ id: 'u2', username: 'linked' }))
    wrap(
      [
        account({
          id: 'u2',
          username: 'linked',
          linked_identity: true,
          linked_provider: 'idp.example.com',
        }),
      ],
      onChanged,
    )

    const row = screen.getByText('linked').closest('tr')
    if (row === null) {
      throw new Error('the linked account has no row')
    }
    expect(within(row).getByText(/via idp\.example\.com/i)).toBeInTheDocument()

    await user.click(within(row).getByRole('button', { name: 'Unlink single sign-on' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/Unlink single sign-on for linked\?/)).toBeInTheDocument()
    expect(unlink).not.toHaveBeenCalled()

    await user.click(within(dialog).getByRole('button', { name: 'Unlink' }))
    await waitFor(() => expect(onChanged).toHaveBeenCalled())
    expect(unlink).toHaveBeenCalledWith('u2')
    unlink.mockRestore()
  })

  /**
   * First-use linking needs exactly one account for a person, and there is no
   * operation that links a chosen one. With two people an unlink cannot be
   * undone by anybody signing in, and that is said before the click.
   */
  it('warns that nothing can be linked again when there are several people', async () => {
    const user = userEvent.setup()
    wrap([
      account({ id: 'u1', username: 'linked', linked_identity: true }),
      account({ id: 'u2', username: 'colleague' }),
    ])

    await user.click(screen.getByRole('button', { name: 'Unlink single sign-on' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/no account here can be linked again/i)).toBeInTheDocument()
    expect(within(dialog).getByText(/2 accounts for people/i)).toBeInTheDocument()
  })

  /**
   * After an unlink, with one person, the next single sign-on from the local
   * network links that account, whoever completes it. That window is said
   * before the click, with what closes it.
   */
  it('says who links the account next, and to link again right away', async () => {
    const user = userEvent.setup()
    wrap([
      account({ id: 'u1', username: 'linked', linked_identity: true }),
      account({ id: 'u2', username: 'ci-bot', kind: 'service' }),
    ])

    await user.click(screen.getByRole('button', { name: 'Unlink single sign-on' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/whoever completes it/i)).toBeInTheDocument()
    expect(within(dialog).getByText(/link it again right after this/i)).toBeInTheDocument()
  })

  /** A service account never signs in through the provider, so it does not count. */
  it('does not count a service account as somebody who blocks linking', async () => {
    const user = userEvent.setup()
    wrap([
      account({ id: 'u1', username: 'linked', linked_identity: true }),
      account({ id: 'u2', username: 'ci-bot', kind: 'service' }),
    ])

    await user.click(screen.getByRole('button', { name: 'Unlink single sign-on' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/from the local network address/i)).toBeInTheDocument()
    expect(within(dialog).queryByText(/no account here can be linked again/i)).toBeNull()
  })

  /**
   * A session that came in through single sign-on ends with the link it came
   * through. When it is the caller's own, the dialog says so before the click
   * and the page leaves for the sign-in page afterwards, instead of reloading a
   * list it can no longer read.
   */
  it('says your own single sign-on session ends, and leaves for the sign-in page', async () => {
    const user = userEvent.setup()
    const onChanged = vi.fn()
    const onOwnSessionEnded = vi.fn()
    const unlink = vi
      .spyOn(api.users, 'unlinkIdentity')
      .mockResolvedValue(account({ id: 'u1', self: true }))
    wrap([account({ id: 'u1', role: 'admin', linked_identity: true, self: true })], onChanged, {
      me: me(true),
      onOwnSessionEnded,
    })

    await user.click(screen.getByRole('button', { name: 'Unlink single sign-on' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/this session ends with the link/i)).toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: 'Unlink' }))
    await waitFor(() => expect(onOwnSessionEnded).toHaveBeenCalled())
    expect(onChanged).not.toHaveBeenCalled()
    unlink.mockRestore()
  })

  it('keeps a password session signed in and says so', async () => {
    const user = userEvent.setup()
    const onChanged = vi.fn()
    const onOwnSessionEnded = vi.fn()
    const unlink = vi
      .spyOn(api.users, 'unlinkIdentity')
      .mockResolvedValue(account({ id: 'u1', self: true }))
    wrap([account({ id: 'u1', role: 'admin', linked_identity: true, self: true })], onChanged, {
      me: me(false),
      onOwnSessionEnded,
    })

    await user.click(screen.getByRole('button', { name: 'Unlink single sign-on' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/you stay signed in/i)).toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: 'Unlink' }))
    await waitFor(() => expect(onChanged).toHaveBeenCalled())
    expect(onOwnSessionEnded).not.toHaveBeenCalled()
    unlink.mockRestore()
  })

  /** A refusal belongs to the attempt that met it, not to the next one. */
  it('does not show the last attempt’s refusal when the dialog opens again', async () => {
    const user = userEvent.setup()
    const unlink = vi
      .spyOn(api.users, 'unlinkIdentity')
      .mockRejectedValue(new Error('The account changed while this was being saved'))
    wrap([account({ id: 'u2', username: 'linked', linked_identity: true })])

    await user.click(screen.getByRole('button', { name: 'Unlink single sign-on' }))
    let dialog = await screen.findByRole('dialog')
    await user.click(within(dialog).getByRole('button', { name: 'Unlink' }))
    await within(dialog).findByText(/changed while this was being saved/i)
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }))

    await user.click(screen.getByRole('button', { name: 'Unlink single sign-on' }))
    dialog = await screen.findByRole('dialog')
    expect(within(dialog).queryByText(/changed while this was being saved/i)).toBeNull()
    unlink.mockRestore()
  })

  /**
   * A service account never signs in through the provider, but an earlier
   * release could link one. The page shows the leftover link and offers to
   * remove it, rather than hiding the one binding nobody wants.
   */
  it('shows a service account’s leftover link and offers to remove it', async () => {
    const user = userEvent.setup()
    wrap([
      account({
        id: 'u2',
        username: 'ci-bot',
        kind: 'service',
        linked_identity: true,
        linked_provider: 'idp.example.com',
      }),
    ])

    const row = screen.getByText('ci-bot').closest('tr')
    if (row === null) {
      throw new Error('the service account has no row')
    }
    expect(
      within(row).getByText(/leftover single sign-on link to idp\.example\.com/i),
    ).toBeInTheDocument()
    await user.click(within(row).getByRole('button', { name: 'Unlink single sign-on' }))
    const dialog = await screen.findByRole('dialog')
    expect(
      within(dialog).getByText(/never signs in through idp\.example\.com/i),
    ).toBeInTheDocument()
  })
})
