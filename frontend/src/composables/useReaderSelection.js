import { getCurrentInstance, onBeforeUnmount, unref } from 'vue'
import {
  normalizeReaderSelectionText,
  readerSelectionBelongsToRoot,
} from '../utils/readerSelection.js'

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

  function schedule(delay = 0, scheduleOptions = {}) {
    if (options.getAction?.() === '忽略') return false
    clearTimeout(operateTimer)
    const selectedNow = selectedText()
    const retry = scheduleOptions?.retry !== false
    if (!selectedNow && !retry) return false
    const retryInterval = Math.max(20, Number(options.retryInterval) || 80)
    const retryWindow = Math.max(retryInterval, Number(options.retryWindow) || 720)
    const startedAt = Date.now()
    const attempt = async () => {
      operateTimer = null
      const text = selectedText()
      if (!text) {
        if (
          retry
          && Date.now() - startedAt < retryWindow
          && options.getAction?.() !== '忽略'
        ) {
          operateTimer = setTimeout(attempt, retryInterval)
        }
        return
      }
      if (operating || options.getAction?.() === '忽略') return
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
    }
    operateTimer = setTimeout(attempt, Math.max(0, Number(delay) || 0))
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

  if (getCurrentInstance()) {
    onBeforeUnmount(() => {
      clearTimeout(operateTimer)
      clearTimeout(releaseTimer)
    })
  }

  return {
    consumeSuppressedContentClick,
    schedule,
    selectedText,
    suppressContentClick,
  }
}
