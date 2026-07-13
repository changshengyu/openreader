<template>
  <section class="app-page search-page workspace-result-page">
    <header class="workspace-result-head">
      <div>
        <h1 class="app-page-title">搜索 ({{ searchMode === 'local' ? shownLocalResults.length : results.length }})</h1>
        <p class="workspace-result-subtitle">{{ searchMode === 'local' ? '本地书籍' : '书源搜索' }}</p>
      </div>
      <div class="workspace-result-actions">
        <button type="button" @click="backToShelf">书架</button>
      </div>
    </header>

    <div v-loading="searching" class="result-area">
      <RemoteBookResultGroups
        v-if="searchMode === 'remote' && groupedResults.length"
        :groups="groupedResults"
        @preview="openPreview"
      />

      <div v-else-if="searchMode === 'local' && shownLocalResults.length" class="local-result-list">
        <article
          v-for="item in shownLocalResults"
          :key="localResultKey(item)"
          class="local-result-card app-panel"
        >
          <el-icon class="local-file-icon"><Document /></el-icon>
          <div class="result-main">
            <div class="result-title">
              <h3>{{ localBookTitle(item) }}</h3>
              <el-tag size="small" :type="item.book ? 'success' : 'info'" effect="plain">{{ item.book ? '已在书架' : (item.extension || '文件') }}</el-tag>
            </div>
            <p>{{ localBookSubline(item) }}</p>
            <p class="latest-chapter">{{ localBookMeta(item) }}</p>
          </div>
          <div class="result-actions" @click.stop>
            <template v-if="item.book">
              <el-button type="primary" size="small" @click="readLocalShelfBook(item.book)">阅读</el-button>
              <el-button size="small" @click="openLocalShelfDetail(item.book)">详情</el-button>
            </template>
            <el-button v-else type="primary" size="small" :loading="importingLocal" @click="importLocalOne(item)">导入书架</el-button>
          </div>
        </article>
      </div>

      <el-empty v-else-if="searched && !searching" :description="searchMode === 'local' ? '没有找到本地书籍文件' : '没有找到相关书籍'" />
      <el-empty v-else :description="searchMode === 'local' ? '输入关键词搜索本地书仓，或直接搜索显示全部可导入文件' : '输入关键词后开始搜索书源'" />
    </div>

    <div v-if="searchMode === 'remote' && searched && (results.length || remoteHasMore)" class="load-more-row">
      <el-button :loading="loadingMore" :disabled="!remoteHasMore" @click="loadMoreRemote">
        {{ remoteHasMore ? '加载更多' : '没有更多' }}
      </el-button>
    </div>

  </section>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Document } from '@element-plus/icons-vue'
import { createRemoteBook } from '../api/books'
import { importFromLocalStore, listLocalStore } from '../api/localStore'
import api from '../api/client'
import RemoteBookResultGroups from '../components/RemoteBookResultGroups.vue'
import { useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore } from '../stores/reader'
import { usePreferencesStore } from '../stores/preferences'
import { useIndexWorkspaceStore } from '../stores/indexWorkspace'
import { useBookInfoAddToShelf } from '../composables/useBookInfoAddToShelf'
import {
  buildBookInfoReadActions,
  buildBookInfoStartReadActions,
  buildSearchAddBookActions,
  buildSearchExistingBookActions,
} from '../utils/bookInfoOverlayActions'
import { newestBookProgress } from '../utils/bookOrder'
import { isLocalBook, localBookSearchText, normalizeLocalBookSearch } from '../utils/localBook'
import { readerRouteQueryFromBook } from '../utils/readerRoute'
import {
  remoteBookCreatePayload,
  remoteBookKey,
  remoteBookSourceId,
  remoteBookSourceName,
  remoteBookUrl,
} from '../utils/remoteBookResult'

const router = useRouter()
const emit = defineEmits(['back-to-shelf'])
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const reader = useReaderStore()
const preferences = usePreferencesStore()
const workspace = useIndexWorkspaceStore()

const keyword = ref('')
const searchMode = ref('remote')
const sources = ref([])
const selectedIds = ref([])
const selectedGroup = ref(preferences.search.group)
const singleSourceId = ref(Number(preferences.search.sourceId || 0) || null)
const targetCategoryIds = ref([])
const searchType = ref(preferences.search.searchType)
const concurrentOptions = [8, 16, 32, 60]
const concurrentCount = ref(concurrentOptions.includes(Number(preferences.search.concurrent)) ? Number(preferences.search.concurrent) : 60)
const results = ref([])
const searching = ref(false)
const loadingMore = ref(false)
const searched = ref(false)
const searchPage = ref(1)
const searchLastIndex = ref(-1)
const remoteHasMore = ref(false)
const activeSearchKeyword = ref('')
const activeSourceIds = ref([])
const activeConcurrentCount = ref(1)
const addToShelf = useBookInfoAddToShelf({
  selectCategories: initialCategoryIds => overlay.selectBookAddCategories(initialCategoryIds),
  buildPayload: (book, categoryIds, context) => remoteBookCreatePayload(book, categoryIds, context),
  createRemoteBook,
  upsertBook: book => bookshelf.upsertBook(book),
  onSuccess: message => ElMessage.success(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})
const addingBook = addToShelf.addingBookKey
const localItems = ref([])
const localRecursiveScan = ref(true)
const importingLocal = ref(false)
const workspaceSearchReady = ref(false)

const enabledSources = computed(() => sources.value.filter(source => source.enabled))
const groupedResults = computed(() => {
  const groups = new Map()
  for (const item of results.value) {
    const key = remoteBookSourceId(item) || remoteBookSourceName(item) || 'unknown'
    if (!groups.has(key)) {
      groups.set(key, {
        sourceId: key,
        sourceName: remoteBookSourceName(item),
        items: [],
      })
    }
    groups.get(key).items.push(item)
  }
  return [...groups.values()]
})

const localShelfBooks = computed(() => (bookshelf.books || []).filter(isLocalBook))
const shownLocalResults = computed(() => {
  if (!searched.value || searchMode.value !== 'local') return []
  const value = normalizeLocalSearch(keyword.value)
  const shelfResults = localShelfBooks.value
    .filter(book => !value || localShelfSearchText(book).includes(value))
    .map(book => ({
      type: 'shelf',
      book,
      name: book.title,
      path: book.originalFile || book.libraryPath || book.url || '',
      extension: fileExtension(book.originalFile || book.libraryPath || book.title),
      importable: false,
    }))
  const storeResults = localItems.value
    .filter(item => {
      if (!item.importable) return false
      if (!value) return true
      return localFileSearchText(item).includes(value)
    })
    .map(item => ({ ...item, type: 'file' }))
  return [...shelfResults, ...storeResults]
})
onMounted(async () => {
  applyWorkspaceSearchIntent()
  await warmSearchShelf()
  if (searchMode.value === 'remote') {
    try {
      await loadSources()
    } catch (err) {
      ElMessage.warning(readError(err, '加载书源失败'))
    }
  } else {
    loadSources().catch(() => {})
  }
  syncSelection()
  workspaceSearchReady.value = true
  if (keyword.value || searchMode.value === 'local') doSearch()
})

async function warmSearchShelf() {
  const jobs = [
    ['categories', bookshelf.ensureCategoriesLoaded()],
    ['books', bookshelf.ensureBooksLoaded({ all: true })],
  ]
  const results = await Promise.allSettled(jobs.map(([, job]) => job))
  results.forEach((result, index) => {
    if (result.status !== 'rejected') return
    const type = jobs[index][0]
    if (type === 'books') {
      ElMessage.warning(readError(result.reason, '加载书架失败，部分已入架状态可能暂不可用'))
    } else {
      ElMessage.warning(readError(result.reason, '分组加载失败，部分筛选状态可能暂不可用'))
    }
  })
}

watch(searchType, () => {
  syncSelection()
  saveSearchPreference()
})
watch([selectedGroup, singleSourceId, concurrentCount], saveSearchPreference)
watch(
  () => [workspace.mode, workspace.searchRevision],
  () => {
    if (workspace.mode !== 'search') return
    applyWorkspaceSearchIntent()
    if (!workspaceSearchReady.value) return
    if (keyword.value || searchMode.value === 'local') doSearch()
  },
)

async function loadSources() {
  const { data } = await api.get('/sources')
  sources.value = data
  if (!selectedGroup.value && sourceGroups.value.length) selectedGroup.value = sourceGroups.value[0].value
  if (!singleSourceId.value && enabledSources.value.length) singleSourceId.value = enabledSources.value[0].id
}

function syncSelection() {
  if (searchType.value === 'all') {
    selectedIds.value = enabledSources.value.map(source => source.id)
  } else if (searchType.value === 'group') {
    selectedIds.value = enabledSources.value
      .filter(source => (source.group || '默认分组') === selectedGroup.value)
      .map(source => source.id)
  } else if (searchType.value === 'single') {
    selectedIds.value = singleSourceId.value ? [singleSourceId.value] : []
  }
}

function saveSearchPreference() {
  if (searchType.value === 'custom') return
  preferences.setSearchConfig({
    searchType: searchType.value,
    group: selectedGroup.value,
    sourceId: singleSourceId.value || '',
    concurrent: concurrentCount.value,
  })
}

async function doSearch() {
  if (searchMode.value === 'local') {
    await searchLocalBooks()
    return
  }
  const value = keyword.value.trim()
  if (!value) return
  if (!selectedIds.value.length) {
    ElMessage.warning('请至少选择一个书源')
    return
  }
  workspace.setResultLoading(true)
  searching.value = true
  searched.value = false
  results.value = []
  resetRemotePagination()
  activeSearchKeyword.value = value
  activeSourceIds.value = [...selectedIds.value]
  activeConcurrentCount.value = searchType.value === 'single' ? 1 : concurrentCount.value
  try {
    const added = await requestRemoteSearch(false)
    searched.value = true
    ElMessage.success(added ? `找到 ${added} 条结果` : '没有找到相关书籍')
  } catch (err) {
    ElMessage.error(readError(err, '搜索失败'))
  } finally {
    searching.value = false
    workspace.setResultLoading(false)
  }
}

async function loadMoreRemote() {
  if (loadingMore.value || !remoteHasMore.value) return
  loadingMore.value = true
  workspace.setResultLoading(true)
  try {
    searchPage.value += 1
    const added = await requestRemoteSearch(true)
    if (!added) {
      ElMessage.info(remoteHasMore.value ? '本批没有新增结果' : '没有更多了')
    }
  } catch (err) {
    searchPage.value = Math.max(1, searchPage.value - 1)
    ElMessage.error(readError(err, '加载更多失败'))
  } finally {
    loadingMore.value = false
    workspace.setResultLoading(false)
  }
}

async function requestRemoteSearch(append) {
  const { data } = await api.post('/search', {
    keyword: activeSearchKeyword.value,
    sourceIds: activeSourceIds.value,
    concurrentCount: activeConcurrentCount.value,
    page: searchPage.value,
    lastIndex: searchLastIndex.value,
    searchSize: 20,
  })
  const incoming = Array.isArray(data) ? data : (data?.list || [])
  const added = appendRemoteResults(incoming, append)
  searchPage.value = Number(data?.page || searchPage.value)
  searchLastIndex.value = Number.isInteger(data?.lastIndex) ? data.lastIndex : searchLastIndex.value
  remoteHasMore.value = Boolean(data?.hasMore)
  workspace.replaceResultRows(results.value, remoteWorkspaceContinuation())
  return added
}

function appendRemoteResults(incoming, append) {
  const next = append ? [...results.value] : []
  const seen = new Set(next.map(remoteResultDedupKey).filter(Boolean))
  let added = 0
  for (const item of incoming) {
    const key = remoteResultDedupKey(item)
    if (key && seen.has(key)) continue
    if (key) seen.add(key)
    next.push(item)
    added += 1
  }
  results.value = next
  return added
}

function remoteResultDedupKey(item) {
  return remoteBookUrl(item) || remoteBookKey(item)
}

function resetRemotePagination() {
  searchPage.value = 1
  searchLastIndex.value = -1
  remoteHasMore.value = false
  loadingMore.value = false
  activeSearchKeyword.value = ''
  activeSourceIds.value = []
  activeConcurrentCount.value = 1
}

async function searchLocalBooks() {
  workspace.setResultLoading(true)
  searching.value = true
  searched.value = false
  results.value = []
  try {
    const [storeResult, shelfResult] = await Promise.allSettled([
      listLocalStore('', localRecursiveScan.value),
      bookshelf.loadBooks({ all: true }),
    ])
    if (storeResult.status === 'rejected' && shelfResult.status === 'rejected') {
      throw storeResult.reason || shelfResult.reason
    }
    localItems.value = storeResult.status === 'fulfilled' ? (storeResult.value.data.items || []) : []
    searched.value = true
    workspace.replaceResultRows(shownLocalResults.value, {
      page: 1,
      lastIndex: -1,
      hasMore: false,
    })
    if (shelfResult.status === 'rejected') {
      ElMessage.warning(`书架本地书加载失败，已仅搜索本地书仓：${readError(shelfResult.reason, '加载失败')}`)
    }
    if (storeResult.status === 'rejected') {
      ElMessage.warning(`本地书仓扫描失败，已仅搜索书架本地书：${readError(storeResult.reason, '扫描失败')}`)
      return
    }
    ElMessage.success(shownLocalResults.value.length ? `找到 ${shownLocalResults.value.length} 条本地结果` : '没有找到本地书籍')
  } catch (err) {
    ElMessage.error(readError(err, '搜索本地书仓失败'))
  } finally {
    searching.value = false
    workspace.setResultLoading(false)
  }
}

function remoteWorkspaceContinuation() {
  return {
    page: searchPage.value,
    lastIndex: searchLastIndex.value,
    hasMore: remoteHasMore.value,
  }
}

function applyWorkspaceSearchIntent() {
  const intent = workspace.search
  searchMode.value = intent.mode === 'local' ? 'local' : 'remote'
  keyword.value = intent.keyword || ''
  searchType.value = ['all', 'group', 'single', 'custom'].includes(intent.searchType) ? intent.searchType : 'all'
  selectedGroup.value = intent.group || ''
  singleSourceId.value = Number(intent.sourceId || 0) || null
  concurrentCount.value = concurrentOptions.includes(Number(intent.concurrent)) ? Number(intent.concurrent) : 60
}

function backToShelf() {
  workspace.backToShelf()
  emit('back-to-shelf')
}

async function importLocalOne(item) {
  if (!item?.importable) return
  await importLocalPaths([item.path])
}

async function importLocalPaths(paths) {
  importingLocal.value = true
  try {
    const categoryIds = targetCategoryIds.value.map(Number).filter(Boolean)
    const { data } = await importFromLocalStore(paths, categoryIds)
    const imported = data.imported || []
    imported.forEach(item => {
      if (item.book) bookshelf.upsertBook(item.book)
    })
    markImportedLocalItems(imported)
    const success = imported.filter(item => item.book).length
    const failed = imported.filter(item => item.error).length
    ElMessage.success(`导入 ${success} 本` + (failed ? `，${failed} 本失败` : ''))
  } catch (err) {
    ElMessage.error(readError(err, '导入本地书失败'))
  } finally {
    importingLocal.value = false
  }
}

function markImportedLocalItems(imported) {
  const importedByPath = new Map(
    imported
      .filter(item => item?.book && item?.path)
      .map(item => [item.path, item.book]),
  )
  if (!importedByPath.size) return
  localItems.value = localItems.value.map(item => {
    const book = importedByPath.get(item.path)
    if (!book) return item
    return { ...item, book, importable: false }
  })
}

function localBookTitle(item) {
  if (item?.book) return item.book.title || '未命名本地书'
  return String(item?.name || '未命名本地书').replace(/\.[^.]+$/, '')
}

function localBookSubline(item) {
  if (item?.book) {
    const parts = []
    if (item.book.author) parts.push(item.book.author)
    if (item.book.chapterCount) parts.push(`共${item.book.chapterCount}章`)
    return parts.join(' · ') || item.path || '本地书籍'
  }
  return item?.path || ''
}

function localBookMeta(item) {
  if (item?.book) {
    if (item.book.lastChapter) return `最新：${item.book.lastChapter}`
    if (item.path) return `来源：${item.path}`
    return '已导入书架'
  }
  return `大小：${formatSize(item?.size)}`
}

function localResultKey(item) {
  return item?.book ? `shelf-${item.book.id}` : `file-${item.path}`
}

function localShelfSearchText(book) {
  return localBookSearchText(book, [
    localBookSubline({ book }),
    localBookMeta({ book }),
  ])
}

function localFileSearchText(item) {
  return normalizeLocalSearch([
    item.name,
    item.path,
    item.extension,
    item.mimeType,
  ].filter(Boolean).join(' '))
}

function normalizeLocalSearch(value) {
  return normalizeLocalBookSearch(value)
}

function fileExtension(value) {
  const match = String(value || '').match(/\.([^.\\/]+)$/)
  return match ? match[1].toUpperCase() : '本地'
}

function readLocalShelfBook(book) {
  router.push({ name: 'reader', params: { id: book.id }, query: readerRouteQueryForLocalBook(book) })
}

function readerRouteQueryForLocalBook(book) {
  return readerRouteQueryFromBook(book, readerProgressForBook(book))
}

function readerProgressForBook(book) {
  const shelfBook = bookshelf.books.find(item => item.id === book?.id)
  const mergedBook = shelfBook ? { ...book, progress: shelfBook.progress || book?.progress } : book
  return newestBookProgress(mergedBook, reader.progressByBook)
}

function openLocalShelfDetail(book) {
  overlay.openBookInfo(book, {
    statusLabel: '本地书籍',
    statusType: 'info',
  })
}

function formatSize(bytes) {
  if (!bytes) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

async function addRemoteBook(item, shouldRead) {
  const key = remoteBookKey(item)
  const data = await addToShelf.addRemoteBook(item, {
    key,
    categoryIds: targetCategoryIds.value,
    sourceId: remoteBookSourceId(item),
    sourceName: remoteBookSourceName(item),
  })
  if (!data) return
  if (shouldRead) {
    overlay.closeBookInfo()
    router.push({ name: 'reader', params: { id: data.id } })
    return
  }
  overlay.openBookInfo(data, {
    sourceName: remoteBookSourceName(item),
    statusLabel: '已加入书架',
    statusType: 'success',
    progress: 0,
    actions: buildBookInfoStartReadActions({ read: () => openExistingReader(data) }),
  })
}

function openPreview(item) {
  const existing = findExistingBook(item)
  overlay.openBookInfo(item, {
    sourceName: remoteBookSourceName(item),
    statusLabel: existing ? '已在书架' : '搜索结果',
    statusType: existing ? 'warning' : 'success',
    progress: readerProgressForBook(existing)?.percent || 0,
    actions: existing
      ? buildSearchExistingBookActions({
          openInfo: () => openExistingInfo(existing, remoteBookSourceName(item)),
          read: () => openExistingReader(existing),
        })
      : buildSearchAddBookActions({
          add: () => addRemoteBook(item, false),
          addAndRead: () => addRemoteBook(item, true),
          loading: addingBook.value === remoteBookKey(item),
        }),
  })
}

function findExistingBook(item) {
  return bookshelf.books.find(book => (
    Number(book.sourceId || 0) === Number(remoteBookSourceId(item) || 0)
    && String(book.url || book.bookUrl || '') === String(remoteBookUrl(item) || '')
  )) || null
}

function openExistingInfo(book, sourceName = '') {
  overlay.openBookInfo(book, {
    sourceName,
    statusLabel: '已在书架',
    statusType: 'warning',
    progress: readerProgressForBook(book)?.percent || 0,
    actions: buildBookInfoReadActions({ read: () => openExistingReader(book) }),
  })
}

function openExistingReader(book) {
  overlay.closeBookInfo()
  router.push({ name: 'reader', params: { id: book.id }, query: readerRouteQueryForLocalBook(book) })
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.search-page {
  display: grid;
  min-width: 0;
  gap: 16px;
}

.workspace-result-page {
  grid-template-rows: auto minmax(0, 1fr) auto;
  box-sizing: border-box;
  height: 100vh;
  max-height: 100vh;
  gap: 0;
  padding: 48px;
  overflow: hidden;
}

.workspace-result-head {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 4px 0 18px;
  border-bottom: 1px solid var(--app-border);
}

.workspace-result-head > div:first-child {
  min-width: 0;
}

.workspace-result-head .app-page-title {
  margin: 0;
  color: #26394a;
  font-size: 22px;
  font-weight: 800;
  line-height: 1.25;
}

.workspace-result-subtitle {
  margin: 5px 0 0;
  color: var(--app-text-muted);
  font-size: 13px;
}

.workspace-result-actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
}

.workspace-result-actions button {
  padding: 0;
  color: #26394a;
  background: transparent;
  border: 0;
  cursor: pointer;
  font: inherit;
  font-size: 14px;
  font-weight: 700;
  line-height: 28px;
}

.workspace-result-actions button:hover {
  color: var(--app-accent);
}

.workspace-result-page .result-area {
  min-width: 0;
  min-height: 0;
  padding: 18px 0;
  overflow: auto;
  overscroll-behavior: contain;
}

.load-more-row {
  display: flex;
  justify-content: center;
  padding-bottom: 8px;
}

.search-head,
.search-console,
.search-options,
.search-status,
.result-card,
.result-title,
.result-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.search-head,
.result-title {
  justify-content: space-between;
}

.search-console {
  min-width: 0;
  flex-wrap: wrap;
  padding: 14px;
}

.mode-switch {
  max-width: 100%;
}

.search-console > .el-input {
  min-width: min(260px, 100%);
  flex: 1;
}

.search-options {
  min-width: 0;
  width: 100%;
  flex-wrap: wrap;
}

.search-options :deep(.el-select),
.search-options :deep(.el-radio-group) {
  max-width: 100%;
}

.source-collapse {
  width: 100%;
}

.source-checks {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 16px;
}

.search-status {
  flex-wrap: wrap;
}

.source-result-list,
.result-list,
.local-result-list {
  display: grid;
  min-width: 0;
  gap: 12px;
}

.source-result-group {
  display: grid;
  gap: 10px;
}

.source-result-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.source-result-head h2 {
  margin: 0;
  color: var(--app-text);
  font-size: 16px;
}

.result-card,
.local-result-card {
  padding: 14px;
  align-items: start;
  cursor: pointer;
}

.result-card:hover,
.local-result-card:hover,
.local-result-card.selected {
  border-color: var(--app-primary);
}

.local-result-card {
  display: flex;
  gap: 12px;
}

.local-file-icon {
  display: grid;
  width: 42px;
  height: 54px;
  place-items: center;
  flex: 0 0 42px;
  color: var(--app-primary-strong);
  background: var(--app-primary-soft);
  border-radius: 5px;
  font-size: 24px;
}

.result-main {
  display: grid;
  min-width: 0;
  flex: 1;
  gap: 6px;
}

.result-main h3,
.result-main p {
  margin: 0;
}

.result-main h3 {
  font-size: 17px;
}

.result-main p {
  color: var(--app-text-muted);
  font-size: 13px;
}

.result-intro {
  display: -webkit-box;
  overflow: hidden;
  line-height: 1.6;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.latest-chapter {
  color: var(--app-primary-strong) !important;
}

.result-actions {
  flex-wrap: wrap;
  justify-content: flex-end;
}

.preview-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 6px;
}

.preview-actions .el-select {
  min-width: 180px;
  flex: 1;
}

@media (max-width: 750px) {
  .search-page {
    gap: 8px;
    padding-bottom: 14px;
  }

  .workspace-result-page {
    height: 100vh;
    height: 100dvh;
    max-height: none;
    gap: 0;
    padding: 0;
  }

  .workspace-result-head {
    min-height: 64px;
    padding: max(16px, env(safe-area-inset-top)) 24px 12px;
  }

  .workspace-result-head .app-page-title {
    font-size: 20px;
  }

  .workspace-result-page .result-area {
    padding: 12px 20px calc(16px + env(safe-area-inset-bottom));
  }

  .search-head,
  .search-console,
  .search-options,
  .result-card,
  .result-actions {
    display: grid;
  }

  .search-head {
    gap: 6px;
  }

  .search-head :deep(.el-button),
  .search-console :deep(.el-button) {
    min-height: 38px;
  }

  .search-console {
    gap: 8px;
    padding: 8px;
  }

  .search-console > .el-input,
  .search-console > :deep(.el-button),
  .mode-switch,
  .search-options :deep(.el-select),
  .search-options :deep(.el-radio-group) {
    width: 100%;
  }

  .search-options,
  .local-search-options {
    gap: 8px;
  }

  .mode-switch,
  .search-options :deep(.el-radio-group) {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .mode-switch :deep(.el-radio-button__inner),
  .search-options :deep(.el-radio-button__inner) {
    display: block;
    min-height: 36px;
    overflow: hidden;
    padding: 8px 6px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .source-checks {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
  }

  .source-checks :deep(.el-checkbox) {
    min-width: 0;
    margin-right: 0;
  }

  .source-checks :deep(.el-checkbox__label) {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .search-status {
    gap: 6px;
  }

  .source-result-list,
  .result-list,
  .local-result-list {
    gap: 8px;
  }

  .source-result-head {
    align-items: flex-start;
    display: grid;
    gap: 4px;
  }

  .result-actions {
    justify-content: stretch;
  }

  .result-actions :deep(.el-button) {
    width: 100%;
    min-height: 36px;
    margin-left: 0;
  }

  .result-card,
  .local-result-card {
    grid-template-columns: 42px minmax(0, 1fr);
    gap: 10px;
    padding: 10px;
  }

  .local-result-card {
    display: grid;
    grid-template-columns: auto 34px minmax(0, 1fr);
  }

  .local-file-icon {
    width: 34px;
    height: 46px;
    font-size: 20px;
  }

  .result-title {
    display: grid;
    gap: 4px;
  }

  .result-main h3 {
    overflow: hidden;
    font-size: 16px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .result-main p {
    min-width: 0;
    font-size: 12px;
  }

  .result-intro {
    -webkit-line-clamp: 2;
  }
}
</style>
