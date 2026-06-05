export const MINI_INTERFACE_MAX_WIDTH = 750

export function isMobileLikeViewport(width = currentViewportWidth()) {
  return width <= MINI_INTERFACE_MAX_WIDTH
}

export function shouldUseMiniInterface(pageMode, width = currentViewportWidth()) {
  return pageMode === 'mobile' || isMobileLikeViewport(width)
}

export function currentViewportWidth() {
  if (typeof window === 'undefined') return 1280
  return window.innerWidth || document.documentElement?.clientWidth || 1280
}
