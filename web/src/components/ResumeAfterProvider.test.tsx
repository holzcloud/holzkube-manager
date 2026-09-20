import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import * as apiModule from '@/api'
import {
  forgetSudoIntent,
  ResumeAfterProvider,
  rememberSudoIntent,
  SudoFailureNotice,
} from '@/components/ResumeAfterProvider'

const navigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
}))

/** The banner lives inside the query client, because running an action has to
 * invalidate what the screens are showing. */
function render_(node: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

afterEach(() => {
  forgetSudoIntent()
  navigate.mockClear()
  vi.restoreAllMocks()
})

/**
 * The trap this closes, walked into by the operator it was built for.
 *
 * They clicked Forget on a cluster, were sent to their identity provider, came
 * back re-authenticated -- and the cluster was still there, on every screen,
 * because the request waiting behind the dialog cannot survive a full page
 * navigation and was settled as refused before leaving. Nothing said so. An
 * operator cannot be left guessing whether a destructive action ran.
 */
describe('ResumeAfterProvider', () => {
  it('says plainly that the action has not run, and names it', () => {
    rememberSudoIntent('Forgetting this cluster', '/clusters')

    render_(<ResumeAfterProvider />)

    // The whole sentence, not the emphasised half: getByText matches the
    // innermost element, and asserting on that would pass over a banner whose
    // surrounding words had been changed to say something else entirely.
    // The action is the dialog's TITLE, not a phrase inside the sentence: the
    // labels are imperative sentences of their own ("Forget this machine"), and
    // inlining one produced "and Forget this machine has not run yet".
    expect(screen.getByRole('heading', { name: 'Forgetting this cluster' })).toBeInTheDocument()
    const sentence = screen.getByText(/has not run yet/i)
    expect(sentence.textContent).toContain('re-authenticated')
    expect(sentence.textContent).toContain('not be carried across')
  })

  it('shows nothing when no action was interrupted', () => {
    render_(<ResumeAfterProvider />)
    expect(screen.queryByText(/has not run yet/i)).toBeNull()
  })

  it('forgets an intent too old to still be about what the operator is doing', () => {
    rememberSudoIntent('Forgetting this cluster', '/clusters')

    // Eleven minutes: past the ten-minute window. A banner from an hour ago
    // about something long since dealt with another way is noise, and noise is
    // what teaches an operator to dismiss banners without reading them.
    vi.spyOn(Date, 'now').mockReturnValue(Date.now() + 11 * 60 * 1000)
    render_(<ResumeAfterProvider />)
    vi.restoreAllMocks()

    expect(screen.queryByText(/has not run yet/i)).toBeNull()
  })

  it('takes the operator back to the screen the action started on', async () => {
    rememberSudoIntent('Forgetting this cluster', '/clusters')

    render_(<ResumeAfterProvider />)
    screen.getByRole('button', { name: /back to where you were/i }).click()

    expect(navigate).toHaveBeenCalledWith({ to: '/clusters' })
  })

  it('never carries a request body across, however helpful that would be', () => {
    // The 428 flow replays the ORIGINAL REQUEST, and the password change is
    // gated the same way -- so carrying a request across a navigation would put
    // a password in sessionStorage. An action with a body therefore gets no
    // replay at all, and the caller in api.ts is what decides that.
    rememberSudoIntent('Changing the operator password', '/settings')

    const stored = sessionStorage.getItem('holzkube.sudo-intent') ?? ''
    expect(JSON.parse(stored)).toEqual({
      action: 'Changing the operator password',
      path: '/settings',
      at: expect.any(Number),
    })
  })

  it('runs the action from the banner when it can be run again', async () => {
    // What the operator reported: they pressed Forget on a machine, went to
    // their provider and back, and were asked to find the button again. The
    // journal shows the window opened and no second attempt was ever made --
    // they read the banner as the deletion having failed.
    const ran = vi.spyOn(apiModule, 'replaySudoAction').mockResolvedValue(undefined)
    rememberSudoIntent('Forgetting this machine', '/nodes/holzkube-01', {
      method: 'DELETE',
      path: '/api/v1/machines/holzkube-01',
    })

    render_(<ResumeAfterProvider />)
    await userEvent.click(screen.getByRole('button', { name: /run it now/i }))

    await waitFor(() =>
      expect(ran).toHaveBeenCalledWith({
        method: 'DELETE',
        path: '/api/v1/machines/holzkube-01',
      }),
    )
    // And the banner goes, because the thing it was about has happened.
    await waitFor(() => expect(screen.queryByText(/has not run yet/i)).toBeNull())
  })

  it('offers no run button for an action it cannot re-issue', () => {
    rememberSudoIntent('Changing the operator password', '/settings')

    render_(<ResumeAfterProvider />)

    expect(screen.queryByRole('button', { name: /run it now/i })).toBeNull()
    // And says what to do instead, rather than leaving a dead end.
    expect(screen.getByText(/Go back and run it again/i)).toBeInTheDocument()
  })

  it('says why it did not run rather than losing the reason', async () => {
    vi.spyOn(apiModule, 'replaySudoAction').mockRejectedValue(
      new Error('the node is still a member of etcd'),
    )
    rememberSudoIntent('Forgetting this machine', '/nodes/holzkube-01', {
      method: 'DELETE',
      path: '/api/v1/machines/holzkube-01',
    })

    render_(<ResumeAfterProvider />)
    await userEvent.click(screen.getByRole('button', { name: /run it now/i }))

    // The banner is the only place this attempt exists: an error that escaped
    // it would vanish, and the operator would be back to guessing.
    expect(await screen.findByText(/still a member of etcd/)).toBeInTheDocument()
  })

  it('refuses a stored replay that is not a request to this API', () => {
    // It comes out of storage the page itself wrote, but a malformed one would
    // build a request from whatever was there.
    sessionStorage.setItem(
      'holzkube.sudo-intent',
      JSON.stringify({
        action: 'Forgetting this machine',
        path: '/nodes',
        replay: { method: 'DELETE', path: 'https://elsewhere.invalid/wipe' },
        at: Date.now(),
      }),
    )

    render_(<ResumeAfterProvider />)

    expect(screen.queryByRole('button', { name: /run it now/i })).toBeNull()
    expect(screen.getByText(/has not run yet/i)).toBeInTheDocument()
  })
})

/**
 * The three refusals, and the screen that used to be a page of JSON.
 *
 * The operator hit oidc.no-auth-time three times on their own installation.
 * Each attempt ended on a problem document the browser rendered as raw JSON in
 * the address bar -- the failSignIn comment names that exact outcome as "where
 * this flow has actually left people standing", and the sudo flow had been
 * deliberately left out of the fix. They could not tell which refusal it was,
 * and the two that matter are repaired from different places: one is a claim
 * the provider does not emit, the other is the provider declining to re-prompt.
 */
describe('SudoFailureNotice', () => {
  function withQuery(search: string) {
    window.history.replaceState({}, '', `/${search}`)
  }

  afterEach(() => {
    window.history.replaceState({}, '', '/')
  })

  it('names the missing claim, and says it cannot succeed by retrying', () => {
    withQuery('?sudo_error=oidc.no-auth-time')

    render_(<SudoFailureNotice />)

    const text = screen.getByText(/auth_time/).textContent ?? ''
    expect(text).toContain('however many times')
    expect(text).toContain('local account')
  })

  it('tells the other two apart', () => {
    withQuery('?sudo_error=oidc.not-fresh')
    const stale = render(<SudoFailureNotice />)
    expect(stale.container.textContent).toContain('did not ask you again')
    stale.unmount()

    withQuery('?sudo_error=oidc.other-identity')
    const other = render(<SudoFailureNotice />)
    expect(other.container.textContent).toContain('different account')
  })

  it('still says something for a code it does not know', () => {
    withQuery('?sudo_error=oidc.invented-later')

    render_(<SudoFailureNotice />)

    const text = screen.getByText(/oidc.invented-later/).textContent ?? ''
    expect(text).toContain('Nothing was confirmed')
  })

  it('takes the code out of the address bar, so a reload does not resurrect it', () => {
    withQuery('?sudo_error=oidc.not-fresh')

    render_(<SudoFailureNotice />)

    expect(window.location.search).toBe('')
  })

  it('shows nothing when the operator did not come back from a refusal', () => {
    render_(<SudoFailureNotice />)
    expect(screen.queryByText(/identity provider/i)).toBeNull()
  })

  it('is a window over the page, not a strip above it', async () => {
    rememberSudoIntent('Forget this machine', '/nodes/holzkube-01', {
      method: 'DELETE',
      path: '/api/v1/machines/holzkube-01',
    })

    render_(<ResumeAfterProvider />)

    // A banner at the top of a long page scrolls away, and the operator read
    // one as "it failed" and stopped. A dialog over a blurred page cannot be
    // scrolled past and says by its shape that something is still to be done.
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
  })
})
