<template>
  <section class="app-page discover-page">
    <header class="discover-head">
      <div>
        <h1 class="app-page-title">书海</h1>
        <p class="discover-subtitle">{{ sourceCountText }}</p>
      </div>
      <el-button :icon="Refresh" :loading="loadingSources" @click="loadSources">刷新书源</el-button>
    </header>

    <section class="discover-toolbar app-panel">
      <el-select v-model="targetCategoryId" placeholder="加入书架分组（可选）" clearable>
        <el-option label="未分组" value="" />
        <el-option v-for="category in bookshelf.categories" :key="category.id" :label="category.name" :value="String(category.id)" />
      </el-select>
      <span v-if="activeSource" class="source-status">
        {{ activeSource.name }} · {{ activeExploreName || '默认' }} · 第 {{ page }} 页
      </span>
    </section>

    <section class="source-group-tabs app-panel">
      <button
        v-for="group in sourceGroups"
        :key="group.value"
        type="button"
        :class="{ active: selectedGroup === group.value }"
        @click="selectGroup(group.value)"
      >
        {{ group.label }} <span>{{ group.count }}</span>
      </button>
    </section>

    <div class="discover-main">
      <aside class="source-panel app-panel">
        <el-collapse v-model="expandedSources" accordion>
          <el-collapse-item v-for="source in filteredSources" :key="source.id" :name="String(source.id)">
            <template #title>
              <span class="source-title">{{ source.name }}</span>
              <span class="source-group">{{ source.group || '未分组' }}</span>
            </template>
            <div v-for="(group, groupIndex) in sourceExploreGroups(source)" :key="`${source.id}-${groupIndex}`" class="explore-entry-row">
              <button
                v-for="entry in group"
                :key="entry.url"
                type="button"
                :class="{ active: selectedSourceId === source.id && activeExploreUrl === entry.url }"
                @click="loadBooksFromEntry(source, entry)"
              >
                {{ entry.name }}
              </button>
            </div>
          </el-collapse-item>
        </el-collapse>
        <el-empty v-if="!loadingSources && !filteredSources.length" description="没有配置 exploreUrl 的书源" />
      </aside>

      <section>
        <div v-loading="loadingBooks" class="discover-results">
          <RemoteBookResultGroups v-if="books.length" :groups="exploreResultGroups" @preview="openPreview" />
          <el-empty v-if="!loadingBooks && !books.length" :description="sources.length ? '选择左侧书源入口开始探索' : '没有配置 exploreUrl 的书源'" />
        </div>

        <div v-if="books.length" class="load-more-row">
          <el-button :loading="loadingMore" :disabled="!hasMore" @click="loadMoreBooks">
            {{ hasMore ? '加载更多' : '没有更多结果' }}
          </el-button>
        </div>
      </section>
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
import RemoteBookResultGroups from '../components/RemoteBookResultGroups.vue'
import { useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore } from '../stores/reader'
import { newestBookProgress } from '../utils/bookOrder'
import { readerRouteQueryFromBook } from '../utils/readerRoute'
import {
  remoteBookCreatePayload,
  remoteBookKey,
  remoteBookSourceId,
  remoteBookSourceName,
  remoteBookTitle,
  remoteBookUrl,
} from '../utils/remoteBookResult'

const router = useRouter()
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const reader = useReaderStore()
const sources = ref([])
const books = ref([])
const selectedSourceId = ref('')
const selectedGroup = ref('')
const activeExploreUrl = ref('')
const activeExploreName = ref('')
const targetCategoryId = ref('')
const loadingSources = ref(false)
const loadingBooks = ref(false)
const addingBook = ref(null)
const page = ref(1)
const hasMore = ref(false)
const loadingMore = ref(false)
const expandedSources = ref('')

const activeSource = computed(() => sources.value.find(source => source.id === selectedSourceId.value))
const sourceCountText = computed(() => {
  if (!sources.value.length) return '暂无可用书源'
  if (!selectedGroup.value) return `共 ${sources.value.length} 个可用书源`
  return `${selectedGroup.value} · ${filteredSources.value.length} / ${sources.value.length} 个可用书源`
})
const exploreResultGroups = computed(() => {
  const groups = new Map()
  for (const book of books.value) {
    const key = activeRemoteSourceId(book)
    if (!groups.has(key)) {
      groups.set(key, {
        sourceId: key,
        sourceName: activeRemoteSourceName(book),
        items: [],
      })
    }
    groups.get(key).items.push(book)
  }
  return [...groups.values()]
})
const sourceGroups = computed(() => {
  const groups = new Map()
  for (const source of sources.value) {
    const name = source.group || '未分组'
    groups.set(name, (groups.get(name) || 0) + 1)
  }
  return [
    { label: '全部', value: '', count: sources.value.length },
    ...[...groups.entries()].map(([label, count]) => ({ label, value: label, count })).sort((a, b) => a.label.localeCompare(b.label)),
  ]
})
const filteredSources = computed(() => {
  if (!selectedGroup.value) return sources.value
  return sources.value.filter(source => (source.group || '未分组') === selectedGroup.value)
})

onMounted(async () => {
  await Promise.all([loadSources(), bookshelf.loadCategories(), bookshelf.loadBooks({ all: true })])
  if (selectedSourceId.value) await loadBooks()
})

async function loadSources() {
  loadingSources.value = true
  try {
    const { data } = await listExploreSources()
    sources.value = data || []
    ensureActiveEntry()
  } catch (err) {
    ElMessage.error(readError(err, '加载探索书源失败'))
  } finally {
    loadingSources.value = false
  }
}

function selectGroup(group) {
  selectedGroup.value = group
  const exists = filteredSources.value.some(source => source.id === selectedSourceId.value)
  if (!exists) {
    selectedSourceId.value = ''
    activeExploreUrl.value = ''
    activeExploreName.value = ''
    ensureActiveEntry()
  }
  books.value = []
  hasMore.value = false
  if (selectedSourceId.value) loadBooks()
}

function ensureActiveEntry() {
  if (selectedSourceId.value && activeExploreUrl.value) {
    expandedSources.value = String(selectedSourceId.value)
    return
  }
  const source = filteredSources.value[0]
  const entry = source ? firstExploreEntry(source) : null
  if (!source || !entry) return
  selectedSourceId.value = source.id
  activeExploreUrl.value = entry.url
  activeExploreName.value = entry.name
  expandedSources.value = String(source.id)
}

function sourceExploreGroups(source) {
  return Array.isArray(source?.exploreGroups) ? source.exploreGroups.filter(group => Array.isArray(group) && group.length) : []
}

function firstExploreEntry(source) {
  for (const group of sourceExploreGroups(source)) {
    if (group[0]) return group[0]
  }
  return null
}

function loadBooksFromEntry(source, entry) {
  selectedSourceId.value = source.id
  activeExploreUrl.value = entry.url
  activeExploreName.value = entry.name
  expandedSources.value = String(source.id)
  loadBooks()
}

async function loadBooks() {
  ensureActiveEntry()
  if (!selectedSourceId.value || !activeExploreUrl.value) return
  loadingBooks.value = true
  try {
    page.value = 1
    const { data } = await exploreBooks(selectedSourceId.value, { page: page.value, url: activeExploreUrl.value })
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
  if (!selectedSourceId.value || !activeExploreUrl.value || loadingMore.value || !hasMore.value) return
  loadingMore.value = true
  try {
    const nextPage = page.value + 1
    const { data } = await exploreBooks(selectedSourceId.value, { page: nextPage, url: activeExploreUrl.value })
    const result = normalizeExploreResult(data, nextPage)
    const known = new Set(books.value.map(book => activeRemoteKey(book)))
    const nextItems = result.items.filter(book => !known.has(activeRemoteKey(book)))
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
    sourceName: activeRemoteSourceName(book),
    statusLabel: existing ? '已在书架' : '探索结果',
    statusType: existing ? 'warning' : 'success',
    progress: existingProgress(existing)?.percent || 0,
    actions: existing
      ? [
          { label: '查看详情', plain: true, handler: () => openExistingInfo(existing, activeRemoteSourceName(book)) },
          { label: '继续阅读', type: 'primary', handler: () => openExistingReader(existing) },
        ]
      : [
          { label: '加入书架', plain: true, loading: addingBook.value === activeRemoteKey(book), handler: () => addRemoteBook(book, false) },
          { label: '加入并阅读', type: 'primary', loading: addingBook.value === activeRemoteKey(book), handler: () => addRemoteBook(book, true) },
        ],
  })
}

async function addRemoteBook(book, shouldRead) {
  addingBook.value = activeRemoteKey(book)
  try {
    const payload = remoteBookCreatePayload(book, targetCategoryId.value, {
      sourceId: activeRemoteSourceId(book),
      sourceName: activeRemoteSourceName(book),
    })
    const { data } = await createRemoteBook(payload)
    bookshelf.upsertBook(data)
    ElMessage.success(`已加入书架：《${remoteBookTitle(book)}》`)
    if (shouldRead) {
      overlay.closeBookInfo()
      router.push({ name: 'reader', params: { id: data.id } })
      return
    }
    overlay.openBookInfo(data, {
      sourceName: activeRemoteSourceName(book),
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
    Number(item.sourceId || 0) === Number(activeRemoteSourceId(book) || 0)
    && String(item.url || item.bookUrl || '') === String(remoteBookUrl(book) || '')
  )) || null
}

function activeRemoteSourceId(book) {
  return remoteBookSourceId(book, activeSource.value?.id) || 'unknown'
}

function activeRemoteSourceName(book) {
  return remoteBookSourceName(book, activeSource.value?.name)
}

function activeRemoteKey(book) {
  return remoteBookKey(book, activeRemoteSourceId(book))
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
    progress: existingProgress(book)?.percent || 0,
    actions: [
      { label: '完整详情', plain: true, handler: () => openExistingDetail(book) },
      { label: '继续阅读', type: 'primary', handler: () => openExistingReader(book) },
    ],
  })
}

function openExistingReader(book) {
  overlay.closeBookInfo()
  router.push({ name: 'reader', params: { id: book.id }, query: readerRouteQuery(book) })
}

function readerRouteQuery(book) {
  return readerRouteQueryFromBook(book, existingProgress(book))
}

function existingProgress(book) {
  return newestBookProgress(book, reader.progressByBook)
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

.discover-subtitle {
  margin: 4px 0 0;
  color: var(--app-text-muted);
  font-size: 13px;
}

.discover-toolbar .el-select {
  min-width: min(280px, 100%);
}

.source-status {
  color: var(--app-text-muted);
  font-size: 13px;
}

.source-group-tabs {
  display: flex;
  min-width: 0;
  gap: 8px;
  overflow-x: auto;
  padding: 8px;
}

.source-group-tabs button {
  border: 0;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: var(--app-text);
  cursor: pointer;
  font: inherit;
  min-height: 34px;
  padding: 0 16px;
  white-space: nowrap;
}

.source-group-tabs button.active {
  border-color: var(--app-accent);
  color: var(--app-accent);
}

.source-group-tabs span {
  color: var(--app-text-muted);
  font-size: 12px;
}

.discover-main {
  display: grid;
  min-width: 0;
  grid-template-columns: minmax(240px, 320px) minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}

.source-panel {
  min-width: 0;
  padding: 8px 12px;
}

.source-panel :deep(.el-collapse),
.source-panel :deep(.el-collapse-item__wrap) {
  border: 0;
}

.source-panel :deep(.el-collapse-item__header) {
  min-width: 0;
  gap: 8px;
  border: 0;
}

.source-title {
  min-width: 0;
  overflow: hidden;
  flex: 1;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.source-group {
  color: var(--app-text-muted);
  font-size: 12px;
}

.explore-entry-row {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  gap: 8px;
  padding: 4px 0 8px;
}

.explore-entry-row button {
  min-width: 64px;
  max-width: 100%;
  min-height: 32px;
  border: 1px solid var(--app-border);
  border-radius: 4px;
  background: var(--app-surface);
  color: var(--app-text);
  cursor: pointer;
  padding: 4px 10px;
}

.explore-entry-row button.active {
  border-color: var(--app-accent);
  color: var(--app-accent);
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

@media (max-width: 750px) {
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

  .source-group-tabs {
    margin-right: -8px;
    margin-left: -8px;
    border-radius: 0;
  }

  .discover-main {
    grid-template-columns: minmax(0, 1fr);
    gap: 8px;
  }

  .source-panel {
    padding: 6px 8px;
  }

  .source-panel :deep(.el-collapse-item__header) {
    min-height: 38px;
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
