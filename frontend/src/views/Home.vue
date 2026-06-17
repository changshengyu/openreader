<template>
  <section class="app-page shelf-page" :class="{ 'mobile-shelf': isMobileShelf }">
    <div class="shelf-title">
      <div class="shelf-title-main">
        <button v-if="isMobileShelf" class="mobile-menu-trigger" type="button" aria-label="打开侧边栏" @click.stop="toggleMobileNavigation">
          <el-icon><Menu /></el-icon>
        </button>
        <strong>书架 ({{ totalBookCount }})</strong>
      </div>
      <div class="title-actions">
        <button v-if="isNormalPage" type="button" @click="router.push({ name: 'discover' })">书海</button>
        <button v-if="isNormalPage" type="button" @click="overlay.openRSS()">RSS</button>
        <button type="button" @click="refreshShelf">
          {{ refreshLoading ? '刷新中...' : '刷新' }}
        </button>
        <button v-if="isNormalPage" type="button" @click="showBookEditButton = !showBookEditButton">
          {{ showBookEditButton ? '取消' : '编辑' }}
        </button>
        <button v-if="!isMobileShelf" class="view-switch" type="button" :class="{ active: effectiveShelfView === 'grid' }" title="网格显示" @click="setShelfView('grid')">
          <el-icon><Grid /></el-icon>
          <span>网格</span>
        </button>
        <button v-if="!isMobileShelf" class="view-switch" type="button" :class="{ active: effectiveShelfView === 'list' }" title="列表显示" @click="setShelfView('list')">
          <el-icon><List /></el-icon>
          <span>列表</span>
        </button>
      </div>
    </div>

    <div class="book-group-wrapper" role="tablist" aria-label="书架分组">
      <button
        v-for="item in groupItems"
        :key="item.id"
        class="group-chip"
        :class="{ active: selectedGroup === item.id }"
        type="button"
        role="tab"
        :aria-selected="selectedGroup === item.id"
        :title="`${item.name} (${item.count})`"
        @click="selectedGroup = item.id"
      >
        <span>{{ item.name }}</span>
      </button>
    </div>

    <main class="shelf-main" :class="`${effectiveShelfView}-view`">
      <div class="books-wrapper">
        <div v-if="bookshelf.loading" class="book-list">
          <article v-for="i in 8" :key="i" class="book-row skeleton-row">
            <el-skeleton :rows="2" animated />
          </article>
        </div>

        <template v-else-if="displayedBooks.length">
          <div class="book-list">
            <article
              v-for="book in displayedBooks"
              :key="book.id"
              class="book-row"
              :class="{ editing: showBookEditButton }"
              role="button"
              tabindex="0"
              @click="handleBookRowClick(book)"
              @keyup.enter="handleBookRowClick(book)"
            >
              <span
                class="list-cover"
                :class="{ 'has-cover': hasBookCover(book) }"
                :style="coverStyle(book)"
                @click.stop="openDetail(book)"
              >{{ coverInitial(book) }}</span>
              <span class="list-main">
                <span class="book-operation">
                  <button v-if="showBookEditButton" class="operation-icon danger" type="button" title="删除" @click.stop="deleteManagedBook(book)">
                    <el-icon><Close /></el-icon>
                  </button>
                  <button v-if="showBookEditButton" class="operation-icon" type="button" title="编辑" @click.stop="goEditBook(book)">
                    <el-icon><Edit /></el-icon>
                  </button>
                  <el-badge
                    v-if="!showBookEditButton && unreadCount(book) > 0"
                    class="unread-num-badge"
                    :max="99"
                    :value="unreadCount(book)"
                  />
                </span>
                <strong>{{ book.title }}</strong>
                <small>{{ bookAuthorLine(book) }}</small>
                <small v-if="readChapterTitle(book)">已读：{{ readChapterTitle(book) }}</small>
                <small v-if="latestChapterTitle(book)">{{ latestChapterLabel(book) }}：{{ latestChapterTitle(book) }}</small>
              </span>
            </article>
          </div>
        </template>

        <div v-else class="empty-panel">
          <el-empty :description="emptyText" />
        </div>
      </div>
    </main>

  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Close, Edit, Grid, List, Menu } from '@element-plus/icons-vue'
import { useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore } from '../stores/reader'
import { usePreferencesStore } from '../stores/preferences'
import { bookCoverUrl, hasBookCover } from '../utils/bookCover'
import { newestBookProgress, sortByShelfOrder } from '../utils/bookOrder'
import { isLocalBook } from '../utils/localBook'
import { readerRouteQueryFromBook } from '../utils/readerRoute'
import { currentViewportWidth, shouldUseMiniInterface } from '../utils/responsive'

const router = useRouter()
const route = useRoute()
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const reader = useReaderStore()
const preferences = usePreferencesStore()

const selectedGroup = ref('')
const showBookEditButton = ref(false)
const refreshLoading = ref(false)
const shelfView = computed(() => preferences.shelf.view)
const windowWidth = ref(currentViewportWidth())

const groupItems = computed(() => {
  const countByCategory = new Map()
  const books = Array.isArray(bookshelf.books) ? bookshelf.books : []
  const categories = Array.isArray(bookshelf.categories) ? bookshelf.categories : []
  const localCount = books.filter(isLocalBook).length
  for (const book of books) {
    const key = book.categoryId ? String(book.categoryId) : 'none'
    countByCategory.set(key, (countByCategory.get(key) || 0) + 1)
  }
  const noneCount = countByCategory.get('none') || 0
  return [
    { id: '', name: '全部', count: books.length, builtin: true },
    localCount ? { id: 'local', name: '本地', count: localCount, builtin: true } : null,
    noneCount ? { id: 'none', name: '未分组', count: noneCount, builtin: true } : null,
    ...categories.filter(category => category.show !== false && (countByCategory.get(String(category.id)) || 0) > 0).map(category => ({
      id: String(category.id),
      name: category.name,
      count: countByCategory.get(String(category.id)) || 0,
      sortOrder: category.sortOrder || 0,
      builtin: false,
    })),
  ].filter(Boolean)
})

const sortedBooks = computed(() => sortByShelfOrder(Array.isArray(bookshelf.books) ? bookshelf.books : [], reader.progressByBook))
const totalBookCount = computed(() => Array.isArray(bookshelf.books) ? bookshelf.books.length : 0)

const displayedBooks = computed(() => {
  const filtered = sortedBooks.value.filter(book => {
    if (!selectedGroup.value) return true
    if (selectedGroup.value === 'local') return isLocalBook(book)
    if (selectedGroup.value === 'none') return !book.categoryId
    return String(book.categoryId) === selectedGroup.value
  })
  return filtered
})

const isMobileShelf = computed(() => shouldUseMiniInterface(reader.pageMode, windowWidth.value))
const isNormalPage = computed(() => !['kindle', 'simple', 'Kindle'].includes(reader.pageType))
const effectiveShelfView = computed(() => isMobileShelf.value ? 'list' : shelfView.value)

const emptyText = computed(() => {
  if (selectedGroup.value) return '这个分组里还没有书'
  return '暂无书籍'
})

onMounted(async () => {
  updateViewportFlags()
  window.addEventListener('resize', updateViewportFlags)
  window.addEventListener('orientationchange', updateViewportFlags)
  await warmHomeShelf()
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', updateViewportFlags)
  window.removeEventListener('orientationchange', updateViewportFlags)
})

watch(
  () => route.query.import,
  (value) => {
    if (value === '1') overlay.openImportBook()
  },
  { immediate: true },
)

watch(groupItems, (items) => {
  if (selectedGroup.value && !items.some(item => item.id === selectedGroup.value)) {
    selectedGroup.value = ''
  }
})

watch(isNormalPage, (normal) => {
  if (!normal) showBookEditButton.value = false
})

async function deleteManagedBook(book) {
  try {
    await ElMessageBox.confirm(`确定删除《${book.title}》吗？阅读进度和书签也会一并删除。`, '删除书籍', { type: 'warning' })
    await bookshelf.removeBook(book.id)
    ElMessage.success('书籍已删除')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除失败'))
  }
}

async function refreshShelf() {
  refreshLoading.value = true
  try {
    const [categoryResult, booksResult] = await Promise.allSettled([
      bookshelf.loadCategories({ force: true }),
      bookshelf.loadBooks({ force: true, all: true }),
    ])
    if (booksResult.status === 'rejected') throw booksResult.reason
    if (categoryResult.status === 'rejected') {
      ElMessage.warning(readError(categoryResult.reason, '书架已刷新，分组刷新失败'))
    } else {
      ElMessage.success('书架已刷新')
    }
  } catch (err) {
    ElMessage.error(readError(err, '刷新书架失败'))
  } finally {
    refreshLoading.value = false
  }
}

function goEditBook(book) {
  router.push({ name: 'book-detail', params: { id: book.id } })
}

function openDetail(book) {
  overlay.openBookInfo(book, {
    categoryName: categoryName(book.categoryId),
    progress: (bookProgress(book)?.percent || 0),
  })
}

function continueRead(book) {
  router.push({ name: 'reader', params: { id: book.id }, query: readerRouteQuery(book) })
}

function handleBookRowClick(book) {
  continueRead(book)
}

function setShelfView(view) {
  preferences.setShelfView(view)
}

function readChapterTitle(book) {
  const progress = bookProgress(book)
  if (progress?.chapterTitle) return progress.chapterTitle
  if (Number.isInteger(progress?.chapterIndex)) return `第 ${progress.chapterIndex + 1} 章`
  return ''
}

function unreadCount(book) {
  const progress = bookProgress(book)
  const chapterIndex = Number.isInteger(progress?.chapterIndex) ? progress.chapterIndex : -1
  const chapterCount = Number(book.chapterCount || 0)
  return Math.max(0, chapterCount - 1 - chapterIndex)
}

function bookProgress(book) {
  return newestBookProgress(book, reader.progressByBook)
}

function bookAuthorLine(book) {
  const parts = []
  if (book.author) parts.push(book.author)
  if (book.chapterCount) parts.push(`共${book.chapterCount}章`)
  return parts.join(' · ') || '未知作者'
}

function latestChapterTitle(book) {
  return book.lastChapter || book.latestChapterTitle || book.latestChapter || ''
}

function latestChapterLabel(book) {
  const rawTime = book.lastCheckTime || book.shelfOrderAt || book.updatedAt
  return rawTime ? relativeTimeLabel(rawTime) : '最新'
}

function relativeTimeLabel(value) {
  const timestamp = typeof value === 'number' ? value : Date.parse(value)
  if (!Number.isFinite(timestamp)) return '最新'
  const seconds = Math.max(0, Math.floor((Date.now() - timestamp) / 1000))
  if (seconds <= 30) return '刚刚'
  if (seconds < 60) return `${seconds}秒前`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}分钟前`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}小时前`
  if (seconds < 2592000) return `${Math.floor(seconds / 86400)}天前`
  if (seconds < 31536000) return `${Math.floor(seconds / 2592000)}月前`
  return `${Math.floor(seconds / 31536000)}年前`
}

function readerRouteQuery(book) {
  return readerRouteQueryFromBook(book, bookProgress(book))
}

function categoryName(id) {
  if (!id) return '未分组'
  return bookshelf.categories.find(category => String(category.id) === String(id))?.name || '未分组'
}

function coverInitial(book) {
  return hasBookCover(book) ? '' : '暂无封面'
}

function coverStyle(book) {
  const url = bookCoverUrl(book)
  if (url) {
    return { backgroundImage: `url(${url})`, backgroundSize: 'cover', backgroundPosition: 'center', color: 'transparent' }
  }
  return {}
}

function updateViewportFlags() {
  windowWidth.value = currentViewportWidth()
}

function toggleMobileNavigation() {
  window.dispatchEvent(new CustomEvent('openreader:toggle-mobile-nav'))
}

async function warmHomeShelf() {
  const jobs = [
    ['categories', bookshelf.ensureCategoriesLoaded()],
    ['books', bookshelf.ensureBooksLoaded({ all: true })],
  ]
  const results = await Promise.allSettled(jobs.map(([, job]) => job))
  results.forEach((result, index) => {
    if (result.status !== 'rejected') return
    const type = jobs[index][0]
    if (type === 'books') {
      ElMessage.error(readError(result.reason, '加载书架失败'))
    } else {
      ElMessage.warning(readError(result.reason, '分组加载失败，书架仍可使用'))
    }
  })
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.shelf-page,
.shelf-main,
.books-wrapper {
  min-width: 0;
  max-width: 100%;
}

.shelf-page {
  display: grid;
  grid-template-rows: auto auto minmax(0, 1fr);
  box-sizing: border-box;
  height: 100vh;
  max-height: 100vh;
  gap: 0;
  padding: 48px 48px;
  background: #fff;
  overflow: hidden;
}

.shelf-main {
  display: grid;
  min-height: 0;
  gap: 0;
  overflow: hidden;
}

.shelf-title {
  display: flex;
  z-index: 2;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 4px 0 12px;
  background: #fff;
  border: 0;
  border-radius: 0;
  box-shadow: none;
}

.shelf-title strong {
  color: #26394a;
  font-size: 22px;
  font-weight: 800;
  line-height: 1.25;
}

.shelf-title-main {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}

.shelf-title-main strong {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-menu-trigger {
  display: inline-grid;
  width: 30px;
  height: 30px;
  place-items: center;
  flex: 0 0 30px;
  padding: 0;
  color: var(--app-text);
  background: transparent;
  border: 0;
  cursor: pointer;
}

.title-actions {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 16px;
}

.title-actions button {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  padding: 0;
  color: #26394a;
  background: transparent;
  border: 0;
  cursor: pointer;
  font-size: 14px;
  font-weight: 700;
  line-height: 28px;
}

.title-actions .view-switch {
  color: #9aa1aa;
  font-weight: 600;
}

.title-actions .view-switch.active {
  color: #1f6feb;
}

.list-cover {
  display: grid;
  place-items: center;
  font-weight: 900;
  color: #8f866f;
  background:
    radial-gradient(circle at 76% 18%, rgba(203, 186, 132, 0.22), transparent 24%),
    linear-gradient(135deg, #fbfaf4 0%, #f4f0df 100%);
  border: 1px solid rgba(190, 178, 142, 0.32);
  line-height: 1.35;
  text-align: center;
  writing-mode: vertical-rl;
}

.list-cover.has-cover {
  border-color: transparent;
  writing-mode: initial;
}

.book-group-wrapper {
  display: flex;
  min-width: 0;
  max-width: 100%;
  gap: 0;
  padding: 0;
  background: #fff;
  border: 0;
  border-bottom: 1px solid #dfe3ea;
  border-radius: 0;
  box-shadow: none;
  overflow-x: auto;
  scrollbar-width: none;
}

.book-group-wrapper::-webkit-scrollbar {
  display: none;
}

.group-chip {
  display: inline-flex;
  position: relative;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-width: 0;
  max-width: none;
  height: 48px;
  flex: 1 0 126px;
  padding: 0 16px;
  color: #33373d;
  background: transparent;
  border: 0;
  border-radius: 0;
  cursor: pointer;
  font-size: 14px;
  font-weight: 700;
}

.group-chip span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.group-chip.active {
  color: #1f6feb;
  background: transparent;
}

.group-chip.active::after {
  position: absolute;
  right: 0;
  bottom: -1px;
  left: 0;
  height: 2px;
  background: #409eff;
  content: "";
}

.book-list {
  min-width: 0;
  background: #fff;
  border: 0;
  box-shadow: none;
}

.books-wrapper {
  min-height: 0;
  overflow-x: hidden;
  overflow-y: auto;
  scrollbar-width: none;
}

.books-wrapper::-webkit-scrollbar {
  width: 0;
  height: 0;
}

.shelf-main.grid-view .book-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 360px));
  justify-content: space-around;
  gap: 18px 16px;
  padding: 22px 0 28px;
  overflow: visible;
}

.shelf-main.grid-view .book-row {
  grid-template-columns: 84px minmax(0, 1fr);
  gap: 18px;
  width: min(360px, 100%);
  min-height: 142px;
  align-items: start;
  padding: 12px 8px;
  border-bottom: 0;
}

.shelf-main.grid-view .book-row:hover,
.shelf-main.grid-view .book-row:focus-visible {
  background: #fafafa;
}

.shelf-main.grid-view .list-cover {
  width: 84px;
  height: 112px;
  border-radius: 0;
}

.shelf-main.grid-view .list-main {
  height: 112px;
  align-content: space-between;
  justify-content: start;
  gap: 3px;
}

.shelf-main.grid-view .list-main strong {
  display: -webkit-box;
  max-height: 46px;
  padding-right: 40px;
  color: #33373d;
  font-size: 16px;
  font-weight: 800;
  line-height: 1.35;
  white-space: normal;
  word-break: break-word;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.shelf-main.grid-view .book-row.editing .list-main strong {
  padding-right: 58px;
}

.shelf-main.grid-view .list-main small {
  color: #6b6b6b;
  font-size: 13px;
  font-weight: 600;
}

.shelf-main.grid-view .book-operation {
  position: absolute;
  top: 4px;
  right: 0;
  min-height: 22px;
}

.shelf-main.grid-view .unread-num-badge :deep(.el-badge__content) {
  border: 0;
  background: #f56c6c;
  font-weight: 700;
}

.book-row {
  position: relative;
  display: grid;
  grid-template-columns: 74px minmax(0, 1fr);
  gap: 16px;
  align-items: start;
  min-width: 0;
  max-width: 100%;
  width: 100%;
  box-sizing: border-box;
  padding: 14px 0;
  color: var(--app-text);
  background: transparent;
  border: 0;
  border-bottom: 1px solid var(--app-border);
  cursor: pointer;
  outline: 0;
  text-align: left;
}

.book-row > * {
  min-width: 0;
}

.book-row:hover,
.book-row:focus-visible {
  background: var(--app-bg-soft);
}

.list-cover {
  width: 74px;
  height: 98px;
  border-radius: 5px;
  cursor: zoom-in;
}

.list-main {
  display: grid;
  min-width: 0;
  box-sizing: border-box;
  align-content: start;
  gap: 6px;
  padding-right: 44px;
}

.list-main strong,
.list-main small {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}

.list-main strong {
  color: #33373d;
  font-size: 16px;
  font-weight: 800;
  line-height: 1.35;
  white-space: nowrap;
}

.list-main small {
  color: var(--app-text-muted);
  font-size: 13px;
  line-height: 1.4;
  white-space: nowrap;
}

.book-operation {
  display: flex;
  min-height: 20px;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}

.operation-icon {
  display: inline-grid;
  width: 22px;
  height: 22px;
  place-items: center;
  flex: 0 0 22px;
  padding: 0;
  color: #969ba3;
  background: transparent;
  border: 0;
  cursor: pointer;
  font-size: 18px;
}

.operation-icon:hover {
  color: #1f6feb;
}

.operation-icon.danger:hover {
  color: #b5463e;
}

.empty-panel {
  display: grid;
  min-height: 360px;
  place-items: center;
}

.skeleton-row {
  grid-template-columns: 1fr;
}

.shelf-page.mobile-shelf {
  gap: 0;
  height: auto;
  max-height: none;
  min-height: 100vh;
  width: 100%;
  max-width: 100%;
  min-width: 0;
  padding: 0 0 18px;
  overflow-x: hidden;
  overflow-y: visible;
}

.shelf-page.mobile-shelf .shelf-main,
.shelf-page.mobile-shelf .books-wrapper,
.shelf-page.mobile-shelf .shelf-title,
.shelf-page.mobile-shelf .book-group-wrapper,
.shelf-page.mobile-shelf .book-list,
.shelf-page.mobile-shelf .empty-panel {
  width: 100%;
  max-width: 100%;
  min-width: 0;
  box-sizing: border-box;
  border-radius: 0;
  border-right: 0;
  border-left: 0;
  box-shadow: none;
}

.shelf-page.mobile-shelf .shelf-title {
  display: flex;
  flex-wrap: nowrap;
  gap: 12px;
  align-items: center;
  justify-content: space-between;
  padding: 22px 16px 10px;
  overflow: hidden;
}

.shelf-page.mobile-shelf .shelf-title-main {
  flex: 1 1 auto;
  min-width: 0;
}

.shelf-page.mobile-shelf .shelf-title strong {
  font-size: 30px;
  line-height: 1.2;
  white-space: nowrap;
}

.shelf-page.mobile-shelf .title-actions {
  max-width: 58%;
  flex: 0 1 auto;
  flex-wrap: nowrap;
  justify-content: flex-start;
  gap: 14px;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: none;
}

.shelf-page.mobile-shelf .title-actions::-webkit-scrollbar {
  display: none;
}

.shelf-page.mobile-shelf .title-actions button {
  flex: 0 0 auto;
  font-size: 15px;
  line-height: 28px;
  white-space: nowrap;
}

.shelf-page.mobile-shelf .title-actions .view-switch {
  display: none;
}

.shelf-page.mobile-shelf .book-group-wrapper {
  margin-right: 0;
  margin-left: 0;
  padding: 0;
}

.shelf-page.mobile-shelf .group-chip {
  height: 54px;
  flex: 1 0 25%;
  padding: 0 10px;
  font-size: 16px;
}

.shelf-page.mobile-shelf .shelf-main.grid-view .book-list {
  display: block;
  padding: 0;
}

.shelf-page.mobile-shelf .book-row {
  display: grid;
  grid-template-columns: clamp(76px, 24vw, 92px) minmax(0, 1fr);
  min-height: clamp(120px, 36vw, 142px);
  align-items: start;
  gap: clamp(12px, 4vw, 18px);
  width: 100%;
  box-sizing: border-box;
  padding: 14px 16px;
  border-bottom: 0;
  contain: inline-size paint;
}

.shelf-page.mobile-shelf .list-cover {
  width: 100%;
  height: auto;
  aspect-ratio: 3 / 4;
  border-radius: 0;
}

.shelf-page.mobile-shelf .book-operation {
  position: absolute;
  top: 10px;
  right: 14px;
  display: flex;
  min-width: 0;
  min-height: 0;
  justify-content: flex-end;
  overflow: hidden;
}

.shelf-page.mobile-shelf .list-main {
  width: auto;
  min-height: clamp(102px, 32vw, 122px);
  box-sizing: border-box;
  align-content: space-between;
  justify-content: stretch;
  gap: 6px;
  padding-right: 0;
  overflow: hidden;
}

.shelf-page.mobile-shelf .list-main strong {
  display: -webkit-box;
  max-height: 45px;
  font-size: 16px;
  line-height: 1.35;
  overflow-wrap: anywhere;
  white-space: normal;
  word-break: break-word;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.shelf-page.mobile-shelf .list-main small {
  font-size: 13px;
  line-height: 1.35;
  overflow-wrap: anywhere;
  white-space: normal;
  word-break: break-word;
}

.shelf-page.mobile-shelf .read-button {
  display: none;
}

@media (max-width: 750px) {
  .shelf-page {
    gap: 0;
    height: auto;
    max-height: none;
    min-height: 100vh;
    width: 100%;
    max-width: 100%;
    min-width: 0;
    padding: 0 0 18px;
    overflow-x: hidden;
    overflow-y: visible;
  }

  .shelf-main {
    width: 100%;
    max-width: 100%;
    min-width: 0;
    overflow: visible;
    overflow-x: hidden;
  }

  .books-wrapper {
    width: 100%;
    max-width: 100%;
    min-width: 0;
    overflow-x: hidden;
    overflow-y: visible;
  }

  .shelf-main.grid-view .book-list {
    display: block;
    padding: 0;
  }

  .shelf-main.grid-view .book-row {
    width: 100%;
  }

}

@media (max-width: 520px) {
  .shelf-page.mobile-shelf .book-group-wrapper,
  .book-group-wrapper {
    width: 100%;
    max-width: 100%;
    margin-right: 0;
    margin-left: 0;
    padding: 5px 0;
  }

  .shelf-page.mobile-shelf .group-chip,
  .group-chip {
    max-width: none;
    height: 48px;
    flex: 1 0 25%;
    padding: 0 8px;
    font-size: 14px;
  }

  .shelf-page.mobile-shelf .shelf-title,
  .shelf-title {
    padding: 18px 14px 0;
  }

  .shelf-page.mobile-shelf .shelf-title strong,
  .shelf-title strong {
    font-size: 28px;
  }

  .shelf-page.mobile-shelf .title-actions,
  .title-actions {
    gap: 10px;
  }

  .shelf-page.mobile-shelf .title-actions button,
  .title-actions button {
    font-size: 13px;
  }

  .shelf-page.mobile-shelf .book-row,
  .book-row {
    display: grid;
    grid-template-columns: clamp(72px, 22vw, 86px) minmax(0, 1fr);
    gap: clamp(10px, 3.5vw, 14px);
    min-height: clamp(116px, 34vw, 134px);
    width: 100%;
    max-width: 100%;
    box-sizing: border-box;
    padding: 12px clamp(12px, 3.5vw, 14px);
  }

  .shelf-page.mobile-shelf .list-cover,
  .list-cover {
    width: 100%;
    height: auto;
    aspect-ratio: 3 / 4;
    flex-basis: auto;
  }

  .shelf-page.mobile-shelf .list-main,
  .list-main {
    width: auto;
    max-width: 100%;
    min-height: 114px;
    box-sizing: border-box;
    gap: 4px;
    padding-right: 0;
    overflow: hidden;
  }

  .shelf-page.mobile-shelf .book-operation,
  .book-operation {
    top: 10px;
    right: 12px;
  }

  .shelf-page.mobile-shelf .list-main strong,
  .list-main strong {
    padding-right: clamp(38px, 12vw, 48px);
  }

  .shelf-page.mobile-shelf .book-row.editing .list-main strong,
  .book-row.editing .list-main strong {
    padding-right: clamp(48px, 14vw, 58px);
  }

  .shelf-page.mobile-shelf .list-main small,
  .list-main small {
    font-size: 12px;
  }
}
</style>
