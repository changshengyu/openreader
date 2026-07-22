import { ref } from 'vue'
import { useOverlayBookCacheState } from './useOverlayBookCacheState.js'

export function useOverlayBookInfo(options) {
  const refreshingBookId = ref(null)
  const coverUploadingBookId = ref(null)
  const updatingBookId = ref(null)
  const editingBookSaving = ref(false)
  const cacheState = useOverlayBookCacheState(options)
  const {
    refreshBookInfoBrowserCacheCount,
    invalidateBookReaderCaches,
    refreshBookChaptersCache,
    mergedShelfBook,
    applyUpdatedBookToOverlay,
  } = cacheState

  async function saveEditedBook(payload) {
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
        customCoverUrl: uploadResult.url,
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
    ...cacheState,
    refreshingBookId,
    coverUploadingBookId,
    updatingBookId,
    editingBookSaving,
    saveEditedBook,
    refreshLocalBookInfo,
    uploadBookInfoCover,
    toggleBookCanUpdate,
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
