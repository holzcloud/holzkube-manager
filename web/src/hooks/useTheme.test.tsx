import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { stubMatchMedia } from '@/test/setup'
import { THEME_STORAGE_KEY, useTheme } from './useTheme'

/**
 * D-11 as an executed test rather than a claim: dark-first, follow the system
 * when nothing was chosen, and let an explicit choice survive a reload.
 *
 * "Reload" is modelled as unmounting and mounting again with the localStorage
 * content the first mount wrote. That is the property that matters -- the hook
 * must read its answer back out of storage, not out of a module-level cache
 * that a real page load would have thrown away.
 */

function Probe() {
  const { theme, followsSystem, setTheme, toggleTheme } = useTheme()
  return (
    <div>
      <span data-testid="theme">{theme}</span>
      <span data-testid="follows-system">{followsSystem ? 'yes' : 'no'}</span>
      <button type="button" onClick={toggleTheme}>
        Toggle theme
      </button>
      <button type="button" onClick={() => setTheme('light')}>
        Use light
      </button>
    </div>
  )
}

describe('useTheme', () => {
  it('follows a system preference of dark when nothing was chosen', () => {
    stubMatchMedia(true)

    render(<Probe />)

    expect(screen.getByTestId('theme')).toHaveTextContent('dark')
    expect(screen.getByTestId('follows-system')).toHaveTextContent('yes')
  })

  it('follows a system preference of light when nothing was chosen', () => {
    stubMatchMedia(false)

    render(<Probe />)

    expect(screen.getByTestId('theme')).toHaveTextContent('light')
    expect(screen.getByTestId('follows-system')).toHaveTextContent('yes')
  })

  it('is dark when the browser cannot answer the media query at all', () => {
    stubMatchMedia(true)
    // A browser without matchMedia gets the product default, not a white page.
    Reflect.deleteProperty(window, 'matchMedia')

    render(<Probe />)

    expect(screen.getByTestId('theme')).toHaveTextContent('dark')
  })

  it('writes an explicit choice of light to localStorage under a fixed key', async () => {
    stubMatchMedia(true)
    const user = userEvent.setup()

    render(<Probe />)
    expect(screen.getByTestId('theme')).toHaveTextContent('dark')

    await user.click(screen.getByRole('button', { name: 'Toggle theme' }))

    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('light')
    expect(screen.getByTestId('theme')).toHaveTextContent('light')
    expect(screen.getByTestId('follows-system')).toHaveTextContent('no')
  })

  it('puts the dark class on <html> only while the theme is dark', async () => {
    stubMatchMedia(true)
    const user = userEvent.setup()

    render(<Probe />)

    await user.click(screen.getByRole('button', { name: 'Use light' }))
    expect(document.documentElement).not.toHaveClass('dark')
    expect(document.documentElement.style.colorScheme).toBe('light')

    await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
    expect(document.documentElement).toHaveClass('dark')
    expect(document.documentElement.style.colorScheme).toBe('dark')
  })

  it('lets the stored choice win over the system preference on a remount', async () => {
    stubMatchMedia(true)
    const user = userEvent.setup()

    const first = render(<Probe />)
    await user.click(screen.getByRole('button', { name: 'Use light' }))
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe('light')
    first.unmount()

    // The reload: same storage, and a system that still asks for dark.
    stubMatchMedia(true)
    render(<Probe />)

    expect(screen.getByTestId('theme')).toHaveTextContent('light')
    expect(screen.getByTestId('follows-system')).toHaveTextContent('no')
  })

  it('follows the system again once the explicit choice is cleared', async () => {
    stubMatchMedia(true)
    const user = userEvent.setup()

    const first = render(<Probe />)
    await user.click(screen.getByRole('button', { name: 'Use light' }))
    expect(screen.getByTestId('theme')).toHaveTextContent('light')
    first.unmount()

    localStorage.removeItem(THEME_STORAGE_KEY)
    // A fresh mount reads storage rather than a cache, so the choice is gone
    // and the system preference decides again.
    render(<Probe />)

    expect(screen.getByTestId('theme')).toHaveTextContent('dark')
    expect(screen.getByTestId('follows-system')).toHaveTextContent('yes')
  })
})
