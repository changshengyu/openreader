import { unref } from 'vue'

export function useReaderChapterMaintenance(options) {
  async function loadChapters() {
    if (unref(options.isTemporaryReader)) return unref(options.chapters)
    const targetBookId = unref(options.bookId)
    const data = await options.fetchChapters(targetBookId)
    if (unref(options.bookId) !== targetBookId) return unref(options.chapters)

    const nextChapters = Array.isArray(data) ? data : []
    options.chapters.value = nextChapters
    options.currentIndex.value = Math.max(
      0,
      Math.min(
        unref(options.currentIndex),
        Math.max(nextChapters.length - 1, 0),
      ),
    )
    await options.writeDataCache({
      bookId: targetBookId,
      chaptersData: nextChapters,
    })
    return nextChapters
  }

  async function resetCaches(resetOptions = {}) {
    const targetBook = resetOptions.book || unref(options.book)
    const targetBookId = targetBook?.id || unref(options.bookId)
    options.clearMemory(targetBook, targetBookId)
    options.resetBrowserState()
    if (!resetOptions.clearBrowser) return 0
    try {
      return await options.clearBrowserCache(targetBook, targetBookId)
    } catch {
      return 0
    }
  }

  async function reloadChapter() {
    await options.loadChapter(
      unref(options.currentIndex),
      options.getCurrentOffset(),
      { refresh: true },
    )
    options.notify?.('章节已重新载入')
  }

  async function clearCurrentBookCache() {
    if (unref(options.isTemporaryReader)) {
      options.onUnavailable?.()
      return
    }
    if (!unref(options.isRemoteBook)) return
    try {
      const data = await options.clearServerCache([unref(options.bookId)])
      const localCleared = await options.clearCurrentBrowserCache()
      await loadChapters()
      options.notify?.(`已清理服务器 ${data.cleared || 0} 章，本地 ${localCleared} 章`)
    } catch (error) {
      options.onError?.(error, '清理缓存失败')
    }
  }

  return {
    clearCurrentBookCache,
    loadChapters,
    reloadChapter,
    resetCaches,
  }
}
