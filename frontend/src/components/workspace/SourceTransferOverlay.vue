<template>
  <input
    ref="sourceUploadRef"
    class="source-upload-input"
    type="file"
    accept=".json,application/json"
    @change="handleSourceFile"
    @cancel="requestClose"
  />

  <el-dialog
    v-model="showImportPreview"
    title="导入书源"
    width="min(1000px, max(750px, 70vw))"
    top="15vh"
    :fullscreen="isMobile"
    class="source-import-preview-dialog"
    @closed="handlePreviewClosed"
  >
    <el-checkbox-group
      v-model="checkedImportSourceIndexes"
      class="source-import-list"
      @change="handleImportSelectionChange"
    >
      <el-checkbox
        v-for="(source, index) in importPreviewSources"
        :key="index"
        :label="index"
        class="source-import-item"
      >
        <strong>{{ importSourceName(source) || '未命名书源' }}</strong>
        <span>{{ importSourceURL(source) || '未设置地址' }}</span>
        <em v-if="importSourceTags(source)">{{ importSourceTags(source) }}</em>
        <small
          v-if="importSourceCompatibilityHint(source)"
          class="source-compatibility-hint"
        >
          {{ importSourceCompatibilityHint(source) }}
        </small>
      </el-checkbox>
    </el-checkbox-group>

    <template #footer>
      <div class="source-import-footer">
        <el-checkbox
          v-model="importCheckAll"
          :indeterminate="importCheckIndeterminate"
          border
          class="float-left"
          @change="toggleImportCheckAll"
        >
          全选
        </el-checkbox>
        <span class="check-tip">已选择 {{ checkedImportSourceIndexes.length }} 个</span>
        <el-button @click="closePreviewAndOwner">取消</el-button>
        <el-button type="primary" :loading="importPreviewSaving" @click="confirmImport">
          确定
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { nextTick, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  exportSources,
  importSources,
  previewRemoteSource,
} from '../../api/sources'
import { useAuthenticatedOperationGuard } from '../../composables/useAuthenticatedOperationGuard'
import { useSourceTransfer } from '../../composables/useSourceTransfer'
import { useUserStore } from '../../stores/user'

const props = defineProps({
  visible: { type: Boolean, default: false },
  intent: { type: String, default: 'import' },
  isMobile: { type: Boolean, default: false },
})
const emit = defineEmits(['close'])
const operations = useAuthenticatedOperationGuard()
const user = useUserStore()
const ownerClosing = ref(false)
let activation = 0

const {
  remoteURL,
  sourceUploadRef,
  showImportPreview,
  importPreviewSources,
  checkedImportSourceIndexes,
  importCheckAll,
  importCheckIndeterminate,
  importPreviewSaving,
  importFile,
  openSourceImportPicker,
  importRemote,
  closeImportPreview,
  toggleImportCheckAll,
  handleImportSelectionChange,
  importSourceName,
  importSourceURL,
  importSourceTags,
  importSourceCompatibilityHint,
  saveSelectedImportSources,
} = useSourceTransfer({
  operationGuard: operations,
  previewRemoteSource,
  importSources,
  exportSources,
  reloadSources: async () => {},
  getSelection: () => [],
  download: () => {},
  onInfo: message => ElMessage.info(message),
  onWarning: message => ElMessage.warning(message),
  onSuccess: message => ElMessage.success(message),
  onError: (error, fallback) => {
    const detail = readError(error, '')
    ElMessage.error(detail ? `${fallback} ${detail}` : fallback)
  },
})

watch(
  () => [props.visible, props.intent],
  async ([visible, intent]) => {
    activation += 1
    const current = activation
    ownerClosing.value = false
    if (!visible) {
      closeImportPreview()
      return
    }
    if (intent === 'import') {
      await nextTick()
      if (current !== activation || !props.visible) return
      if (sourceUploadRef.value) sourceUploadRef.value.value = ''
      openSourceImportPicker()
      return
    }
    if (intent === 'remote') {
      await openRemotePrompt(current)
      return
    }
    requestClose()
  },
  { immediate: true },
)

async function handleSourceFile(event) {
  const file = event.target?.files?.[0]
  if (!file) {
    requestClose()
    return
  }
  await importFile({ raw: file })
  event.target.value = ''
  if (!showImportPreview.value) requestClose()
}

async function openRemotePrompt(current) {
  const storageKey = `${user.profile?.username || user.profile?.id || 'user'}@lastRemoteSourceUrl`
  const inputValue = safeLocalStorageGet(storageKey)
  let result
  try {
    result = await ElMessageBox.prompt('请输入远程书源链接', '导入远程书源文件', {
      inputValue,
      confirmButtonText: '确定',
      cancelButtonText: '取消',
    })
  } catch {
    if (current === activation) requestClose()
    return
  }
  if (current !== activation || !props.visible) return
  remoteURL.value = String(result?.value || '').trim()
  if (!remoteURL.value) {
    requestClose()
    return
  }
  const acceptedURL = remoteURL.value
  await importRemote()
  if (current !== activation || !props.visible) return
  if (showImportPreview.value) safeLocalStorageSet(storageKey, acceptedURL)
  else requestClose()
}

async function confirmImport() {
  await saveSelectedImportSources()
  if (!showImportPreview.value) requestClose()
}

function closePreviewAndOwner() {
  ownerClosing.value = true
  closeImportPreview()
  requestClose()
}

function handlePreviewClosed() {
  if (ownerClosing.value || !showImportPreview.value) requestClose()
}

function requestClose() {
  emit('close')
}

function safeLocalStorageGet(key) {
  try {
    return localStorage.getItem(key) || ''
  } catch {
    return ''
  }
}

function safeLocalStorageSet(key, value) {
  try {
    localStorage.setItem(key, value)
  } catch {
    // A blocked storage backend must not block the import flow.
  }
}

function readError(error, fallback) {
  return error?.response?.data?.error?.message || error?.response?.data?.error || fallback
}
</script>

<style scoped>
.source-upload-input {
  position: fixed;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip-path: inset(50%);
  opacity: 0;
  pointer-events: none;
}

.source-import-list {
  display: grid;
  max-height: min(62dvh, 560px);
  min-width: 0;
  overflow: auto;
  gap: 8px;
}

.source-import-item {
  display: block;
  min-width: 0;
  margin-right: 0;
  padding: 9px 10px;
  border-bottom: 1px solid var(--app-border);
}

.source-import-item :deep(.el-checkbox__label) {
  display: grid;
  min-width: 0;
  gap: 2px;
  line-height: 1.5;
  white-space: normal;
}

.source-import-item span,
.source-import-item em,
.source-import-item small {
  overflow-wrap: anywhere;
  color: var(--app-text-muted);
  font-size: 12px;
  font-style: normal;
}

.source-compatibility-hint {
  color: #b26a00;
}

.source-import-footer {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
}

.source-import-footer .float-left {
  margin-right: auto;
}

.source-import-footer .check-tip {
  color: var(--app-text-muted);
  white-space: nowrap;
}

@media (max-width: 750px) {
  .source-import-list {
    max-height: calc(100dvh - 150px);
  }

  .source-import-footer {
    flex-wrap: wrap;
  }

  .source-import-footer .float-left {
    margin-right: 0;
  }
}
</style>
