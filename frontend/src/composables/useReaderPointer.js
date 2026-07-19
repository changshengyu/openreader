import { unref } from 'vue'
import {
  didReaderTouchMove,
  isReaderTouchTap,
  MOBILE_READER_TAP_MOVE_TOLERANCE,
  readerTapPointAction,
  readerTapZoneAction,
  shouldHandleReaderHorizontalSwipe,
  shouldPreventReaderTouchMove,
} from '../utils/readerInteraction.js'

export function useReaderPointer(options) {
  const windowTarget = options.windowTarget ?? window
  const now = options.now ?? Date.now
  let touchStart = null
  let touchMoved = false
  let touchMove = { x: 0, y: 0 }
  let handledTouchTapAt = 0

  function resetTouch() {
    touchStart = null
    touchMoved = false
    touchMove = { x: 0, y: 0 }
  }

  function applyAction(action, actionOptions = {}) {
    if (!action) return
    if (action === 'toggle-chrome') {
      if (actionOptions.mobileOnly && !unref(options.isMobileReader)) return
      options.toggleChrome()
      return
    }
    if (actionOptions.hideChrome) options.mobileChromeVisible.value = false
    if (action === 'next') options.nextPage()
    if (action === 'previous') options.previousPage()
  }

  function tapPoint(point, mobile, tapOptions = {}) {
    if (unref(options.isOverlayOpen) || !point?.rect) return
    if (
      !tapOptions.selectionChecked
      && options.scheduleSelectedTextOperation(0, { retry: false })
    ) {
      options.suppressContentClick()
      return
    }
    const viewportWidth = windowTarget.innerWidth || point.rect.width
    const viewportHeight = windowTarget.innerHeight || point.rect.height
    const pointX = Number.isFinite(point.clientX) ? point.clientX : point.relX
    const pointY = Number.isFinite(point.clientY) ? point.clientY : point.relY
    if (unref(options.isAudio)) {
      const midX = viewportWidth / 2
      const midY = viewportHeight / 2
      const inCenter = Math.abs(pointX - midX) <= viewportWidth * 0.2
        && Math.abs(pointY - midY) <= viewportHeight * 0.2
      if (inCenter) options.toggleChrome()
      return
    }
    const action = readerTapPointAction({
      mobile,
      pointX,
      pointY,
      viewportWidth,
      viewportHeight,
      clickMethod: options.reader.clickMethod,
      mode: options.reader.mode,
      autoReading: unref(options.autoReading),
    })
    // The upstream read bar only guards the menu-toggle branch. It keeps the
    // normal non-slide page actions available outside the center zone.
    if (action === 'toggle-chrome' && unref(options.ttsBarVisible)) {
      return
    }
    applyAction(action, {
      hideChrome: mobile,
    })
  }

  function handleTapZone(zone) {
    if (unref(options.isOverlayOpen)) return
    if (unref(options.isAudio)) {
      if (zone === 'center') options.toggleChrome()
      return
    }
    const action = readerTapZoneAction({
      zone,
      clickMethod: options.reader.clickMethod,
      mode: options.reader.mode,
      autoReading: unref(options.autoReading),
    })
    if (action === 'toggle-chrome' && unref(options.ttsBarVisible)) {
      return
    }
    applyAction(action, {
      hideChrome: options.reader.clickMethod === 'next',
      mobileOnly: true,
    })
  }

  function handleContentClick(event) {
    const page = unref(options.pageEl)
    if (unref(options.isOverlayOpen) || !page) return
    if (now() - handledTouchTapAt < 450) return
    if (options.consumeSuppressedContentClick()) return
    if (event.defaultPrevented || event.button !== 0) return
    const target = event.target
    if (target?.closest?.('button, a, input, textarea, select, [role="button"]')) return
    const rect = page.getBoundingClientRect()
    tapPoint({
      rect,
      relX: event.clientX - rect.left,
      relY: event.clientY - rect.top,
      clientX: event.clientX,
      clientY: event.clientY,
    }, unref(options.isMobileReader))
  }

  function handleTouchStart(event) {
    if (!unref(options.isMobileReader) || event.touches?.length !== 1) return
    const touch = event.touches[0]
    touchStart = { x: touch.clientX, y: touch.clientY, at: now() }
    touchMoved = false
    touchMove = { x: 0, y: 0 }
  }

  function handleTouchMove(event) {
    if (
      !unref(options.isMobileReader)
      || !touchStart
      || event.touches?.length !== 1
    ) return
    const touch = event.touches[0]
    const moveX = touch.clientX - touchStart.x
    const moveY = touch.clientY - touchStart.y
    touchMove = { x: moveX, y: moveY }
    if (
      !touchMoved
      && didReaderTouchMove(touchMove, MOBILE_READER_TAP_MOVE_TOLERANCE)
    ) {
      touchMoved = true
      options.cancelPageAnimation?.()
    }
    if (shouldPreventReaderTouchMove({
      mode: options.reader.mode,
      moveX,
      moveY,
    })) {
      event.preventDefault()
      event.stopPropagation()
    }
  }

  function handleTouchEnd(event) {
    if (!unref(options.isMobileReader)) return
    const touch = event.changedTouches?.[0]
    const elapsed = touchStart ? now() - touchStart.at : 0
    const retrySelection = elapsed >= 350
    if (options.scheduleSelectedTextOperation(0, { retry: retrySelection })) {
      options.suppressContentClick()
      resetTouch()
      return
    }
    if (retrySelection) {
      options.suppressContentClick()
      resetTouch()
      return
    }
    const isTap = isReaderTouchTap({
      move: touchMove,
      elapsed,
      hasTouch: touch,
      tolerance: MOBILE_READER_TAP_MOVE_TOLERANCE,
    })
    if (touch) options.suppressContentClick(360)
    if (isTap) handledTouchTapAt = now()

    if (
      touchMoved
      && !unref(options.isOverlayOpen)
      && shouldHandleReaderHorizontalSwipe({
        mode: options.reader.mode,
        move: touchMove,
      })
    ) {
      if (touchMove.x > 0) options.previousPage()
      else options.nextPage()
    } else if (
      !touchMoved
      && !unref(options.isOverlayOpen)
      && unref(options.pageEl)
      && touch
    ) {
      const rect = unref(options.pageEl).getBoundingClientRect()
      tapPoint({
        rect,
        relX: touch.clientX - rect.left,
        relY: touch.clientY - rect.top,
        clientX: touch.clientX,
        clientY: touch.clientY,
      }, true, { selectionChecked: true })
    }
    resetTouch()
  }

  function handleTouchCancel() {
    resetTouch()
  }

  return {
    handleContentClick,
    handleTapZone,
    handleTouchCancel,
    handleTouchEnd,
    handleTouchMove,
    handleTouchStart,
    tapPoint,
  }
}
