import { ref } from 'vue'
import { bookCategoryIds } from '../utils/bookCategory.js'

export function useOverlayBookInfo(options) {
  const localCacheCounts = ref({})
  const refreshingBookId = ref(null)
  const coverUploadingBookId = ref(null)
  const updatingBookId = ref(null)
  const editingBookSaving = ref(false)

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

  async function saveEditedBook(payload) {
    const book = options.overlay.bookEditBook
    if (!book?.id) return
    editingBookSaving.value = true
    try {
      const { data } = await options.updateBook(book.id, {
        ...payload,
        categoryIds: bookCategoryIds(book),
        canUpdate: book.canUpdate !== false,
      })
      const nextBook = applyUpdatedBookToOverlay(data)
      options.overlay.bookEditBook = nextBook
      options.overlay.bookEditVisible = false
      options.onSuccess('书籍已更新')
    } catch (error) {
      options.onError(error, '更新书籍失败')
    } finally {
      editingBookSaving.value = false
    }
  }

  async function refreshLocalBookInfo(book) {
    if (!book?.id) return
    refreshingBookId.value = book.id
    try {
      const { data } = await options.refreshLocalBook(book.id)
      await invalidateBookReaderCaches(book, { clearBrowser: true })
      const updatedBook = data?.book || data
      if (updatedBook?.id) {
        const mergedBook = mergedShelfBook(updatedBook)
        const chapters = await refreshBookChaptersCache(mergedBook)
        applyUpdatedBookToOverlay(mergedBook, chapters)
        await refreshBookInfoBrowserCacheCount(mergedBook)
      } else {
        await options.bookshelf.loadBooks({ force: true, all: true })
      }
      options.onSuccess(
        `本地书已刷新，共 ${data?.chapterCount || updatedBook?.chapterCount || 0} 章`,
      )
    } catch (error) {
      options.onError(error, '刷新本地书失败')
    } finally {
      refreshingBookId.value = null
    }
  }

  async function uploadBookInfoCover(file) {
    const book = options.overlay.bookInfoBook
    if (!book?.id || !file) return
    coverUploadingBookId.value = book.id
    try {
      const { data: uploadResult } = await options.uploadAsset({
        file,
        type: 'cover',
      })
      const { data: updatedBook } = await options.updateBook(book.id, {
        title: book.title,
        author: book.author || '',
        customCoverUrl: uploadResult.url,
        intro: book.intro || '',
        categoryIds: bookCategoryIds(book),
        canUpdate: book.canUpdate !== false,
      })
      applyUpdatedBookToOverlay(updatedBook)
      options.onSuccess('封面已更新')
    } catch (error) {
      options.onError(error, '更新封面失败')
    } finally {
      coverUploadingBookId.value = null
    }
  }

  async function toggleBookCanUpdate(value) {
    const book = options.overlay.bookInfoBook
    if (!book?.id || !book.sourceId) return
    updatingBookId.value = book.id
    try {
      const { data: updatedBook } = await options.updateBook(book.id, {
        title: book.title,
        author: book.author || '',
        coverUrl: book.coverUrl || '',
        intro: book.intro || '',
        categoryIds: bookCategoryIds(book),
        canUpdate: value,
      })
      applyUpdatedBookToOverlay(updatedBook)
      options.onSuccess(value ? '已开启追更' : '已关闭追更')
    } catch (error) {
      options.onError(error, '更新追更状态失败')
    } finally {
      updatingBookId.value = null
    }
  }

  return {
    localCacheCounts,
    refreshingBookId,
    coverUploadingBookId,
    updatingBookId,
    editingBookSaving,
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
    saveEditedBook,
    refreshLocalBookInfo,
    uploadBookInfoCover,
    toggleBookCanUpdate,
  }
}
