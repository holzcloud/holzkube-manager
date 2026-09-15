import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import type { Release } from '@/api'
import { ReleaseNotes } from './WhatsNew'

/**
 * The panel behind the version number.
 *
 * What is under test is not layout. It is the two ways this panel can lie: by
 * showing a version other than the one the instance is running, and by opening
 * on a series the operator is not on — which on a host that is behind is the
 * difference between "here is what you have" and "here is what somebody else
 * has".
 */

function release(over: Partial<Release> = {}): Release {
  return {
    version: 'v1.16.0',
    series: 'v1.16',
    date: '2026-09-15',
    changes: [{ icon: '🏷️', text: 'The version is on screen now.' }],
    ...over,
  }
}

const RELEASES: Release[] = [
  release(),
  release({
    version: 'v1.16.0-beta.1',
    date: '2026-09-14',
    changes: [
      { icon: '🗳️', text: 'Removing a node asks etcd first.' },
      { icon: '🔑', text: 'kubeconfig without talosctl.' },
    ],
  }),
  release({
    version: 'v0.1.0',
    series: 'v0.1',
    date: '2026-09-03',
    changes: [{ icon: '🌱', text: 'The first public release.' }],
  }),
]

function show(version: string, releases = RELEASES) {
  return render(
    <ReleaseNotes open onOpenChange={() => undefined} version={version} releases={releases} />,
  )
}

describe('the release notes panel', () => {
  it('says which version this instance is running, and marks it in the list', async () => {
    show('v1.16.0')

    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('This instance is running v1.16.0')
    // The badge for the running release is labelled, so that a reader scanning
    // a list of versions does not have to compare strings themselves.
    expect(dialog).toHaveTextContent('running here')
  })

  it('opens on the series the instance is running, not on the newest one', async () => {
    // The case that matters: a host that has not been updated. Opening on the
    // newest series would show it a list of things it does not have, under a
    // heading that does not say so.
    show('v0.1.0')

    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('The first public release.')
    expect(dialog).not.toHaveTextContent('Removing a node asks etcd first.')

    const tab = screen.getByRole('button', { name: 'v0.1' })
    expect(tab).toHaveAttribute('aria-pressed', 'true')
  })

  it('lets the operator read a series they are not on', async () => {
    const user = userEvent.setup()
    show('v0.1.0')

    await user.click(screen.getByRole('button', { name: 'v1.16' }))

    const dialog = screen.getByRole('dialog')
    expect(dialog).toHaveTextContent('Removing a node asks etcd first.')
    // And the running badge does not follow the selection: it is a fact about
    // the instance, not about what is on screen.
    expect(dialog).not.toHaveTextContent('running here')
  })

  it('counts the changes of each release', async () => {
    const user = userEvent.setup()
    show('v1.16.0')
    await user.click(screen.getByRole('button', { name: 'v1.16' }))

    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByText('1 change')).toBeInTheDocument()
    expect(within(dialog).getByText('2 changes')).toBeInTheDocument()
  })

  /**
   * A build whose version is in no release note — a development build, or a
   * host running something the changelog was never updated for. It must still
   * open, and it must not claim the operator is running the newest release.
   */
  it('opens on something when the running version is in no release note', async () => {
    show('v9.9.9-dev')

    const dialog = await screen.findByRole('dialog')
    expect(dialog).toHaveTextContent('This instance is running v9.9.9-dev')
    expect(dialog).not.toHaveTextContent('running here')
    // Falls back to the newest series rather than to a blank panel.
    expect(dialog).toHaveTextContent('The version is on screen now.')
  })

  it('renders a release that changed nothing without breaking', async () => {
    show('v1.16.0', [release({ changes: [] })])

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('0 changes')).toBeInTheDocument()
  })
})
