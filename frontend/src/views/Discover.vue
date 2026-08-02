<template>
  <section class="app-page shelf-page result-shelf-page discover-page">
    <div class="shelf-title">
      <div class="shelf-title-main">
        <strong>探索 ({{ books.length }})</strong>
      </div>
      <div class="title-actions">
        <button type="button" @click.stop="openExploreChooser">书海</button>
        <button type="button" :disabled="loadingMore || !hasMore" @click="loadMoreBooks">
          {{ exploreActionLabel }}
        </button>
        <button type="button" @click="backToShelf">书架</button>
      </div>
    </div>

    <main class="shelf-main">
      <div ref="discoverResults" class="books-wrapper">
        <RemoteBookResultList
          :books="books"
          :adding-book-key="addingRemoteBookKey"
          :fallback-source-id="workspace.explore.sourceId"
          :is-night="reader.themeType === 'night'"
          @preview="openPreview"
          @read="openRemoteReader"
          @add="addResultToShelf"
          @edit="openResultEditor"
        />
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
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { exploreBooks } from '../api/explore'
import { createRemoteBook } from '../api/books'
import { createRemoteReaderSession } from '../api/remoteReader'
import RemoteBookResultList from '../components/RemoteBookResultList.vue'
import RemoteBookJsonEditorDialog from '../components/RemoteBookJsonEditorDialog.vue'
import { useRemoteBookAddToShelf } from '../composables/useRemoteBookAddToShelf'
import { useRemoteBookResultEditor } from '../composables/useRemoteBookResultEditor'
import { useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore } from '../stores/reader'
import { useIndexWorkspaceStore } from '../stores/indexWorkspace'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation'
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
const workspace = useIndexWorkspaceStore()
const discoverResults = ref(null)
const resultWindowWidth = ref(currentViewportWidth())
const loadingMore = ref(false)
const exploreRequestGate = createAsyncRequestGate()
const discoverSessionOperations = createAuthenticatedOperationGuard()
const resultAddToShelf = useRemoteBookAddToShelf({
  operationGuard: discoverSessionOperations,
  selectCategories: initialCategoryIds => overlay.selectBookAddCategories(initialCategoryIds),
  buildPayload: (book, categoryIds, context) => remoteBookCreatePayload(book, categoryIds, context),
  createRemoteBook,
  upsertBook: book => bookshelf.upsertBook(book),
  onSuccess: message => ElMessage.success(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})
const addingRemoteBookKey = resultAddToShelf.addingBookKey
const resultEditor = useRemoteBookResultEditor({
  operationGuard: discoverSessionOperations,
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

const books = computed(() => workspace.resultRows)
const hasMore = computed(() => workspace.continuation.hasMore)
const isMobileResult = computed(() => shouldUseMiniInterface(reader.pageMode, resultWindowWidth.value))
const exploreActionLabel = computed(() => {
  if (loadingMore.value) return '加载中...'
  return hasMore.value ? '加载更多' : '没有更多了'
})

onMounted(() => {
  updateResultViewport()
  window.addEventListener('resize', updateResultViewport)
  window.addEventListener('orientationchange', updateResultViewport)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', updateResultViewport)
  window.removeEventListener('orientationchange', updateResultViewport)
  exploreRequestGate.invalidate()
  resultEditor.reset()
  discoverSessionOperations.reset()
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

function openExploreChooser() {
  workspace.requestExplore()
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

async function addResultToShelf(book) {
  await resultAddToShelf.addRemoteBookWithCategories(book, {
    key: remoteBookKey(book, workspace.explore.sourceId),
    sourceId: activeRemoteSourceId(book),
    sourceName: activeRemoteSourceName(book),
  })
}

function openResultEditor(book) {
  resultEditor.open(book, {
    sourceId: activeRemoteSourceId(book),
    sourceName: activeRemoteSourceName(book),
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

async function openRemoteReader(book) {
  const operation = discoverSessionOperations.begin('remote-reader')
  try {
    const { data } = await createRemoteReaderSession(remoteBookReaderPayload(book, {
      sourceId: activeRemoteSourceId(book),
      sourceName: activeRemoteSourceName(book),
    }))
    if (!discoverSessionOperations.canCommit(operation)) return
    if (!data?.id) throw new Error('远程阅读会话无效')
    router.push({ name: 'remote-reader', params: { sessionId: data.id }, query: { chapter: 0 } })
  } catch (error) {
    if (discoverSessionOperations.canCommit(operation)) {
      ElMessage.error(readError(error, '打开临时阅读失败'))
    }
  }
}

function activeRemoteSourceId(book) {
  return remoteBookSourceId(book, workspace.explore.sourceId)
}

function activeRemoteSourceName(book) {
  return remoteBookSourceName(book, workspace.explore.sourceName)
}

function readError(error, fallback) {
  return error?.response?.data?.error?.message || error?.response?.data?.error || error?.message || fallback
}
</script>
