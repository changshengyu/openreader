export const MINI_INTERFACE_MAX_WIDTH = 750

export function isCoarsePointerDevice() {
  if (typeof window === 'undefined') return false
  const mediaMatches = typeof window.matchMedia === 'function' && window.matchMedia('(pointer: coarse)').matches
  const touchPoints = Number(window.navigator?.maxTouchPoints || 0) > 0
  return mediaMatches || touchPoints
}

export function isMobileLikeViewport(width = currentViewportWidth()) {
  if (width <= MINI_INTERFACE_MAX_WIDTH) return true
  if (!isCoarsePointerDevice()) return false
  const screenWidth = Number(window.screen?.width || width)
  const screenHeight = Number(window.screen?.height || width)
  const shortSide = Math.min(screenWidth, screenHeight)
  return width <= 1100 && shortSide <= 1100
}

export function shouldUseMiniInterface(pageMode, width = currentViewportWidth()) {
  return pageMode === 'mobile' || isMobileLikeViewport(width)
}

export function currentViewportWidth() {
  if (typeof window === 'undefined') return 1280
  return window.innerWidth || document.documentElement?.clientWidth || 1280
}
