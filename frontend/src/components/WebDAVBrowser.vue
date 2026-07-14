<template>
  <section class="webdav-browser">
    <el-table
      v-loading="loading"
      :data="tableItems"
      row-key="path"
      stripe
      class="webdav-table"
      :selectable="row => !row.toParent"
      @selection-change="onSelectionChange"
      @row-dblclick="openItem"
    >
      <el-table-column type="selection" width="42" />
      <el-table-column prop="name" label="名称" min-width="238" show-overflow-tooltip>
        <template #default="{ row }">
          <button class="file-name" type="button" @click="openItem(row)">
            <el-icon><component :is="row.isDir ? FolderOpened : Document" /></el-icon>
            <span>{{ row.name }}</span>
          </button>
        </template>
      </el-table-column>
      <el-table-column label="大小" width="112">
        <template #default="{ row }">{{ row.isDir || row.toParent ? '-' : formatSize(row.size) }}</template>
      </el-table-column>
      <el-table-column label="修改时间" width="156">
        <template #default="{ row }">{{ formatDate(row.lastModified) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="230" fixed="right">
        <template #default="{ row }">
          <el-button v-if="isBackupFile(row)" text type="primary" :loading="restoring === row.path" @click="restoreBackupFile(row)">恢复</el-button>
          <el-button v-if="!row.isDir && !row.toParent" text type="primary" @click="downloadFile(row)">下载</el-button>
          <el-button v-if="canImport(row)" text type="primary" :loading="storageImportPending" @click="importBook(row)">加入书架</el-button>
          <el-button v-if="!row.toParent" text type="danger" @click="deleteItem(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-empty v-if="!loading && !items.length" description="WebDAV 目录为空" />

    <footer v-if="items.length" class="webdav-batch-footer">
      <span class="check-tip">已选择 {{ selection.length }} 个</span>
      <el-button type="primary" plain :disabled="!selection.length" @click="deleteSelected">批量删除</el-button>
      <el-button
        type="primary"
        :disabled="!importSelection.length || storageImportPending"
        :loading="storageImportPending"
        @click="importSelected"
      >
        批量加入书架 {{ importSelection.length || '' }}
      </el-button>
      <input ref="uploadInput" class="hidden-upload" type="file" multiple @change="uploadFiles">
      <el-button :loading="uploading" @click="chooseFiles">上传文件</el-button>
      <el-button @click="clearSelection">取消</el-button>
    </footer>
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Document, FolderOpened } from '@element-plus/icons-vue'
import { restoreWebDAVBackup } from '../api/backup'
import { deleteWebDAV, downloadWebDAV, listWebDAV, uploadWebDAV } from '../api/webdav'
import { useOverlayStore } from '../stores/overlay'
import { applyRestoreResult } from '../utils/restoreSync'
import { isWebDAVImportable } from '../utils/storageImportable'

const overlay = useOverlayStore()
const path = ref('')
const items = ref([])
const selection = ref([])
const loading = ref(false)
const uploading = ref(false)
const restoring = ref('')
const uploadInput = ref(null)

const tableItems = computed(() => {
  if (!path.value) return items.value
  return [{ name: '..', path: parentPath(path.value), isDir: true, toParent: true }, ...items.value]
})
const importSelection = computed(() => selection.value.filter(canImport))
const storageImportPending = computed(() => overlay.storageImportVisible)

onMounted(load)

async function load() {
  loading.value = true
  try {
    const { data } = await listWebDAV(path.value)
    items.value = parseWebDAVListing(data)
    clearSelection()
  } catch (err) {
    ElMessage.error(readError(err, '加载 WebDAV 失败'))
  } finally {
    loading.value = false
  }
}

async function openItem(row) {
  if (!row.isDir) return
  path.value = row.path
  await load()
}

function onSelectionChange(rows) {
  selection.value = rows.filter(row => !row.toParent)
}

function clearSelection() {
  selection.value = []
}

function chooseFiles() {
  uploadInput.value?.click()
}

async function uploadFiles(event) {
  const files = Array.from(event.target?.files || [])
  if (!files.length) return
  uploading.value = true
  try {
    for (const file of files) await uploadWebDAV({ path: path.value, file })
    ElMessage.success('WebDAV 文件已上传')
    await load()
  } catch (err) {
    ElMessage.error(readError(err, '上传 WebDAV 失败'))
  } finally {
    uploading.value = false
    if (event.target) event.target.value = ''
  }
}

async function downloadFile(row) {
  try {
    const resp = await downloadWebDAV(row.path)
    downloadBlob(resp.data, row.name)
  } catch (err) {
    ElMessage.error(readError(err, '下载 WebDAV 文件失败'))
  }
}

async function restoreBackupFile(row) {
  try {
    await ElMessageBox.confirm(`确定从 WebDAV 文件“${row.name}”恢复备份吗？`, '恢复 WebDAV 备份', { type: 'warning' })
    restoring.value = row.path
    const { data } = await restoreWebDAVBackup(row.path)
    ElMessage.success(`恢复完成：书源 ${data.sources || 0}，书籍 ${data.books || 0}，进度 ${data.progress || 0}`)
    await applyRestoreResult(data)
    await load()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '恢复 WebDAV 备份失败'))
  } finally {
    restoring.value = ''
  }
}

async function deleteItem(row) {
  try {
    await ElMessageBox.confirm(`确定删除“${row.name}”吗？`, '删除 WebDAV 项目', { type: 'warning' })
    await deleteWebDAV(row.path)
    ElMessage.success('已删除')
    await load()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除 WebDAV 项目失败'))
  }
}

async function deleteSelected() {
  if (!selection.value.length) return
  try {
    await ElMessageBox.confirm(`确定删除选中的 ${selection.value.length} 个 WebDAV 项目吗？`, '批量删除 WebDAV 项目', { type: 'warning' })
    for (const row of selection.value) await deleteWebDAV(row.path)
    ElMessage.success('已批量删除')
    await load()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '批量删除 WebDAV 项目失败'))
  }
}

function canImport(row) {
  return !row?.isDir && !row?.toParent && isWebDAVImportable(row.path || row.name)
}

function importBook(row) {
  if (canImport(row)) importBooks([row.path])
}

function importSelected() {
  if (importSelection.value.length) importBooks(importSelection.value.map(row => row.path))
}

function importBooks(paths) {
  overlay.openStorageImport('webdav', paths)
}

function parseWebDAVListing(xml) {
  const doc = new DOMParser().parseFromString(xml, 'application/xml')
  return [...doc.querySelectorAll('prop')].map((node) => {
    const name = node.querySelector('displayname')?.textContent || ''
    return {
      name,
      path: joinPath(path.value, name),
      isDir: node.querySelector('iscollection')?.textContent === 'true',
      size: Number(node.querySelector('getcontentlength')?.textContent || 0),
      lastModified: node.querySelector('lastmodified')?.textContent || '',
    }
  }).filter(item => item.name && item.path !== path.value)
}

function isBackupFile(row) {
  return !row?.isDir && !row?.toParent && String(row.name || '').toLowerCase().endsWith('.zip')
}

function parentPath(value) {
  const parts = String(value || '').split('/').filter(Boolean)
  parts.pop()
  return parts.join('/')
}

function joinPath(base, name) {
  return [base, name].filter(Boolean).join('/')
}

function formatSize(bytes) {
  const value = Number(bytes || 0)
  if (!value) return '0 B'
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}

function formatDate(value) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
}

function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob instanceof Blob ? blob : new Blob([blob]))
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.webdav-browser {
  display: grid;
  gap: 14px;
}

.webdav-table {
  width: 100%;
}

.file-name {
  display: inline-flex;
  max-width: 100%;
  align-items: center;
  gap: 8px;
  padding: 0;
  color: var(--app-text);
  background: transparent;
  border: 0;
  cursor: pointer;
}

.file-name span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.webdav-batch-footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.check-tip {
  margin-right: auto;
  color: var(--app-text-muted);
  font-size: 13px;
}

.hidden-upload {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  clip-path: inset(50%);
  white-space: nowrap;
}

@media (max-width: 750px) {
  .webdav-table {
    overflow-x: auto;
  }

  .webdav-batch-footer {
    position: sticky;
    z-index: 2;
    bottom: max(10px, env(safe-area-inset-bottom));
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    background: var(--app-surface);
    box-shadow: 0 -8px 22px rgba(15, 23, 42, 0.08);
  }

  .check-tip {
    grid-column: 1 / -1;
    margin: 0;
  }

  .webdav-batch-footer :deep(.el-button) {
    width: 100%;
    min-height: 38px;
    margin-left: 0;
  }
}
</style>
