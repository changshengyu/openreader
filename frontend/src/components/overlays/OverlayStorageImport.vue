<template>
  <el-dialog
    :model-value="phase === 'preflight'"
    :title="`${sourceLabel}导入预检`"
    width="760px"
    :fullscreen="isMobile"
    class="storage-import-preflight-dialog"
    @update:model-value="handlePreflightVisible"
  >
    <p class="storage-import-tip">
      已先冻结所选文件的字节。修正规则后可在同一份暂存数据上重新解析，不会重新读取书仓或 WebDAV 原文件。
    </p>
    <div class="storage-import-preflight-list">
      <article v-for="row in rows" :key="row.path" :class="['storage-import-preflight-row', { failed: row.error }]">
        <el-tag :type="row.valid ? 'success' : 'danger'" effect="plain">{{ row.valid ? '可导入' : '待修复' }}</el-tag>
        <div>
          <strong>{{ row.path }}</strong>
          <p v-if="row.error">{{ row.error }}</p>
          <span v-else>已解析 {{ row.chapterCount }} 章</span>
        </div>
        <div v-if="isRuleConfigurable(row.path)" class="storage-import-rule-row">
          <el-input v-if="isTextLocalPath(row.path)" v-model="row.tocRule" placeholder="TXT 目录规则（可选）" />
          <el-select v-else v-model="row.tocRule" placeholder="EPUB 目录规则">
            <el-option v-for="rule in epubTocRuleOptions" :key="rule.value" :label="rule.label" :value="rule.value" />
          </el-select>
          <el-button :loading="row.reparsing" :disabled="!row.importToken" @click="reparse(row)">重新解析</el-button>
        </div>
      </article>
    </div>
    <template #footer>
      <el-button @click="workflow.cancelAll">取消</el-button>
      <el-button type="primary" :disabled="!validRows.length" @click="workflow.continueAfterPreflight">
        继续导入 {{ validRows.length || '' }} 本
      </el-button>
    </template>
  </el-dialog>

  <el-dialog
    :model-value="phase === 'choose-mode'"
    title="提示"
    width="440px"
    :fullscreen="isMobile"
    class="storage-import-mode-dialog"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    @update:model-value="handleModeVisible"
  >
    <p class="storage-import-tip">你选择导入多本书籍，请选择导入方式？</p>
    <template #footer>
      <el-button type="primary" @click="workflow.chooseBatch">批量导入</el-button>
      <el-button @click="workflow.chooseSequential">逐一确认导入</el-button>
    </template>
  </el-dialog>

  <el-dialog
    :model-value="phase === 'batch-groups'"
    title="统一设置分组"
    width="440px"
    :fullscreen="isMobile"
    class="storage-import-groups-dialog"
    @update:model-value="handleBatchGroupsVisible"
  >
    <div class="storage-import-groups-body">
      <span>请选择分组：</span>
      <el-select v-model="batchCategoryIds" multiple clearable filterable placeholder="未分组">
        <el-option v-for="category in bookshelf.categories" :key="category.id" :label="category.name" :value="Number(category.id)" />
      </el-select>
    </div>
    <template #footer>
      <el-button @click="workflow.cancelBatchGroups">取消导入</el-button>
      <el-button type="primary" :loading="busy" @click="workflow.confirmBatch">确定</el-button>
    </template>
  </el-dialog>

  <el-dialog
    :model-value="phase === 'single'"
    :title="`导入本地书籍${currentLabel}`"
    width="min(760px, calc(100vw - 48px))"
    :fullscreen="isMobile"
    class="storage-import-single-dialog"
    @update:model-value="handleSingleVisible"
  >
    <template v-if="currentRow">
      <div class="storage-import-single-form">
        <el-input v-model="currentRow.title" placeholder="书名" />
        <el-input v-model="currentRow.author" placeholder="作者（可选）" />
        <el-select v-model="currentRow.categoryIds" multiple clearable filterable placeholder="未分组">
          <el-option v-for="category in bookshelf.categories" :key="category.id" :label="category.name" :value="Number(category.id)" />
        </el-select>
        <div v-if="isRuleConfigurable(currentRow.path)" class="storage-import-rule-row">
          <el-input v-if="isTextLocalPath(currentRow.path)" v-model="currentRow.tocRule" placeholder="TXT 目录规则（可选）" />
          <el-select v-else v-model="currentRow.tocRule" placeholder="EPUB 目录规则">
            <el-option v-for="rule in epubTocRuleOptions" :key="rule.value" :label="rule.label" :value="rule.value" />
          </el-select>
          <el-button :loading="currentRow.reparsing" :disabled="!currentRow.importToken" @click="reparse(currentRow)">刷新目录</el-button>
        </div>
        <p v-if="currentRow.lastError" class="storage-import-error">{{ currentRow.lastError }}</p>
      </div>
      <div class="storage-import-chapter-title">章节列表（{{ currentRow.chapters.length }}）</div>
      <div class="storage-import-chapter-list">
        <p v-for="chapter in currentRow.chapters" :key="chapter.index">{{ Number(chapter.index) + 1 }}. {{ chapter.title }}</p>
      </div>
    </template>
    <template #footer>
      <el-button @click="workflow.skipCurrent">取消</el-button>
      <el-button type="primary" :loading="busy" :disabled="!currentRow?.valid" @click="workflow.confirmCurrent">确定导入</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { importFromLocalStore, previewLocalStoreImport } from '../../api/localStore'
import { importFromWebDAV, previewWebDAVImport } from '../../api/webdav'
import { useStorageImportWorkflow } from '../../composables/useStorageImportWorkflow'
import { useBookshelfStore } from '../../stores/bookshelf'
import { useOverlayStore } from '../../stores/overlay'
import { epubTocRuleOptions, isEPUBLocalPath, isTextLocalPath } from '../../utils/localBookToc'

defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const overlay = useOverlayStore()
const bookshelf = useBookshelfStore()
const workflow = useStorageImportWorkflow({
  loadCategories: () => bookshelf.ensureCategoriesLoaded(),
  preview: async (source, payload) => {
    const response = source === 'local-store'
      ? await previewLocalStoreImport(payload)
      : await previewWebDAVImport(payload)
    return response.data
  },
  importItem: async (source, item, categoryIds) => {
    const response = source === 'local-store'
      ? await importFromLocalStore([item], categoryIds)
      : await importFromWebDAV([item], categoryIds)
    return response.data
  },
  onImported: book => bookshelf.upsertBook(book),
  onError: message => ElMessage.error(message),
  onComplete: summary => {
    if (summary.succeeded) {
      ElMessage.success(`导入 ${summary.succeeded} 本${summary.failed ? `，${summary.failed} 本失败` : ''}`)
    } else if (summary.failed) {
      ElMessage.warning(`导入失败 ${summary.failed} 本`)
    }
    overlay.closeStorageImport()
  },
  onCancel: () => overlay.closeStorageImport(),
})

const {
  phase,
  source,
  rows,
  validRows,
  currentRow,
  currentLabel,
  batchCategoryIds,
  busy,
} = workflow

const sourceLabel = computed(() => source.value === 'webdav' ? 'WebDAV ' : '本地书仓')

watch(
  () => overlay.storageImportRequest?.requestId,
  requestId => {
    if (!requestId || !overlay.storageImportVisible || !overlay.storageImportRequest) return
    workflow.start(overlay.storageImportRequest)
  },
)

watch(
  () => overlay.storageImportVisible,
  visible => {
    if (!visible) workflow.reset()
  },
)

function handlePreflightVisible(visible) {
  if (!visible) workflow.cancelAll()
}

function handleModeVisible(visible) {
  if (!visible) workflow.cancelMode()
}

function handleBatchGroupsVisible(visible) {
  if (!visible) workflow.cancelBatchGroups()
}

function handleSingleVisible(visible) {
  if (!visible) workflow.skipCurrent()
}

function isRuleConfigurable(path) {
  return isTextLocalPath(path) || isEPUBLocalPath(path)
}

function reparse(row) {
  workflow.reparse(row)
}
</script>

<style scoped>
.storage-import-tip,
.storage-import-preflight-row p,
.storage-import-error {
  margin: 0;
  color: var(--app-text-muted);
  line-height: 1.6;
}

.storage-import-preflight-list,
.storage-import-single-form,
.storage-import-chapter-list {
  display: grid;
  gap: 10px;
}

.storage-import-preflight-list {
  max-height: min(56vh, 560px);
  overflow: auto;
  margin-top: 14px;
}

.storage-import-preflight-row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 10px;
  padding: 12px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.storage-import-preflight-row.failed {
  background: var(--el-color-danger-light-9);
}

.storage-import-preflight-row strong,
.storage-import-preflight-row span {
  display: block;
  overflow-wrap: anywhere;
}

.storage-import-rule-row {
  display: flex;
  grid-column: 1 / -1;
  gap: 8px;
}

.storage-import-rule-row > :first-child {
  min-width: 0;
  flex: 1;
}

.storage-import-groups-body {
  display: grid;
  gap: 10px;
}

.storage-import-chapter-title {
  margin-top: 16px;
  font-weight: 700;
}

.storage-import-chapter-list {
  max-height: min(46vh, 420px);
  overflow: auto;
  margin-top: 8px;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
  background: var(--app-bg-soft);
}

.storage-import-chapter-list p {
  margin: 0;
}

.storage-import-error {
  color: var(--el-color-danger);
}

@media (max-width: 750px) {
  .storage-import-rule-row {
    display: grid;
    grid-template-columns: 1fr;
  }
}
</style>
