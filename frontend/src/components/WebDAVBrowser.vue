<template>
  <section class="webdav-browser">
    <header class="webdav-head">
      <span class="webdav-current-path">{{ currentPathLabel }}</span>
      <div class="webdav-actions">
        <el-select v-model="targetCategoryIds" size="small" placeholder="导入分组（可多选）" multiple clearable class="webdav-category-select">
          <el-option v-for="category in bookshelf.categories" :key="category.id" :label="category.name" :value="String(category.id)" />
        </el-select>
        <el-button size="small" :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        <el-button size="small" :icon="FolderOpened" @click="createFolder">新建目录</el-button>
        <el-upload class="webdav-batch-command" :show-file-list="false" :auto-upload="false" @change="uploadFile">
          <el-button size="small" :icon="Upload" :loading="uploading">上传</el-button>
        </el-upload>
        <el-button class="webdav-batch-command" size="small" type="danger" plain :disabled="!selection.length" @click="deleteSelected">
          删除 {{ selection.length }}
        </el-button>
        <el-button class="webdav-batch-command" size="small" type="primary" :disabled="!importSelection.length" :loading="importing" @click="importSelected">
          加入书架 {{ importSelection.length }}
        </el-button>
      </div>
    </header>

    <el-breadcrumb separator="/" class="webdav-breadcrumb">
      <el-breadcrumb-item>
        <button type="button" @click="goPath('')">webdav</button>
      </el-breadcrumb-item>
      <el-breadcrumb-item v-for="crumb in breadcrumbs" :key="crumb.path">
        <button type="button" @click="goPath(crumb.path)">{{ crumb.name }}</button>
      </el-breadcrumb-item>
    </el-breadcrumb>

    <el-table
      :data="items"
      stripe
      v-loading="loading"
      class="webdav-table desktop-webdav-table"
      @selection-change="selection = $event"
    >
      <el-table-column type="selection" width="42" />
      <el-table-column prop="name" label="名称" min-width="220" show-overflow-tooltip>
        <template #default="{ row }">
          <button class="file-name" type="button" @click="openItem(row)">
            <el-icon><component :is="row.isDir ? FolderOpened : Document" /></el-icon>
            <span>{{ row.name }}</span>
          </button>
        </template>
      </el-table-column>
      <el-table-column label="类型" width="90">
        <template #default="{ row }">{{ row.isDir ? '目录' : '文件' }}</template>
      </el-table-column>
      <el-table-column label="大小" width="110">
        <template #default="{ row }">{{ row.isDir ? '-' : formatSize(row.size) }}</template>
      </el-table-column>
      <el-table-column label="修改时间" width="150">
        <template #default="{ row }">{{ formatDate(row.lastModified) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="280" fixed="right">
        <template #default="{ row }">
          <el-button v-if="!row.isDir && isBackupFile(row)" text type="primary" :loading="restoring === row.name" @click="restoreBackupFile(row)">恢复</el-button>
          <el-button v-if="!row.isDir" text type="primary" @click="downloadFile(row)">下载</el-button>
          <el-button v-if="row.importable" text type="primary" :loading="importing" @click="importBook(row)">加入书架</el-button>
          <el-button v-else-if="row.isDir" text type="primary" :loading="importing" @click="importDirectory(row)">加入目录</el-button>
          <el-button text @click="renameItem(row)">重命名</el-button>
          <el-button text type="danger" @click="deleteItem(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div v-if="items.length" class="mobile-file-select-actions">
      <span>已选 {{ selection.length }} 个</span>
      <div>
        <el-button size="small" text @click="selectShownFiles">全选当前</el-button>
        <el-button size="small" text @click="selection = []">清空</el-button>
      </div>
    </div>

    <div v-if="items.length" v-loading="loading" class="mobile-file-list">
      <article v-for="row in items" :key="row.name" class="mobile-file-card">
        <header>
          <button class="mobile-file-name" type="button" @click="openItem(row)">
            <el-icon><component :is="row.isDir ? FolderOpened : Document" /></el-icon>
            <span>{{ row.name }}</span>
          </button>
          <el-checkbox
            :model-value="selection.some(item => item.name === row.name)"
            @change="value => toggleSelection(row, value)"
          />
        </header>
        <p>{{ joinPath(path, row.name) }}</p>
        <div class="mobile-file-meta">
          <el-tag size="small" effect="plain">{{ row.isDir ? '目录' : '文件' }}</el-tag>
          <el-tag v-if="!row.isDir" size="small" effect="plain">{{ formatSize(row.size) }}</el-tag>
          <el-tag v-if="row.lastModified" size="small" effect="plain">{{ formatDate(row.lastModified) }}</el-tag>
          <el-tag v-if="row.importable" size="small" type="success" effect="plain">可加入书架</el-tag>
          <el-tag v-if="!row.isDir && isBackupFile(row)" size="small" type="warning" effect="plain">备份</el-tag>
        </div>
        <footer>
          <el-button v-if="!row.isDir && isBackupFile(row)" size="small" text type="primary" :loading="restoring === row.name" @click="restoreBackupFile(row)">恢复</el-button>
          <el-button v-if="!row.isDir" size="small" text type="primary" @click="downloadFile(row)">下载</el-button>
          <el-button v-if="row.importable" size="small" text type="primary" :loading="importing" @click="importBook(row)">加入书架</el-button>
          <el-button v-else-if="row.isDir" size="small" text type="primary" :loading="importing" @click="importDirectory(row)">加入目录</el-button>
          <el-button size="small" text @click="renameItem(row)">重命名</el-button>
          <el-button size="small" text type="danger" @click="deleteItem(row)">删除</el-button>
        </footer>
      </article>
    </div>

    <el-empty v-if="!loading && !items.length" description="WebDAV 目录为空" />

    <div v-if="items.length" class="webdav-batch-footer">
      <span class="check-tip">已选择 {{ selection.length }} 个</span>
      <el-button type="primary" plain :disabled="!selection.length" @click="deleteSelected">批量删除</el-button>
      <el-button type="primary" :disabled="!importSelection.length" :loading="importing" @click="importSelected">
        批量加入书架 {{ importSelection.length || '' }}
      </el-button>
      <el-upload :show-file-list="false" :auto-upload="false" @change="uploadFile">
        <el-button :loading="uploading">上传文件</el-button>
      </el-upload>
      <el-button @click="selection = []">取消</el-button>
    </div>

    <LocalBookImportPreviewDialog
      v-model="previewDialog"
      title="WebDAV 导入预览"
      :items="previewItems"
      :categories="bookshelf.categories"
      :category-ids="targetCategoryIds"
      :loading="importing"
      @confirm="confirmPreviewImport"
    />

    <el-dialog v-model="importResultDialog" title="WebDAV 导入结果" width="560px" :fullscreen="isMobile">
      <div class="result-list">
        <div v-for="(item, index) in importResults" :key="index" class="result-row">
          <el-tag :type="item.book ? 'success' : 'danger'" effect="plain">{{ item.book ? '成功' : '失败' }}</el-tag>
          <span>{{ item.book?.title || item.path }}</span>
          <small>{{ item.error || `${item.book?.chapterCount || 0} 章` }}</small>
        </div>
      </div>
    </el-dialog>
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Document, FolderOpened, Refresh, Upload } from '@element-plus/icons-vue'
import { restoreWebDAVBackup } from '../api/backup'
import { createWebDAVDirectory, deleteWebDAV, downloadWebDAV, importFromWebDAV, listWebDAV, previewWebDAVImport, renameWebDAV, uploadWebDAV } from '../api/webdav'
import LocalBookImportPreviewDialog from './LocalBookImportPreviewDialog.vue'
import { useBookshelfStore } from '../stores/bookshelf'
import { applyRestoreResult } from '../utils/restoreSync'

defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['imported'])

const bookshelf = useBookshelfStore()
const path = ref('')
const items = ref([])
const selection = ref([])
const loading = ref(false)
const uploading = ref(false)
const restoring = ref('')
const importing = ref(false)
const previewDialog = ref(false)
const previewItems = ref([])
const importResultDialog = ref(false)
const importResults = ref([])
const targetCategoryIds = ref([])

const currentPathLabel = computed(() => path.value || '/')
const breadcrumbs = computed(() => {
  if (!path.value) return []
  const parts = path.value.split('/').filter(Boolean)
  return parts.map((name, index) => ({ name, path: parts.slice(0, index + 1).join('/') }))
})
const importSelection = computed(() => selection.value.filter(row => row.importable))

onMounted(load)

async function load() {
  loading.value = true
  try {
    await warmWebDAVCategories().catch((err) => {
      ElMessage.warning(readError(err, '分组加载失败，仍可浏览 WebDAV'))
    })
    const { data } = await listWebDAV(path.value)
    items.value = parseWebDAVListing(data)
    selection.value = []
  } catch (err) {
    ElMessage.error(readError(err, '加载 WebDAV 失败'))
  } finally {
    loading.value = false
  }
}

async function warmWebDAVCategories() {
  return bookshelf.ensureCategoriesLoaded()
}

async function goPath(nextPath) {
  path.value = nextPath
  await load()
}

function openItem(row) {
  if (row.isDir) goPath(joinPath(path.value, row.name))
}

function toggleSelection(row, checked) {
  if (checked) {
    if (!selection.value.some(item => item.name === row.name)) selection.value.push(row)
    return
  }
  selection.value = selection.value.filter(item => item.name !== row.name)
}

function selectShownFiles() {
  selection.value = [...items.value]
}

async function uploadFile(data) {
  const file = data.raw
  if (!file) return
  uploading.value = true
  try {
    await uploadWebDAV({ path: path.value, file })
    ElMessage.success('WebDAV 文件已上传')
    await load()
  } catch (err) {
    ElMessage.error(readError(err, '上传 WebDAV 失败'))
  } finally {
    uploading.value = false
  }
}

async function createFolder() {
  try {
    const { value } = await ElMessageBox.prompt('输入目录名称', '新建 WebDAV 目录', {
      inputValidator: value => !!value?.trim() || '目录名称不能为空',
    })
    await createWebDAVDirectory({ path: path.value, name: value.trim() })
    ElMessage.success('WebDAV 目录已创建')
    await load()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '创建 WebDAV 目录失败'))
  }
}

async function downloadFile(row) {
  try {
    const resp = await downloadWebDAV(joinPath(path.value, row.name))
    downloadBlob(resp.data, row.name)
  } catch (err) {
    ElMessage.error(readError(err, '下载 WebDAV 文件失败'))
  }
}

async function restoreBackupFile(row) {
  const backupPath = joinPath(path.value, row.name)
  try {
    await ElMessageBox.confirm(`确定从 WebDAV 文件“${row.name}”恢复备份吗？`, '恢复 WebDAV 备份', { type: 'warning' })
    restoring.value = row.name
    const { data } = await restoreWebDAVBackup(backupPath)
    ElMessage.success(`恢复完成：书源 ${data.sources || 0}，书籍 ${data.books || 0}，进度 ${data.progress || 0}`)
    await applyRestoreResult(data)
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '恢复 WebDAV 备份失败'))
  } finally {
    restoring.value = ''
  }
}

async function renameItem(row) {
  try {
    const { value } = await ElMessageBox.prompt('输入新的名称', '重命名 WebDAV 项目', {
      inputValue: row.name,
      inputValidator: value => !!value?.trim() || '名称不能为空',
    })
    const name = value.trim()
    if (!name || name === row.name) return
    await renameWebDAV({
      path: joinPath(path.value, row.name),
      newPath: joinPath(path.value, name),
    })
    ElMessage.success('已重命名')
    await load()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '重命名 WebDAV 项目失败'))
  }
}

async function deleteItem(row) {
  try {
    await ElMessageBox.confirm(`确定删除“${row.name}”吗？`, '删除 WebDAV 项目', { type: 'warning' })
    await deleteWebDAV(joinPath(path.value, row.name))
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
    for (const row of selection.value) {
      await deleteWebDAV(joinPath(path.value, row.name))
    }
    ElMessage.success('已批量删除')
    await load()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '批量删除 WebDAV 项目失败'))
  }
}

async function importBook(row) {
  if (!row.importable) return
  await importBooks([joinPath(path.value, row.name)])
}

async function importDirectory(row) {
  if (!row.isDir) return
  try {
    await ElMessageBox.confirm(`将递归导入 WebDAV 目录“${row.name}”下的可导入文件，是否继续？`, '加入 WebDAV 目录', { type: 'info' })
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    throw err
  }
  await importBooks([joinPath(path.value, row.name)])
}

async function importSelected() {
  const paths = importSelection.value.map(row => joinPath(path.value, row.name))
  if (paths.length) await importBooks(paths)
}

async function importBooks(paths) {
  importing.value = true
  try {
    const { data } = await previewWebDAVImport(paths)
    previewItems.value = data.items || []
    if (!previewItems.value.some(item => item.book)) {
      ElMessage.warning('没有可导入的书籍')
      return
    }
    previewDialog.value = true
  } catch (err) {
    ElMessage.error(readError(err, '解析 WebDAV 文件失败'))
  } finally {
    importing.value = false
  }
}

async function confirmPreviewImport({ items: selectedItems, categoryIds }) {
  importing.value = true
  try {
    const { data } = await importFromWebDAV(selectedItems, categoryIds)
    importResults.value = data.imported || []
    const success = importResults.value.filter(item => item.book).length
    const failed = importResults.value.filter(item => item.error).length
    ElMessage.success(`导入 ${success} 本` + (failed ? `，${failed} 本失败` : ''))
    importResultDialog.value = true
    previewDialog.value = false
    importResults.value.forEach(item => {
      if (item.book) bookshelf.upsertBook(item.book)
    })
    emit('imported', importResults.value)
    await load()
  } catch (err) {
    ElMessage.error(readError(err, '导入 WebDAV 文件失败'))
  } finally {
    importing.value = false
  }
}

function parseWebDAVListing(xml) {
  const doc = new DOMParser().parseFromString(xml, 'application/xml')
  return [...doc.querySelectorAll('prop')].map((node) => ({
    name: node.querySelector('displayname')?.textContent || '',
    isDir: node.querySelector('iscollection')?.textContent === 'true',
    size: Number(node.querySelector('getcontentlength')?.textContent || 0),
    lastModified: node.querySelector('lastmodified')?.textContent || '',
  })).filter(item => item.name && item.name !== path.value).map(item => ({
    ...item,
    importable: !item.isDir && isImportableBookFile(item.name),
  }))
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
  const year = String(date.getFullYear()).slice(2)
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hour = String(date.getHours()).padStart(2, '0')
  const minute = String(date.getMinutes()).padStart(2, '0')
  return `${year}-${month}-${day} ${hour}:${minute}`
}

function isBackupFile(row) {
  return String(row.name || '').toLowerCase().endsWith('.zip')
}

function isImportableBookFile(name) {
  return /\.(txt|text|md|epub|pdf|umd|cbz)$/i.test(name || '')
}

function joinPath(base, name) {
  return [base, name].filter(Boolean).join('/')
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
  gap: 12px;
}

.webdav-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.webdav-current-path,
.mobile-file-card p,
.result-row small {
  color: var(--app-text-muted);
  font-size: 12px;
}

.webdav-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.webdav-category-select {
  width: 140px;
}

.webdav-breadcrumb button,
.file-name,
.mobile-file-name {
  display: inline-flex;
  align-items: center;
  min-width: 0;
  gap: 6px;
  padding: 0;
  color: var(--app-text);
  background: transparent;
  border: 0;
  cursor: pointer;
}

.webdav-breadcrumb button {
  color: var(--app-primary);
}

.file-name span,
.mobile-file-name span,
.mobile-file-card p,
.result-row span,
.result-row small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.webdav-table {
  width: 100%;
}

.mobile-file-list {
  display: none;
}

.mobile-file-select-actions {
  display: none;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px;
  color: var(--app-text-muted);
  font-weight: 700;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.webdav-batch-footer {
  display: none;
}

.mobile-file-select-actions div {
  display: flex;
  gap: 4px;
}

.mobile-file-card {
  display: grid;
  gap: 8px;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.mobile-file-card header,
.mobile-file-card footer,
.mobile-file-meta {
  display: flex;
  align-items: center;
  gap: 8px;
}

.mobile-file-card header {
  justify-content: space-between;
}

.mobile-file-card p {
  margin: 0;
}

.mobile-file-card footer {
  flex-wrap: wrap;
}

.result-list {
  display: grid;
  gap: 8px;
}

.result-row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  padding: 8px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
  background: var(--app-bg-soft);
}

@media (max-width: 750px) {
  .webdav-head {
    align-items: flex-start;
    display: grid;
  }

  .webdav-actions {
    justify-content: flex-start;
  }

  .webdav-batch-command {
    display: none;
  }

  .webdav-category-select {
    width: 100%;
  }

  .desktop-webdav-table {
    display: none;
  }

  .mobile-file-list {
    display: grid;
    max-height: 48vh;
    overflow: auto;
    gap: 10px;
  }

  .mobile-file-select-actions {
    display: flex;
  }

  .webdav-batch-footer {
    position: sticky;
    z-index: 2;
    bottom: max(10px, env(safe-area-inset-bottom));
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
    padding: 10px;
    background: var(--app-surface);
    border: 1px solid var(--app-border);
    border-radius: var(--app-radius-sm);
    box-shadow: 0 -8px 22px rgba(15, 23, 42, 0.08);
  }

  .webdav-batch-footer .check-tip {
    grid-column: 1 / -1;
    color: var(--app-text-muted);
    font-size: 13px;
  }

  .webdav-batch-footer :deep(.el-button),
  .webdav-batch-footer :deep(.el-upload) {
    width: 100%;
    min-height: 38px;
    margin-left: 0;
  }

  .webdav-batch-footer :deep(.el-upload .el-button) {
    width: 100%;
  }

  .result-row {
    grid-template-columns: 1fr;
  }
}
</style>
