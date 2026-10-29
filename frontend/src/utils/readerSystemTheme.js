// This device's prefers-color-scheme, or null when the browser cannot tell.
export function systemPrefersDarkColorScheme() {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return null
  return window.matchMedia('(prefers-color-scheme: dark)').matches === true
}
