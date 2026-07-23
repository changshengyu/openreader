export async function initializeReaderTheme({
  authenticated = false,
  loadSettings,
  applyTheme,
} = {}) {
  if (authenticated) {
    try {
      await loadSettings?.()
    } catch {
      // Reader settings already expose their own sync error state. Theme
      // selection must still be available while the server is unreachable.
    }
  }
  return applyTheme?.()
}
