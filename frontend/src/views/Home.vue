<template>
  <section class="app-page shelf-page">
    <div class="shelf-title app-panel">
      <strong>书架 ({{ displayedBooks.length }})</strong>
      <div class="title-actions">
        <button type="button" @click="showBookEditButton = !showBookEditButton">
          {{ showBookEditButton ? '取消' : '编辑' }}
        </button>
        <button type="button" @click="refreshShelf">
          {{ refreshLoading ? '刷新中...' : '刷新' }}
        </button>
      </div>
    </div>

    <div class="book-group-wrapper app-panel">
      <el-tabs v-model="selectedGroup" stretch>
        <el-tab-pane v-for="item in groupItems" :key="item.id" :label="`${item.name} ${item.count}`" :name="item.id" />
      </el-tabs>
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
          <button v-for="book in displayedBooks" :key="book.id" class="book-row" type="button" @click="openDetail(book)">
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
              <span class="mobile-row-actions">
                <span>{{ progressLabel(book) }}</span>
                <em>点击详情</em>
              </span>
            </span>
            <el-button class="read-button" size="small" type="primary" plain @click.stop="continueRead(book)">阅读</el-button>
          </button>
        </div>
      </template>

      <div v-else class="empty-panel app-panel">
        <el-empty :description="emptyText" />
      </div>
    </main>

  </section>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import { useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore } from '../stores/reader'

const router = useRouter()
const route = useRoute()
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const reader = useReaderStore()

const keyword = ref('')
const selectedGroup = ref('')
const showBookEditButton = ref(false)
const refreshLoading = ref(false)

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

const displayedBooks = computed(() => {
  const value = keyword.value.trim().toLowerCase()
  const books = Array.isArray(bookshelf.books) ? bookshelf.books : []
  return books
    .filter(book => {
      const matchesKeyword = !value || `${book.title || ''} ${book.author || ''}`.toLowerCase().includes(value)
      if (!matchesKeyword) return false
      if (!selectedGroup.value) return true
      if (selectedGroup.value === 'none') return !book.categoryId
      return String(book.categoryId) === selectedGroup.value
    })
    .sort(compareByReadingOrder)
})

const emptyText = computed(() => {
  if (keyword.value.trim()) return '没有匹配的书籍'
  if (selectedGroup.value) return '这个分组里还没有书'
  return '书架还是空的，请从左侧侧边栏导入书籍或搜索远程书'
})

onMounted(async () => {
  try {
    await Promise.all([bookshelf.loadCategories(), bookshelf.loadBooks()])
  } catch (err) {
    ElMessage.error(readError(err, '加载书架失败'))
  }
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
    await Promise.all([bookshelf.loadCategories(), bookshelf.loadBooks()])
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
  router.push({ name: 'reader', params: { id: book.id } })
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

function compareByReadingOrder(a, b) {
  const aProgress = bookProgress(a)
  const bProgress = bookProgress(b)
  const aReadAt = new Date(aProgress?.updatedAt || 0).getTime()
  const bReadAt = new Date(bProgress?.updatedAt || 0).getTime()
  if (aReadAt !== bReadAt) return bReadAt - aReadAt
  return new Date(b.updatedAt || 0).getTime() - new Date(a.updatedAt || 0).getTime()
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

.shelf-title strong {
  font-size: 18px;
}

.title-actions {
  display: flex;
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
  color: var(--app-text-muted);
  font-size: 13px;
}

.list-main strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.book-group-wrapper {
  min-width: 0;
  padding: 0 10px;
  overflow: hidden;
}

.book-group-wrapper :deep(.el-tabs__header) {
  margin: 0;
}

.book-group-wrapper :deep(.el-tabs__item) {
  height: 42px;
  font-size: 14px;
}

.book-group-wrapper :deep(.el-tabs__nav-scroll) {
  overflow-x: auto;
  scrollbar-width: none;
}

.book-group-wrapper :deep(.el-tabs__nav) {
  min-width: 0;
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
  width: 100%;
  padding: 12px;
  color: var(--app-text);
  background: transparent;
  border: 0;
  border-bottom: 1px solid var(--app-border);
  cursor: pointer;
  text-align: left;
}

.book-row:hover {
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

.mobile-row-actions {
  display: none;
}

.empty-panel {
  display: grid;
  min-height: 360px;
  place-items: center;
}

.skeleton-row {
  grid-template-columns: 1fr;
}

@media (max-width: 700px) {
  .shelf-page {
    gap: 8px;
    width: 100%;
    max-width: 100%;
    overflow-x: hidden;
  }

  .shelf-title,
  .shelf-toolbar {
    border-radius: 0;
  }

  .shelf-title {
    gap: 10px;
    align-items: start;
    min-width: 0;
  }

  .title-actions {
    flex: 0 0 auto;
    gap: 8px;
  }

  .title-actions button {
    font-size: 13px;
  }

  .shelf-toolbar {
    padding: 8px 10px;
  }

  .book-row {
    grid-template-columns: 42px minmax(0, 1fr);
    gap: 10px;
    padding: 10px;
  }

  .list-cover {
    width: 42px;
    height: 56px;
  }

  .book-operation {
    position: absolute;
    top: 8px;
    right: 8px;
    min-height: 0;
  }

  .book-operation :deep(.el-button) {
    padding: 0 2px;
  }

  .list-main {
    padding-right: 28px;
  }

  .read-button {
    display: none;
  }

  .mobile-row-actions {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    color: var(--app-primary-strong);
    font-size: 12px;
  }

  .mobile-row-actions em {
    color: var(--app-text-subtle);
    font-style: normal;
  }
}

@media (max-width: 420px) {
  .book-group-wrapper {
    padding: 0 6px;
  }

  .book-group-wrapper :deep(.el-tabs__item) {
    min-width: 58px;
    padding: 0 8px;
    font-size: 12px;
  }

  .shelf-title {
    padding: 10px 8px;
  }

  .shelf-title strong {
    font-size: 15px;
  }

  .book-row {
    padding: 9px 8px;
  }
}
</style>
