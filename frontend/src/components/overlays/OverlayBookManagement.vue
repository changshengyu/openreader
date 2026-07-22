<template>
  <el-dialog
    v-model="overlay.bookManageVisible"
    title="书架管理"
    width="min(1180px, calc(100vw - 48px))"
    :fullscreen="isMobile"
    destroy-on-close
    class="global-book-manage-dialog"
  >
    <template #header>
      <span class="book-manage-title">
        <span>书架管理</span>
        <span class="book-manage-cache-tip">❗️只能缓存文本内容</span>
      </span>
    </template>
    <section class="book-manage-dialog-body">
      <BookManagementToolbar
        v-model="manageKeyword"
        @select-all="selectAllManagedBooks"
        @clear-selection="clearManagedSelection"
      />

      <BookManagementDesktopTable
        :books="filteredManagedBooks"
        :is-caching-book="isCachingBook"
        :cache-progress-label="cacheProgressLabel"
        :category-name="categoryName"
        :progress-label="progressLabel"
        :server-cache-count="serverCacheCount"
        :local-cache-count="localCacheCount"
        @selection-change="onManageSelectionChange"
        @open-info="overlay.openBookInfo"
        @open-edit="overlay.openBookEdit"
        @set-group="setBookGroup"
        @cache="cacheBook"
        @cancel-cache="cancelBookCache"
        @export="exportBook"
      />

      <BookManagementMobileList
        :books="filteredManagedBooks"
        :selected-book-ids="selectedBookIds"
        :is-caching-book="isCachingBook"
        :cache-progress-label="cacheProgressLabel"
        :category-name="categoryName"
        :progress-label="progressLabel"
        :server-cache-count="serverCacheCount"
        :local-cache-count="localCacheCount"
        @toggle-selection="toggleManagedBook"
        @open-info="overlay.openBookInfo"
        @open-edit="overlay.openBookEdit"
        @set-group="setBookGroup"
        @cache="cacheBook"
        @cancel-cache="cancelBookCache"
        @export="exportBook"
      />

      <BookManagementBatchFooter
        :categories="bookshelf.categories"
        :selected-count="selectedBookIds.length"
        :busy="batchBusy"
        @delete-selected="batchDeleteBooks"
        @add-category="batchAddCategory"
        @remove-category="batchRemoveCategory"
        @close="overlay.bookManageVisible = false"
      />
    </section>
  </el-dialog>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { cacheBookContent, cacheBookContentStream, listChapters } from '../../api/books'
import { useOverlayBookCacheState } from '../../composables/useOverlayBookCacheState'
import { useOverlayBookManagement } from '../../composables/useOverlayBookManagement'
import { useBookshelfStore } from '../../stores/bookshelf'
import { useOverlayStore } from '../../stores/overlay'
import { useReaderStore } from '../../stores/reader'
import {
  cacheBookChaptersToBrowser,
  clearBookBrowserChapterCache,
  countBooksBrowserCachedChapters,
} from '../../utils/bookChapterCache'
import { createBookCategoryNameResolver } from '../../utils/bookCategory'
import { localBookSearchText, normalizeLocalBookSearch } from '../../utils/localBook'
import { newestBookProgress, sortByShelfOrder } from '../../utils/bookOrder'
import BookManagementBatchFooter from './BookManagementBatchFooter.vue'
import BookManagementDesktopTable from './BookManagementDesktopTable.vue'
import BookManagementMobileList from './BookManagementMobileList.vue'
import BookManagementToolbar from './BookManagementToolbar.vue'

defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const reader = useReaderStore()
const categoryName = createBookCategoryNameResolver(() => bookshelf.categories)
const manageKeyword = ref('')
const managedBooks = computed(() => (
  sortByShelfOrder(bookshelf.books, reader.progressByBook)
))
const filteredManagedBooks = computed(() => {
  const value = normalizeLocalBookSearch(manageKeyword.value)
  if (!value) return managedBooks.value
  return managedBooks.value.filter(book => manageBookSearchText(book).includes(value))
})

const {
  refreshManagedBrowserCacheCounts,
  localCacheCount,
  serverCacheCount,
  updateServerCacheCount,
} = useOverlayBookCacheState({
  overlay,
  bookshelf,
  getManagedBooks: () => managedBooks.value,
  countBrowserCachedChapters: countBooksBrowserCachedChapters,
})

const {
  selectedBookIds,
  batchBusy,
  cacheProgressLabel,
  isCachingBook,
  onManageSelectionChange,
  toggleManagedBook,
  selectAllManagedBooks,
  clearManagedSelection,
  pruneManagedSelection,
  batchAddCategory,
  batchRemoveCategory,
  batchDeleteBooks,
  cacheBook,
  cancelBookCache,
  exportBook,
} = useOverlayBookManagement({
  bookshelf,
  getManagedBooks: () => managedBooks.value,
  getFilteredManagedBooks: () => filteredManagedBooks.value,
  getBookProgress: bookProgress,
  cacheBookContent,
  cacheBookContentStream,
  listChapters,
  cacheBrowserChapters: cacheBookChaptersToBrowser,
  clearBrowserChapterCache: clearBookBrowserChapterCache,
  updateServerCacheCount,
  refreshManagedBrowserCacheCounts,
  saveBlob: downloadBlob,
  confirm: (...args) => ElMessageBox.confirm(...args),
  now: () => Date.now(),
  onSuccess: message => ElMessage.success(message),
  onInfo: message => ElMessage.info(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})

watch(
  () => overlay.bookManageVisible,
  async (visible) => {
    if (!visible) {
      manageKeyword.value = ''
      clearManagedSelection()
      return
    }
    const [categoryResult, booksResult] = await Promise.allSettled([
      bookshelf.ensureCategoriesLoaded(),
      bookshelf.ensureBooksLoaded({ all: true }),
    ])
    if (booksResult.status === 'rejected') {
      ElMessage.error(readError(booksResult.reason, '加载书架数据失败'))
      return
    }
    if (categoryResult.status === 'rejected') {
      ElMessage.warning(
        readError(categoryResult.reason, '分组加载失败，书架管理仍可使用'),
      )
    }
    await refreshManagedBrowserCacheCounts()
  },
)

watch(
  () => managedBooks.value.map(book => Number(book.id)),
  bookIds => pruneManagedSelection(bookIds),
  { flush: 'sync' },
)

function manageBookSearchText(book) {
  return localBookSearchText(book, [
    progressLabel(book),
    categoryName(book),
  ])
}

function progressLabel(book) {
  return `${Math.round((bookProgress(book)?.percent || 0) * 100)}%`
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

function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

function readError(error, fallback) {
  return error?.response?.data?.error?.message ||
    error?.response?.data?.error ||
    fallback
}
</script>

<style scoped>
.book-manage-dialog-body {
  display: grid;
  min-width: 0;
  gap: 12px;
}

.book-manage-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding-right: 32px;
}

.book-manage-cache-tip {
  color: var(--app-text-muted);
  font-size: 13px;
  font-weight: 400;
}

@media (max-width: 750px) {
  .book-manage-title {
    align-items: flex-start;
    flex-direction: column;
    gap: 2px;
  }
}
</style>
