<template>
  <section class="app-page discover-page">
    <header class="discover-head">
      <div>
        <p class="eyebrow">Explore</p>
        <h1 class="app-page-title">书海探索</h1>
        <p class="app-page-subtitle">仅显示配置了 exploreUrl 的书源，按书源真实发现页加载书籍。</p>
      </div>
      <el-button :icon="Refresh" :loading="loadingSources" @click="loadSources">刷新书源</el-button>
    </header>

    <section class="discover-toolbar app-panel">
      <el-select v-model="selectedGroup" placeholder="全部分组" clearable @change="onGroupChange">
        <el-option v-for="group in sourceGroups" :key="group.value" :label="`${group.label} (${group.count})`" :value="group.value" />
      </el-select>
      <el-select v-model="selectedSourceId" placeholder="选择探索书源" filterable @change="loadBooks">
        <el-option v-for="source in filteredSources" :key="source.id" :label="source.name" :value="source.id" />
      </el-select>
      <el-select v-model="targetCategoryId" placeholder="加入书架分组（可选）" clearable>
        <el-option label="未分组" value="" />
        <el-option v-for="category in bookshelf.categories" :key="category.id" :label="category.name" :value="String(category.id)" />
      </el-select>
      <el-button type="primary" :loading="loadingBooks" :disabled="!selectedSourceId" @click="loadBooks">加载</el-button>
      <span v-if="activeSource" class="source-status">{{ activeSource.group || '默认分组' }} · 第 {{ page }} 页</span>
    </section>

    <div v-loading="loadingBooks" class="discover-results">
      <article v-for="book in books" :key="book.sourceId + book.bookUrl" class="discover-card app-panel" @click="openPreview(book)">
        <BookCover :book="book" />
        <div>
          <h2>{{ book.title }}</h2>
          <p>{{ book.author || '未知作者' }} · {{ book.sourceName }}</p>
          <p v-if="book.latestChapter" class="latest">最新：{{ book.latestChapter }}</p>
          <p class="intro">{{ book.intro || '无简介' }}</p>
        </div>
      </article>
      <el-empty v-if="!loadingBooks && !books.length" :description="sources.length ? '当前书源没有返回发现结果' : '没有配置 exploreUrl 的书源'" />
    </div>

    <div v-if="books.length" class="load-more-row">
      <el-button :loading="loadingMore" :disabled="!hasMore" @click="loadMoreBooks">
        {{ hasMore ? '加载更多' : '没有更多结果' }}
      </el-button>
    </div>

  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { createRemoteBook } from '../api/books'
import { exploreBooks, listExploreSources } from '../api/explore'
import BookCover from '../components/BookCover.vue'
import { useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'

const router = useRouter()
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const sources = ref([])
const books = ref([])
const selectedSourceId = ref('')
const selectedGroup = ref('')
const targetCategoryId = ref('')
const loadingSources = ref(false)
const loadingBooks = ref(false)
const addingBook = ref(null)
const page = ref(1)
const hasMore = ref(false)
const loadingMore = ref(false)

const activeSource = computed(() => sources.value.find(source => source.id === selectedSourceId.value))
const sourceGroups = computed(() => {
  const groups = new Map()
  for (const source of sources.value) {
    const name = source.group || '默认分组'
    groups.set(name, (groups.get(name) || 0) + 1)
  }
  return [...groups.entries()].map(([label, count]) => ({ label, value: label, count })).sort((a, b) => a.label.localeCompare(b.label))
})
const filteredSources = computed(() => {
  if (!selectedGroup.value) return sources.value
  return sources.value.filter(source => (source.group || '默认分组') === selectedGroup.value)
})

onMounted(async () => {
  await Promise.all([loadSources(), bookshelf.loadCategories(), bookshelf.loadBooks()])
  if (selectedSourceId.value) await loadBooks()
})

async function loadSources() {
  loadingSources.value = true
  try {
    const { data } = await listExploreSources()
    sources.value = data || []
    if (!selectedSourceId.value && filteredSources.value.length) selectedSourceId.value = filteredSources.value[0].id
  } catch (err) {
    ElMessage.error(readError(err, '加载探索书源失败'))
  } finally {
    loadingSources.value = false
  }
}

function onGroupChange() {
  const exists = filteredSources.value.some(source => source.id === selectedSourceId.value)
  if (!exists) selectedSourceId.value = filteredSources.value[0]?.id || ''
  books.value = []
  hasMore.value = false
  if (selectedSourceId.value) loadBooks()
}

async function loadBooks() {
  if (!selectedSourceId.value) return
  loadingBooks.value = true
  try {
    page.value = 1
    const { data } = await exploreBooks(selectedSourceId.value, { page: page.value })
    const result = normalizeExploreResult(data, page.value)
    books.value = result.items
    hasMore.value = result.hasMore
  } catch (err) {
    ElMessage.error(readError(err, '加载探索结果失败'))
  } finally {
    loadingBooks.value = false
  }
}

async function loadMoreBooks() {
  if (!selectedSourceId.value || loadingMore.value || !hasMore.value) return
  loadingMore.value = true
  try {
    const nextPage = page.value + 1
    const { data } = await exploreBooks(selectedSourceId.value, { page: nextPage })
    const result = normalizeExploreResult(data, nextPage)
    const known = new Set(books.value.map(book => `${book.sourceId}-${book.bookUrl}`))
    const nextItems = result.items.filter(book => !known.has(`${book.sourceId}-${book.bookUrl}`))
    books.value = [...books.value, ...nextItems]
    page.value = result.page || nextPage
    hasMore.value = result.hasMore && nextItems.length > 0
  } catch (err) {
    ElMessage.error(readError(err, '加载更多失败'))
  } finally {
    loadingMore.value = false
  }
}

function normalizeExploreResult(data, fallbackPage) {
  if (Array.isArray(data)) {
    return { items: data, page: fallbackPage, hasMore: false }
  }
  return {
    items: Array.isArray(data?.items) ? data.items : [],
    page: Number(data?.page || fallbackPage),
    hasMore: !!data?.hasMore,
  }
}

function openPreview(book) {
  const existing = findExistingBook(book)
  overlay.openBookInfo(book, {
    sourceName: book.sourceName,
    statusLabel: existing ? '已在书架' : '探索结果',
    statusType: existing ? 'warning' : 'success',
    progress: existing?.progress?.percent || 0,
    actions: existing
      ? [
          { label: '查看详情', plain: true, handler: () => openExistingInfo(existing, book.sourceName) },
          { label: '继续阅读', type: 'primary', handler: () => openExistingReader(existing) },
        ]
      : [
          { label: '加入书架', plain: true, loading: addingBook.value === book.bookUrl, handler: () => addRemoteBook(book, false) },
          { label: '加入并阅读', type: 'primary', loading: addingBook.value === book.bookUrl, handler: () => addRemoteBook(book, true) },
        ],
  })
}

async function addRemoteBook(book, shouldRead) {
  addingBook.value = book.bookUrl
  try {
    const { data } = await createRemoteBook({
      title: book.title,
      author: book.author,
      coverUrl: book.coverUrl,
      intro: book.intro,
      bookUrl: book.bookUrl,
      sourceId: book.sourceId,
      sourceName: book.sourceName,
      categoryId: targetCategoryId.value ? Number(targetCategoryId.value) : null,
    })
    bookshelf.upsertBook(data)
    ElMessage.success(`已加入书架：《${book.title}》`)
    if (shouldRead) {
      overlay.closeBookInfo()
      router.push({ name: 'reader', params: { id: data.id } })
      return
    }
    overlay.openBookInfo(data, {
      sourceName: book.sourceName,
      statusLabel: '已加入书架',
      statusType: 'success',
      progress: 0,
      actions: [
        { label: '完整详情', plain: true, handler: () => openExistingDetail(data) },
        { label: '开始阅读', type: 'primary', handler: () => openExistingReader(data) },
      ],
    })
  } catch (err) {
    ElMessage.error(readError(err, '加入书架失败'))
  } finally {
    addingBook.value = null
  }
}

function findExistingBook(book) {
  return bookshelf.books.find(item => (
    Number(item.sourceId || 0) === Number(book.sourceId || 0)
    && String(item.url || item.bookUrl || '') === String(book.bookUrl || '')
  )) || null
}

function openExistingDetail(book) {
  overlay.closeBookInfo()
  router.push({ name: 'book-detail', params: { id: book.id } })
}

function openExistingInfo(book, sourceName = '') {
  overlay.openBookInfo(book, {
    sourceName,
    statusLabel: '已在书架',
    statusType: 'warning',
    progress: book.progress?.percent || 0,
    actions: [
      { label: '完整详情', plain: true, handler: () => openExistingDetail(book) },
      { label: '继续阅读', type: 'primary', handler: () => openExistingReader(book) },
    ],
  })
}

function openExistingReader(book) {
  overlay.closeBookInfo()
  router.push({ name: 'reader', params: { id: book.id } })
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.discover-page {
  display: grid;
  min-width: 0;
  gap: 16px;
}

.discover-head,
.discover-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  justify-content: space-between;
}

.discover-toolbar {
  min-width: 0;
  flex-wrap: wrap;
  justify-content: flex-start;
  padding: 12px;
}

.discover-toolbar .el-select {
  min-width: min(280px, 100%);
}

.source-status {
  color: var(--app-text-muted);
  font-size: 13px;
}

.discover-results {
  display: grid;
  min-width: 0;
  grid-template-columns: repeat(auto-fill, minmax(min(320px, 100%), 1fr));
  gap: 14px;
}

.discover-card {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 14px;
  align-items: center;
  padding: 14px;
  cursor: pointer;
}

.discover-card h2 {
  margin: 0 0 6px;
  font-size: 18px;
}

.discover-card p {
  margin: 0;
  color: var(--app-text-muted);
}

.discover-card .intro {
  display: -webkit-box;
  margin-top: 8px;
  overflow: hidden;
  line-height: 1.6;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.discover-card .latest {
  margin-top: 4px;
  color: var(--app-accent);
  font-size: 13px;
}

.load-more-row {
  display: flex;
  justify-content: center;
}

.preview-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  justify-content: center;
}

.preview-actions .el-select {
  min-width: 180px;
  flex: 1;
}

@media (max-width: 860px), (hover: none) and (pointer: coarse) {
  .discover-page {
    gap: 8px;
    padding-bottom: 14px;
  }

  .discover-head,
  .discover-toolbar {
    display: grid;
    gap: 8px;
    justify-content: stretch;
  }

  .discover-head .app-page-subtitle {
    display: none;
  }

  .discover-toolbar {
    padding: 8px;
  }

  .discover-toolbar .el-select,
  .discover-toolbar :deep(.el-button) {
    width: 100%;
  }

  .discover-toolbar :deep(.el-button) {
    min-height: 38px;
  }

  .source-status {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .discover-results {
    gap: 8px;
    grid-template-columns: minmax(0, 1fr);
  }

  .discover-card {
    grid-template-columns: 42px minmax(0, 1fr);
    gap: 10px;
    padding: 10px;
  }

  .discover-card > div {
    min-width: 0;
  }

  .discover-card h2 {
    overflow: hidden;
    font-size: 16px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .discover-card p {
    min-width: 0;
    overflow: hidden;
    font-size: 12px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .discover-card .intro {
    -webkit-line-clamp: 2;
    white-space: normal;
  }

  .load-more-row {
    display: grid;
  }

  .load-more-row :deep(.el-button) {
    width: 100%;
    min-height: 38px;
  }
}
</style>
