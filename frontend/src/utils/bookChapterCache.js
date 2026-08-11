import { getChapterContent } from '../api/books.js'
import { getBrowserCache, listBrowserCacheKeys, removeBrowserCache, removeBrowserCacheKeys, setBrowserCache } from './browserCache.js'
import { currentUserScope } from './authScope.js'

export function chapterCacheBookKey(book, fallbackBookId) {
  const currentBook = book || {}
  return currentBook.url || currentBook.bookUrl || currentBook.libraryPath || `book:${fallbackBookId}`
}

export function chapterCacheKeyPrefix(book, fallbackBookId, scope = currentUserScope()) {
  const currentBook = book || {}
  return [
    scope,
    `${currentBook.title || currentBook.name || 'book'}_${currentBook.author || ''}`,
    chapterCacheBookKey(currentBook, fallbackBookId),
  ].join('@')
}

export function chapterCacheKey(
  book,
  fallbackBookId,
  index,
  scope = currentUserScope(),
) {
  return `${chapterCacheKeyPrefix(book, fallbackBookId, scope)}@chapterContent-${index}`
}

export function isValidChapterContentResponse(data) {
  return Boolean(data?.chapter && typeof data.content === 'string' && data.content.trim())
}

export async function loadBrowserChapterContent(book, bookId, index, options = {}) {
  const scope = options.scope || currentUserScope()
  const getCache = options.getCache || getBrowserCache
  const setCache = options.setCache || setBrowserCache
  const fetchChapter = options.getChapterContent || getChapterContent
  const cacheKey = chapterCacheKey(book, bookId, index, scope)
  if (!options.refresh) {
    const cached = await getValidCachedChapter(cacheKey, getCache)
    if (cached) return cached
  }
  const { data } = await fetchChapter(bookId, index)
  if (isValidChapterContentResponse(data)) await setCache(cacheKey, data)
  return data
}

export async function listBookBrowserCachedChapters(book, bookId, options = {}) {
  const scope = options.scope || currentUserScope()
  const prefix = `${chapterCacheKeyPrefix(book, bookId, scope)}@chapterContent-`
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
  const scope = currentUserScope()
  const prefixRows = rows.map(book => ({
    book,
    prefixes: [
      `localCache@${chapterCacheKeyPrefix(book, book.id, scope)}@chapterContent-`,
    ],
    indexes: new Set(),
  }))
  const keys = await listBrowserCacheKeys('')
  keys.forEach(key => {
    const row = prefixRows.find(item => item.prefixes.some(prefix => key.startsWith(prefix)))
    if (row) {
      const index = Number(key.slice(key.lastIndexOf('@chapterContent-') + '@chapterContent-'.length))
      if (Number.isInteger(index) && index >= 0) row.indexes.add(index)
    }
  })
  return Object.fromEntries(prefixRows.map(row => [row.book.id, row.indexes.size]))
}

export async function clearBookBrowserChapterCache(book, bookId, options = {}) {
  const scope = options.scope || currentUserScope()
  const removeKeys = options.removeKeys || removeBrowserCacheKeys
  return removeKeys(
    `${chapterCacheKeyPrefix(book, bookId, scope)}@chapterContent-`,
  )
}

export async function currentUserBrowserChapterCacheStats() {
  const keys = await currentUserBrowserChapterCacheKeys()
  let size = 0
  await Promise.all(keys.map(async (key) => {
    const cached = await getBrowserCache(key)
    size += estimateCacheValueSize(cached)
  }))
  return {
    chapters: keys.length,
    files: keys.length,
    size,
  }
}

export async function clearCurrentUserBrowserChapterCache() {
  const keys = await currentUserBrowserChapterCacheKeys()
  await Promise.all(keys.map(key => removeBrowserCache(key)))
  return keys.length
}

export async function cacheBookChaptersToBrowser(book, bookId, chapters, options = {}) {
  const loadChapterContent = options.loadChapterContent || loadBrowserChapterContent
  const scope = options.scope || currentUserScope()
  const startIndex = Math.max(0, Number(options.startIndex || 0))
  const count = options.count === true ? chapters.length : Number(options.count || chapters.length)
  const endIndex = Math.min(chapters.length, startIndex + count)
  const targets = []
  for (let index = startIndex; index < endIndex; index += 1) {
    targets.push(index)
  }
  let finished = 0
  let cached = 0
  const total = targets.length
  const workers = Array.from({ length: Math.min(Number(options.concurrency || 2), total || 1) }, async () => {
    while (targets.length && !options.cancelled?.()) {
      const index = targets.shift()
      try {
        const data = await loadChapterContent(book, bookId, index, { scope })
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

async function getValidCachedChapter(cacheKey, getCache = getBrowserCache) {
  const cached = await getCache(cacheKey)
  return isValidChapterContentResponse(cached) ? cached : null
}

async function currentUserBrowserChapterCacheKeys() {
  const scope = currentUserScope()
  const keys = await listBrowserCacheKeys('')
  return keys.filter(key => isCurrentUserChapterCacheKey(key, scope))
}

function isCurrentUserChapterCacheKey(key, scope) {
  if (!String(key).includes('@chapterContent-')) return false
  const unprefixed = String(key).replace(/^localCache@/, '')
  return unprefixed.startsWith(`${scope}@`)
}

function estimateCacheValueSize(value) {
  if (!value) return 0
  try {
    return new Blob([JSON.stringify(value)]).size
  } catch {
    try {
      return JSON.stringify(value).length
    } catch {
      return 0
    }
  }
}
