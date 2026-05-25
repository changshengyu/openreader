<template>
  <section class="app-page shelf-page" :class="{ 'mobile-shelf': isMobileShelf }">
    <button v-if="recentBook" class="recent-strip app-panel" type="button" @click="continueRead(recentBook)">
      <span>
        <small>上次阅读</small>
        <strong>{{ recentBook.title }}</strong>
        <em>{{ readChapterTitle(recentBook) || recentBook.lastChapter || '继续阅读' }}</em>
      </span>
      <b>{{ progressLabel(recentBook) }}</b>
    </button>

    <div class="shelf-title app-panel">
      <div class="shelf-title-main">
        <button v-if="isMobileShelf" class="mobile-menu-trigger" type="button" aria-label="打开侧边栏" @click.stop="toggleMobileNavigation">
          <el-icon><Menu /></el-icon>
        </button>
        <strong>书架 ({{ displayedBooks.length }})</strong>
      </div>
      <div class="title-actions">
        <button type="button" @click="showBookEditButton = !showBookEditButton">
          {{ showBookEditButton ? '取消' : '编辑' }}
        </button>
        <button type="button" @click="refreshShelf">
          {{ refreshLoading ? '刷新中...' : '刷新' }}
        </button>
      </div>
    </div>

    <div class="book-group-wrapper app-panel" role="tablist" aria-label="书架分组">
      <button
        v-for="item in groupItems"
        :key="item.id"
        class="group-chip"
        :class="{ active: selectedGroup === item.id }"
        type="button"
        role="tab"
        :aria-selected="selectedGroup === item.id"
        @click="selectedGroup = item.id"
      >
        <span>{{ item.name }}</span>
        <b>{{ item.count }}</b>
      </button>
    </div>

    <main class="shelf-main">
      <div class="shelf-toolbar app-panel">
        <el-input v-model="keyword" placeholder="搜索书名或作者" clearable>
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
      </div>

      <div v-if="bookshelf.loading" class="book-list app-panel">
        <article v-for="i in 8" :key="i" class="book-row skeleton-row">
          <el-skeleton :rows="2" animated />
        </article>
      </div>

      <template v-else-if="displayedBooks.length">
        <div class="book-list app-panel">
          <article
            v-for="book in displayedBooks"
            :key="book.id"
            class="book-row"
            role="button"
            tabindex="0"
            @click="handleBookRowClick(book)"
            @keyup.enter="handleBookRowClick(book)"
          >
            <span class="list-cover" :style="coverStyle(book)">{{ coverInitial(book) }}</span>
            <span class="list-main">
              <span class="book-operation">
                <el-button v-if="showBookEditButton" size="small" text type="danger" @click.stop="deleteManagedBook(book)">删除</el-button>
                <el-button v-if="showBookEditButton" size="small" text @click.stop="goEditBook(book)">编辑</el-button>
                <el-badge
                  v-if="!showBookEditButton && unreadCount(book) > 0"
                  class="unread-num-badge"
                  :max="99"
                  :value="unreadCount(book)"
                />
              </span>
              <strong>{{ book.title }}</strong>
              <small>{{ book.author || '未知作者' }}<template v-if="book.chapterCount"> · 共{{ book.chapterCount }}章</template></small>
              <small v-if="readChapterTitle(book)">已读：{{ readChapterTitle(book) }}</small>
              <small v-if="book.lastChapter">最新：{{ book.lastChapter }}</small>
            </span>
            <el-button class="read-button" size="small" type="primary" plain @click.stop="continueRead(book)">阅读</el-button>
          </article>
        </div>
      </template>

      <div v-else class="empty-panel app-panel">
        <el-empty :description="emptyText" />
      </div>
    </main>

  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Menu, Search } from '@element-plus/icons-vue'
import { useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore } from '../stores/reader'
import { sortByShelfOrder } from '../utils/bookOrder'

const router = useRouter()
const route = useRoute()
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const reader = useReaderStore()

const keyword = ref('')
const selectedGroup = ref('')
const showBookEditButton = ref(false)
const refreshLoading = ref(false)
const windowWidth = ref(typeof window === 'undefined' ? 1280 : window.innerWidth)
const coarsePointer = ref(false)
const touchDevice = ref(false)

const groupItems = computed(() => {
  const countByCategory = new Map()
  const books = Array.isArray(bookshelf.books) ? bookshelf.books : []
  const categories = Array.isArray(bookshelf.categories) ? bookshelf.categories : []
  for (const book of books) {
    const key = book.categoryId ? String(book.categoryId) : 'none'
    countByCategory.set(key, (countByCategory.get(key) || 0) + 1)
  }
  return [
    { id: '', name: '全部', count: books.length, builtin: true },
    { id: 'none', name: '未分组', count: countByCategory.get('none') || 0, builtin: true },
    ...categories.map(category => ({
      id: String(category.id),
      name: category.name,
      count: countByCategory.get(String(category.id)) || 0,
      sortOrder: category.sortOrder || 0,
      builtin: false,
    })),
  ]
})

const sortedBooks = computed(() => sortByShelfOrder(Array.isArray(bookshelf.books) ? bookshelf.books : [], reader.progressByBook))

const displayedBooks = computed(() => {
  const value = keyword.value.trim().toLowerCase()
  const filtered = sortedBooks.value.filter(book => {
    const matchesKeyword = !value || `${book.title || ''} ${book.author || ''}`.toLowerCase().includes(value)
    if (!matchesKeyword) return false
    if (!selectedGroup.value) return true
    if (selectedGroup.value === 'none') return !book.categoryId
    return String(book.categoryId) === selectedGroup.value
  })
  return filtered
})

const recentBook = computed(() => sortedBooks.value[0] || null)
const isMobileShelf = computed(() => windowWidth.value <= 1180 || coarsePointer.value || touchDevice.value || isMobileUA())

const emptyText = computed(() => {
  if (keyword.value.trim()) return '没有匹配的书籍'
  if (selectedGroup.value) return '这个分组里还没有书'
  return '书架还是空的，请从左侧侧边栏导入书籍或搜索远程书'
})

onMounted(async () => {
  updateViewportFlags()
  window.addEventListener('resize', updateViewportFlags)
  window.addEventListener('orientationchange', updateViewportFlags)
  try {
    await Promise.all([bookshelf.loadCategories(), bookshelf.loadBooks()])
  } catch (err) {
    ElMessage.error(readError(err, '加载书架失败'))
  }
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
    await Promise.all([bookshelf.loadCategories({ force: true }), bookshelf.loadBooks({ force: true })])
    ElMessage.success('书架已刷新')
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
  if (isMobileShelf.value) {
    continueRead(book)
    return
  }
  openDetail(book)
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

function progressLabel(book) {
  const progress = bookProgress(book)
  return `${Math.round(Math.max(0, Math.min(1, progress?.percent || 0)) * 100)}%`
}

function bookProgress(book) {
  return reader.progressByBook[book.id] || book.progress
}

function readerRouteQuery(book) {
  const progress = bookProgress(book)
  if (!progress) return {}
  const query = {}
  const chapterIndex = Number(progress.chapterIndex)
  if (Number.isFinite(chapterIndex)) query.chapter = Math.max(0, Math.floor(chapterIndex))
  const offset = Number(progress.offset)
  if (Number.isFinite(offset) && offset > 0) query.offset = Math.floor(offset)
  const percent = Number(progress.percent)
  if (Number.isFinite(percent) && percent > 0) query.percent = Math.max(0, Math.min(1, percent))
  return query
}

function categoryName(id) {
  if (!id) return '未分组'
  return bookshelf.categories.find(category => String(category.id) === String(id))?.name || '未分组'
}

function coverInitial(book) {
  return (book.title || '?').slice(0, 1)
}

function coverStyle(book) {
  if (book.coverUrl) {
    return { backgroundImage: `url(${book.coverUrl})`, backgroundSize: 'cover', backgroundPosition: 'center', color: 'transparent' }
  }
  const palettes = [
    ['#2f6f6d', '#d9ece7'],
    ['#9c5b34', '#f2decf'],
    ['#5a4f8f', '#dedaf1'],
    ['#406c3d', '#dfead9'],
  ]
  const [fg, bg] = palettes[Number(book.id || 1) % palettes.length]
  return { color: fg, background: bg }
}

function updateViewportFlags() {
  windowWidth.value = window.innerWidth
  coarsePointer.value = window.matchMedia?.('(hover: none) and (pointer: coarse)').matches || false
  touchDevice.value = Number(navigator.maxTouchPoints || 0) > 0
}

function isMobileUA() {
  if (typeof navigator === 'undefined') return false
  return /Android|iPhone|iPad|iPod|Mobile|Tablet|Mobi/i.test(navigator.userAgent || '')
}

function toggleMobileNavigation() {
  window.dispatchEvent(new CustomEvent('openreader:toggle-mobile-nav'))
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.shelf-page,
.shelf-main {
  display: grid;
  min-width: 0;
  gap: 16px;
}

.shelf-title,
.shelf-toolbar {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
}

.shelf-title {
  position: sticky;
  z-index: 2;
  top: 0;
  padding: 12px 14px;
  border-radius: 0;
}

.recent-strip {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  color: var(--app-text);
  cursor: pointer;
  text-align: left;
}

.recent-strip span {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.recent-strip small,
.recent-strip em {
  min-width: 0;
  overflow: hidden;
  color: var(--app-text-muted);
  font-size: 12px;
  font-style: normal;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.recent-strip strong {
  min-width: 0;
  overflow: hidden;
  font-size: 16px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.recent-strip b {
  display: grid;
  width: 48px;
  height: 48px;
  place-items: center;
  flex: 0 0 48px;
  color: var(--app-primary-strong);
  background: var(--app-primary-soft);
  border-radius: 50%;
  font-size: 14px;
}

.shelf-title strong {
  font-size: 18px;
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
  gap: 10px;
}

.title-actions button {
  padding: 0;
  color: var(--app-primary-strong);
  background: transparent;
  border: 0;
  cursor: pointer;
  font-size: 14px;
}

.list-cover {
  display: grid;
  place-items: center;
  font-weight: 900;
}

.list-main small {
  min-width: 0;
  overflow: hidden;
  color: var(--app-text-muted);
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.list-main strong {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.book-group-wrapper {
  display: flex;
  min-width: 0;
  max-width: 100%;
  gap: 8px;
  padding: 8px 10px;
  overflow-x: auto;
  scrollbar-width: none;
}

.book-group-wrapper::-webkit-scrollbar {
  display: none;
}

.group-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  max-width: 132px;
  height: 34px;
  flex: 0 0 auto;
  padding: 0 10px;
  color: var(--app-text-muted);
  background: transparent;
  border: 0;
  border-radius: 4px;
  cursor: pointer;
  font-size: 13px;
}

.group-chip span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.group-chip b {
  color: var(--app-text-subtle);
  font-size: 12px;
  font-weight: 600;
}

.group-chip.active {
  color: var(--app-primary-strong);
  background: var(--app-primary-soft);
  font-weight: 700;
}

.shelf-toolbar {
  padding: 10px 12px;
}

.shelf-toolbar .el-input {
  min-width: 0;
  flex: 1;
}

.book-list {
  min-width: 0;
  overflow: hidden;
}

.book-row {
  position: relative;
  display: grid;
  grid-template-columns: 52px minmax(0, 1fr) auto;
  gap: 12px;
  align-items: center;
  min-width: 0;
  max-width: 100%;
  overflow: hidden;
  width: 100%;
  padding: 12px;
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
  width: 52px;
  height: 68px;
  border-radius: 5px;
}

.list-main {
  display: grid;
  min-width: 0;
  gap: 5px;
}

.book-operation {
  display: grid;
  min-height: 20px;
  justify-items: end;
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
  gap: 8px;
  width: 100%;
  max-width: 100%;
  min-width: 0;
  padding: 0 0 18px;
  overflow-x: hidden;
}

.shelf-page.mobile-shelf .shelf-main {
  width: 100%;
  max-width: 100%;
  min-width: 0;
  overflow-x: hidden;
}

.shelf-page.mobile-shelf .shelf-title,
.shelf-page.mobile-shelf .shelf-toolbar,
.shelf-page.mobile-shelf .recent-strip,
.shelf-page.mobile-shelf .book-group-wrapper,
.shelf-page.mobile-shelf .book-list,
.shelf-page.mobile-shelf .empty-panel {
  max-width: 100%;
  border-radius: 0;
  border-right: 0;
  border-left: 0;
  box-shadow: none;
}

.shelf-page.mobile-shelf .shelf-title {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
  align-items: center;
  min-width: 0;
  padding: 8px 10px;
}

.shelf-page.mobile-shelf .shelf-toolbar {
  display: none;
}

.shelf-page.mobile-shelf .recent-strip {
  display: none;
}

.shelf-page.mobile-shelf .recent-strip strong {
  font-size: 13px;
}

.shelf-page.mobile-shelf .recent-strip b {
  width: 38px;
  height: 38px;
  flex-basis: 38px;
  font-size: 12px;
}

.shelf-page.mobile-shelf .book-group-wrapper {
  padding: 6px;
}

.shelf-page.mobile-shelf .book-row {
  display: flex;
  min-height: 132px;
  align-items: center;
  gap: 18px;
  padding: 10px 20px;
  contain: layout paint;
}

.shelf-page.mobile-shelf .list-cover {
  width: 84px;
  height: 112px;
  flex: 0 0 84px;
}

.shelf-page.mobile-shelf .book-operation {
  position: absolute;
  top: 10px;
  right: 20px;
  display: flex;
  min-width: 0;
  min-height: 0;
  justify-content: flex-end;
  overflow: hidden;
}

.shelf-page.mobile-shelf .list-main {
  min-height: 112px;
  justify-content: space-between;
  gap: 4px;
  padding-right: 48px;
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
  word-break: break-word;
}

.shelf-page.mobile-shelf .read-button {
  display: none;
}

@media (max-width: 1024px), (hover: none) and (pointer: coarse) {
  .shelf-page {
    gap: 8px;
    width: 100%;
    max-width: 100%;
    min-width: 0;
    padding: 0 0 18px;
    overflow-x: hidden;
  }

  .shelf-main {
    width: 100%;
    max-width: 100%;
    min-width: 0;
    overflow-x: hidden;
  }

  .shelf-title,
  .shelf-toolbar,
  .recent-strip,
  .book-group-wrapper,
  .book-list,
  .empty-panel {
    max-width: 100%;
    border-radius: 0;
    border-right: 0;
    border-left: 0;
    box-shadow: none;
  }

  .shelf-title {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 8px;
    align-items: center;
    min-width: 0;
    padding: 8px 10px;
  }

  .title-actions {
    min-width: 0;
    gap: 8px;
  }

  .title-actions button {
    min-width: 0;
    font-size: 13px;
    white-space: nowrap;
  }

  .shelf-toolbar {
    display: none;
  }

  .shelf-toolbar :deep(.el-input__wrapper) {
    min-height: 32px;
  }

  .recent-strip {
    display: none;
  }

  .book-group-wrapper {
    padding: 6px;
  }

  .book-row {
    display: flex;
    min-height: 132px;
    align-items: center;
    gap: 18px;
    padding: 10px 20px;
    contain: layout paint;
  }

  .list-cover {
    width: 84px;
    height: 112px;
    flex: 0 0 84px;
  }

  .book-operation {
    position: absolute;
    top: 10px;
    right: 20px;
    display: flex;
    min-width: 0;
    min-height: 0;
    justify-content: flex-end;
    overflow: hidden;
  }

  .book-operation :deep(.el-button) {
    min-width: 0;
    max-width: 38px;
    overflow: hidden;
    padding: 0 2px;
    text-overflow: ellipsis;
  }

  .list-main {
    width: 100%;
    max-width: 100%;
    min-height: 112px;
    justify-content: space-between;
    padding-right: 0;
    overflow: hidden;
  }

  .list-main strong {
    display: -webkit-box;
    max-height: 45px;
    padding-right: 48px;
    font-size: 16px;
    line-height: 1.35;
    overflow-wrap: anywhere;
    white-space: normal;
    word-break: break-word;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
  }

  .list-main small {
    font-size: 13px;
    line-height: 1.35;
    overflow-wrap: anywhere;
    word-break: break-word;
  }

  .read-button {
    display: none;
  }

}

@media (max-width: 520px) {
  .book-group-wrapper {
    padding: 0 6px;
  }

  .group-chip {
    max-width: 94px;
    height: 32px;
    padding: 0 8px;
    gap: 4px;
    font-size: 12px;
  }

  .shelf-title {
    padding: 8px;
  }

  .shelf-title strong {
    font-size: 15px;
  }

  .book-row {
    grid-template-columns: 36px minmax(0, 1fr);
    gap: 7px;
    padding: 8px 6px;
  }

  .list-cover {
    width: 36px;
    height: 50px;
  }

  .list-main {
    gap: 4px;
  }

  .list-main small {
    font-size: 12px;
  }
}
</style>
