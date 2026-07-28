const MIN_READER_TEXT_CONTRAST = 4.5
const SAFE_DARK_TEXT = '#171717'
const SAFE_LIGHT_TEXT = '#ffffff'
const DAY_PAGE_BORDER = 'rgba(109, 95, 55, 0.28)'
const DAY_PAGE_SHADOW = 'inset 24px 0 44px rgba(90, 71, 28, 0.05), inset -24px 0 44px rgba(90, 71, 28, 0.05)'

export function readerColorContrast(foreground, background) {
  const foregroundRGB = parseCSSColor(foreground)
  const backgroundRGB = parseCSSColor(background)
  if (!foregroundRGB || !backgroundRGB) return 0
  const foregroundLuminance = relativeLuminance(foregroundRGB)
  const backgroundLuminance = relativeLuminance(backgroundRGB)
  const lighter = Math.max(foregroundLuminance, backgroundLuminance)
  const darker = Math.min(foregroundLuminance, backgroundLuminance)
  return (lighter + 0.05) / (darker + 0.05)
}

export function resolveReaderTextColor({
  requestedColor,
  themeTextColor,
  backgroundColor,
  themeType = 'day',
  hasBackgroundImage = false,
  builtInNight = false,
} = {}) {
  if (builtInNight) return SAFE_LIGHT_TEXT

  if (hasBackgroundImage) {
    return themeType === 'night' ? SAFE_LIGHT_TEXT : SAFE_DARK_TEXT
  }

  const requested = normalizedColor(requestedColor) || normalizedColor(themeTextColor)
  if (
    requested
    && readerColorContrast(requested, backgroundColor) >= MIN_READER_TEXT_CONTRAST
  ) {
    return requested
  }

  if (!parseCSSColor(backgroundColor)) {
    return themeType === 'night' ? SAFE_LIGHT_TEXT : SAFE_DARK_TEXT
  }

  return readerColorContrast(SAFE_LIGHT_TEXT, backgroundColor)
    > readerColorContrast(SAFE_DARK_TEXT, backgroundColor)
    ? SAFE_LIGHT_TEXT
    : SAFE_DARK_TEXT
}

export function readerTextShadow({
  textColor,
  hasBackgroundImage = false,
} = {}) {
  if (!hasBackgroundImage) return 'none'
  return readerColorContrast(textColor, '#000000') >= readerColorContrast(textColor, '#ffffff')
    ? '0 1px 2px rgba(0, 0, 0, 0.92), 0 0 1px rgba(0, 0, 0, 0.96)'
    : '0 1px 2px rgba(255, 255, 255, 0.92), 0 0 1px rgba(255, 255, 255, 0.96)'
}

export function resolveReaderSurface({
  theme,
  themeType = 'day',
  themeBackground,
  themeBody,
  themePopup,
  customBackgroundImage = '',
} = {}) {
  const custom = theme === 'custom'
  const builtInNight = themeType === 'night' && !custom

  if (builtInNight) {
    return {
      bodyColor: '#000000',
      bodyImage: 'none',
      pageColor: '#000000',
      pageImage: 'none',
      popupColor: normalizedColor(themePopup) || '#171717',
      pageBorder: 'transparent',
      pageShadow: 'none',
    }
  }

  const customImage = custom ? normalizedColor(customBackgroundImage) : ''
  return {
    bodyColor: normalizedColor(themeBody) || '#d9c27f',
    bodyImage: custom ? 'none' : 'var(--reader-body-texture)',
    pageColor: normalizedColor(themeBackground) || '#f4e9bd',
    pageImage: customImage ? `url(${JSON.stringify(customImage)})` : custom ? 'none' : 'var(--paper-texture)',
    popupColor: normalizedColor(themePopup) || 'rgba(255, 252, 239, 0.94)',
    pageBorder: custom ? 'transparent' : DAY_PAGE_BORDER,
    pageShadow: custom ? 'none' : DAY_PAGE_SHADOW,
  }
}

function normalizedColor(value) {
  return typeof value === 'string' ? value.trim() : ''
}

function parseCSSColor(value) {
  const color = normalizedColor(value).toLowerCase()
  if (!color) return null

  const hex = color.match(/^#([\da-f]{3,8})$/i)?.[1]
  if (hex) {
    if (hex.length === 3 || hex.length === 4) {
      return [
        Number.parseInt(hex[0] + hex[0], 16),
        Number.parseInt(hex[1] + hex[1], 16),
        Number.parseInt(hex[2] + hex[2], 16),
      ]
    }
    if (hex.length === 6 || hex.length === 8) {
      return [
        Number.parseInt(hex.slice(0, 2), 16),
        Number.parseInt(hex.slice(2, 4), 16),
        Number.parseInt(hex.slice(4, 6), 16),
      ]
    }
    return null
  }

  const rgb = color.match(/^rgba?\(\s*([+-]?(?:\d+\.?\d*|\.\d+)%?)\s*[, ]\s*([+-]?(?:\d+\.?\d*|\.\d+)%?)\s*[, ]\s*([+-]?(?:\d+\.?\d*|\.\d+)%?)(?:\s*[,/]\s*[+-]?(?:\d+\.?\d*|\.\d+)%?)?\s*\)$/)
  if (!rgb) return null
  const channels = rgb.slice(1, 4).map(channel => {
    if (channel.endsWith('%')) {
      return Math.round(clamp(Number.parseFloat(channel), 0, 100) * 2.55)
    }
    return Math.round(clamp(Number.parseFloat(channel), 0, 255))
  })
  return channels.every(Number.isFinite) ? channels : null
}

function relativeLuminance(channels) {
  const [red, green, blue] = channels.map(channel => {
    const normalized = channel / 255
    return normalized <= 0.04045
      ? normalized / 12.92
      : ((normalized + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * red + 0.7152 * green + 0.0722 * blue
}

function clamp(value, min, max) {
  return Math.max(min, Math.min(max, value))
}
