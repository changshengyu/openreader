<template>
  <section class="app-page shelf-page result-shelf-page search-page">
    <div class="shelf-title">
      <div class="shelf-title-main">
        <strong>搜索 ({{ searchMode === 'local' ? shownLocalResults.length : results.length }})</strong>
      </div>
      <div class="title-actions">
        <button
          v-if="searchMode === 'remote'"
          type="button"
          :disabled="searching || loadingMore || (searched && !remoteHasMore)"
          @click="loadMoreRemote"
        >{{ remoteSearchActionLabel }}</button>
        <button type="button" @click="backToShelf">书架</button>
      </div>
    </div>

    <main class="shelf-main">
      <div ref="resultArea" class="books-wrapper">
        <RemoteBookResultList
          v-if="searchMode === 'remote'"
          :books="results"
          :adding-book-key="addingRemoteBookKey"
          :is-night="reader.themeType === 'night'"
          @preview="openPreview"
          @read="openRemoteReader"
          @add="addResultToShelf"
          @edit="openResultEditor"
        />

        <div v-else-if="shownLocalResults.length" class="local-result-list">
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

        <el-empty v-else-if="searched && !searching" description="没有找到本地书籍文件" />
        <el-empty v-else-if="searchMode === 'local'" description="输入关键词搜索本地书仓，或直接搜索显示全部可导入文件" />
      </div>
    </main>

    <RemoteBookJsonEditorDialog
      :visible="resultEditorVisible"
      :content="resultEditorContent"
      :saving="resultEditorSaving"
      :is-mobile="isMobileResult"
      @update:content="resultEditorContent = $event"
      @close="closeResultEditor"
      @save="saveResultEditor"
    />
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Document } from '@element-plus/icons-vue'
import { createRemoteReaderSession } from '../api/remoteReader'
import { createRemoteBook } from '../api/books'
import { importFromLocalStore, listLocalStore } from '../api/localStore'
import api from '../api/client'
import RemoteBookResultList from '../components/RemoteBookResultList.vue'
import RemoteBookJsonEditorDialog from '../components/RemoteBookJsonEditorDialog.vue'
import { useRemoteBookAddToShelf } from '../composables/useRemoteBookAddToShelf'
import { useRemoteBookResultEditor } from '../composables/useRemoteBookResultEditor'
import { useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore } from '../stores/reader'
import { usePreferencesStore } from '../stores/preferences'
import { useIndexWorkspaceStore } from '../stores/indexWorkspace'
import { newestBookProgress } from '../utils/bookOrder'
import { isLocalBook, localBookSearchText, normalizeLocalBookSearch } from '../utils/localBook'
import { readerRouteQueryFromBook } from '../utils/readerRoute'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation'
import {
  DEFAULT_SEARCH,
  normalizeSearchConcurrent,
} from '../utils/searchPreference.js'
import {
  remoteBookCreatePayload,
  remoteBookKey,
  remoteBookReaderPayload,
  remoteBookSourceId,
  remoteBookSourceName,
} from '../utils/remoteBookResult'
import {
  captureWorkspaceRequest,
  createAsyncRequestGate,
  isWorkspaceRequestCurrent,
  mergeRemoteSearchResults,
} from '../utils/workspaceContinuation.js'
import { currentViewportWidth, shouldUseMiniInterface } from '../utils/responsive.js'

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
const concurrentCount = ref(normalizeSearchConcurrent(preferences.search.concurrent))
const results = ref([])
const searching = ref(false)
const loadingMore = ref(false)
const searched = ref(false)
const searchStarting = ref(false)
const searchPage = ref(1)
const searchLastIndex = ref(-1)
const remoteHasMore = ref(false)
const activeSearchKeyword = ref('')
const activeSourceIds = ref([])
const activeConcurrentCount = ref(1)
const activeSearchIsSingleSource = ref(false)
const resultArea = ref(null)
const resultWindowWidth = ref(currentViewportWidth())
const remoteRequestGate = createAsyncRequestGate()
const searchSessionOperations = createAuthenticatedOperationGuard()
const resultAddToShelf = useRemoteBookAddToShelf({
  operationGuard: searchSessionOperations,
  selectCategories: initialCategoryIds => overlay.selectBookAddCategories(initialCategoryIds),
  buildPayload: (book, categoryIds, context) => remoteBookCreatePayload(book, categoryIds, context),
  createRemoteBook,
  upsertBook: book => bookshelf.upsertBook(book),
  onSuccess: message => ElMessage.success(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})
const addingRemoteBookKey = resultAddToShelf.addingBookKey
const resultEditor = useRemoteBookResultEditor({
  operationGuard: searchSessionOperations,
  confirm: (...args) => ElMessageBox.confirm(...args),
  createRemoteBook,
  upsertBook: book => bookshelf.upsertBook(book),
  onSuccess: message => ElMessage.success(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})
const {
  visible: resultEditorVisible,
  content: resultEditorContent,
  saving: resultEditorSaving,
} = resultEditor
const localItems = ref([])
const localRecursiveScan = ref(true)
const importingLocal = ref(false)
const workspaceSearchReady = ref(false)

const enabledSources = computed(() => sources.value.filter(source => source.enabled))
const isMobileResult = computed(() => shouldUseMiniInterface(reader.pageMode, resultWindowWidth.value))
const remoteSearchActionLabel = computed(() => {
  if (searchStarting.value || searching.value || loadingMore.value) return '加载中...'
  if (searched.value && !remoteHasMore.value) return '没有更多了'
  return '加载更多'
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
  updateResultViewport()
  window.addEventListener('resize', updateResultViewport)
  window.addEventListener('orientationchange', updateResultViewport)
  const mountOperation = searchSessionOperations.begin('mount')
  applyWorkspaceSearchIntent()
  const shelfReady = await warmSearchShelf()
  if (!shelfReady || !searchSessionOperations.canCommit(mountOperation)) return
  if (searchMode.value === 'remote') {
    try {
      const sourcesReady = await loadSources()
      if (!sourcesReady || !searchSessionOperations.canCommit(mountOperation)) return
    } catch (err) {
      if (!searchSessionOperations.canCommit(mountOperation)) return
      ElMessage.warning(readError(err, '加载书源失败'))
    }
  } else {
    loadSources().catch(() => {})
  }
  if (!searchSessionOperations.canCommit(mountOperation)) return
  syncSelection()
  workspaceSearchReady.value = true
  if (keyword.value || searchMode.value === 'local') doSearch()
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', updateResultViewport)
  window.removeEventListener('orientationchange', updateResultViewport)
  remoteRequestGate.invalidate()
  resultEditor.reset()
  searchSessionOperations.reset()
})

async function warmSearchShelf() {
  const operation = searchSessionOperations.begin('warm-shelf')
  const jobs = [
    ['categories', bookshelf.ensureCategoriesLoaded()],
    ['books', bookshelf.ensureBooksLoaded({ all: true })],
  ]
  const results = await Promise.allSettled(jobs.map(([, job]) => job))
  if (!searchSessionOperations.canCommit(operation)) return false
  results.forEach((result, index) => {
    if (result.status !== 'rejected') return
    const type = jobs[index][0]
    if (type === 'books') {
      ElMessage.warning(readError(result.reason, '加载书架失败，部分已入架状态可能暂不可用'))
    } else {
      ElMessage.warning(readError(result.reason, '分组加载失败，部分筛选状态可能暂不可用'))
    }
  })
  return true
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
  const operation = searchSessionOperations.begin('sources')
  try {
    const { data } = await api.get('/sources')
    if (!searchSessionOperations.canCommit(operation)) return false
    sources.value = data
    if (!singleSourceId.value && enabledSources.value.length) singleSourceId.value = enabledSources.value[0].id
    return true
  } catch (error) {
    if (!searchSessionOperations.canCommit(operation)) return false
    throw error
  }
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
    remoteRequestGate.invalidate()
    await searchLocalBooks()
    return
  }
  searchSessionOperations.invalidate('local-search')
  const value = keyword.value.trim()
  if (!value) {
    searchStarting.value = false
    searching.value = false
    return
  }
  if (!selectedIds.value.length) {
    searchStarting.value = false
    searching.value = false
    ElMessage.warning('未配置书源')
    return
  }
  workspace.setResultLoading(true)
  searching.value = true
  searchStarting.value = false
  searched.value = false
  results.value = []
  resetRemotePagination()
  const requestToken = remoteRequestGate.begin()
  const workspaceStamp = captureWorkspaceRequest(workspace, 'search')
  activeSearchKeyword.value = value
  activeSourceIds.value = [...selectedIds.value]
  activeConcurrentCount.value = searchType.value === 'single' ? 1 : concurrentCount.value
  activeSearchIsSingleSource.value = activeSourceIds.value.length === 1
  try {
    const { added, current } = await requestRemoteSearch({
      append: false,
      page: 1,
      lastIndex: -1,
      requestToken,
      workspaceStamp,
    })
    if (!current) return
    searched.value = true
    ElMessage.success(added ? `找到 ${added} 条结果` : '没有找到相关书籍')
  } catch (err) {
    if (isActiveRemoteRequest(requestToken, workspaceStamp)) {
      ElMessage.error(readError(err, '搜索失败'))
    }
  } finally {
    if (isActiveRemoteRequest(requestToken, workspaceStamp)) {
      searching.value = false
      workspace.setResultLoading(false)
    }
  }
}

async function loadMoreRemote() {
  if (loadingMore.value) return
  if (!remoteHasMore.value) {
    ElMessage.info('没有更多了')
    return
  }
  const requestToken = remoteRequestGate.begin()
  const workspaceStamp = captureWorkspaceRequest(workspace, 'search')
  const nextPage = searchPage.value + 1
  const nextLastIndex = activeSearchIsSingleSource.value ? -1 : searchLastIndex.value
  rememberResultScroll()
  loadingMore.value = true
  workspace.setResultLoading(true)
  try {
    const { added, current } = await requestRemoteSearch({
      append: true,
      page: nextPage,
      lastIndex: nextLastIndex,
      requestToken,
      workspaceStamp,
    })
    if (!current) return
    if (!added) {
      ElMessage.info(remoteHasMore.value ? '本批没有新增结果，仍可继续加载' : '没有更多了')
    }
  } catch (err) {
    if (isActiveRemoteRequest(requestToken, workspaceStamp)) {
      ElMessage.error(readError(err, '加载更多失败'))
    }
  } finally {
    if (isActiveRemoteRequest(requestToken, workspaceStamp)) {
      loadingMore.value = false
      workspace.setResultLoading(false)
    }
  }
}

function isActiveRemoteRequest(requestToken, workspaceStamp) {
  return remoteRequestGate.isCurrent(requestToken)
    && isWorkspaceRequestCurrent(workspace, workspaceStamp)
}

function remoteSearchPayload(page, lastIndex) {
  const payload = {
    keyword: activeSearchKeyword.value,
    sourceIds: activeSourceIds.value,
    concurrentCount: activeConcurrentCount.value,
  }
  if (activeSearchIsSingleSource.value) {
    payload.page = page
  } else {
    payload.lastIndex = lastIndex
    payload.searchSize = 20
  }
  return payload
}

async function requestRemoteSearch({ append, page, lastIndex, requestToken, workspaceStamp }) {
  const { data } = await api.post('/search', remoteSearchPayload(page, lastIndex))
  if (!isActiveRemoteRequest(requestToken, workspaceStamp)) {
    return { added: 0, current: false }
  }
  const incoming = Array.isArray(data) ? data : (data?.list || [])
  const { rows, added } = mergeRemoteSearchResults(append ? results.value : [], incoming)
  results.value = rows
  if (activeSearchIsSingleSource.value) {
    searchPage.value = Number(data?.page || page)
    searchLastIndex.value = -1
  } else {
    searchPage.value = page
    searchLastIndex.value = Number.isInteger(data?.lastIndex) ? data.lastIndex : lastIndex
  }
  remoteHasMore.value = Boolean(data?.hasMore)
  workspace.replaceResultRows(results.value, remoteWorkspaceContinuation())
  return { added, current: true }
}

function resetRemotePagination() {
  searchPage.value = 1
  searchLastIndex.value = -1
  remoteHasMore.value = false
  loadingMore.value = false
  activeSearchKeyword.value = ''
  activeSourceIds.value = []
  activeConcurrentCount.value = 1
  activeSearchIsSingleSource.value = false
}

function rememberResultScroll() {
  workspace.rememberResultScroll(resultArea.value?.scrollTop || 0)
}

async function searchLocalBooks() {
  const operation = searchSessionOperations.begin('local-search')
  const workspaceStamp = captureWorkspaceRequest(workspace, 'search')
  workspace.setResultLoading(true)
  searching.value = true
  searched.value = false
  results.value = []
  try {
    const [storeResult, shelfResult] = await Promise.allSettled([
      listLocalStore('', localRecursiveScan.value),
      bookshelf.loadBooks({ all: true }),
    ])
    if (!isActiveLocalSearch(operation, workspaceStamp)) return
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
    if (isActiveLocalSearch(operation, workspaceStamp)) {
      ElMessage.error(readError(err, '搜索本地书仓失败'))
    }
  } finally {
    if (isActiveLocalSearch(operation, workspaceStamp)) {
      searching.value = false
      workspace.setResultLoading(false)
    }
  }
}

function isActiveLocalSearch(operation, workspaceStamp) {
  return searchSessionOperations.canCommit(operation)
    && isWorkspaceRequestCurrent(workspace, workspaceStamp)
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
  searchType.value = ['all', 'group', 'single', 'custom'].includes(intent.searchType)
    ? intent.searchType
    : DEFAULT_SEARCH.searchType
  selectedGroup.value = intent.group || ''
  singleSourceId.value = Number(intent.sourceId || 0) || null
  concurrentCount.value = normalizeSearchConcurrent(intent.concurrent)
  searched.value = false
  searchStarting.value = searchMode.value === 'remote' && Boolean(keyword.value.trim())
}

function backToShelf() {
  remoteRequestGate.invalidate()
  searchSessionOperations.invalidate('local-search')
  workspace.backToShelf()
  emit('back-to-shelf')
}

async function importLocalOne(item) {
  if (!item?.importable) return
  await importLocalPaths([item.path])
}

async function importLocalPaths(paths) {
  const operation = searchSessionOperations.begin('local-import')
  importingLocal.value = true
  try {
    const categoryIds = targetCategoryIds.value.map(Number).filter(Boolean)
    const { data } = await importFromLocalStore(paths, categoryIds)
    if (!searchSessionOperations.canCommit(operation)) return
    const imported = data.imported || []
    imported.forEach(item => {
      if (item.book) bookshelf.upsertBook(item.book)
    })
    markImportedLocalItems(imported)
    const success = imported.filter(item => item.book).length
    const failed = imported.filter(item => item.error).length
    ElMessage.success(`导入 ${success} 本` + (failed ? `，${failed} 本失败` : ''))
  } catch (err) {
    if (searchSessionOperations.canCommit(operation)) {
      ElMessage.error(readError(err, '导入本地书失败'))
    }
  } finally {
    if (searchSessionOperations.canCommit(operation)) importingLocal.value = false
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

async function openRemoteReader(item) {
  const operation = searchSessionOperations.begin('remote-reader')
  try {
    const { data } = await createRemoteReaderSession(remoteBookReaderPayload(item, {
      sourceId: remoteBookSourceId(item),
      sourceName: remoteBookSourceName(item),
    }))
    if (!searchSessionOperations.canCommit(operation)) return
    if (!data?.id) throw new Error('远程阅读会话无效')
    router.push({ name: 'remote-reader', params: { sessionId: data.id }, query: { chapter: 0 } })
  } catch (error) {
    if (searchSessionOperations.canCommit(operation)) {
      ElMessage.error(readError(error, '打开临时阅读失败'))
    }
  }
}

function formatSize(bytes) {
  if (!bytes) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

function openPreview(item) {
  overlay.openBookInfo(item, {
    sourceName: remoteBookSourceName(item),
    statusLabel: '搜索结果',
    statusType: 'info',
  })
}

async function addResultToShelf(item) {
  await resultAddToShelf.addRemoteBookWithCategories(item, {
    key: remoteBookKey(item),
    sourceId: remoteBookSourceId(item),
    sourceName: remoteBookSourceName(item),
  })
}

function openResultEditor(item) {
  resultEditor.open(item, {
    sourceId: remoteBookSourceId(item),
    sourceName: remoteBookSourceName(item),
  })
}

function closeResultEditor() {
  resultEditor.close()
}

async function saveResultEditor() {
  await resultEditor.save()
}

function updateResultViewport() {
  resultWindowWidth.value = currentViewportWidth()
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || err?.message || fallback
}
</script>

<style scoped>
.search-page {
  display: flex;
  min-width: 0;
  gap: 0;
}

.result-title,
.result-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.result-title {
  justify-content: space-between;
}
.local-result-list {
  display: grid;
  min-width: 0;
  gap: 12px;
}

.local-result-card:hover,
.local-result-card.selected {
  border-color: var(--app-primary);
}

.local-result-card {
  display: flex;
  align-items: start;
  gap: 12px;
  padding: 14px;
  cursor: pointer;
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

.latest-chapter {
  color: var(--app-primary-strong) !important;
}

.result-actions {
  flex-wrap: wrap;
  justify-content: flex-end;
}

@media (max-width: 750px) {
  .search-page {
    gap: 0;
  }

  .result-actions {
    display: grid;
  }
  .local-result-list {
    gap: 8px;
  }

  .result-actions {
    justify-content: stretch;
  }

  .result-actions :deep(.el-button) {
    width: 100%;
    min-height: 36px;
    margin-left: 0;
  }

  .local-result-card {
    display: grid;
    grid-template-columns: 34px minmax(0, 1fr) auto;
    gap: 10px;
    padding: 10px;
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

}
</style>
