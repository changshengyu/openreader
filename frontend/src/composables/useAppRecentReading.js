import { computed, ref } from 'vue'
import { newestBookProgress, progressUpdatedAt } from '../utils/bookOrder.js'
import { readerRouteQueryFromBook } from '../utils/readerRoute.js'

export function useAppRecentReading(options) {
  const suppressedAt = ref(readSuppressedAt())
  const recentBook = computed(() => {
    const rows = Array.isArray(options.getBooks()) ? options.getBooks() : []
    const progressByBook = options.getProgressByBook()
    return [...rows]
      .filter(book => {
        const progress = newestBookProgress(book, progressByBook)
        return hasReadingProgress(progress) &&
          progressUpdatedAt(progress) > suppressedAt.value
      })
      .sort((a, b) => {
        const aTime = progressUpdatedAt(newestBookProgress(a, progressByBook))
        const bTime = progressUpdatedAt(newestBookProgress(b, progressByBook))
        if (aTime !== bTime) return bTime - aTime
        return Number(b?.id || 0) - Number(a?.id || 0)
      })[0] || null
  })

  function open() {
    const book = recentBook.value
    if (!book) return
    const progress = progressForBook(book)
    options.navigate({
      name: 'reader',
      params: { id: book.id },
      query: readerRouteQueryFromBook(book, progress),
    })
  }

  function clear() {
    const progress = recentBook.value
      ? progressForBook(recentBook.value)
      : null
    const nextValue = Math.max(options.now(), progressUpdatedAt(progress))
    suppressedAt.value = nextValue
    writeSuppressedAt(nextValue)
  }

  function subtitle(book) {
    const progress = progressForBook(book)
    if (progress?.chapterTitle) return progress.chapterTitle
    if (Number.isInteger(progress?.chapterIndex)) {
      return `第 ${progress.chapterIndex + 1} 章`
    }
    return book?.lastChapter || book?.author || '继续阅读'
  }

  function progressForBook(book) {
    return newestBookProgress(book, options.getProgressByBook())
  }

  function refreshScope() {
    suppressedAt.value = readSuppressedAt()
  }

  function cacheKey() {
    return `openreader:readingRecentClearedAt:${options.getUserScope()}`
  }

  function readSuppressedAt() {
    try {
      return Number(options.getStorage()?.getItem(cacheKey()) || 0)
    } catch {
      return 0
    }
  }

  function writeSuppressedAt(value) {
    try {
      options.getStorage()?.setItem(cacheKey(), String(Number(value || 0)))
    } catch {
      // Keep the session state when private-mode storage is unavailable.
    }
  }

  return {
    suppressedAt,
    recentBook,
    open,
    clear,
    subtitle,
    progressForBook,
    refreshScope,
  }
}

export function hasReadingProgress(progress) {
  if (!progress?.bookId) return false
  if (progressUpdatedAt(progress) > 0) return true
  if (progress.chapterTitle) return true
  if (Number.isInteger(progress.chapterIndex) && progress.chapterIndex >= 0) {
    return true
  }
  return Number(progress.offset || 0) > 0 ||
    Number(progress.percent || 0) > 0 ||
    Number(progress.chapterPercent || 0) > 0
}
