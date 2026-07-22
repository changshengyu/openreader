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

function legacyChapterCacheKeyPrefix(book, fallbackBookId) {
  const currentBook = book || {}
  return [
    `${currentBook.title || currentBook.name || 'book'}_${currentBook.author || ''}`,
    chapterCacheBookKey(currentBook, fallbackBookId),
  ].join('@')
}

export function chapterCacheKey(book, fallbackBookId, index) {
  return `${chapterCacheKeyPrefix(book, fallbackBookId)}@chapterContent-${index}`
}

function legacyChapterCacheKey(book, fallbackBookId, index) {
  return `${legacyChapterCacheKeyPrefix(book, fallbackBookId)}@chapterContent-${index}`
}

export function isValidChapterContentResponse(data) {
  return Boolean(data?.chapter && typeof data.content === 'string' && data.content.trim())
}

export async function loadBrowserChapterContent(book, bookId, index, options = {}) {
  const cacheKey = chapterCacheKey(book, bookId, index)
  if (!options.refresh) {
    const cached = await getValidCachedChapter(cacheKey)
    if (cached) return cached
    const legacyCached = await getValidCachedChapter(legacyChapterCacheKey(book, bookId, index))
    if (legacyCached) {
      setBrowserCache(cacheKey, legacyCached).catch(() => {})
      return legacyCached
    }
  }
  const { data } = await getChapterContent(bookId, index)
  if (isValidChapterContentResponse(data)) await setBrowserCache(cacheKey, data)
  return data
}

export async function listBookBrowserCachedChapters(book, bookId) {
  const prefix = `${chapterCacheKeyPrefix(book, bookId)}@chapterContent-`
  const legacyPrefix = `${legacyChapterCacheKeyPrefix(book, bookId)}@chapterContent-`
  const keys = [
    ...await listBrowserCacheKeys(prefix),
    ...await listBrowserCacheKeys(legacyPrefix),
  ]
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
    prefixes: [
      `localCache@${chapterCacheKeyPrefix(book, book.id)}@chapterContent-`,
      `localCache@${legacyChapterCacheKeyPrefix(book, book.id)}@chapterContent-`,
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
  const [scoped, legacy] = await Promise.all([
    removeBrowserCacheKeys(`${chapterCacheKeyPrefix(book, bookId, scope)}@chapterContent-`),
    removeBrowserCacheKeys(`${legacyChapterCacheKeyPrefix(book, bookId)}@chapterContent-`),
  ])
  return scoped + legacy
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

async function getValidCachedChapter(cacheKey) {
  const cached = await getBrowserCache(cacheKey)
  return isValidChapterContentResponse(cached) ? cached : null
}

async function currentUserBrowserChapterCacheKeys() {
  const keys = await listBrowserCacheKeys('')
  return keys.filter(isCurrentUserOrLegacyChapterCacheKey)
}

function isCurrentUserOrLegacyChapterCacheKey(key) {
  if (!String(key).includes('@chapterContent-')) return false
  const unprefixed = String(key).replace(/^localCache@/, '')
  const scope = currentUserScope()
  if (unprefixed.startsWith(`${scope}@`)) return true
  if (/^(user:[^@]+|anonymous)@/.test(unprefixed)) return false
  return true
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
