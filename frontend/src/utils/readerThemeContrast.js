const MIN_READER_TEXT_CONTRAST = 4.5
const SAFE_DARK_TEXT = '#171717'
const SAFE_LIGHT_TEXT = '#f2eee4'

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
} = {}) {
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
