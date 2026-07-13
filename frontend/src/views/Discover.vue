<template>
  <section class="app-page discover-page workspace-result-page">
    <header class="workspace-result-head">
      <div>
        <h1 class="app-page-title">探索 ({{ books.length }})</h1>
        <p class="workspace-result-subtitle">{{ activeSource ? `${activeSource.name} · ${activeExploreName || '默认'}` : '选择书源入口开始探索' }}</p>
      </div>
      <div class="workspace-result-actions">
        <button
          v-if="books.length || hasMore"
          type="button"
          :disabled="loadingMore || !hasMore"
          @click="loadMoreBooks"
        >{{ loadingMore ? '加载中...' : (hasMore ? '加载更多' : '没有更多了') }}</button>
        <button type="button" @click="backToShelf">书架</button>
      </div>
    </header>

    <section v-if="sourceGroups.length" class="source-group-tabs">
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
        <div ref="discoverResults" v-loading="loadingBooks" class="discover-results">
          <RemoteBookResultGroups v-if="books.length" :groups="exploreResultGroups" @preview="openPreview" @read="openRemoteReader" />
          <el-empty v-if="!loadingBooks && !books.length" :description="sources.length ? '选择左侧书源入口开始探索' : '没有配置 exploreUrl 的书源'" />
        </div>
      </section>
    </div>

  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { createRemoteReaderSession } from '../api/remoteReader'
import { exploreBooks, listExploreSources } from '../api/explore'
import RemoteBookResultGroups from '../components/RemoteBookResultGroups.vue'
import { useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useIndexWorkspaceStore } from '../stores/indexWorkspace'
import {
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

const router = useRouter()
const emit = defineEmits(['back-to-shelf'])
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const workspace = useIndexWorkspaceStore()
const sources = ref([])
const books = ref([])
const selectedSourceId = ref('')
const selectedGroup = ref('')
const activeExploreUrl = ref('')
const activeExploreName = ref('')
const loadingSources = ref(false)
const loadingBooks = ref(false)
const page = ref(1)
const hasMore = ref(false)
const loadingMore = ref(false)
const expandedSources = ref('')
const workspaceExploreReady = ref(false)
const discoverResults = ref(null)
const exploreRequestGate = createAsyncRequestGate()

const activeSource = computed(() => sources.value.find(source => source.id === selectedSourceId.value))
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
  let ungrouped = 0
  for (const source of sources.value) {
    const name = String(source.group || '').trim()
    if (!name) {
      ungrouped += 1
      continue
    }
    groups.set(name, (groups.get(name) || 0) + 1)
  }
  const rows = [...groups.entries()]
    .map(([label, count]) => ({ label, value: label, count }))
    .sort((a, b) => a.label.localeCompare(b.label))
  if (ungrouped > 0) rows.push({ label: '未分组', value: '未分组', count: ungrouped })
  return rows
})
const filteredSources = computed(() => {
  if (!selectedGroup.value) return sources.value
  return sources.value.filter(source => (source.group || '未分组') === selectedGroup.value)
})

onMounted(async () => {
  applyWorkspaceExploreIntent()
  const [sourcesResult, shelfResult] = await Promise.allSettled([
    loadSources(),
    warmDiscoverShelf(),
  ])
  if (shelfResult.status === 'rejected') {
    ElMessage.warning(readError(shelfResult.reason, '加载书架失败，已入架状态和分组可能暂不可用'))
  }
  if (sourcesResult.status === 'rejected') {
    ElMessage.warning(readError(sourcesResult.reason, '加载探索书源失败'))
  }
  workspaceExploreReady.value = true
  if (selectedSourceId.value) await loadBooks()
})

onBeforeUnmount(() => {
  exploreRequestGate.invalidate()
})

watch(
  () => [workspace.mode, workspace.exploreRevision],
  () => {
    if (workspace.mode !== 'explore') return
    applyWorkspaceExploreIntent()
    if (workspaceExploreReady.value && selectedSourceId.value) loadBooks()
  },
)

function applyWorkspaceExploreIntent() {
  const intent = workspace.explore
  selectedSourceId.value = intent.sourceId || ''
  selectedGroup.value = intent.sourceGroup || ''
  activeExploreUrl.value = intent.url || ''
  activeExploreName.value = intent.name || ''
}

async function warmDiscoverShelf() {
  const jobs = [
    bookshelf.ensureCategoriesLoaded(),
    bookshelf.ensureBooksLoaded({ all: true }),
  ]
  const results = await Promise.allSettled(jobs)
  const failed = results.find(result => result.status === 'rejected')
  if (failed) throw failed.reason
}

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
  selectedGroup.value = selectedGroup.value === group ? '' : group
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
  const requestToken = exploreRequestGate.begin()
  const workspaceStamp = captureWorkspaceRequest(workspace, 'explore')
  const intent = exploreWorkspaceIntent({ page: 1, hasMore: false })
  workspace.showExploreResults([], intent)
  workspace.setResultLoading(true)
  loadingBooks.value = true
  try {
    const { data } = await exploreBooks(intent.sourceId, { page: 1, url: intent.url })
    if (!isActiveExploreRequest(requestToken, workspaceStamp)) return
    const result = normalizeExploreResult(data, 1)
    books.value = result.items
    page.value = result.page
    hasMore.value = result.hasMore
    workspace.showExploreResults(books.value, exploreWorkspaceIntent({ page: page.value, hasMore: hasMore.value }))
  } catch (err) {
    if (isActiveExploreRequest(requestToken, workspaceStamp)) {
      ElMessage.error(readError(err, '加载探索结果失败'))
    }
  } finally {
    if (isActiveExploreRequest(requestToken, workspaceStamp)) {
      loadingBooks.value = false
      workspace.setResultLoading(false)
    }
  }
}

async function loadMoreBooks() {
  if (!selectedSourceId.value || !activeExploreUrl.value || loadingMore.value) return
  if (!hasMore.value) {
    ElMessage.info('没有更多了')
    return
  }
  const requestToken = exploreRequestGate.begin()
  const workspaceStamp = captureWorkspaceRequest(workspace, 'explore')
  const nextPage = page.value + 1
  const intent = exploreWorkspaceIntent({ page: nextPage, hasMore: hasMore.value })
  workspace.rememberResultScroll(discoverResults.value?.scrollTop || 0)
  loadingMore.value = true
  workspace.setResultLoading(true)
  try {
    const { data } = await exploreBooks(intent.sourceId, { page: nextPage, url: intent.url })
    if (!isActiveExploreRequest(requestToken, workspaceStamp)) return
    const result = normalizeExploreResult(data, nextPage)
    const previousLength = books.value.length
    const { rows, added } = mergeRemoteSearchResults(books.value, result.items, intent.sourceId)
    books.value = rows
    page.value = result.page || nextPage
    hasMore.value = result.hasMore
    workspace.appendResultRows(rows.slice(previousLength), exploreWorkspaceIntent({ page: page.value, hasMore: hasMore.value }))
    if (!added) {
      ElMessage.info(hasMore.value ? '本批没有新增结果，仍可继续加载' : '没有更多了')
    }
  } catch (err) {
    if (isActiveExploreRequest(requestToken, workspaceStamp)) {
      ElMessage.error(readError(err, '加载更多失败'))
    }
  } finally {
    if (isActiveExploreRequest(requestToken, workspaceStamp)) {
      loadingMore.value = false
      workspace.setResultLoading(false)
    }
  }
}

function isActiveExploreRequest(requestToken, workspaceStamp) {
  return exploreRequestGate.isCurrent(requestToken)
    && isWorkspaceRequestCurrent(workspace, workspaceStamp)
}

function exploreWorkspaceIntent(values = {}) {
  return {
    sourceId: selectedSourceId.value,
    sourceGroup: activeSource.value?.group || selectedGroup.value,
    url: activeExploreUrl.value,
    name: activeExploreName.value,
    page: values.page ?? page.value,
    hasMore: values.hasMore ?? hasMore.value,
  }
}

function backToShelf() {
  exploreRequestGate.invalidate()
  workspace.backToShelf()
  emit('back-to-shelf')
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
  overlay.openBookInfo(book, {
    sourceName: activeRemoteSourceName(book),
    statusLabel: '探索结果',
    statusType: 'info',
  })
}

async function openRemoteReader(book) {
  try {
    const { data } = await createRemoteReaderSession(remoteBookReaderPayload(book, {
      sourceId: activeRemoteSourceId(book),
      sourceName: activeRemoteSourceName(book),
    }))
    if (!data?.id) throw new Error('远程阅读会话无效')
    router.push({ name: 'remote-reader', params: { sessionId: data.id }, query: { chapter: 0 } })
  } catch (error) {
    ElMessage.error(readError(error, '打开临时阅读失败'))
  }
}

function activeRemoteSourceId(book) {
  return remoteBookSourceId(book, activeSource.value?.id) || 'unknown'
}

function activeRemoteSourceName(book) {
  return remoteBookSourceName(book, activeSource.value?.name)
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

.workspace-result-page {
  grid-template-rows: auto minmax(0, 1fr);
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
  min-width: 0;
  margin: 5px 0 0;
  overflow: hidden;
  color: var(--app-text-muted);
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
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

.workspace-result-page .discover-main {
  min-height: 0;
  padding: 18px 0;
  overflow: auto;
  overscroll-behavior: contain;
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
  padding: 2px 0 4px;
}

.source-group-tabs button {
  flex: 0 0 auto;
  border: 1px solid var(--app-border);
  border-radius: 4px;
  background: var(--app-surface);
  color: var(--app-text);
  cursor: pointer;
  font: inherit;
  min-height: 30px;
  padding: 3px 10px;
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

  .workspace-result-page .discover-main {
    padding: 12px 20px calc(16px + env(safe-area-inset-bottom));
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
    padding-right: 8px;
    padding-left: 8px;
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
