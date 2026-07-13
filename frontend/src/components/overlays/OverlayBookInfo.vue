<template>
  <BookInfoDialog
    v-model="overlay.bookInfoVisible"
    :book="overlay.bookInfoBook"
    :source-name="bookInfoSourceName"
    :category-name="bookInfoCategory"
    :progress="bookInfoProgress"
    :chapters="overlay.bookInfoBook?.chapterCount || 0"
    :status-label="overlay.bookInfoOptions.statusLabel || sourceStatusLabel"
    :status-type="overlay.bookInfoOptions.statusType || 'info'"
    :cover-editable="bookInfoInShelf"
    :cover-uploading="coverUploadingBookId === overlay.bookInfoBook?.id"
    :show-update-switch="bookInfoInShelf && Number(overlay.bookInfoBook?.sourceId || 0) > 0"
    :can-update="overlay.bookInfoBook?.canUpdate !== false"
    :update-switch-loading="updatingBookId === overlay.bookInfoBook?.id"
    :browser-cache-count="bookInfoBrowserCacheCount"
    :in-shelf="bookInfoInShelf"
    :show-category-action="bookInfoInShelf"
    :show-local-refresh-action="bookInfoInShelf && Number(overlay.bookInfoBook?.sourceId || 0) <= 0"
    :local-refresh-loading="refreshingBookId === overlay.bookInfoBook?.id"
    :show-add-action="canAddBookInfoToShelf"
    :add-loading="addingBookInfoToShelf"
    @cover-upload="uploadBookInfoCover"
    @can-update-change="toggleBookCanUpdate"
    @category-action="setBookGroup(overlay.bookInfoBook)"
    @local-refresh="refreshLocalBookInfo(overlay.bookInfoBook)"
    @add="addBookInfoToShelf"
  />

  <BookEditDialog
    v-model="overlay.bookEditVisible"
    :book="overlay.bookEditBook"
    :saving="editingBookSaving"
    @save="saveEditedBook"
  />
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { createRemoteBook, listChapters, refreshLocalBook, updateBook } from '../../api/books'
import { useBookInfoAddToShelf } from '../../composables/useBookInfoAddToShelf'
import { listSources } from '../../api/sources'
import { uploadAsset } from '../../api/uploads'
import { useOverlayBookInfo } from '../../composables/useOverlayBookInfo'
import { mergeShelfBook, useBookshelfStore } from '../../stores/bookshelf'
import { useOverlayStore } from '../../stores/overlay'
import { useReaderStore } from '../../stores/reader'
import {
  clearBookBrowserChapterCache,
  countBooksBrowserCachedChapters,
  listBookBrowserCachedChapters,
} from '../../utils/bookChapterCache'
import { createBookCategoryNameResolver } from '../../utils/bookCategory'
import { newestBookProgress, sortByShelfOrder } from '../../utils/bookOrder'
import { remoteBookCreatePayload, remoteBookKey } from '../../utils/remoteBookResult'
import {
  invalidateReaderDataCache,
  writeReaderDataCache,
} from '../../utils/readerDataCache'
import BookEditDialog from '../BookEditDialog.vue'
import BookInfoDialog from '../BookInfoDialog.vue'

const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const reader = useReaderStore()
const categoryName = createBookCategoryNameResolver(() => bookshelf.categories)
const sourceRows = ref([])
const managedBooks = computed(() => (
  sortByShelfOrder(bookshelf.books, reader.progressByBook)
))
let sourceRowsRefreshTimer

const bookInfoCategory = computed(() => (
  overlay.bookInfoOptions.categoryName || categoryName(overlay.bookInfoBook)
))
const bookInfoSourceName = computed(() => {
  if (overlay.bookInfoOptions.sourceName) return overlay.bookInfoOptions.sourceName
  const sourceId = overlay.bookInfoBook?.sourceId
  if (!sourceId) return '本地'
  return sourceRows.value
    .find(source => Number(source.id) === Number(sourceId))
    ?.name || '远程书籍'
})
const bookInfoProgress = computed(() => (
  bookProgress(overlay.bookInfoBook)?.percent || 0
))
const bookInfoBrowserCacheCount = computed(() => (
  overlay.bookInfoBook?.id ? localCacheCount(overlay.bookInfoBook) : -1
))
const bookInfoInShelf = computed(() => isShelfBook(overlay.bookInfoBook))
const sourceStatusLabel = computed(() => (
  overlay.bookInfoBook?.sourceId ? '远程书籍' : '本地书籍'
))
const addToShelf = useBookInfoAddToShelf({
  selectCategories: initialCategoryIds => overlay.selectBookAddCategories(initialCategoryIds),
  buildPayload: (book, categoryIds, context) => remoteBookCreatePayload(book, categoryIds, context),
  createRemoteBook,
  upsertBook: book => bookshelf.upsertBook(book),
  onSuccess: message => ElMessage.success(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})
const addingBookInfoToShelf = computed(() => (
  addToShelf.addingBookKey.value === bookInfoKey(overlay.bookInfoBook)
))
const canAddBookInfoToShelf = computed(() => (
  !bookInfoInShelf.value && isRemoteBookInfo(overlay.bookInfoBook)
))

const {
  refreshingBookId,
  coverUploadingBookId,
  updatingBookId,
  editingBookSaving,
  refreshBookInfoBrowserCacheCount,
  localCacheCount,
  saveEditedBook,
  refreshLocalBookInfo,
  uploadBookInfoCover,
  toggleBookCanUpdate,
} = useOverlayBookInfo({
  overlay,
  bookshelf,
  getManagedBooks: () => managedBooks.value,
  countBrowserCachedChapters: countBooksBrowserCachedChapters,
  listBrowserCachedChapters: listBookBrowserCachedChapters,
  clearBrowserChapterCache: clearBookBrowserChapterCache,
  invalidateReaderData: invalidateReaderDataCache,
  listChapters,
  writeReaderData: writeReaderDataCache,
  refreshLocalBook,
  uploadAsset,
  updateBook,
  mergeBook: mergeShelfBook,
  emitBookInfoUpdated: book => {
    window.dispatchEvent(new CustomEvent('openreader:book-info-updated', {
      detail: { book },
    }))
  },
  emitReaderBookDataUpdated: detail => {
    window.dispatchEvent(new CustomEvent(
      'openreader:reader-book-data-updated',
      { detail },
    ))
  },
  onSuccess: message => ElMessage.success(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})

watch(
  () => overlay.bookInfoVisible,
  async (visible) => {
    if (!visible) return
    const warmTasks = [
      bookshelf.ensureCategoriesLoaded(),
      bookshelf.ensureBooksLoaded({ all: true }),
    ]
    const [categoryResult, booksResult] = await Promise.allSettled(warmTasks)
    if (categoryResult.status === 'rejected') {
      ElMessage.warning(
        readError(categoryResult.reason, '分组加载失败，书籍信息仍可查看'),
      )
    }
    if (booksResult?.status === 'rejected') {
      ElMessage.warning(
        readError(booksResult.reason, '书架状态加载失败，书籍信息仍可查看'),
      )
    }
    resolveBookInfoShelfRecord()
    if (overlay.bookInfoBook?.sourceId && !sourceRows.value.length) {
      await loadSourceRows().catch((error) => {
        ElMessage.warning(
          readError(error, '书源加载失败，书籍信息仍可查看'),
        )
      })
    }
    if (overlay.bookInfoBook?.id) {
      await refreshBookInfoBrowserCacheCount(overlay.bookInfoBook)
    }
  },
)

onMounted(() => {
  window.addEventListener('openreader:sources-update', handleSourcesUpdated)
})

onBeforeUnmount(() => {
  window.removeEventListener('openreader:sources-update', handleSourcesUpdated)
  clearSourceRowsRefreshTimer()
})

function isShelfBook(book) {
  if (!book) return false
  if (
    book.id &&
    bookshelf.books.some(item => Number(item.id) === Number(book.id))
  ) {
    return true
  }
  const bookUrl = String(book.url || book.bookUrl || '').trim()
  if (!bookUrl) return false
  return bookshelf.books.some(item => (
    String(item.url || item.bookUrl || '').trim() === bookUrl
  ))
}

function findShelfBook(book) {
  if (!book) return null
  if (book.id) {
    const byID = bookshelf.books.find(item => Number(item.id) === Number(book.id))
    if (byID) return byID
  }
  const bookURL = String(book.url || book.bookUrl || '').trim()
  if (!bookURL) return null
  return bookshelf.books.find(item => (
    String(item.url || item.bookUrl || '').trim() === bookURL
  )) || null
}

function resolveBookInfoShelfRecord() {
  const shelfBook = findShelfBook(overlay.bookInfoBook)
  if (!shelfBook) return
  if (overlay.bookInfoBook !== shelfBook) {
    overlay.bookInfoBook = shelfBook
  }
}

function isRemoteBookInfo(book) {
  return Boolean(
    Number(book?.sourceId || 0) > 0
    && String(book?.url || book?.bookUrl || '').trim(),
  )
}

function bookInfoKey(book) {
  return remoteBookKey(book || {})
}

async function addBookInfoToShelf() {
  const currentBook = overlay.bookInfoBook
  if (!canAddBookInfoToShelf.value || !currentBook) return
  const addedBook = await addToShelf.addRemoteBook(currentBook, {
    key: bookInfoKey(currentBook),
    sourceId: currentBook.sourceId,
    sourceName: bookInfoSourceName.value,
  })
  if (!addedBook) return
  overlay.bookInfoBook = addedBook
  overlay.bookInfoOptions = {
    ...overlay.bookInfoOptions,
    categoryName: categoryName(addedBook),
    statusLabel: '已加入书架',
    statusType: 'success',
  }
}

function setBookGroup(book) {
  overlay.openBookGroup('set', book, {
    categoryName: categoryName(book),
    progress: bookProgress(book)?.percent || 0,
  })
}

function bookProgress(book) {
  return newestBookProgress(book, reader.progressByBook)
}

async function loadSourceRows() {
  const { data } = await listSources()
  sourceRows.value = data || []
}

function handleSourcesUpdated() {
  if (!shouldRefreshSources()) return
  clearSourceRowsRefreshTimer()
  sourceRowsRefreshTimer = window.setTimeout(async () => {
    sourceRowsRefreshTimer = undefined
    try {
      await loadSourceRows()
    } catch {
      // Preserve the last source name; a later source action can recover.
    }
  }, 350)
}

function shouldRefreshSources() {
  return (
    overlay.bookInfoVisible &&
    Number(overlay.bookInfoBook?.sourceId || 0) > 0
  ) || sourceRows.value.length > 0
}

function clearSourceRowsRefreshTimer() {
  if (!sourceRowsRefreshTimer) return
  window.clearTimeout(sourceRowsRefreshTimer)
  sourceRowsRefreshTimer = undefined
}

function readError(error, fallback) {
  return error?.response?.data?.error?.message ||
    error?.response?.data?.error ||
    fallback
}
</script>
