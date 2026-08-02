<template>
  <BookInfoDialog
    v-model="overlay.bookInfoVisible"
    :book="overlay.bookInfoBook"
    :source-name="bookInfoSourceName"
    :category-name="bookInfoCategory"
    :cover-editable="bookInfoInShelf"
    :cover-uploading="coverUploadingBookId === overlay.bookInfoBook?.id"
    :can-update="overlay.bookInfoBook?.canUpdate !== false"
    :update-switch-loading="updatingBookId === overlay.bookInfoBook?.id"
    :in-shelf="bookInfoInShelf"
    :show-local-refresh-action="bookInfoInShelf && isLocalBookInfo(overlay.bookInfoBook)"
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
import { useRemoteBookAddToShelf } from '../../composables/useRemoteBookAddToShelf'
import { useAuthenticatedOperationGuard } from '../../composables/useAuthenticatedOperationGuard'
import { listSources } from '../../api/sources'
import { uploadAsset } from '../../api/uploads'
import { useOverlayBookInfo } from '../../composables/useOverlayBookInfo'
import { mergeShelfBook, useBookshelfStore } from '../../stores/bookshelf'
import { useOverlayStore } from '../../stores/overlay'
import { useReaderStore } from '../../stores/reader'
import {
  clearBookBrowserChapterCache,
} from '../../utils/bookChapterCache'
import { createBookCategoryNameResolver } from '../../utils/bookCategory'
import { bookInfoURL, findShelfBookByURL } from '../../utils/bookInfoIdentity'
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
const operations = useAuthenticatedOperationGuard()
const categoryName = createBookCategoryNameResolver(() => bookshelf.categories)
const sourceRows = ref([])
const managedBooks = computed(() => (
  sortByShelfOrder(bookshelf.books, reader.progressByBook)
))
let sourceRowsRefreshTimer

const bookInfoCategory = computed(() => (
  String(overlay.bookInfoOptions.categoryName || categoryName(overlay.bookInfoBook)).replaceAll('、', ',')
))
const bookInfoSourceName = computed(() => {
  if (overlay.bookInfoOptions.sourceName) return overlay.bookInfoOptions.sourceName
  const sourceId = overlay.bookInfoBook?.sourceId
  if (sourceId) {
    return sourceRows.value
      .find(source => Number(source.id) === Number(sourceId))
      ?.name || overlay.bookInfoBook?.sourceName || overlay.bookInfoBook?.originName || '未知书源'
  }
  return isLocalBookInfo(overlay.bookInfoBook) ? '本地' : '未知书源'
})
const bookInfoInShelf = computed(() => isShelfBook(overlay.bookInfoBook))
const addToShelf = useRemoteBookAddToShelf({
  operationGuard: operations,
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
  saveEditedBook,
  refreshLocalBookInfo,
  uploadBookInfoCover,
  toggleBookCanUpdate,
} = useOverlayBookInfo({
  operationGuard: operations,
  overlay,
  bookshelf,
  getManagedBooks: () => managedBooks.value,
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
    const operation = operations.begin('open-book-info')
    const warmTasks = [
      bookshelf.ensureCategoriesLoaded(),
      bookshelf.ensureBooksLoaded({ all: true }),
    ]
    const [categoryResult, booksResult] = await Promise.allSettled(warmTasks)
    if (!operations.canCommit(operation)) return
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
      await loadSourceRows(operation).catch((error) => {
        if (!operations.canCommit(operation)) return
        ElMessage.warning(
          readError(error, '书源加载失败，书籍信息仍可查看'),
        )
      })
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
  return Boolean(findShelfBook(book))
}

function findShelfBook(book) {
  return findShelfBookByURL(book, bookshelf.books, {
    allowIdFallback: !bookInfoURL(book),
  })
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

function isLocalBookInfo(book) {
  const origin = String(book?.origin || '').trim()
  if (['loc_book', 'local'].includes(origin)) return true
  if (origin) return false
  return Number(book?.sourceId || 0) <= 0
}

function bookInfoKey(book) {
  return remoteBookKey(book || {})
}

async function addBookInfoToShelf() {
  const operation = operations.begin('finish-add-book-info')
  const currentBook = overlay.bookInfoBook
  if (!canAddBookInfoToShelf.value || !currentBook) return
  const addedBook = await addToShelf.addRemoteBook(currentBook, {
    key: bookInfoKey(currentBook),
    sourceId: currentBook.sourceId,
    sourceName: bookInfoSourceName.value,
  })
  if (!addedBook || !operations.canCommit(operation)) return
  overlay.bookInfoBook = addedBook
  overlay.bookInfoOptions = {
    ...overlay.bookInfoOptions,
    categoryName: categoryName(addedBook).replaceAll('、', ','),
  }
}

function setBookGroup(book) {
  overlay.openBookGroup('set', book, {
    categoryName: categoryName(book).replaceAll('、', ','),
    progress: bookProgress(book)?.percent || 0,
  })
}

function bookProgress(book) {
  return newestBookProgress(book, reader.progressByBook)
}

async function loadSourceRows(parentOperation = null) {
  if (parentOperation && !operations.canCommit(parentOperation)) return
  const operation = operations.begin('load-book-info-sources')
  const { data } = await listSources()
  if (!operations.canCommit(operation)) return
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
