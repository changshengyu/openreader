import { onBeforeUnmount, unref } from 'vue'
import {
  normalizeReaderSelectionText,
  readerSelectionBelongsToRoot,
} from '../utils/readerSelection'

export function useReaderSelection(options) {
  let operateTimer = null
  let releaseTimer = null
  let operating = false
  let suppressClick = false

  function selectedText() {
    if (typeof window === 'undefined') return ''
    const root = unref(options.contentBody)
    const selection = window.getSelection?.()
    const text = normalizeReaderSelectionText(selection?.toString?.())
    if (!root || !text || !selection?.rangeCount) return ''

    const range = selection.getRangeAt(0)
    const common = range.commonAncestorContainer
    const container = common?.nodeType === window.Node?.ELEMENT_NODE
      ? common
      : common?.parentElement
    return readerSelectionBelongsToRoot(root, container) ? text : ''
  }

  function schedule(delay = 0) {
    if (options.getAction?.() === '忽略') return false
    clearTimeout(operateTimer)
    const selectedNow = selectedText()
    operateTimer = setTimeout(async () => {
      operateTimer = null
      const text = selectedText()
      if (!text || operating || options.getAction?.() === '忽略') return
      operating = true
      suppressContentClick()
      try {
        await options.onOperate?.(text)
      } catch (error) {
        if (error !== 'cancel' && error !== 'close') options.onError?.(error)
      } finally {
        clearSelection()
        operating = false
        suppressContentClick(320)
      }
    }, Math.max(0, Number(delay) || 0))
    return Boolean(selectedNow)
  }

  function suppressContentClick(duration = 0) {
    suppressClick = true
    clearTimeout(releaseTimer)
    if (duration > 0) {
      releaseTimer = setTimeout(() => {
        releaseTimer = null
        suppressClick = false
      }, duration)
    }
  }

  function consumeSuppressedContentClick() {
    if (!suppressClick) return false
    suppressClick = false
    return true
  }

  function clearSelection() {
    try {
      window.getSelection?.()?.removeAllRanges?.()
    } catch {
      // Selection APIs may be unavailable in embedded browsers.
    }
  }

  onBeforeUnmount(() => {
    clearTimeout(operateTimer)
    clearTimeout(releaseTimer)
  })

  return {
    consumeSuppressedContentClick,
    schedule,
    selectedText,
    suppressContentClick,
  }
}
