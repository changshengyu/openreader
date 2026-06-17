export const MINI_INTERFACE_MAX_WIDTH = 750

export function isMobileBrowser() {
  if (typeof navigator === 'undefined') return false
  const ua = `${navigator.userAgent || ''} ${navigator.platform || ''}`.toLowerCase()
  if (/(android|iphone|ipod|windows phone|mobile|mmbox|xbrowser)/i.test(ua)) return true
  const platform = navigator.platform || ''
  return platform === 'MacIntel' && Number(navigator.maxTouchPoints || 0) > 1
}

export function isMobileLikeViewport(width = currentViewportWidth()) {
  return width <= MINI_INTERFACE_MAX_WIDTH || isMobileBrowser()
}

export function shouldUseMiniInterface(pageMode, width = currentViewportWidth()) {
  return pageMode === 'mobile' || isMobileLikeViewport(width)
}

export function currentViewportWidth() {
  if (typeof window === 'undefined') return 1280
  return window.innerWidth || document.documentElement?.clientWidth || 1280
}
