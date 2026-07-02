import { unref } from 'vue'

export function useReaderExternalUpdates(options) {
  async function handleProgressUpdated(event) {
    const progress = event?.detail?.progress
    if (!progress?.bookId || Number(progress.bookId) !== Number(unref(options.bookId))) return
    if (!unref(options.chapter) || options.isRestoring() || options.isProgressSaveBusy()) return
    const localKey = options.progressKey(options.getCurrentProgress())
    const remoteKey = options.progressKey({
      bookId: progress.bookId,
      chapterId: progress.chapterId,
      chapterIndex: progress.chapterIndex,
      offset: progress.offset,
      percent: progress.percent,
      chapterPercent: progress.chapterPercent,
    })
    if (!remoteKey || remoteKey === localKey) return

    const chapterRows = unref(options.chapters)
    const targetIndex = Math.max(
      0,
      Math.min(Number(progress.chapterIndex || 0), Math.max(chapterRows.length - 1, 0)),
    )
    const targetOffset = Math.max(0, Math.floor(Number(progress.offset || 0)))
    const restorePercent = Number.isFinite(Number(progress.chapterPercent))
      ? Math.max(0, Math.min(1, Number(progress.chapterPercent)))
      : null

    options.cancelProgressSave()
    try {
      await options.navigate({
        chapter: targetIndex,
        ...(targetOffset ? { offset: targetOffset } : {}),
        ...(restorePercent !== null ? { percent: Number(restorePercent.toFixed(6)) } : {}),
      })
      await options.loadChapter(targetIndex, targetOffset, {
        restorePercent,
        saveAfterLoad: false,
      })
      options.markProgressSaved(options.getCurrentProgress())
    } catch {
      // The stored progress remains available for the next reader open.
    }
  }

  async function handleBookDataUpdated(event) {
    const detail = event?.detail || {}
    if (!detail.bookId || Number(detail.bookId) !== Number(unref(options.bookId))) return
    if (detail.book?.id) options.book.value = detail.book
    if (!Array.isArray(detail.chapters)) return

    const restoreOffset = options.getCurrentOffset()
    const restorePercent = options.getCurrentPercent()
    const targetIndex = Math.max(
      0,
      Math.min(unref(options.currentIndex), Math.max(detail.chapters.length - 1, 0)),
    )
    options.chapters.value = detail.chapters
    options.currentIndex.value = targetIndex
    options.clearChapterCache()
    options.resetCachedChapters()
    options.resetContentSearch()
    await options.refreshCachedChapters()
    await options.loadChapter(targetIndex, restoreOffset, {
      restorePercent,
      refresh: true,
      saveAfterLoad: false,
    })
  }

  async function handleReplaceRulesUpdated() {
    if (!unref(options.book)?.id || !unref(options.chapter)) return
    const restorePercent = options.getCurrentPercent()
    try {
      await options.loadChapter(
        unref(options.currentIndex),
        options.getCurrentOffset(),
        { restorePercent, refresh: true },
      )
      options.onReplaceSuccess()
    } catch (error) {
      options.onReplaceError(error)
    }
  }

  return {
    handleBookDataUpdated,
    handleProgressUpdated,
    handleReplaceRulesUpdated,
  }
}
