import { unref } from 'vue'

export function useReaderScrollSync(options) {
  let pendingAnimationSettlement = false
  let lastSynchronizedPosition = null

  function isPageAnimationActive() {
    return typeof options.pageAnimationActive === 'function'
      ? Boolean(options.pageAnimationActive())
      : Boolean(unref(options.pageAnimationActive))
  }

  function scrollPosition() {
    if (typeof options.scrollPosition !== 'function') return null
    const value = Number(options.scrollPosition())
    return Number.isFinite(value) ? value : null
  }

  function synchronize() {
    if (!unref(options.isVerticalRead)) return false
    if (
      unref(options.restoringPosition)
      || unref(options.chapterLoading)
      || unref(options.windowBusy)
    ) return false
    const position = scrollPosition()
    if (
      !pendingAnimationSettlement
      && position !== null
      && position === lastSynchronizedPosition
    ) return false
    pendingAnimationSettlement = false
    lastSynchronizedPosition = position
    if (!unref(options.isContinuousScrollRead)) {
      options.updateLayout()
      options.progressVersion.value += 1
      options.scheduleProgressSave(500)
      return true
    }
    const snapshot = options.captureProgressSnapshot?.()
    options.syncCurrentChapter(snapshot)
    options.maybeExtendChapterWindow()
    options.updateLayout()
    if (unref(options.windowBusy)) return true
    options.progressVersion.value += 1
    options.applyLocalProgress(snapshot)
    options.scheduleProgressSave(500)
    return true
  }

  function handle() {
    if (!unref(options.isVerticalRead)) return false
    if (isPageAnimationActive()) {
      pendingAnimationSettlement = true
      return false
    }
    return synchronize()
  }

  function flush() {
    if (isPageAnimationActive()) return false
    return synchronize()
  }

  return {
    flush,
    handle,
  }
}
