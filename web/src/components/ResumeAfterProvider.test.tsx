import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
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

afterEach(() => {
  forgetSudoIntent()
  navigate.mockClear()
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
  it('says plainly that the action did not run, and names it', () => {
    rememberSudoIntent('Forgetting this cluster', '/clusters')

    render(<ResumeAfterProvider />)

    // The whole sentence, not the emphasised half: getByText matches the
    // innermost element, and asserting on that would pass over a banner whose
    // surrounding words had been changed to say something else entirely.
    const sentence = screen.getByText(/did not run/i).closest('p')
    expect(sentence?.textContent).toContain('Forgetting this cluster')
    expect(sentence?.textContent).toContain('re-authenticated')
    expect(sentence?.textContent).toContain('not carried across')
  })

  it('shows nothing when no action was interrupted', () => {
    render(<ResumeAfterProvider />)
    expect(screen.queryByText(/did not run/i)).toBeNull()
  })

  it('forgets an intent too old to still be about what the operator is doing', () => {
    rememberSudoIntent('Forgetting this cluster', '/clusters')

    // Eleven minutes: past the ten-minute window. A banner from an hour ago
    // about something long since dealt with another way is noise, and noise is
    // what teaches an operator to dismiss banners without reading them.
    vi.spyOn(Date, 'now').mockReturnValue(Date.now() + 11 * 60 * 1000)
    render(<ResumeAfterProvider />)
    vi.restoreAllMocks()

    expect(screen.queryByText(/did not run/i)).toBeNull()
  })

  it('takes the operator back to the screen the action started on', async () => {
    rememberSudoIntent('Forgetting this cluster', '/clusters')

    render(<ResumeAfterProvider />)
    screen.getByRole('button', { name: /back to where you were/i }).click()

    expect(navigate).toHaveBeenCalledWith({ to: '/clusters' })
  })

  it('does not carry the request across, only what it was', () => {
    // The 428 flow replays the ORIGINAL REQUEST, and the password change is
    // gated the same way -- so carrying a request across a navigation would put
    // a password in sessionStorage. This pins that only the sentence and the
    // path are stored, against a future change that decides to be helpful.
    rememberSudoIntent('Changing the operator password', '/settings')

    const stored = sessionStorage.getItem('holzkube.sudo-intent') ?? ''
    expect(JSON.parse(stored)).toEqual({
      action: 'Changing the operator password',
      path: '/settings',
      at: expect.any(Number),
    })
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

    render(<SudoFailureNotice />)

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

    render(<SudoFailureNotice />)

    const text = screen.getByText(/oidc.invented-later/).textContent ?? ''
    expect(text).toContain('Nothing was confirmed')
  })

  it('takes the code out of the address bar, so a reload does not resurrect it', () => {
    withQuery('?sudo_error=oidc.not-fresh')

    render(<SudoFailureNotice />)

    expect(window.location.search).toBe('')
  })

  it('shows nothing when the operator did not come back from a refusal', () => {
    render(<SudoFailureNotice />)
    expect(screen.queryByText(/identity provider/i)).toBeNull()
  })
})
