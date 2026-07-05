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
    @cover-upload="uploadBookInfoCover"
    @can-update-change="toggleBookCanUpdate"
    @category-action="setBookGroup(overlay.bookInfoBook)"
    @local-refresh="refreshLocalBookInfo(overlay.bookInfoBook)"
  >
    <div v-if="overlay.bookInfoOptions.actions?.length" class="overlay-actions">
      <el-button
        v-for="action in overlay.bookInfoOptions.actions"
        :key="action.label"
        :type="action.type || 'default'"
        :plain="action.plain"
        :loading="!!action.loading"
        :disabled="!!action.disabled"
        @click="action.handler?.(overlay.bookInfoBook)"
      >
        {{ action.label }}
      </el-button>
    </div>
  </BookInfoDialog>

  <BookEditDialog
    v-model="overlay.bookEditVisible"
    :book="overlay.bookEditBook"
    :saving="editingBookSaving"
    @save="saveEditedBook"
  />

  <OverlayBookImport :is-mobile="isMobileOverlay" />

  <el-drawer
    v-model="overlay.bookManageVisible"
    title="书架管理"
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
    class="global-manage-drawer"
  >
    <div class="manage-head">
      <el-input v-model="manageKeyword" placeholder="搜索书名、作者或文件名" clearable size="small" />
      <div class="manage-head-actions">
        <el-button size="small" text @click="selectAllManagedBooks">全选</el-button>
        <el-button size="small" text @click="clearManagedSelection">清空</el-button>
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
      <el-table-column prop="title" label="书名" min-width="180" show-overflow-tooltip>
        <template #default="{ row }">
          <el-button text class="text-button" @click="overlay.openBookInfo(row)">{{ row.title }}</el-button>
        </template>
      </el-table-column>
      <el-table-column prop="author" label="作者" min-width="120" show-overflow-tooltip />
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
          <el-button text class="text-button" @click="overlay.openBookEdit(row)">编辑</el-button>
          <el-button text class="text-button" @click="setBookGroup(row)">分组</el-button>
          <el-dropdown @command="cacheBook(row, $event)">
            <el-button text class="text-button" :loading="cachingBookId === row.id">
              缓存<el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="cacheBookLocal">缓存到浏览器</el-dropdown-item>
                <el-dropdown-item v-if="Number(row.sourceId || 0) > 0" command="cacheBook">缓存到服务器</el-dropdown-item>
                <el-dropdown-item command="deleteBookLocalCache">删除浏览器缓存</el-dropdown-item>
                <el-dropdown-item v-if="Number(row.sourceId || 0) > 0" command="deleteBookCache">删除服务器缓存</el-dropdown-item>
                <el-dropdown-item v-if="Number(row.sourceId || 0) === 0" disabled>本地书无需服务器缓存</el-dropdown-item>
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
                <el-dropdown-item command="epub">导出为 Epub</el-dropdown-item>
                <el-dropdown-item command="json">导出书籍数据</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </template>
      </el-table-column>
    </el-table>
    <div v-if="filteredManagedBooks.length" class="mobile-manage-list">
      <article v-for="book in filteredManagedBooks" :key="book.id" class="mobile-manage-card" :class="{ selected: selectedBookIds.includes(book.id) }">
        <header>
          <el-checkbox :model-value="selectedBookIds.includes(book.id)" @change="value => toggleManagedBook(book.id, value)" />
          <span
            class="mobile-manage-cover"
            :class="{ 'has-cover': hasBookCover(book) }"
            :style="coverStyle(book)"
          >{{ coverInitial(book) }}</span>
          <button type="button" @click="overlay.openBookInfo(book)">
            <strong>{{ book.title }}</strong>
            <span>{{ book.author || '未知作者' }} · {{ categoryName(book) }}</span>
            <span>{{ Number(book.sourceId || 0) > 0 ? '远程书籍' : '本地书籍' }} · {{ progressLabel(book) }}</span>
          </button>
        </header>
        <p>共 {{ book.chapterCount || 0 }} 章<template v-if="Number(book.sourceId || 0) > 0"> · 服务器缓存 {{ serverCacheCount(book) }} 章</template> · 浏览器缓存 {{ localCacheCount(book) }} 章<template v-if="book.lastChapter"> · 最新：{{ book.lastChapter }}</template></p>
        <footer>
          <el-button size="small" text @click="overlay.openBookEdit(book)">编辑</el-button>
          <el-button size="small" text @click="setBookGroup(book)">分组</el-button>
          <el-dropdown @command="cacheBook(book, $event)">
            <el-button size="small" text :loading="cachingBookId === book.id">
              缓存<el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="cacheBookLocal">缓存到浏览器</el-dropdown-item>
                <el-dropdown-item v-if="Number(book.sourceId || 0) > 0" command="cacheBook">缓存到服务器</el-dropdown-item>
                <el-dropdown-item command="deleteBookLocalCache">删除浏览器缓存</el-dropdown-item>
                <el-dropdown-item v-if="Number(book.sourceId || 0) > 0" command="deleteBookCache">删除服务器缓存</el-dropdown-item>
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
                <el-dropdown-item command="epub">导出为 Epub</el-dropdown-item>
                <el-dropdown-item command="json">导出书籍数据</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </footer>
      </article>
    </div>
    <el-empty v-else class="mobile-manage-empty" description="没有匹配的书籍" />
    <div class="manage-footer">
      <el-button type="primary" :disabled="!selectedBookIds.length" :loading="batchBusy" @click="batchDeleteBooks">批量删除</el-button>
      <el-dropdown @command="batchAddCategory">
        <el-button type="primary" :disabled="!selectedBookIds.length" :loading="batchBusy">
          批量添加分组<el-icon class="el-icon--right"><ArrowDown /></el-icon>
        </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item v-for="category in bookshelf.categories" :key="category.id" :command="category">{{ category.name }}</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <el-dropdown @command="batchRemoveCategory">
        <el-button type="primary" :disabled="!selectedBookIds.length" :loading="batchBusy">
          批量移除分组<el-icon class="el-icon--right"><ArrowDown /></el-icon>
        </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item v-for="category in bookshelf.categories" :key="category.id" :command="category">{{ category.name }}</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <span class="check-tip">已选择 {{ selectedBookIds.length }} 个</span>
      <el-dropdown @command="handleBatchMoreCommand">
        <el-button :disabled="!selectedBookIds.length" :loading="batchBusy">
          更多批量操作<el-icon class="el-icon--right"><ArrowDown /></el-icon>
        </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item command="cache">批量缓存到服务器</el-dropdown-item>
            <el-dropdown-item command="clear-cache">批量清服务器缓存</el-dropdown-item>
            <el-dropdown-item command="export">批量导出</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
      <el-button @click="overlay.bookManageVisible = false">取消</el-button>
    </div>
  </el-drawer>

  <OverlayBookGroups
    :direction="narrowDrawerDirection"
    :size="narrowDrawerSize"
  />

  <OverlayBookContentSearch
    :direction="narrowDrawerDirection"
    :size="narrowDrawerSize"
  />

  <OverlayBookmarks
    :direction="narrowDrawerDirection"
    :size="narrowDrawerSize"
    :is-mobile="isMobileOverlay"
  />

  <OverlayLocalStore
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
  />

  <OverlayWebDAV
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
    :is-mobile="isMobileOverlay"
  />

  <OverlayBackups
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
  />

  <OverlayUserManagement
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
    :is-mobile="isMobileOverlay"
  />

  <OverlayReplaceRules
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
    :is-mobile="isMobileOverlay"
  />

  <OverlayRSS
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
    :is-mobile="isMobileOverlay"
  />
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown } from '@element-plus/icons-vue'
import { cacheBookContent, listChapters, refreshLocalBook, updateBook } from '../api/books'
import { listSources } from '../api/sources'
import { uploadAsset } from '../api/uploads'
import { mergeShelfBook, useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore } from '../stores/reader'
import { useOverlayBookInfo } from '../composables/useOverlayBookInfo'
import { useOverlayBookManagement } from '../composables/useOverlayBookManagement'
import { bookCoverUrl, hasBookCover } from '../utils/bookCover'
import { cacheBookChaptersToBrowser, clearBookBrowserChapterCache, countBooksBrowserCachedChapters, listBookBrowserCachedChapters } from '../utils/bookChapterCache'
import { newestBookProgress, sortByShelfOrder } from '../utils/bookOrder'
import { createBookCategoryNameResolver } from '../utils/bookCategory'
import { localBookSearchText, normalizeLocalBookSearch } from '../utils/localBook'
import { invalidateReaderDataCache, writeReaderDataCache } from '../utils/readerDataCache'
import { currentViewportWidth, shouldUseMiniInterface } from '../utils/responsive'
import BookEditDialog from './BookEditDialog.vue'
import BookInfoDialog from './BookInfoDialog.vue'
import OverlayBackups from './overlays/OverlayBackups.vue'
import OverlayBookContentSearch from './overlays/OverlayBookContentSearch.vue'
import OverlayBookGroups from './overlays/OverlayBookGroups.vue'
import OverlayBookImport from './overlays/OverlayBookImport.vue'
import OverlayBookmarks from './overlays/OverlayBookmarks.vue'
import OverlayLocalStore from './overlays/OverlayLocalStore.vue'
import OverlayReplaceRules from './overlays/OverlayReplaceRules.vue'
import OverlayRSS from './overlays/OverlayRSS.vue'
import OverlayUserManagement from './overlays/OverlayUserManagement.vue'
import OverlayWebDAV from './overlays/OverlayWebDAV.vue'

const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const reader = useReaderStore()
const categoryName = createBookCategoryNameResolver(() => bookshelf.categories)

const sourceRows = ref([])
const manageKeyword = ref('')
const windowWidth = ref(currentViewportWidth())
let sourceRowsRefreshTimer

const isMobileOverlay = computed(() => shouldUseMiniInterface(reader.pageMode, windowWidth.value))
const wideDrawerDirection = computed(() => isMobileOverlay.value ? 'btt' : 'rtl')
const wideDrawerSize = computed(() => isMobileOverlay.value ? '88%' : '82%')
const narrowDrawerDirection = computed(() => isMobileOverlay.value ? 'btt' : 'rtl')
const narrowDrawerSize = computed(() => isMobileOverlay.value ? '86%' : '420px')
const bookInfoCategory = computed(() => overlay.bookInfoOptions.categoryName || categoryName(overlay.bookInfoBook))
const bookInfoSourceName = computed(() => {
  if (overlay.bookInfoOptions.sourceName) return overlay.bookInfoOptions.sourceName
  const sourceId = overlay.bookInfoBook?.sourceId
  if (!sourceId) return '本地'
  return sourceRows.value.find(source => Number(source.id) === Number(sourceId))?.name || '远程书籍'
})
const bookInfoProgress = computed(() => {
  const book = overlay.bookInfoBook
  return bookProgress(book)?.percent || 0
})
const bookInfoBrowserCacheCount = computed(() => (
  overlay.bookInfoBook?.id ? localCacheCount(overlay.bookInfoBook) : -1
))
const bookInfoInShelf = computed(() => isShelfBook(overlay.bookInfoBook))
const sourceStatusLabel = computed(() => overlay.bookInfoBook?.sourceId ? '远程书籍' : '本地书籍')
const managedBooks = computed(() => sortByShelfOrder(bookshelf.books, reader.progressByBook))
const filteredManagedBooks = computed(() => {
  const value = normalizeLocalBookSearch(manageKeyword.value)
  if (!value) return managedBooks.value
  return managedBooks.value.filter(book => manageBookSearchText(book).includes(value))
})
const {
  refreshingBookId,
  coverUploadingBookId,
  updatingBookId,
  editingBookSaving,
  refreshManagedBrowserCacheCounts,
  refreshBookInfoBrowserCacheCount,
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
  refreshBookInfoBrowserCacheCount,
  saveBlob: downloadBlob,
  confirm: (...args) => ElMessageBox.confirm(...args),
  now: () => Date.now(),
  onSuccess: message => ElMessage.success(message),
  onInfo: message => ElMessage.info(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})
function manageBookSearchText(book) {
  return localBookSearchText(book, [
    progressLabel(book),
    categoryName(book),
  ])
}

function isShelfBook(book) {
  if (!book) return false
  if (book.id && bookshelf.books.some(item => Number(item.id) === Number(book.id))) return true
  const bookUrl = String(book.url || book.bookUrl || '').trim()
  if (!bookUrl) return false
  return bookshelf.books.some(item => String(item.url || item.bookUrl || '').trim() === bookUrl)
}
onMounted(() => {
  window.addEventListener('resize', updateWindowWidth, { passive: true })
  window.addEventListener('openreader:sources-update', handleSourcesUpdated)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', updateWindowWidth)
  window.removeEventListener('openreader:sources-update', handleSourcesUpdated)
  clearSourceRowsRefreshTimer()
})

function updateWindowWidth() {
  windowWidth.value = currentViewportWidth()
}

watch(
  () => overlay.bookManageVisible,
  async (visible) => {
    if (!visible) {
      manageKeyword.value = ''
      clearManagedSelection()
      return
    }
    const [categoryResult, booksResult] = await Promise.allSettled([
      warmOverlayCategories(),
      warmOverlayBooks(),
    ])
    if (booksResult.status === 'rejected') {
      ElMessage.error(readError(booksResult.reason, '加载书架数据失败'))
      return
    }
    if (categoryResult.status === 'rejected') {
      ElMessage.warning(readError(categoryResult.reason, '分组加载失败，书架管理仍可使用'))
    }
    await refreshManagedBrowserCacheCounts()
  },
)

watch(
  () => overlay.bookInfoVisible,
  async (visible) => {
    if (!visible) return
    const warmTasks = [warmOverlayCategories()]
    if (overlay.bookInfoBook?.id) warmTasks.push(warmOverlayBooks())
    const [categoryResult, booksResult] = await Promise.allSettled(warmTasks)
    if (categoryResult.status === 'rejected') {
      ElMessage.warning(readError(categoryResult.reason, '分组加载失败，书籍信息仍可查看'))
    }
    if (booksResult?.status === 'rejected') {
      ElMessage.warning(readError(booksResult.reason, '书架状态加载失败，书籍信息仍可查看'))
    }
    if (overlay.bookInfoBook?.sourceId && !sourceRows.value.length) {
      await loadSourceRows().catch((err) => {
        ElMessage.warning(readError(err, '书源加载失败，书籍信息仍可查看'))
      })
    }
    if (overlay.bookInfoBook?.id) {
      await refreshBookInfoBrowserCacheCount(overlay.bookInfoBook)
    }
  },
)

async function warmOverlayCategories(options = {}) {
  return bookshelf.ensureCategoriesLoaded(options)
}

async function warmOverlayBooks(options = {}) {
  return bookshelf.ensureBooksLoaded({ all: true, ...options })
}

function progressLabel(book) {
  const progress = bookProgress(book)
  return `${Math.round((progress?.percent || 0) * 100)}%`
}

async function loadSourceRows() {
  const { data } = await listSources()
  sourceRows.value = data || []
}

function handleSourcesUpdated() {
  if (!shouldRefreshOverlaySources()) return
  scheduleSourceRowsRefresh()
}

function shouldRefreshOverlaySources() {
  return (overlay.bookInfoVisible && Number(overlay.bookInfoBook?.sourceId || 0) > 0) ||
    sourceRows.value.length > 0
}

function scheduleSourceRowsRefresh() {
  clearSourceRowsRefreshTimer()
  sourceRowsRefreshTimer = window.setTimeout(async () => {
    sourceRowsRefreshTimer = undefined
    try {
      await loadSourceRows()
    } catch {
      // Keep existing source names/groups; the next source action can recover.
    }
  }, 350)
}

function clearSourceRowsRefreshTimer() {
  if (!sourceRowsRefreshTimer) return
  window.clearTimeout(sourceRowsRefreshTimer)
  sourceRowsRefreshTimer = undefined
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

function joinPath(base, name) {
  return [base, name].filter(Boolean).join('/')
}

function buildSourceGroupOptions(rows) {
  const counts = new Map()
  for (const item of rows || []) {
    if (item?.enabled === false) continue
    const group = String(item?.group || '').trim()
    if (!group) continue
    counts.set(group, (counts.get(group) || 0) + 1)
  }
  return [...counts.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([value, count]) => ({ value, label: value, count }))
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.overlay-actions,
.manage-footer {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.overlay-actions {
  margin-top: 4px;
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

  .overlay-actions {
    display: grid;
  }

  .overlay-actions :deep(.el-button) {
    width: 100%;
    min-height: 38px;
    margin-left: 0;
  }

}

</style>
