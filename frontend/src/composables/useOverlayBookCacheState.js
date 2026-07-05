import { ref } from 'vue'

export function useOverlayBookCacheState(options) {
  const localCacheCounts = ref({})

  async function refreshManagedBrowserCacheCounts() {
    const rows = options.getManagedBooks().filter(book => book?.id)
    try {
      localCacheCounts.value = await options.countBrowserCachedChapters(rows)
    } catch {
      localCacheCounts.value = Object.fromEntries(rows.map(book => [book.id, 0]))
    }
  }

  async function refreshBookInfoBrowserCacheCount(book) {
    if (!book?.id) return
    try {
      const map = await options.listBrowserCachedChapters(book, book.id)
      setLocalCacheCount(book.id, Object.keys(map).length)
    } catch {
      setLocalCacheCount(book.id, 0)
    }
  }

  function setLocalCacheCount(bookId, count) {
    localCacheCounts.value = {
      ...localCacheCounts.value,
      [bookId]: Math.max(0, Number(count || 0)),
    }
  }

  async function invalidateBookReaderCaches(book, invalidateOptions = {}) {
    if (!book?.id) return
    await options.invalidateReaderData(book.id, { book: true, chapters: true })
    if (invalidateOptions.clearBrowser) {
      await options.clearBrowserChapterCache(book, book.id).catch(() => 0)
      setLocalCacheCount(book.id, 0)
    }
  }

  async function refreshBookChaptersCache(book) {
    if (!book?.id) return null
    try {
      const { data } = await options.listChapters(book.id)
      const chapters = Array.isArray(data) ? data : []
      await options.writeReaderData(book.id, {
        bookData: book,
        chaptersData: chapters,
      })
      return chapters
    } catch {
      await options.writeReaderData(book.id, { bookData: book })
      return null
    }
  }

  function mergedShelfBook(book) {
    if (!book?.id) return book
    const current = options.bookshelf.books
      .find(item => Number(item.id) === Number(book.id)) ||
      (
        Number(options.overlay.bookInfoBook?.id) === Number(book.id)
          ? options.overlay.bookInfoBook
          : null
      )
    return options.mergeBook(current, book)
  }

  function applyUpdatedBookToOverlay(book, chapters = null) {
    if (!book?.id) return book
    const nextBook = mergedShelfBook(book)
    options.bookshelf.upsertBook(nextBook)
    if (
      Number(options.overlay.bookInfoBook?.id) === Number(nextBook.id)
    ) {
      options.overlay.bookInfoBook = nextBook
    }
    options.emitBookInfoUpdated(nextBook)
    options.emitReaderBookDataUpdated({
      bookId: nextBook.id,
      book: nextBook,
      chapters,
    })
    return nextBook
  }

  function localCacheCount(book) {
    return localCacheCounts.value[book?.id] || 0
  }

  function serverCacheCount(book) {
    return Number(book?.cachedChapterCount || 0)
  }

  function updateServerCacheCount(book, count) {
    if (!book?.id) return
    const nextCount = Math.max(0, Number(count || 0))
    const nextBook = { ...book, cachedChapterCount: nextCount }
    options.bookshelf.upsertBook(nextBook)
    if (Number(options.overlay.bookInfoBook?.id) === Number(book.id)) {
      options.overlay.bookInfoBook = {
        ...options.overlay.bookInfoBook,
        cachedChapterCount: nextCount,
      }
    }
  }

  return {
    localCacheCounts,
    refreshManagedBrowserCacheCounts,
    refreshBookInfoBrowserCacheCount,
    setLocalCacheCount,
    invalidateBookReaderCaches,
    refreshBookChaptersCache,
    mergedShelfBook,
    applyUpdatedBookToOverlay,
    localCacheCount,
    serverCacheCount,
    updateServerCacheCount,
  }
}
