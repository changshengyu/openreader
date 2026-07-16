<template>
  <section class="app-page discover-page workspace-result-page">
    <header class="workspace-result-head">
      <div>
        <h1 class="app-page-title">探索 ({{ books.length }})</h1>
        <p class="workspace-result-subtitle">{{ exploreSubtitle }}</p>
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

    <div ref="discoverResults" v-loading="loadingMore" class="discover-results">
      <RemoteBookResultGroups
        v-if="books.length"
        :groups="exploreResultGroups"
        @preview="openPreview"
        @read="openRemoteReader"
      />
      <el-empty v-else description="从书海选择书源入口开始探索" />
    </div>
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { exploreBooks } from '../api/explore'
import { createRemoteReaderSession } from '../api/remoteReader'
import RemoteBookResultGroups from '../components/RemoteBookResultGroups.vue'
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
const overlay = useOverlayStore()
const workspace = useIndexWorkspaceStore()
const discoverResults = ref(null)
const loadingMore = ref(false)
const exploreRequestGate = createAsyncRequestGate()

const books = computed(() => workspace.resultRows)
const hasMore = computed(() => workspace.continuation.hasMore)
const exploreSubtitle = computed(() => {
  const source = workspace.explore.sourceName || '书源'
  const entry = workspace.explore.name || '默认'
  return `${source} · ${entry}`
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

onBeforeUnmount(() => {
  exploreRequestGate.invalidate()
})

async function loadMoreBooks() {
  const sourceId = workspace.explore.sourceId
  const url = workspace.explore.url
  if (!workspace.isExploreResult || !sourceId || !url || loadingMore.value) return
  if (!hasMore.value) {
    ElMessage.info('没有更多了')
    return
  }
  const requestToken = exploreRequestGate.begin()
  const workspaceStamp = captureWorkspaceRequest(workspace, 'explore')
  const nextPage = Number(workspace.continuation.page || 1) + 1
  const intent = {
    ...workspace.explore,
    page: nextPage,
    hasMore: hasMore.value,
  }
  workspace.rememberResultScroll(discoverResults.value?.scrollTop || 0)
  loadingMore.value = true
  workspace.setResultLoading(true)
  try {
    const { data } = await exploreBooks(sourceId, { page: nextPage, url })
    if (!isActiveExploreRequest(requestToken, workspaceStamp)) return
    const result = normalizeExploreResult(data, nextPage)
    const previousLength = books.value.length
    const { rows, added } = mergeRemoteSearchResults(books.value, result.items, sourceId)
    workspace.appendResultRows(rows.slice(previousLength), {
      ...intent,
      page: result.page || nextPage,
      hasMore: result.hasMore,
    })
    if (!added) ElMessage.info(result.hasMore ? '本批没有新增结果，仍可继续加载' : '没有更多了')
  } catch (error) {
    if (isActiveExploreRequest(requestToken, workspaceStamp)) {
      ElMessage.error(readError(error, '加载更多失败'))
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

function backToShelf() {
  exploreRequestGate.invalidate()
  workspace.backToShelf()
  emit('back-to-shelf')
}

function normalizeExploreResult(data, fallbackPage) {
  if (Array.isArray(data)) return { items: data, page: fallbackPage, hasMore: false }
  return {
    items: Array.isArray(data?.items) ? data.items : [],
    page: Number(data?.page || fallbackPage),
    hasMore: Boolean(data?.hasMore),
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
  return remoteBookSourceId(book, workspace.explore.sourceId) || 'unknown'
}

function activeRemoteSourceName(book) {
  return remoteBookSourceName(book, workspace.explore.sourceName)
}

function readError(error, fallback) {
  return error?.response?.data?.error?.message || error?.response?.data?.error || error?.message || fallback
}
</script>

<style scoped>
.discover-page {
  display: grid;
  min-width: 0;
}

.workspace-result-page {
  grid-template-rows: auto minmax(0, 1fr);
  box-sizing: border-box;
  height: 100vh;
  max-height: 100vh;
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

.discover-results {
  display: grid;
  min-width: 0;
  grid-template-columns: repeat(auto-fill, minmax(min(320px, 100%), 1fr));
  gap: 14px;
  padding: 18px 0;
  overflow: auto;
  overscroll-behavior: contain;
  scrollbar-width: none;
}

.discover-results::-webkit-scrollbar {
  display: none;
}

@media (max-width: 750px) {
  .workspace-result-page {
    height: 100vh;
    height: 100dvh;
    max-height: none;
    padding: 0;
  }

  .workspace-result-head {
    min-height: 64px;
    padding: max(16px, env(safe-area-inset-top)) 24px 12px;
  }

  .workspace-result-head .app-page-title {
    font-size: 20px;
  }

  .discover-results {
    grid-template-columns: minmax(0, 1fr);
    gap: 8px;
    padding: 12px 20px calc(16px + env(safe-area-inset-bottom));
  }
}
</style>
