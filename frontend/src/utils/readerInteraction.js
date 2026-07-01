export const MOBILE_READER_TAP_MOVE_TOLERANCE = 14

export function readerTouchDistance(move = {}) {
  return Math.hypot(Number(move.x || 0), Number(move.y || 0))
}

export function didReaderTouchMove(move, tolerance = MOBILE_READER_TAP_MOVE_TOLERANCE) {
  return readerTouchDistance(move) > tolerance
}

export function isReaderTouchTap({
  move,
  elapsed,
  hasTouch,
  tolerance = MOBILE_READER_TAP_MOVE_TOLERANCE,
  maxDuration = 650,
}) {
  return readerTouchDistance(move) <= tolerance
    && Number(elapsed || 0) < maxDuration
    && Boolean(hasTouch)
}

export function shouldPreventReaderTouchMove({ mode, moveX, moveY }) {
  return mode === 'flip'
    && Math.abs(Number(moveX || 0)) > 12
    && Math.abs(Number(moveX || 0)) > Math.abs(Number(moveY || 0)) + 8
}

export function shouldHandleReaderHorizontalSwipe({ mode, move }) {
  if (mode !== 'flip') return false
  const moveX = Number(move?.x || 0)
  const moveY = Number(move?.y || 0)
  return Math.abs(moveX) >= 42 && Math.abs(moveX) > Math.abs(moveY) * 1.2
}

export function readerTapPointAction({
  mobile,
  pointX,
  pointY,
  viewportWidth,
  viewportHeight,
  clickMethod,
  mode,
  autoReading,
}) {
  const midX = viewportWidth / 2
  const midY = viewportHeight / 2
  const inCenter = Math.abs(pointX - midX) <= viewportWidth * 0.2
    && Math.abs(pointY - midY) <= viewportHeight * 0.2

  if (mobile) {
    if (inCenter || autoReading || clickMethod === 'none') return 'toggle-chrome'
  } else if (inCenter || clickMethod === 'none') {
    return null
  }

  if (clickMethod === 'next') return 'next'
  if (mode === 'flip') return pointX > midX ? 'next' : 'previous'
  return pointY > midY ? 'next' : 'previous'
}

export function readerTapZoneAction({
  zone,
  clickMethod,
  mode,
  autoReading,
}) {
  if (zone === 'center' || autoReading || clickMethod === 'none') return 'toggle-chrome'
  if (clickMethod === 'next') return 'next'
  if (mode === 'flip') {
    if (zone === 'left') return 'previous'
    if (zone === 'right') return 'next'
    return null
  }
  if (zone === 'upper') return 'previous'
  if (zone === 'lower') return 'next'
  return null
}

export function normalizedReaderWheelDelta({
  deltaX,
  deltaY,
  deltaMode,
  fontSize,
  lineHeight,
  pageHeight,
}) {
  const rawDelta = Math.abs(deltaX) > Math.abs(deltaY) ? deltaX : deltaY
  if (deltaMode === 1) {
    const renderedLineHeight = Number(fontSize || 18) * Number(lineHeight || 1.8)
    return rawDelta * Math.max(12, renderedLineHeight)
  }
  if (deltaMode === 2) {
    return rawDelta * Number(pageHeight || 800)
  }
  return rawDelta
}
