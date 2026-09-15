import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  forgetSudoIntent,
  ResumeAfterProvider,
  rememberSudoIntent,
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
