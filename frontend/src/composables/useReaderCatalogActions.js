import { unref } from 'vue'

export function useReaderCatalogActions(options) {
  function applyUpdatedBook(updated, { syncMatchingOverlay = false } = {}) {
    const merged = options.mergeLoadedBook(updated)
    options.book.value = merged
    options.upsertBook(merged)
    if (
      syncMatchingOverlay &&
      options.getOverlayBook?.()?.id === updated.id
    ) {
      options.setOverlayBook?.(merged)
    }
    return merged
  }

  async function changeLocalTocRule() {
    if (!unref(options.book) || !unref(options.canChangeLocalTocRule)) return
    const tocRule = await options.chooseLocalTocRule()
    if (tocRule === null) return

    try {
      await options.runTocRefreshing(async () => {
        const currentBook = unref(options.book)
        const data = await options.refreshLocalBook(currentBook.id, { tocRule })
        await options.invalidateDataCache({ chapters: true, book: true })
        await options.resetChapterCaches({ clearBrowser: true })
        const updated = data?.book || data
        if (updated?.id) {
          const merged = applyUpdatedBook(updated, { syncMatchingOverlay: true })
          await options.writeDataCache({ bookData: merged })
        }
        const chapters = await options.loadChapters()
        const nextIndex = Math.min(
          unref(options.currentIndex),
          Math.max(chapters.length - 1, 0),
        )
        await options.loadChapter(nextIndex, 0, {
          refresh: true,
          saveAfterLoad: true,
        })
        await options.refreshBrowserCachedChapters()
        options.locateCurrentTocChapter()
        options.notify?.(`目录规则已更新，共 ${data?.chapterCount || chapters.length} 章`)
      })
    } catch (error) {
      options.onError?.(error, '更新目录规则失败')
    }
  }

  async function refreshRemoteCatalog() {
    const currentBook = unref(options.book)
    if (!currentBook?.id || Number(currentBook.sourceId || 0) <= 0) return
    try {
      const restoreOffset = options.getCurrentOffset()
      const restorePercent = options.getCurrentChapterPercent()
      const data = await options.refreshRemoteBook(currentBook.id)
      await options.invalidateDataCache({ book: true, chapters: true })
      await options.resetChapterCaches({ clearBrowser: true })
      const updated = data?.book || data
      if (updated?.id) {
        const merged = applyUpdatedBook(updated)
        await options.writeDataCache({ bookData: merged })
      }
      await options.loadChapters()
      await options.loadChapter(unref(options.currentIndex), restoreOffset, {
        restorePercent,
        refresh: true,
      })
      options.setOverlayBook?.(unref(options.book))
      options.notify?.('目录已刷新', 1400)
    } catch (error) {
      options.onError?.(error, '刷新目录失败')
    }
  }

  async function applySourceChange({ book: updatedBook, previousBook }) {
    await options.invalidateDataCache({ book: true, chapters: true })
    await options.resetChapterCaches({ clearBrowser: true, book: previousBook })
    const merged = applyUpdatedBook(updatedBook)
    const chapters = await options.fetchChapters(unref(options.bookId))
    options.chapters.value = Array.isArray(chapters) ? chapters : []
    await options.writeDataCache({
      bookData: merged,
      chaptersData: options.chapters.value,
    })
    options.currentIndex.value = Math.min(
      unref(options.currentIndex),
      Math.max(options.chapters.value.length - 1, 0),
    )
    await options.loadChapter(unref(options.currentIndex), 0)
    options.resetContentSearch()
    await options.refreshSourceCandidates()
    options.closeSourceDrawer()
  }

  return {
    applySourceChange,
    changeLocalTocRule,
    refreshRemoteCatalog,
  }
}
