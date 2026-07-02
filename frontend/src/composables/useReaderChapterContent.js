import { unref } from 'vue'
import {
  chapterCacheBookKey,
  isValidChapterContentResponse,
  loadBrowserChapterContent,
} from '../utils/bookChapterCache.js'
import { nearbyReaderChapterIndexes } from '../utils/readerChapterWindow.js'

export function useReaderChapterContent(options) {
  const loadBrowserContent = options.loadBrowserContent ?? loadBrowserChapterContent
  const preloadRadius = Math.max(0, Number(options.preloadRadius) || 0)

  function cacheKey(targetBook = unref(options.book), fallbackBookId = unref(options.bookId)) {
    return chapterCacheBookKey(targetBook, fallbackBookId)
  }

  function get(index, targetCacheKey = cacheKey()) {
    const cached = options.memoryCache.get(targetCacheKey, index)
    return isValidChapterContentResponse(cached) ? cached : null
  }

  function set(index, data, targetCacheKey = cacheKey()) {
    if (!isValidChapterContentResponse(data)) return
    options.memoryCache.set(targetCacheKey, index, data)
  }

  function clear(targetBook = unref(options.book), fallbackBookId = unref(options.bookId)) {
    options.memoryCache.clearBook(cacheKey(targetBook, fallbackBookId))
  }

  async function load(index, loadOptions = {}) {
    const targetBook = { ...(unref(options.book) || {}) }
    const targetBookId = unref(options.bookId)
    const targetCacheKey = cacheKey(targetBook, targetBookId)
    if (!loadOptions.refresh) {
      const cached = get(index, targetCacheKey)
      if (cached) return cached
    }

    const data = await loadBrowserContent(
      targetBook,
      targetBookId,
      index,
      { refresh: Boolean(loadOptions.refresh) },
    )
    set(index, data, targetCacheKey)
    if (
      isValidChapterContentResponse(data)
      && Number(unref(options.bookId)) === Number(targetBookId)
      && cacheKey() === targetCacheKey
    ) {
      options.markCached(index)
    }
    return data
  }

  function preload(index) {
    const chapterRows = unref(options.chapters) || []
    if (!unref(options.book) || !chapterRows.length) return
    nearbyReaderChapterIndexes({
      chapterIndex: index,
      totalChapters: chapterRows.length,
      radius: preloadRadius,
    }).forEach(target => {
      if (get(target)) return
      load(target).catch(() => {})
    })
  }

  return {
    cacheKey,
    clear,
    get,
    load,
    preload,
    set,
  }
}
