<template>
  <el-drawer
    v-model="overlay.bookManageVisible"
    title="书架管理"
    :direction="direction"
    :size="size"
    class="global-manage-drawer"
  >
    <div class="manage-head">
      <el-input
        v-model="manageKeyword"
        placeholder="搜索书名、作者或文件名"
        clearable
        size="small"
      />
      <div class="manage-head-actions">
        <el-button size="small" text @click="selectAllManagedBooks">
          全选
        </el-button>
        <el-button size="small" text @click="clearManagedSelection">
          清空
        </el-button>
      </div>
    </div>

    <BookManagementDesktopTable
      :books="filteredManagedBooks"
      :caching-book-id="cachingBookId"
      :category-name="categoryName"
      :progress-label="progressLabel"
      :server-cache-count="serverCacheCount"
      :local-cache-count="localCacheCount"
      @selection-change="onManageSelectionChange"
      @open-info="overlay.openBookInfo"
      @open-edit="overlay.openBookEdit"
      @set-group="setBookGroup"
      @cache="cacheBook"
      @export="exportBook"
    />

    <BookManagementMobileList
      :books="filteredManagedBooks"
      :selected-book-ids="selectedBookIds"
      :caching-book-id="cachingBookId"
      :category-name="categoryName"
      :progress-label="progressLabel"
      :server-cache-count="serverCacheCount"
      :local-cache-count="localCacheCount"
      @toggle-selection="toggleManagedBook"
      @open-info="overlay.openBookInfo"
      @open-edit="overlay.openBookEdit"
      @set-group="setBookGroup"
      @cache="cacheBook"
      @export="exportBook"
    />

    <div class="manage-footer">
      <el-button
        type="primary"
        :disabled="!selectedBookIds.length"
        :loading="batchBusy"
        @click="batchDeleteBooks"
      >
        批量删除
      </el-button>
      <el-dropdown @command="batchAddCategory">
        <el-button
          type="primary"
          :disabled="!selectedBookIds.length"
          :loading="batchBusy"
        >
          批量添加分组
          <el-icon class="el-icon--right"><ArrowDown /></el-icon>
        </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item
              v-for="category in bookshelf.categories"
              :key="category.id"
              :command="category"
            >
              {{ category.name }}
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <el-dropdown @command="batchRemoveCategory">
        <el-button
          type="primary"
          :disabled="!selectedBookIds.length"
          :loading="batchBusy"
        >
          批量移除分组
          <el-icon class="el-icon--right"><ArrowDown /></el-icon>
        </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item
              v-for="category in bookshelf.categories"
              :key="category.id"
              :command="category"
            >
              {{ category.name }}
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <span class="check-tip">已选择 {{ selectedBookIds.length }} 个</span>
      <el-dropdown @command="handleBatchMoreCommand">
        <el-button
          :disabled="!selectedBookIds.length"
          :loading="batchBusy"
        >
          更多批量操作
          <el-icon class="el-icon--right"><ArrowDown /></el-icon>
        </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="cache">
              批量缓存到服务器
            </el-dropdown-item>
            <el-dropdown-item command="clear-cache">
              批量清服务器缓存
            </el-dropdown-item>
            <el-dropdown-item command="export">
              批量导出
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <el-button @click="overlay.bookManageVisible = false">取消</el-button>
    </div>
  </el-drawer>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { ArrowDown } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { cacheBookContent, listChapters } from '../../api/books'
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
import BookManagementDesktopTable from './BookManagementDesktopTable.vue'
import BookManagementMobileList from './BookManagementMobileList.vue'

defineProps({
  direction: {
    type: String,
    required: true,
  },
  size: {
    type: [String, Number],
    required: true,
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
  cachingBookId,
  onManageSelectionChange,
  toggleManagedBook,
  selectAllManagedBooks,
  clearManagedSelection,
  batchAddCategory,
  batchRemoveCategory,
  batchDeleteBooks,
  handleBatchMoreCommand,
  cacheBook,
  exportBook,
} = useOverlayBookManagement({
  bookshelf,
  getManagedBooks: () => managedBooks.value,
  getFilteredManagedBooks: () => filteredManagedBooks.value,
  getBookProgress: bookProgress,
  cacheBookContent,
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
.manage-footer {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.manage-head {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  margin-bottom: 10px;
}

.manage-head-actions {
  display: none;
  flex: 0 0 auto;
  gap: 6px;
}

.manage-footer {
  align-items: center;
  padding-top: 10px;
  border-top: 1px solid var(--app-border);
}

.check-tip {
  color: var(--app-text-muted);
  font-size: 13px;
}

@media (max-width: 750px) {
  .manage-footer {
    align-items: stretch;
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
  }

  .manage-footer :deep(.el-button),
  .manage-footer :deep(.el-dropdown),
  .manage-footer :deep(.el-dropdown .el-button) {
    width: 100%;
    min-height: 38px;
    margin-left: 0;
  }

  .manage-footer .check-tip {
    grid-column: 1 / -1;
    order: -1;
  }

  .manage-head {
    grid-template-columns: 1fr;
  }

  .manage-head-actions {
    display: flex;
    justify-content: flex-end;
  }
}
</style>
