import { getChapterContent } from '../api/books'
import { cacheFirstRequest, listBrowserCacheKeys, removeBrowserCacheKeys } from './browserCache'

export function chapterCacheBookKey(book, fallbackBookId) {
  const currentBook = book || {}
  return currentBook.url || currentBook.bookUrl || currentBook.libraryPath || `book:${fallbackBookId}`
}

export function chapterCacheKeyPrefix(book, fallbackBookId) {
  const currentBook = book || {}
  return [
    `${currentBook.title || currentBook.name || 'book'}_${currentBook.author || ''}`,
    chapterCacheBookKey(currentBook, fallbackBookId),
  ].join('@')
}

export function chapterCacheKey(book, fallbackBookId, index) {
  return `${chapterCacheKeyPrefix(book, fallbackBookId)}@chapterContent-${index}`
}

export function isValidChapterContentResponse(data) {
  return Boolean(data?.chapter && typeof data.content === 'string' && data.content.trim())
}

export async function loadBrowserChapterContent(book, bookId, index, options = {}) {
  const { data } = await cacheFirstRequest(
    () => getChapterContent(bookId, index),
    chapterCacheKey(book, bookId, index),
    { refresh: Boolean(options.refresh), validate: isValidChapterContentResponse },
  )
  return data
}

export async function listBookBrowserCachedChapters(book, bookId) {
  const prefix = `${chapterCacheKeyPrefix(book, bookId)}@chapterContent-`
  const keys = await listBrowserCacheKeys(prefix)
  const map = {}
  keys.forEach(key => {
    const index = Number(key.slice(key.lastIndexOf('@chapterContent-') + '@chapterContent-'.length))
    if (Number.isInteger(index) && index >= 0) map[index] = true
  })
  return map
}

export async function countBooksBrowserCachedChapters(books = []) {
  const rows = Array.isArray(books) ? books : []
  const prefixRows = rows.map(book => ({
    book,
    prefix: `localCache@${chapterCacheKeyPrefix(book, book.id)}@chapterContent-`,
    count: 0,
  }))
  const keys = await listBrowserCacheKeys('')
  keys.forEach(key => {
    const row = prefixRows.find(item => key.startsWith(item.prefix))
    if (row) row.count += 1
  })
  return Object.fromEntries(prefixRows.map(row => [row.book.id, row.count]))
}

export async function clearBookBrowserChapterCache(book, bookId) {
  return removeBrowserCacheKeys(`${chapterCacheKeyPrefix(book, bookId)}@chapterContent-`)
}

export async function cacheBookChaptersToBrowser(book, bookId, chapters, options = {}) {
  const cachedMap = await listBookBrowserCachedChapters(book, bookId)
  const startIndex = Math.max(0, Number(options.startIndex || 0))
  const count = options.count === true ? chapters.length : Number(options.count || chapters.length)
  const endIndex = Math.min(chapters.length, startIndex + count)
  const targets = []
  for (let index = startIndex; index < endIndex; index += 1) {
    if (!cachedMap[index]) targets.push(index)
  }
  let finished = 0
  let cached = 0
  const total = targets.length
  const workers = Array.from({ length: Math.min(Number(options.concurrency || 2), total || 1) }, async () => {
    while (targets.length && !options.cancelled?.()) {
      const index = targets.shift()
      try {
        const data = await loadBrowserChapterContent(book, bookId, index)
        if (isValidChapterContentResponse(data)) cached += 1
      } catch {
        // Keep parity with upstream batch caching: failed chapters should not stop the queue.
      } finally {
        finished += 1
        options.onProgress?.({ finished, total, cached })
      }
    }
  })
  await Promise.all(workers)
  return { cached, requested: total, cancelled: Boolean(options.cancelled?.()) }
}
