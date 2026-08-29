import { useSyncExternalStore } from 'react'

/**
 * Dark-first theming (D-11).
 *
 * There are exactly two sources of truth, in this order:
 *
 *   1. an explicit choice the operator made, stored under THEME_STORAGE_KEY;
 *   2. otherwise the system preference reported by `prefers-color-scheme`.
 *
 * The resolved value is recomputed from those two on every read rather than
 * cached in a module variable. That is deliberate: a cache would survive a
 * remount, and the case D-11 actually cares about -- reloading the page and
 * getting the stored choice back -- would then be proven by the cache instead
 * of by localStorage.
 *
 * Nothing security-relevant lives here. localStorage holds the theme and, since
 * phase 2, the Images screen's last-used architecture -- preferences, and
 * nothing a session or a credential could be reconstructed from (threat
 * T-01-32). The session and the sudo window live on the server and the UI only
 * reflects them.
 */

export const THEME_STORAGE_KEY = 'holzkube.theme'

export type Theme = 'dark' | 'light'

const DARK_QUERY = '(prefers-color-scheme: dark)'

const listeners = new Set<() => void>()

function isTheme(value: unknown): value is Theme {
  return value === 'dark' || value === 'light'
}

/** The explicit choice, or null when the operator has never made one. */
export function storedTheme(): Theme | null {
  if (typeof localStorage === 'undefined') {
    return null
  }
  try {
    const raw = localStorage.getItem(THEME_STORAGE_KEY)
    return isTheme(raw) ? raw : null
  } catch {
    // A browser with storage disabled still gets a working theme; it just
    // cannot remember the choice across a reload.
    return null
  }
}

/** What the operating system asks for. Dark when it does not say. */
export function systemTheme(): Theme {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return 'dark'
  }
  return window.matchMedia(DARK_QUERY).matches ? 'dark' : 'light'
}

/** The theme in effect: the stored choice if there is one, else the system's. */
export function resolveTheme(): Theme {
  return storedTheme() ?? systemTheme()
}

/** Put the theme on <html>. The same class the inline boot script sets. */
export function applyTheme(theme: Theme): void {
  if (typeof document === 'undefined') {
    return
  }
  document.documentElement.classList.toggle('dark', theme === 'dark')
  document.documentElement.style.colorScheme = theme
}

function notify(): void {
  for (const listener of listeners) {
    listener()
  }
}

function subscribe(onStoreChange: () => void): () => void {
  listeners.add(onStoreChange)

  let media: MediaQueryList | null = null
  if (typeof window !== 'undefined' && typeof window.matchMedia === 'function') {
    media = window.matchMedia(DARK_QUERY)
    media.addEventListener?.('change', onStoreChange)
  }

  return () => {
    listeners.delete(onStoreChange)
    media?.removeEventListener?.('change', onStoreChange)
  }
}

/**
 * Record an explicit choice. Survives a reload, which is the whole point of
 * writing it down.
 */
export function setTheme(theme: Theme): void {
  try {
    localStorage.setItem(THEME_STORAGE_KEY, theme)
  } catch {
    // Storage refused. The choice still applies for this page load.
  }
  applyTheme(theme)
  notify()
}

/** Forget the explicit choice and follow the system again. */
export function clearThemeChoice(): void {
  try {
    localStorage.removeItem(THEME_STORAGE_KEY)
  } catch {
    // See setTheme.
  }
  applyTheme(systemTheme())
  notify()
}

export interface ThemeState {
  theme: Theme
  /** True while the theme is following the system rather than a stored choice. */
  followsSystem: boolean
  setTheme: (theme: Theme) => void
  toggleTheme: () => void
  clearThemeChoice: () => void
}

export function useTheme(): ThemeState {
  const theme = useSyncExternalStore(subscribe, resolveTheme, () => 'dark' as Theme)
  const followsSystem = useSyncExternalStore(
    subscribe,
    () => storedTheme() === null,
    () => true,
  )

  return {
    theme,
    followsSystem,
    setTheme,
    toggleTheme: () => setTheme(theme === 'dark' ? 'light' : 'dark'),
    clearThemeChoice,
  }
}
