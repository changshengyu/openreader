import { removeBrowserCache, setBrowserCache } from './browserCache.js'
import { currentUserScope } from './authScope.js'

export function readerDataCacheKey(bookId, type) {
  return `reader@${currentUserScope()}@${type}:${bookId}`
}

export async function invalidateReaderDataCache(bookId, options = {}) {
  if (!bookId) return
  const tasks = []
  if (options.book !== false) tasks.push(removeBrowserCache(readerDataCacheKey(bookId, 'book')))
  if (options.chapters !== false) tasks.push(removeBrowserCache(readerDataCacheKey(bookId, 'chapters')))
  await Promise.allSettled(tasks)
}

export async function writeReaderDataCache(bookId, options = {}) {
  if (!bookId) return
  const tasks = []
  if (options.bookData?.id) tasks.push(setBrowserCache(readerDataCacheKey(bookId, 'book'), options.bookData))
  if (Array.isArray(options.chaptersData)) tasks.push(setBrowserCache(readerDataCacheKey(bookId, 'chapters'), options.chaptersData))
  await Promise.allSettled(tasks)
}
