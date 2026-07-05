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

    <el-table
      :data="filteredManagedBooks"
      row-key="id"
      height="calc(100vh - 188px)"
      class="manage-table desktop-manage-table"
      @selection-change="onManageSelectionChange"
    >
      <el-table-column type="selection" width="42" />
      <el-table-column
        prop="title"
        label="书名"
        min-width="180"
        show-overflow-tooltip
      >
        <template #default="{ row }">
          <el-button
            text
            class="text-button"
            @click="overlay.openBookInfo(row)"
          >
            {{ row.title }}
          </el-button>
        </template>
      </el-table-column>
      <el-table-column
        prop="author"
        label="作者"
        min-width="120"
        show-overflow-tooltip
      />
      <el-table-column label="分组" min-width="120">
        <template #default="{ row }">{{ categoryName(row) }}</template>
      </el-table-column>
      <el-table-column label="章节" min-width="150">
        <template #default="{ row }">
          <span>共 {{ row.chapterCount || 0 }} 章</span><br>
          <span>阅读进度：{{ progressLabel(row) }}</span>
          <template v-if="Number(row.sourceId || 0) > 0">
            <br><span>服务器缓存：{{ serverCacheCount(row) }} 章</span>
          </template>
          <br><span>浏览器缓存：{{ localCacheCount(row) }} 章</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="150" fixed="right">
        <template #default="{ row }">
          <el-button
            text
            class="text-button"
            @click="overlay.openBookEdit(row)"
          >
            编辑
          </el-button>
          <el-button text class="text-button" @click="setBookGroup(row)">
            分组
          </el-button>
          <el-dropdown @command="cacheBook(row, $event)">
            <el-button
              text
              class="text-button"
              :loading="cachingBookId === row.id"
            >
              缓存<el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="cacheBookLocal">
                  缓存到浏览器
                </el-dropdown-item>
                <el-dropdown-item
                  v-if="Number(row.sourceId || 0) > 0"
                  command="cacheBook"
                >
                  缓存到服务器
                </el-dropdown-item>
                <el-dropdown-item command="deleteBookLocalCache">
                  删除浏览器缓存
                </el-dropdown-item>
                <el-dropdown-item
                  v-if="Number(row.sourceId || 0) > 0"
                  command="deleteBookCache"
                >
                  删除服务器缓存
                </el-dropdown-item>
                <el-dropdown-item
                  v-if="Number(row.sourceId || 0) === 0"
                  disabled
                >
                  本地书无需服务器缓存
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-dropdown @command="exportBook(row, $event)">
            <el-button text class="text-button">
              导出<el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="txt">导出为 TXT</el-dropdown-item>
                <el-dropdown-item command="epub">
                  导出为 Epub
                </el-dropdown-item>
                <el-dropdown-item command="json">
                  导出书籍数据
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </template>
      </el-table-column>
    </el-table>

    <div v-if="filteredManagedBooks.length" class="mobile-manage-list">
      <article
        v-for="book in filteredManagedBooks"
        :key="book.id"
        class="mobile-manage-card"
        :class="{ selected: selectedBookIds.includes(book.id) }"
      >
        <header>
          <el-checkbox
            :model-value="selectedBookIds.includes(book.id)"
            @change="value => toggleManagedBook(book.id, value)"
          />
          <span
            class="mobile-manage-cover"
            :class="{ 'has-cover': hasBookCover(book) }"
            :style="coverStyle(book)"
          >{{ coverInitial(book) }}</span>
          <button type="button" @click="overlay.openBookInfo(book)">
            <strong>{{ book.title }}</strong>
            <span>
              {{ book.author || '未知作者' }} · {{ categoryName(book) }}
            </span>
            <span>
              {{ Number(book.sourceId || 0) > 0 ? '远程书籍' : '本地书籍' }}
              · {{ progressLabel(book) }}
            </span>
          </button>
        </header>
        <p>
          共 {{ book.chapterCount || 0 }} 章
          <template v-if="Number(book.sourceId || 0) > 0">
            · 服务器缓存 {{ serverCacheCount(book) }} 章
          </template>
          · 浏览器缓存 {{ localCacheCount(book) }} 章
          <template v-if="book.lastChapter">
            · 最新：{{ book.lastChapter }}
          </template>
        </p>
        <footer>
          <el-button size="small" text @click="overlay.openBookEdit(book)">
            编辑
          </el-button>
          <el-button size="small" text @click="setBookGroup(book)">
            分组
          </el-button>
          <el-dropdown @command="cacheBook(book, $event)">
            <el-button
              size="small"
              text
              :loading="cachingBookId === book.id"
            >
              缓存<el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="cacheBookLocal">
                  缓存到浏览器
                </el-dropdown-item>
                <el-dropdown-item
                  v-if="Number(book.sourceId || 0) > 0"
                  command="cacheBook"
                >
                  缓存到服务器
                </el-dropdown-item>
                <el-dropdown-item command="deleteBookLocalCache">
                  删除浏览器缓存
                </el-dropdown-item>
                <el-dropdown-item
                  v-if="Number(book.sourceId || 0) > 0"
                  command="deleteBookCache"
                >
                  删除服务器缓存
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-dropdown @command="exportBook(book, $event)">
            <el-button size="small" text>
              导出<el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="txt">导出为 TXT</el-dropdown-item>
                <el-dropdown-item command="epub">
                  导出为 Epub
                </el-dropdown-item>
                <el-dropdown-item command="json">
                  导出书籍数据
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </footer>
      </article>
    </div>
    <el-empty
      v-else
      class="mobile-manage-empty"
      description="没有匹配的书籍"
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
import { bookCoverUrl, hasBookCover } from '../../utils/bookCover'
import { localBookSearchText, normalizeLocalBookSearch } from '../../utils/localBook'
import { newestBookProgress, sortByShelfOrder } from '../../utils/bookOrder'

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

function coverInitial(book) {
  if (hasBookCover(book)) return ''
  return (book?.title || '?').slice(0, 1)
}

function coverStyle(book) {
  const url = bookCoverUrl(book)
  return url ? { backgroundImage: `url(${url})` } : {}
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

.manage-table {
  margin-bottom: 12px;
}

.mobile-manage-list {
  display: none;
}

.mobile-manage-card {
  display: grid;
  gap: 8px;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.mobile-manage-card.selected {
  border-color: var(--app-primary);
  background: var(--app-primary-soft);
}

.mobile-manage-card header,
.mobile-manage-card footer {
  display: flex;
  align-items: center;
  gap: 8px;
}

.mobile-manage-card header button {
  display: grid;
  min-width: 0;
  flex: 1;
  gap: 3px;
  padding: 0;
  color: var(--app-text);
  background: transparent;
  border: 0;
  cursor: pointer;
  text-align: left;
}

.mobile-manage-cover {
  display: grid;
  width: 34px;
  height: 46px;
  place-items: center;
  flex: 0 0 34px;
  color: #fffdf8;
  background: var(--app-primary);
  border-radius: 4px;
  font-size: 16px;
  font-weight: 800;
}

.mobile-manage-cover.has-cover {
  background-position: center;
  background-size: cover;
  color: transparent;
}

.mobile-manage-card strong,
.mobile-manage-card span,
.mobile-manage-card p {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-manage-card strong {
  font-size: 14px;
}

.mobile-manage-card span,
.mobile-manage-card p {
  color: var(--app-text-muted);
  font-size: 12px;
}

.mobile-manage-card p {
  margin: 0;
}

.mobile-manage-card footer {
  flex-wrap: wrap;
  justify-content: flex-end;
}

.mobile-manage-empty {
  display: none;
}

.text-button {
  padding: 0;
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
  .desktop-manage-table {
    display: none;
  }

  .mobile-manage-list {
    display: grid;
    max-height: calc(100vh - 220px);
    overflow: auto;
    gap: 10px;
    margin-bottom: 12px;
  }

  .mobile-manage-empty {
    display: block;
  }

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
