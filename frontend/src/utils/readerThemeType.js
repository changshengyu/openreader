export function inferredReaderThemeType(theme) {
  return theme === 'dark' || theme === 'black' ? 'night' : 'day'
}

export function normalizeReaderThemeType(themeType, theme) {
  if (theme !== 'custom') return inferredReaderThemeType(theme)
  return themeType === 'night' ? 'night' : 'day'
}

export function themeTypeForTheme(theme, currentThemeType) {
  return normalizeReaderThemeType(currentThemeType, theme)
}
