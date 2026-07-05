import { ref } from 'vue'

export function useOverlayBookItemActions(options, sharedState = {}) {
  const cachingBookId = ref(null)
  const batchBusy = sharedState.batchBusy || ref(false)

  function cacheStartChapterIndex(book) {
    const progress = options.getBookProgress(book)
    const chapterIndex = Number(progress?.chapterIndex)
    return Number.isInteger(chapterIndex) && chapterIndex > 0
      ? chapterIndex
      : 0
  }

  async function cacheBook(book, command) {
    if (
      Number(book?.sourceId || 0) === 0 &&
      command !== 'cacheBookLocal' &&
      command !== 'deleteBookLocalCache'
    ) {
      options.onInfo('本地书无需服务器缓存')
      return
    }
    if (command === 'deleteBookCache') {
      await clearBookCache(book)
      return
    }
    if (command === 'deleteBookLocalCache') {
      await clearBookLocalCache(book)
      return
    }
    if (command === 'cacheBookLocal') {
      await cacheBookLocal(book)
      return
    }
    cachingBookId.value = book.id
    try {
      const chapterIndex = cacheStartChapterIndex(book)
      const { data } = await options.cacheBookContent(book.id, {
        all: true,
        count: 20,
        chapterIndex,
      })
      if (data?.book) options.bookshelf.upsertBook(data.book)
      options.onSuccess(`已缓存 ${data.cached || 0}/${data.requested || 0} 章`)
    } catch (error) {
      options.onError(error, '缓存失败')
    } finally {
      cachingBookId.value = null
    }
  }

  async function cacheBookLocal(book) {
    cachingBookId.value = book.id
    try {
      const { data } = await options.listChapters(book.id)
      const chapterIndex = cacheStartChapterIndex(book)
      const result = await options.cacheBrowserChapters(
        book,
        book.id,
        Array.isArray(data) ? data : [],
        {
          startIndex: chapterIndex,
          count: 100,
        },
      )
      options.onSuccess(
        `已缓存到浏览器 ${result.cached}/${result.requested} 章`,
      )
      await refreshBrowserCacheCounts()
    } catch (error) {
      options.onError(error, '缓存到浏览器失败')
    } finally {
      cachingBookId.value = null
    }
  }

  async function clearBookCache(book) {
    cachingBookId.value = book.id
    try {
      const data = await options.bookshelf.batchClearCache([book.id])
      options.updateServerCacheCount(book, 0)
      options.onSuccess(`已清理 ${data.cleared || 0} 个章节缓存`)
    } catch (error) {
      options.onError(error, '清理缓存失败')
    } finally {
      cachingBookId.value = null
    }
  }

  async function clearBookLocalCache(book) {
    cachingBookId.value = book.id
    try {
      const removed = await options.clearBrowserChapterCache(book, book.id)
      await refreshBrowserCacheCounts()
      options.onSuccess(`已清理浏览器缓存 ${removed} 章`)
    } catch (error) {
      options.onError(error, '清理浏览器缓存失败')
    } finally {
      cachingBookId.value = null
    }
  }

  async function refreshBrowserCacheCounts() {
    await options.refreshManagedBrowserCacheCounts()
  }

  async function exportBook(book, format = 'txt') {
    batchBusy.value = true
    try {
      const normalizedFormat = ['json', 'txt', 'epub'].includes(format)
        ? format
        : 'txt'
      const blob = await options.bookshelf.exportSelectedBooks(
        [book.id],
        normalizedFormat,
      )
      options.saveBlob(blob, exportBookFilename(book, normalizedFormat))
      options.onSuccess(`已导出《${book.title}》`)
    } catch (error) {
      options.onError(error, '导出失败')
    } finally {
      batchBusy.value = false
    }
  }

  function exportBookFilename(book, format) {
    const fallback = `book-${book?.id || options.now()}`
    const title = String(book?.title || fallback)
      .replace(/[\\/:*?"<>|]/g, '-')
      .trim() || fallback
    const extension = format === 'json'
      ? 'json'
      : format === 'epub'
        ? 'epub'
        : 'txt'
    return `${title}.${extension}`
  }

  return {
    cachingBookId,
    cacheStartChapterIndex,
    cacheBook,
    cacheBookLocal,
    clearBookCache,
    clearBookLocalCache,
    exportBook,
    exportBookFilename,
  }
}
