<template>
  <el-dialog
    v-if="isNormalPage"
    :model-value="visible"
    :title="isFailureMode ? '失效书源管理' : '书源管理'"
    width="min(1000px, max(750px, 70vw))"
    top="max(15dvh, calc((100dvh - 584px) / 2))"
    :fullscreen="isMobile"
    class="global-source-manage-dialog"
    @update:model-value="handleDialogModel"
    @open="handleOpen"
    @closed="handleClosed"
  >
    <template #header>
      <div class="source-dialog-title">
        <span>{{ isFailureMode ? '失效书源管理' : '书源管理' }}</span>
        <div v-if="!isFailureMode" class="source-title-actions">
          <el-button link type="primary" @click="openEditor()">新增</el-button>
          <el-button link type="primary" @click="exportAllSources">导出</el-button>
          <el-button link type="primary" :loading="restoring" @click="restoreDefaults">恢复默认</el-button>
          <el-button link type="danger" :loading="clearing" @click="clearAllSources">清空</el-button>
        </div>
      </div>
    </template>

    <section class="source-manager-body">
      <div v-if="isFailureMode" class="source-check-form">
        <span class="source-check-label">搜索词：</span>
        <el-input v-model="checkConfig.keyword" size="small" />
        <span class="source-check-label source-timeout-label">超时(ms)：</span>
        <el-input-number
          v-model="checkConfig.timeout"
          :min="1000"
          :max="15000"
          :step="500"
          size="small"
        />
        <span class="source-check-label">并发数：</span>
        <el-input-number
          v-model="checkConfig.concurrent"
          :min="3"
          :max="15"
          :step="1"
          size="small"
        />
      </div>

      <div class="source-group-wrapper">
        <el-tag
          v-for="group in sourceShowGroups"
          :key="group"
          type="info"
          class="source-group-btn"
          :effect="selectedGroup === group ? 'dark' : 'light'"
          :class="{ selected: selectedGroup === group }"
          @click="setSourceGroup(group)"
        >
          {{ group }}
        </el-tag>
      </div>

      <el-table
        ref="tableRef"
        :key="isFailureMode"
        :data="pagedSources"
        :height="tableHeight"
        class="source-table"
        @selection-change="selection = $event"
      >
        <el-table-column
          type="selection"
          width="25"
          :fixed="isMobile"
          :selectable="isSourceSelectable"
        />
        <el-table-column
          prop="name"
          label="书源名称"
          min-width="120"
          :fixed="isMobile"
        />
        <el-table-column prop="baseUrl" label="书源链接" min-width="120">
          <template #default="{ row }">
            <el-link type="primary" :href="row.baseUrl" target="_blank">
              {{ row.baseUrl }}
            </el-link>
          </template>
        </el-table-column>
        <el-table-column
          v-if="isFailureMode"
          prop="errorMessage"
          label="错误信息"
          min-width="120"
        />
        <el-table-column label="书架书籍" min-width="120">
          <template #default="{ row }">
            <pre class="source-used-books">{{ (row.usedBookNames || []).join('\n') }}</pre>
          </template>
        </el-table-column>
        <el-table-column v-if="!isFailureMode" label="操作" width="100px">
          <template #default="{ row }">
            <el-button link type="primary" @click="openEditor(row)">编辑</el-button>
          </template>
        </el-table-column>
      </el-table>

      <div class="source-pagination">
        <el-pagination
          v-model:current-page="sourcePage"
          v-model:page-size="sourcePageSize"
          :page-sizes="sourcePageSizes"
          layout="total, sizes, prev, pager, next"
          :pager-count="isMobile ? 5 : 7"
          :total="filteredSources.length"
        />
      </div>
    </section>

    <template #footer>
      <div class="source-dialog-footer">
        <el-button type="primary" class="float-left" :loading="deleting" @click="deleteSelectedSources">
          批量删除
        </el-button>
        <span class="check-tip">已选择 {{ selection.length }} 个</span>
        <el-button
          v-if="isFailureMode"
          :disabled="checking"
          @click="checkInvalidSources"
        >
          {{ checking ? '正在' : '' }}检测书源 {{ checkProgress }}
        </el-button>
        <el-button @click="requestClose">取消</el-button>
      </div>
    </template>
  </el-dialog>

  <el-dialog
    v-model="showEditor"
    title="编辑书源"
    width="min(1000px, max(750px, 70vw))"
    top="10vh"
    :fullscreen="isMobile"
    append-to-body
    class="source-json-editor-dialog"
  >
    <el-alert
      v-if="editorCompatibilityMessage"
      class="source-json-compatibility-warning"
      type="warning"
      :closable="false"
      show-icon
      title="此书源包含当前服务不会执行或仅保留的配置"
      :description="editorCompatibilityMessage"
    />
    <el-input
      v-model="sourceJSON"
      class="source-json-editor"
      type="textarea"
      :rows="isMobile ? 24 : 22"
      spellcheck="false"
    />
    <template #footer>
      <el-button @click="showEditor = false">取 消</el-button>
      <el-button type="primary" :loading="saving" @click="saveSourceJSON">保 存</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  batchSources,
  batchTestSources,
  clearSources,
  createSource,
  exportSources,
  getSource,
  listInvalidSources,
  listSources,
  restoreDefaultSources,
  updateSource,
} from '../../api/sources'
import { useAuthenticatedOperationGuard } from '../../composables/useAuthenticatedOperationGuard'
import { useReaderStore } from '../../stores/reader'
import {
  analyzeSourceCompatibility,
  sourceCompatibilityMessage,
} from '../../utils/bookSourceCompatibility'
import {
  buildBookSourcePayload,
  buildReaderDevBookSource,
  sourceToEditorSnapshot,
} from '../../utils/bookSourceEditor'

const props = defineProps({
  visible: { type: Boolean, default: false },
  failureMode: { type: Boolean, default: false },
  isMobile: { type: Boolean, default: false },
})
const emit = defineEmits(['close'])

const reader = useReaderStore()
const operations = useAuthenticatedOperationGuard()
const tableRef = ref(null)
const sources = ref([])
const failureSources = ref([])
const selection = ref([])
const selectedGroup = ref('')
const sourcePage = ref(1)
const sourcePageSize = ref(25)
const sourcePageSizes = [25, 50, 100, 200, 300, 400]
const checkConfig = reactive({
  keyword: '斗罗大陆',
  timeout: 5000,
  concurrent: 5,
})
const checking = ref(false)
const checkProgress = ref('')
const deleting = ref(false)
const clearing = ref(false)
const restoring = ref(false)
const showEditor = ref(false)
const editingSourceId = ref(null)
const sourceJSON = ref('')
const saving = ref(false)
let sourceReloadTimer

const isNormalPage = computed(() => reader.pageType === 'normal')
const isFailureMode = computed(() => props.failureMode)
const sourceList = computed(() => (
  isFailureMode.value ? failureSources.value : sources.value
))
const sourceShowGroups = computed(() => {
  if (isFailureMode.value) return [...failureGroupOrder]
  const groups = []
  const seen = new Set()
  for (const source of sourceList.value) {
    const group = String(source?.group || '').trim()
    if (!group || seen.has(group)) continue
    seen.add(group)
    groups.push(group)
  }
  groups.push('未分组')
  return groups
})
const filteredSources = computed(() => {
  if (!selectedGroup.value) return sourceList.value
  if (isFailureMode.value) {
    return sourceList.value.filter(source => (
      String(source.errorMessage || '').includes(selectedGroup.value)
    ))
  }
  if (selectedGroup.value === '未分组') {
    return sourceList.value.filter(source => !String(source.group || '').trim())
  }
  return sourceList.value.filter(source => source.group === selectedGroup.value)
})
const pagedSources = computed(() => {
  const start = (sourcePage.value - 1) * sourcePageSize.value
  if (start > filteredSources.value.length) return []
  return filteredSources.value.slice(start, start + sourcePageSize.value)
})
const tableHeight = computed(() => {
  if (props.isMobile) {
    return `calc(100dvh - ${isFailureMode.value ? 300 : 268}px)`
  }
  return `calc(min(70dvh - 184px, 400px) - ${isFailureMode.value ? 116 : 84}px)`
})
const editorCompatibilityMessage = computed(() => {
  try {
    return sourceCompatibilityMessage(analyzeSourceCompatibility(JSON.parse(sourceJSON.value)))
  } catch {
    return ''
  }
})

const failureGroupOrder = [
  'UnknownHostException',
  'ConnectException: Failed to connect',
  'SocketException: Connection reset',
  'SSLHandshakeException',
  'responseCode: 307',
  'responseCode: 400',
  'responseCode: 403',
  'responseCode: 404',
  'responseCode: 500',
  'responseCode: 502',
  'responseCode: 503',
  'responseCode: 504',
  'responseCode: 513',
  'timeout',
]

watch(isNormalPage, normal => {
  if (!normal && props.visible) requestClose()
})

watch(
  () => [props.visible, props.failureMode],
  async ([visible, failure], [wasVisible, wasFailure] = []) => {
    if (!visible) {
      showEditor.value = false
      return
    }
    if (!wasVisible || !failure || wasFailure) return
    await loadInvalidSourceHealth()
  },
)

watch(sourceShowGroups, groups => {
  if (selectedGroup.value && !groups.includes(selectedGroup.value)) {
    selectedGroup.value = ''
  }
})

if (typeof window !== 'undefined') {
  window.addEventListener('openreader:sources-update', handleSourcesUpdate)
}

onBeforeUnmount(() => {
  if (typeof window !== 'undefined') {
    window.removeEventListener('openreader:sources-update', handleSourcesUpdate)
    if (sourceReloadTimer) window.clearTimeout(sourceReloadTimer)
  }
})

async function handleOpen() {
  const operation = operations.begin('open-source-manager')
  try {
    await loadSources(operation)
    if (isFailureMode.value) await loadInvalidSourceHealth(operation)
  } catch (error) {
    if (operations.canCommit(operation)) {
      ElMessage.error(readError(error, '加载书源失败'))
    }
  }
}

function handleDialogModel(value) {
  if (!value) requestClose()
}

function handleClosed() {
  selectedGroup.value = ''
  checkProgress.value = ''
  operations.reset()
}

function requestClose() {
  emit('close')
}

async function loadSources(parentOperation = null) {
  if (parentOperation && !operations.canCommit(parentOperation)) return false
  const operation = operations.begin('load-source-list')
  const { data } = await listSources()
  if (!operations.canCommit(operation)) return false
  sources.value = Array.isArray(data) ? data : []
  return true
}

async function loadInvalidSourceHealth(parentOperation = null) {
  if (parentOperation && !operations.canCommit(parentOperation)) return false
  const operation = operations.begin('load-invalid-source-health')
  const { data } = await listInvalidSources()
  if (!operations.canCommit(operation)) return false
  failureSources.value = (Array.isArray(data) ? data : []).map(source => ({
    ...source,
    errorMessage: visibleFailureCategory(source.errorMessage),
    usedBookNames: Array.isArray(source.usedBookNames)
      ? source.usedBookNames
      : sourceNamesFromActiveList(source.id),
  }))
  return true
}

function sourceNamesFromActiveList(sourceId) {
  return sources.value.find(source => Number(source.id) === Number(sourceId))?.usedBookNames || []
}

function handleSourcesUpdate() {
  if (!props.visible || typeof window === 'undefined') return
  if (sourceReloadTimer) window.clearTimeout(sourceReloadTimer)
  sourceReloadTimer = window.setTimeout(async () => {
    sourceReloadTimer = undefined
    const operation = operations.begin('sync-source-manager')
    try {
      await loadSources(operation)
      if (isFailureMode.value) await loadInvalidSourceHealth(operation)
    } catch {
      // Keep the current list visible; the next durable event/open retries.
    }
  }, 250)
}

function setSourceGroup(group) {
  selectedGroup.value = selectedGroup.value === group ? '' : group
}

function isSourceSelectable(source) {
  return Number(source?.usedBookCount || 0) === 0
}

async function deleteSelectedSources() {
  if (!selection.value.length) {
    ElMessage.error('请选择需要删除的源')
    return
  }
  const operation = operations.begin('delete-selected-sources')
  try {
    await ElMessageBox.confirm('确认要删除所选择的书源吗?', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })
    if (!operations.canCommit(operation)) return
    deleting.value = true
    const sourceIds = selection.value.map(source => source.id).filter(Boolean)
    await batchSources({ action: 'delete', sourceIds })
    if (!operations.canCommit(operation)) return
    selection.value = []
    tableRef.value?.clearSelection()
    await loadSources(operation)
    if (isFailureMode.value) await loadInvalidSourceHealth(operation)
    if (!operations.canCommit(operation)) return
    ElMessage.success('删除书源成功')
  } catch (error) {
    if (!operations.canCommit(operation) || isCancel(error)) return
    ElMessage.error(readError(error, '删除书源失败'))
  } finally {
    if (operations.canCommit(operation)) deleting.value = false
  }
}

async function clearAllSources() {
  const operation = operations.begin('clear-all-sources')
  try {
    await ElMessageBox.confirm('确认要清空所有书源吗?', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })
    if (!operations.canCommit(operation)) return
    clearing.value = true
    await clearSources()
    if (!operations.canCommit(operation)) return
    sources.value = []
    failureSources.value = []
    selection.value = []
    tableRef.value?.clearSelection()
    ElMessage.success('清空书源成功')
  } catch (error) {
    if (!operations.canCommit(operation) || isCancel(error)) return
    ElMessage.error(readError(error, '清空书源失败'))
  } finally {
    if (operations.canCommit(operation)) clearing.value = false
  }
}

async function restoreDefaults() {
  const operation = operations.begin('restore-default-sources')
  try {
    await ElMessageBox.confirm('确认要恢复默认书源吗?', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    })
    if (!operations.canCommit(operation)) return
    restoring.value = true
    await restoreDefaultSources()
    if (!operations.canCommit(operation)) return
    await loadSources(operation)
    if (!operations.canCommit(operation)) return
    ElMessage.success('恢复默认书源成功')
  } catch (error) {
    if (!operations.canCommit(operation) || isCancel(error)) return
    ElMessage.error(`操作失败 ${readError(error, '')}`.trimEnd())
  } finally {
    if (operations.canCommit(operation)) restoring.value = false
  }
}

async function exportAllSources() {
  const operation = operations.begin('export-all-sources')
  try {
    const response = await exportSources([])
    if (!operations.canCommit(operation)) return
    downloadJSON(response.data, `reader书源-${currentDateTime()}.json`)
  } catch (error) {
    if (operations.canCommit(operation)) {
      ElMessage.error(`导出书源失败 ${readError(error, '')}`.trimEnd())
    }
  }
}

async function openEditor(source = null) {
  const operation = operations.begin('open-source-editor')
  try {
    if (!source) {
      editingSourceId.value = null
      sourceJSON.value = JSON.stringify(newSourceTemplate(), null, 4)
      showEditor.value = true
      return
    }
    const { data } = await getSource(source.id)
    if (!operations.canCommit(operation)) return
    const snapshot = sourceToEditorSnapshot(data)
    const readerDevSource = buildReaderDevBookSource(snapshot.form, snapshot.rules)
    editingSourceId.value = source.id
    sourceJSON.value = JSON.stringify(readerDevSource, null, 4)
    showEditor.value = true
  } catch (error) {
    if (operations.canCommit(operation)) {
      ElMessage.error(`加载书源信息失败 ${readError(error, '')}`.trimEnd())
    }
  }
}

async function saveSourceJSON() {
  let parsed
  try {
    parsed = JSON.parse(sourceJSON.value)
  } catch {
    ElMessage.error('书源必须是JSON格式')
    return
  }
  const name = String(parsed.bookSourceName || parsed.name || '').trim()
  if (!name) {
    ElMessage.error('书源名称不能为空')
    return
  }
  const url = String(parsed.bookSourceUrl || parsed.baseUrl || '').trim()
  if (!url) {
    ElMessage.error('书源链接不能为空')
    return
  }

  let payload
  try {
    const snapshot = sourceToEditorSnapshot(parsed)
    payload = buildBookSourcePayload(snapshot.form, snapshot.rules)
  } catch {
    ElMessage.error('书源必须是JSON格式')
    return
  }

  const sourceId = editingSourceId.value
  const operation = operations.begin('save-source-json')
  saving.value = true
  try {
    if (sourceId) await updateSource(sourceId, payload)
    else await createSource(payload)
    if (!operations.canCommit(operation)) return
    showEditor.value = false
    await loadSources(operation)
    if (!operations.canCommit(operation)) return
    ElMessage.success('保存书源成功')
  } catch (error) {
    if (operations.canCommit(operation)) {
      ElMessage.error(`保存书源失败 ${readError(error, '')}`.trimEnd())
    }
  } finally {
    if (operations.canCommit(operation)) saving.value = false
  }
}

async function checkInvalidSources() {
  if (!String(checkConfig.keyword || '').trim()) {
    ElMessage.error('请输入搜索关键词')
    return
  }
  const sourceRows = [...sources.value]
  if (!sourceRows.length) {
    checkProgress.value = '0/0'
    return
  }
  const operation = operations.begin('check-invalid-sources')
  checking.value = true
  checkProgress.value = `0/${sourceRows.length}`
  let checked = 0
  try {
    const batchSize = Math.max(3, Math.min(15, Number(checkConfig.concurrent) || 5))
    for (let offset = 0; offset < sourceRows.length; offset += batchSize) {
      const chunk = sourceRows.slice(offset, offset + batchSize)
      const { data } = await batchTestSources({
        sourceIds: chunk.map(source => source.id),
        keyword: String(checkConfig.keyword).trim(),
        timeout: checkConfig.timeout,
        concurrent: checkConfig.concurrent,
      })
      if (!operations.canCommit(operation)) return
      mergeHealthFailures(data?.results || [])
      checked += chunk.length
      checkProgress.value = `${checked}/${sourceRows.length}`
    }
  } catch (error) {
    if (operations.canCommit(operation)) {
      ElMessage.error(readError(error, '检测书源失败'))
    }
  } finally {
    if (operations.canCommit(operation)) checking.value = false
  }
}

function mergeHealthFailures(results) {
  for (const result of results) {
    if (result?.ok) continue
    const source = sources.value.find(item => Number(item.id) === Number(result.sourceId))
    if (!source) continue
    const row = {
      ...source,
      errorMessage: visibleHealthError(result),
    }
    const index = failureSources.value.findIndex(item => Number(item.id) === Number(source.id))
    if (index >= 0) failureSources.value.splice(index, 1, row)
    else failureSources.value.push(row)
  }
}

function visibleHealthError(result) {
  return visibleFailureCategory(result?.message, result?.code)
}

function visibleFailureCategory(rawMessage, code = '') {
  const message = String(rawMessage || '')
  if (failureGroupOrder.some(type => message.includes(type))) return message
  if (/timeout|超时/i.test(message)) return 'timeout'
  const status = message.match(/\b(307|400|403|404|500|502|503|504|513)\b/)?.[1]
  if (status) return `responseCode: ${status}`
  if (code === 'source_request_failed' || message === '请求书源失败' || message === 'failed to request book source') {
    return 'ConnectException: Failed to connect'
  }
  return message || '请求书源失败'
}

function newSourceTemplate() {
  return {
    bookSourceComment: '',
    bookSourceGroup: '',
    bookSourceName: '新增书源',
    bookSourceType: 0,
    bookSourceUrl: '',
    bookUrlPattern: '',
    enabled: true,
    enabledExplore: true,
    exploreUrl: '',
    ruleBookInfo: {},
    ruleContent: { content: '' },
    ruleExplore: {},
    ruleSearch: {
      author: '',
      bookList: '',
      bookUrl: '',
      coverUrl: '',
      intro: '',
      kind: '',
      lastChapter: '',
      name: '',
    },
    ruleToc: {
      chapterList: '',
      chapterName: '',
      chapterUrl: '',
    },
    searchUrl: '',
  }
}

function downloadJSON(data, filename) {
  const blob = data instanceof Blob
    ? data
    : new Blob([data], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}

function currentDateTime() {
  const date = new Date()
  const pad = value => String(value).padStart(2, '0')
  return [
    date.getFullYear(),
    pad(date.getMonth() + 1),
    pad(date.getDate()),
    pad(date.getHours()),
    pad(date.getMinutes()),
    pad(date.getSeconds()),
  ].join('-')
}

function isCancel(error) {
  return error === 'cancel' || error === 'close'
}

function readError(error, fallback) {
  return error?.response?.data?.error?.message || error?.response?.data?.error || fallback
}
</script>

<style scoped>
.source-dialog-title,
.source-title-actions,
.source-dialog-footer,
.source-check-form,
.source-group-wrapper,
.source-pagination {
  display: flex;
  align-items: center;
}

.source-dialog-title {
  min-width: 0;
  justify-content: space-between;
  gap: 16px;
}

.source-title-actions {
  min-width: 0;
  overflow-x: auto;
  gap: 4px;
}

.source-title-actions :deep(.el-button) {
  margin-left: 0;
  white-space: nowrap;
}

.source-manager-body {
  display: grid;
  min-width: 0;
  gap: 10px;
}

.source-check-form {
  min-width: 0;
  gap: 8px;
}

.source-check-form :deep(.el-input) {
  min-width: 120px;
  flex: 1;
}

.source-check-form :deep(.el-input-number) {
  width: 128px;
}

.source-check-label {
  flex: 0 0 auto;
  color: var(--app-text-muted);
  white-space: nowrap;
}

.source-timeout-label {
  min-width: 68px;
}

.source-group-wrapper {
  min-width: 0;
  overflow-x: auto;
  gap: 8px;
  padding-bottom: 2px;
}

.source-group-btn {
  flex: 0 0 auto;
  cursor: pointer;
  user-select: none;
}

.source-group-btn.selected {
  border-color: var(--app-primary);
  color: var(--app-primary);
}

.source-table {
  width: 100%;
}

.source-used-books {
  margin: 0;
  color: inherit;
  font: inherit;
  line-height: 1.55;
  white-space: pre-wrap;
}

.source-pagination {
  min-width: 0;
  justify-content: flex-end;
  overflow-x: auto;
}

.source-dialog-footer {
  min-width: 0;
  justify-content: flex-end;
  gap: 10px;
}

.source-dialog-footer .float-left {
  margin-right: auto;
}

.source-dialog-footer .check-tip {
  color: var(--app-text-muted);
  white-space: nowrap;
}

.source-json-compatibility-warning {
  margin-bottom: 12px;
}

.source-json-editor {
  width: 100%;
}

.source-json-editor :deep(.el-textarea__inner) {
  min-height: min(62dvh, 560px) !important;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
  line-height: 1.55;
  tab-size: 4;
}

@media (max-width: 750px) {
  .source-dialog-title {
    align-items: flex-start;
  }

  .source-title-actions {
    max-width: calc(100vw - 110px);
  }

  .source-check-form {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
  }

  .source-check-form :deep(.el-input),
  .source-check-form :deep(.el-input-number) {
    width: 100%;
  }

  .source-pagination {
    justify-content: flex-start;
  }

  .source-dialog-footer {
    flex-wrap: wrap;
  }

  .source-dialog-footer .float-left {
    margin-right: 0;
  }

  .source-json-editor :deep(.el-textarea__inner) {
    min-height: calc(100dvh - 168px) !important;
  }
}
</style>
