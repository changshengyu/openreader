<template>
  <section class="rss-manager">
    <el-dialog
      :model-value="visible"
      width="500px"
      :fullscreen="isMobile"
      class="global-rss-dialog"
      destroy-on-close
      @update:model-value="handleRootVisibleChange"
    >
      <template #header>
        <div class="rss-dialog-title">
          <span class="rss-dialog-title-text">RSS订阅({{ sources.length }})</span>
          <span class="rss-title-actions">
            <span class="rss-title-action" @click="openEditor()">新增</span>
            <span class="rss-title-action" @click="triggerSourceImport">导入</span>
            <span class="rss-title-action" @click="rssEditMode = !rssEditMode">{{ rssEditMode ? '取消' : '编辑' }}</span>
          </span>
        </div>
      </template>

      <input
        ref="sourceImportInput"
        class="rss-source-import-input"
        type="file"
        accept=".json,application/json"
        @change="readRSSSourceFile"
      />
      <RSSSourceGrid
        :sources="sources"
        :edit-mode="rssEditMode"
        @open="selectSource"
        @edit="openEditor"
        @remove="removeSource"
      />
    </el-dialog>

    <RSSJsonEditorDialog
      v-model="editorVisible"
      v-model:content="editorContent"
      :is-mobile="isMobile"
      :saving="savingSource"
      @close="closeEditor"
      @save="saveSource"
    />

    <RSSImportDialog
      v-model="importDialogVisible"
      v-model:selected="selectedImportIndexes"
      :sources="importSources"
      :check-all="importCheckAll"
      :indeterminate="importIndeterminate"
      :is-mobile="isMobile"
      :saving="importingSources"
      @check-all="handleImportCheckAll"
      @cancel="closeImportDialog"
      @confirm="saveImportedSources"
    />

    <RSSArticleListDialog
      v-model="articleListDialogVisible"
      :title="sourceName(selectedSource)"
      :is-mobile="isMobile"
      :sort-options="selectedSortOptions"
      :sort-name="selectedSortName"
      :articles="articles"
      :loading="articlesLoading"
      :loading-more="articlesLoadingMore"
      :has-more="hasMoreArticles"
      @close="closeArticleList"
      @sort-change="handleSortChange"
      @open-article="openArticle"
      @load-more="loadMoreArticles"
    />

    <RSSArticleDialog
      v-model="articleDialogVisible"
      :title="selectedArticle?.title || ''"
      :content="articleBodyHTML(selectedArticle)"
      :is-mobile="isMobile"
      @close="closeArticle"
      @preview-images="openArticleImagePreview"
    />

    <el-image-viewer
      v-if="articleImagePreviewVisible"
      :url-list="articlePreviewImages"
      :initial-index="articlePreviewIndex"
      @close="articleImagePreviewVisible = false"
    />
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  createRSSSource,
  deleteRSSSource,
  getRSSArticleContent,
  importRSSSourcesBatch,
  listRSSSources,
  refreshRSSSource,
  updateRSSSource,
} from '../api/rss'
import { useAuthenticatedOperationGuard } from '../composables/useAuthenticatedOperationGuard'
import { currentUserScope } from '../utils/authScope'
import { cacheFirstRequest, networkFirstRequest, removeBrowserCache } from '../utils/browserCache'
import { createRSSArticleRequestGate } from '../utils/rssArticleRequestGate'
import {
  createDefaultRSSSource,
  normalizeRSSSourceImport,
  parseRSSSortOptions,
  safeRSSImportIndexes,
  toUpstreamRSSSource,
} from '../utils/rssSourceImport'
import RSSArticleDialog from './rss/RSSArticleDialog.vue'
import RSSArticleListDialog from './rss/RSSArticleListDialog.vue'
import RSSImportDialog from './rss/RSSImportDialog.vue'
import RSSJsonEditorDialog from './rss/RSSJsonEditorDialog.vue'
import RSSSourceGrid from './rss/RSSSourceGrid.vue'

const props = defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
  visible: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['close'])
const operations = useAuthenticatedOperationGuard()
const articleListRequestGate = createRSSArticleRequestGate()
const articleLoadMoreRequestGate = createRSSArticleRequestGate()

const sources = ref([])
const sourcesLoading = ref(false)
const rssEditMode = ref(false)
const sourceImportInput = ref(null)

const editorVisible = ref(false)
const editorContent = ref('')
const editingSourceId = ref(null)
const savingSource = ref(false)

const importDialogVisible = ref(false)
const importSources = ref([])
const selectedImportIndexes = ref([])
const importingSources = ref(false)

const selectedSourceId = ref('')
const selectedSortName = ref('')
const selectedSortURL = ref('')
const articleListDialogVisible = ref(false)
const articles = ref([])
const articlePage = ref(1)
const hasMoreArticles = ref(true)
const articlesLoading = ref(false)
const articlesLoadingMore = ref(false)

const articleDialogVisible = ref(false)
const selectedArticle = ref(null)
const articleImagePreviewVisible = ref(false)
const articlePreviewImages = ref([])
const articlePreviewIndex = ref(0)
let articleOpenRequest = 0

const selectedSource = computed(() => sources.value.find(source => source.id === selectedSourceId.value) || null)
const selectedSortOptions = computed(() => parseRSSSortOptions(toUpstreamRSSSource(selectedSource.value || {})))
const importCheckAll = computed(() => (
  importSources.value.length > 0 && selectedImportIndexes.value.length === importSources.value.length
))
const importIndeterminate = computed(() => (
  selectedImportIndexes.value.length > 0 && selectedImportIndexes.value.length < importSources.value.length
))

onMounted(() => {
  window.addEventListener('openreader:rss-updated', handleRSSUpdated)
  if (props.visible) openRSSWorkspace()
})

onBeforeUnmount(() => {
  window.removeEventListener('openreader:rss-updated', handleRSSUpdated)
})

watch(() => props.visible, (visible) => {
  if (visible) {
    openRSSWorkspace()
    return
  }
  resetRSSWorkspace()
})

watch(articleListDialogVisible, (visible) => {
  if (!visible) resetSourceArticleState({ resetSort: true })
})

async function openRSSWorkspace() {
  const operation = operations.begin('open-rss-workspace')
  await loadSources(operation)
}

function handleRootVisibleChange(visible) {
  if (!visible) emit('close')
}

function rssSourcesCacheKey() {
  return `rssSources@${currentUserScope()}`
}

async function invalidateRSSSourcesCache() {
  await removeBrowserCache(rssSourcesCacheKey())
}

async function loadSources(parentOperation = null) {
  if (parentOperation && !operations.canCommit(parentOperation)) return false
  const operation = operations.begin('load-rss-sources')
  sourcesLoading.value = true
  try {
    const response = await cacheFirstRequest(
      () => listRSSSources(),
      rssSourcesCacheKey(),
      { validate: data => Array.isArray(data) },
    )
    if (!operations.canCommit(operation) || !props.visible) return false
    applyRSSSources(response.data)
    if (response.fromCache) refreshRSSSourcesCache(operation).catch(() => {})
    return true
  } catch (error) {
    if (operations.canCommit(operation)) ElMessage.error(readError(error, '加载 RSS 源失败'))
    return false
  } finally {
    if (operations.canCommit(operation)) sourcesLoading.value = false
  }
}

async function refreshRSSSourcesCache(parentOperation = null) {
  if (parentOperation && !operations.canCommit(parentOperation)) return false
  const operation = operations.begin('refresh-rss-source-cache')
  const response = await networkFirstRequest(
    () => listRSSSources(),
    rssSourcesCacheKey(),
    { validate: data => Array.isArray(data) },
  )
  if (!operations.canCommit(operation) || !props.visible) return false
  applyRSSSources(response.data)
  return true
}

function applyRSSSources(data) {
  sources.value = [...(Array.isArray(data) ? data : [])]
    .sort((left, right) => Number(left.customOrder || 0) - Number(right.customOrder || 0))
  if (!sources.value.length) rssEditMode.value = false
  if (selectedSourceId.value && !sources.value.some(source => source.id === selectedSourceId.value)) {
    selectedSourceId.value = ''
  }
}

function sourceName(source) {
  return source?.sourceName || source?.title || ''
}

function sourceURL(source) {
  return source?.sourceUrl || source?.url || ''
}

function triggerSourceImport() {
  sourceImportInput.value?.click()
}

async function readRSSSourceFile(event) {
  const file = event?.target?.files?.[0]
  if (event?.target) event.target.value = ''
  if (!file) return
  const operation = operations.begin('read-rss-import')
  try {
    const parsed = JSON.parse(await file.text())
    if (!operations.canCommit(operation)) return
    if (!Array.isArray(parsed) || !parsed.length) throw new Error('invalid RSS source file')
    importSources.value = normalizeRSSSourceImport(parsed)
    selectedImportIndexes.value = []
    importDialogVisible.value = true
  } catch {
    if (operations.canCommit(operation)) ElMessage.error('RSS源文件错误')
  }
}

function handleImportCheckAll(checked) {
  if (!checked) {
    selectedImportIndexes.value = []
    return
  }
  const safeIndexes = safeRSSImportIndexes(importSources.value)
  selectedImportIndexes.value = safeIndexes
  if (safeIndexes.length < importSources.value.length) {
    ElMessage.info('部分使用了Javascript和Webview的书源未勾选')
  }
}

function closeImportDialog() {
  importDialogVisible.value = false
  selectedImportIndexes.value = []
  importSources.value = []
}

async function saveImportedSources() {
  if (!selectedImportIndexes.value.length) {
    ElMessage.error('请选择需要导入的源')
    return
  }
  const selectedSources = selectedImportIndexes.value.map(index => importSources.value[index])
  const operation = operations.begin('save-rss-import')
  importingSources.value = true
  try {
    await importRSSSourcesBatch(selectedSources)
    if (!operations.canCommit(operation)) return
    ElMessage.success('导入RSS源成功')
    closeImportDialog()
    await invalidateRSSSourcesCache()
    if (operations.canCommit(operation)) await loadSources(operation)
  } catch (error) {
    if (operations.canCommit(operation)) ElMessage.error(readError(error, '导入RSS源失败'))
  } finally {
    if (operations.canCommit(operation)) importingSources.value = false
  }
}

function openEditor(source = null) {
  const upstreamSource = source ? toUpstreamRSSSource(source) : createDefaultRSSSource()
  editingSourceId.value = source?.id || null
  editorContent.value = JSON.stringify(upstreamSource, null, 4)
  editorVisible.value = true
}

function closeEditor() {
  editorVisible.value = false
  editingSourceId.value = null
  editorContent.value = ''
}

async function saveSource() {
  let source
  try {
    source = JSON.parse(editorContent.value)
  } catch {
    ElMessage.error('RSS源必须是JSON格式')
    return
  }
  if (!source.sourceName) {
    ElMessage.error('RSS源名称不能为空')
    return
  }
  if (!source.sourceUrl) {
    ElMessage.error('RSS源链接不能为空')
    return
  }
  const sourceId = editingSourceId.value
  const operation = operations.begin('save-rss-source')
  savingSource.value = true
  try {
    if (sourceId) await updateRSSSource(sourceId, source)
    else await createRSSSource(source)
    if (!operations.canCommit(operation)) return
    closeEditor()
    ElMessage.success('保存RSS源成功')
    await invalidateRSSSourcesCache()
    if (operations.canCommit(operation)) await loadSources(operation)
  } catch (error) {
    if (operations.canCommit(operation)) ElMessage.error(readError(error, '保存RSS源失败'))
  } finally {
    if (operations.canCommit(operation)) savingSource.value = false
  }
}

async function removeSource(source) {
  const operation = operations.begin(`delete-rss-source:${source.id}`)
  try {
    await ElMessageBox.confirm('确认要删除该RSS订阅源吗?', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })
    if (!operations.canCommit(operation)) return
    await deleteRSSSource(source.id)
    if (!operations.canCommit(operation)) return
    await invalidateRSSSourcesCache()
    if (!operations.canCommit(operation)) return
    await loadSources(operation)
    if (operations.canCommit(operation)) ElMessage.success('删除成功')
  } catch (error) {
    if (!operations.canCommit(operation) || error === 'cancel' || error === 'close') return
    ElMessage.error(readError(error, '删除失败'))
  }
}

async function selectSource(source) {
  const operation = operations.begin('select-rss-source')
  resetSourceArticleState({ resetSort: true })
  selectedSourceId.value = source.id
  setInitialSort(source)
  articleListDialogVisible.value = true
  await loadArticlePage(1, false, operation)
}

function setInitialSort(source) {
  const options = parseRSSSortOptions(toUpstreamRSSSource(source || {}))
  selectedSortName.value = options[0]?.name || ''
  selectedSortURL.value = options[0]?.url || sourceURL(source)
}

async function handleSortChange(sortName) {
  const option = selectedSortOptions.value.find(item => item.name === sortName)
  if (!option) return
  const operation = operations.begin('select-rss-sort')
  resetSourceArticleState()
  selectedSortName.value = option.name
  selectedSortURL.value = option.url
  await loadArticlePage(1, false, operation)
}

function articleRequestQuery(page) {
  return {
    rootVisible: props.visible,
    listVisible: articleListDialogVisible.value,
    sourceId: selectedSourceId.value,
    sortName: selectedSortName.value,
    sortURL: selectedSortURL.value,
    page,
  }
}

function isArticleRequestQueryCurrent(query) {
  const current = articleRequestQuery(query.page)
  return Object.keys(query).every(key => current[key] === query[key])
}

async function loadArticlePage(page, append, parentOperation = null) {
  if (parentOperation && !operations.canCommit(parentOperation)) return false
  if (!props.visible || !articleListDialogVisible.value || !selectedSource.value) return false
  const operation = operations.begin(append ? 'load-more-rss-articles' : 'load-rss-articles')
  const query = articleRequestQuery(page)
  const gate = append ? articleLoadMoreRequestGate : articleListRequestGate
  const request = gate.begin(query)
  if (append) articlesLoadingMore.value = true
  else articlesLoading.value = true
  try {
    const { data } = await refreshRSSSource(selectedSourceId.value, {
      page,
      sortName: selectedSortName.value,
      sortUrl: selectedSortURL.value,
    })
    if (!operations.canCommit(operation) || !gate.isCurrent(request, articleRequestQuery(page)) || !isArticleRequestQueryCurrent(query)) return false
    const nextItems = Array.isArray(data?.items) ? data.items : []
    if (!nextItems.length) {
      ElMessage.error('没有数据')
      hasMoreArticles.value = false
      if (!append) articles.value = []
      return true
    }
    articles.value = append ? articles.value.concat(nextItems) : nextItems
    articlePage.value = Number(data.page || page)
    hasMoreArticles.value = data.hasMore !== false
    return true
  } catch (error) {
    if (operations.canCommit(operation) && gate.isCurrent(request, articleRequestQuery(page))) {
      ElMessage.error(readError(error, '加载RSS文章列表失败'))
    }
    return false
  } finally {
    if (operations.canCommit(operation) && gate.isCurrent(request, articleRequestQuery(page))) {
      if (append) articlesLoadingMore.value = false
      else articlesLoading.value = false
    }
  }
}

async function loadMoreArticles() {
  if (!hasMoreArticles.value || articlesLoading.value || articlesLoadingMore.value) return
  await loadArticlePage(articlePage.value + 1, true)
}

async function openArticle(article) {
  const operation = operations.begin('open-rss-article')
  const request = ++articleOpenRequest
  try {
    const { data } = await getRSSArticleContent(article.id)
    if (request !== articleOpenRequest || !operations.canCommit(operation)) return
    selectedArticle.value = { ...article, ...data }
    articleDialogVisible.value = true
  } catch (error) {
    if (operations.canCommit(operation)) ElMessage.error(readError(error, '加载RSS文章内容失败'))
  }
}

function closeArticleList() {
  articleListDialogVisible.value = false
}

function closeArticle() {
  articleOpenRequest += 1
  operations.invalidate('open-rss-article')
  articleDialogVisible.value = false
  selectedArticle.value = null
  articleImagePreviewVisible.value = false
}

function openArticleImagePreview({ images, index }) {
  articlePreviewImages.value = images
  articlePreviewIndex.value = index
  articleImagePreviewVisible.value = true
}

function articleBodyHTML(article) {
  return article?.content || article?.description || article?.summary || ''
}

function invalidateArticleRequests() {
  articleListRequestGate.invalidate()
  articleLoadMoreRequestGate.invalidate()
}

function resetSourceArticleState({ resetSort = false } = {}) {
  operations.invalidate('load-rss-articles')
  operations.invalidate('load-more-rss-articles')
  operations.invalidate('open-rss-article')
  invalidateArticleRequests()
  articleOpenRequest += 1
  articles.value = []
  articlePage.value = 1
  hasMoreArticles.value = true
  articlesLoading.value = false
  articlesLoadingMore.value = false
  if (resetSort) {
    selectedSortName.value = ''
    selectedSortURL.value = ''
  }
  articleDialogVisible.value = false
  selectedArticle.value = null
  articleImagePreviewVisible.value = false
  articlePreviewImages.value = []
  articlePreviewIndex.value = 0
}

function resetRSSWorkspace() {
  operations.reset()
  articleListDialogVisible.value = false
  resetSourceArticleState({ resetSort: true })
  sources.value = []
  selectedSourceId.value = ''
  rssEditMode.value = false
  closeEditor()
  closeImportDialog()
}

function handleRSSUpdated(event) {
  if (!props.visible || !event?.detail?.sources) return
  invalidateRSSSourcesCache()
    .then(() => loadSources())
    .catch(() => {})
}

function readError(error, fallback) {
  return error?.response?.data?.error?.message || error?.response?.data?.error || fallback
}
</script>

<style scoped>
.rss-manager {
  display: contents;
}

.rss-dialog-title {
  display: flex;
  align-items: center;
  width: 100%;
  min-width: 0;
}

.rss-dialog-title-text {
  color: var(--app-text);
  font-size: var(--el-dialog-title-font-size);
  line-height: var(--el-dialog-font-line-height);
}

.rss-title-actions {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  margin-left: auto;
  margin-right: 28px;
}

.rss-title-action {
  color: var(--app-primary-strong);
  font-size: 15px;
  cursor: pointer;
}

.rss-source-import-input {
  display: none;
}

@media (max-width: 750px) {
  .rss-title-actions {
    gap: 8px;
    margin-right: 24px;
  }
}
</style>
