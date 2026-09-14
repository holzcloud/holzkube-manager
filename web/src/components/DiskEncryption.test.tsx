import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { DiskEncryption } from '@/components/DiskEncryption'

/**
 * The panel's job is not the checkbox. It is saying what each choice is not
 * worth, on the same screen as the switch, because "the disk is encrypted" is
 * what somebody will remember afterwards.
 */
describe('the disk encryption panel', () => {
  it('says, before anything is ticked, that this is install-time only', () => {
    render(
      <DiskEncryption
        state={false}
        ephemeral={false}
        kind="nodeID"
        secureBoot={false}
        onState={vi.fn()}
        onEphemeral={vi.fn()}
        onKind={vi.fn()}
      />,
    )

    // Not after a choice is made: an operator who skips this screen because
    // they will "turn it on later" has to learn here that there is no later.
    expect(screen.getByText(/only while it is empty/i)).toBeInTheDocument()
    expect(screen.getByText(/already installed keeps its plaintext/i)).toBeInTheDocument()
  })

  it('says what nodeID does not protect against', () => {
    render(
      <DiskEncryption
        state
        ephemeral={false}
        kind="nodeID"
        secureBoot={false}
        onState={vi.fn()}
        onEphemeral={vi.fn()}
        onKind={vi.fn()}
      />,
    )

    expect(
      screen.getByText(/does not protect against anyone who has the machine/i),
    ).toBeInTheDocument()
  })

  it('warns that TPM without SecureBoot is not what it looks like', async () => {
    const onKind = vi.fn()
    const { rerender } = render(
      <DiskEncryption
        state
        ephemeral={false}
        kind="nodeID"
        secureBoot={false}
        onState={vi.fn()}
        onEphemeral={vi.fn()}
        onKind={onKind}
      />,
    )

    await userEvent.selectOptions(screen.getByLabelText('Key'), 'tpm')
    expect(onKind).toHaveBeenCalledWith('tpm')

    rerender(
      <DiskEncryption
        state
        ephemeral={false}
        kind="tpm"
        secureBoot={false}
        onState={vi.fn()}
        onEphemeral={vi.fn()}
        onKind={onKind}
      />,
    )
    expect(screen.getByText(/TPM sealing needs SecureBoot/i)).toBeInTheDocument()

    // And with SecureBoot it says what the seal is worth instead of warning.
    rerender(
      <DiskEncryption
        state
        ephemeral={false}
        kind="tpm"
        secureBoot
        onState={vi.fn()}
        onEphemeral={vi.fn()}
        onKind={onKind}
      />,
    )
    expect(screen.queryByText(/TPM sealing needs SecureBoot/i)).toBeNull()
    expect(screen.getByText(/refuses to unseal/i)).toBeInTheDocument()
  })

  it('mentions the two key kinds it does not offer rather than hiding them', () => {
    render(
      <DiskEncryption
        state
        ephemeral
        kind="nodeID"
        secureBoot={false}
        onState={vi.fn()}
        onEphemeral={vi.fn()}
        onKind={vi.fn()}
      />,
    )

    // An operator who read Talos's documentation is looking for these two.
    // Saying nothing would read as "this product does not know about them".
    expect(screen.getByText(/passphrase written into the configuration/i)).toBeInTheDocument()
    expect(screen.getByText(/network key server/i)).toBeInTheDocument()
  })
})
