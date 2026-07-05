import { ref } from 'vue'
import { bookCategoryIds } from '../utils/bookCategory.js'

function isCancelled(error) {
  return error === 'cancel' || error === 'close'
}

function bookHasCategory(book, categoryId) {
  return bookCategoryIds(book).some(id => String(id) === String(categoryId))
}

export function useOverlayBookManagement(options) {
  const selectedBookIds = ref([])
  const batchBusy = ref(false)
  const cachingBookId = ref(null)

  function onManageSelectionChange(rows) {
    selectedBookIds.value = rows.map(row => row.id)
  }

  function toggleManagedBook(bookId, checked) {
    if (checked) {
      if (!selectedBookIds.value.includes(bookId)) {
        selectedBookIds.value.push(bookId)
      }
      return
    }
    selectedBookIds.value = selectedBookIds.value.filter(id => id !== bookId)
  }

  function selectAllManagedBooks() {
    selectedBookIds.value = options.getFilteredManagedBooks().map(book => book.id)
  }

  function clearManagedSelection() {
    selectedBookIds.value = []
  }

  async function batchAddCategory(category) {
    if (!selectedBookIds.value.length) return
    batchBusy.value = true
    try {
      await options.bookshelf.batchSetCategory(
        [...selectedBookIds.value],
        category.id,
        { action: 'category-add' },
      )
      options.onSuccess(`已添加到“${category.name}”分组`)
    } catch (error) {
      options.onError(error, '批量添加分组失败')
    } finally {
      batchBusy.value = false
    }
  }

  async function batchRemoveCategory(category) {
    if (!selectedBookIds.value.length) return
    const targetIds = options.getManagedBooks()
      .filter(book => (
        selectedBookIds.value.includes(book.id) &&
        bookHasCategory(book, category.id)
      ))
      .map(book => book.id)
    if (!targetIds.length) {
      options.onInfo('选中书籍不在该分组中')
      return
    }
    batchBusy.value = true
    try {
      await options.bookshelf.batchSetCategory(
        targetIds,
        category.id,
        { action: 'category-remove' },
      )
      options.onSuccess(`已从“${category.name}”分组移除`)
    } catch (error) {
      options.onError(error, '批量移除分组失败')
    } finally {
      batchBusy.value = false
    }
  }

  function selectedRemoteBookIds() {
    const selected = new Set(selectedBookIds.value)
    return options.getManagedBooks()
      .filter(book => selected.has(book.id) && Number(book.sourceId || 0) > 0)
      .map(book => book.id)
  }

  async function batchCacheBooks() {
    if (!selectedBookIds.value.length) return
    const remoteBookIds = selectedRemoteBookIds()
    if (!remoteBookIds.length) {
      options.onInfo('选中的本地书无需服务器缓存')
      return
    }
    batchBusy.value = true
    try {
      const data = await options.bookshelf.batchCacheBooks(remoteBookIds)
      options.onSuccess(`已缓存 ${data.cached || 0}/${data.requested || 0} 章`)
      await options.bookshelf.loadBooks({ force: true, all: true })
    } catch (error) {
      options.onError(error, '批量缓存失败')
    } finally {
      batchBusy.value = false
    }
  }

  async function batchClearCache() {
    if (!selectedBookIds.value.length) return
    const remoteBookIds = selectedRemoteBookIds()
    if (!remoteBookIds.length) {
      options.onInfo('选中的本地书没有服务器缓存')
      return
    }
    try {
      await options.confirm(
        `确定清理选中 ${remoteBookIds.length} 本远程书的章节缓存吗？`,
        '清理缓存',
        { type: 'warning' },
      )
      batchBusy.value = true
      const data = await options.bookshelf.batchClearCache(remoteBookIds)
      options.onSuccess(`已清理 ${data.cleared || 0} 个章节缓存`)
      for (const bookId of remoteBookIds) {
        const book = options.getManagedBooks()
          .find(item => Number(item.id) === Number(bookId))
        if (book) options.updateServerCacheCount(book, 0)
      }
    } catch (error) {
      if (isCancelled(error)) return
      options.onError(error, '清理缓存失败')
    } finally {
      batchBusy.value = false
    }
  }

  async function batchDeleteBooks() {
    if (!selectedBookIds.value.length) return
    try {
      await options.confirm(
        `确定删除选中的 ${selectedBookIds.value.length} 本书吗？`,
        '批量删除',
        { type: 'warning' },
      )
      batchBusy.value = true
      await options.bookshelf.batchDeleteBooks([...selectedBookIds.value])
      selectedBookIds.value = []
      options.onSuccess('已批量删除')
    } catch (error) {
      if (isCancelled(error)) return
      options.onError(error, '批量删除失败')
    } finally {
      batchBusy.value = false
    }
  }

  async function batchExportBooks() {
    if (!selectedBookIds.value.length) return
    batchBusy.value = true
    try {
      const bookIds = [...selectedBookIds.value]
      const blob = await options.bookshelf.exportSelectedBooks(bookIds, 'json')
      options.saveBlob(blob, `openreader-books-${bookIds.length}.json`)
      options.onSuccess(`已导出 ${bookIds.length} 本书`)
    } catch (error) {
      options.onError(error, '批量导出失败')
    } finally {
      batchBusy.value = false
    }
  }

  function handleBatchMoreCommand(command) {
    if (command === 'cache') {
      batchCacheBooks()
    } else if (command === 'clear-cache') {
      batchClearCache()
    } else if (command === 'export') {
      batchExportBooks()
    }
  }

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
    selectedBookIds,
    batchBusy,
    cachingBookId,
    onManageSelectionChange,
    toggleManagedBook,
    selectAllManagedBooks,
    clearManagedSelection,
    batchAddCategory,
    batchRemoveCategory,
    selectedRemoteBookIds,
    batchCacheBooks,
    batchClearCache,
    batchDeleteBooks,
    batchExportBooks,
    handleBatchMoreCommand,
    cacheStartChapterIndex,
    cacheBook,
    cacheBookLocal,
    clearBookCache,
    clearBookLocalCache,
    exportBook,
    exportBookFilename,
  }
}
