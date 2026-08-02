import { ref } from 'vue'
import { useOverlayBookCacheState } from './useOverlayBookCacheState.js'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation.js'

export function useOverlayBookInfo(options) {
  const refreshingBookId = ref(null)
  const coverUploadingBookId = ref(null)
  const updatingBookId = ref(null)
  const editingBookSaving = ref(false)
  const operations = options.operationGuard || createAuthenticatedOperationGuard({
    getIdentity: options.getAuthenticatedIdentity,
  })
  const cacheState = useOverlayBookCacheState(options)
  const {
    invalidateBookReaderCaches,
    refreshBookChaptersCache,
    mergedShelfBook,
    applyUpdatedBookToOverlay,
  } = cacheState

  async function saveEditedBook(payload) {
    const operation = operations.begin('save-edit')
    const draftBook = options.overlay.bookEditBook
    const book = options.getManagedBooks().find(item => (
      Number(item?.id) === Number(draftBook?.id)
    ))
    if (!book?.id) {
      options.onError(
        new Error('book edit target is no longer in the shelf'),
        '书籍已不在书架中，请重新打开编辑器',
      )
      return
    }
    editingBookSaving.value = true
    try {
      const { data } = await options.updateBook(book.id, editableBookMetadata(payload))
      if (!operations.canCommit(operation)) return
      const nextBook = applyUpdatedBookToOverlay(data)
      options.overlay.bookEditBook = nextBook
      options.overlay.bookEditVisible = false
      options.onSuccess('书籍已更新')
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '更新书籍失败')
      }
    } finally {
      if (operations.canCommit(operation)) editingBookSaving.value = false
    }
  }

  async function refreshLocalBookInfo(book) {
    if (!book?.id) return
    const operation = operations.begin('refresh-local-book')
    refreshingBookId.value = book.id
    try {
      const { data } = await options.refreshLocalBook(book.id)
      if (!operations.canCommit(operation)) return
      await invalidateBookReaderCaches(book, { clearBrowser: true })
      if (!operations.canCommit(operation)) return
      const updatedBook = data?.book || data
      if (updatedBook?.id) {
        const mergedBook = mergedShelfBook(updatedBook)
        const chapters = await refreshBookChaptersCache(mergedBook)
        if (!operations.canCommit(operation)) return
        applyUpdatedBookToOverlay(mergedBook, chapters)
      } else {
        await options.bookshelf.loadBooks({ force: true, all: true })
        if (!operations.canCommit(operation)) return
      }
      options.onSuccess('更新成功')
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '更新失败')
      }
    } finally {
      if (operations.canCommit(operation)) refreshingBookId.value = null
    }
  }

  async function uploadBookInfoCover(file) {
    const book = options.overlay.bookInfoBook
    if (!book?.id || !file) return
    const operation = operations.begin('upload-cover')
    coverUploadingBookId.value = book.id
    try {
      const { data: uploadResult } = await options.uploadAsset({
        file,
        type: 'cover',
      })
      if (!operations.canCommit(operation)) return
      const { data: updatedBook } = await options.updateBook(book.id, {
        customCoverUrl: uploadResult.url,
      })
      if (!operations.canCommit(operation)) return
      applyUpdatedBookToOverlay(updatedBook)
      options.onSuccess('操作成功')
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '操作失败')
      }
    } finally {
      if (operations.canCommit(operation)) coverUploadingBookId.value = null
    }
  }

  async function toggleBookCanUpdate(value) {
    const book = options.overlay.bookInfoBook
    if (!book?.id || !book.sourceId) return
    const operation = operations.begin('toggle-book-update')
    updatingBookId.value = book.id
    try {
      const { data: updatedBook } = await options.updateBook(book.id, {
        canUpdate: value,
      })
      if (!operations.canCommit(operation)) return
      applyUpdatedBookToOverlay(updatedBook)
      options.onSuccess('操作成功')
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '操作失败')
      }
    } finally {
      if (operations.canCommit(operation)) updatingBookId.value = null
    }
  }

  return {
    ...cacheState,
    refreshingBookId,
    coverUploadingBookId,
    updatingBookId,
    editingBookSaving,
    saveEditedBook,
    refreshLocalBookInfo,
    uploadBookInfoCover,
    toggleBookCanUpdate,
    resetOperations: operations.reset,
  }
}

function editableBookMetadata(payload = {}) {
  return {
    title: String(payload.title || '').trim(),
    author: String(payload.author || '').trim(),
    customCoverUrl: String(payload.customCoverUrl || '').trim(),
    intro: String(payload.intro || ''),
  }
}
