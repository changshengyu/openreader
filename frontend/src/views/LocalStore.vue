<template>
  <section class="app-page store-page" :class="{ 'embedded-store': embedded }">
    <header class="store-head">
      <div v-if="!embedded">
        <h1 class="app-page-title">本地书仓</h1>
      </div>
      <div v-else class="embedded-store-title">
        <strong>文件管理</strong>
        <span>{{ currentPath || 'localStore' }}</span>
      </div>
      <div class="head-actions">
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        <el-button :icon="FolderOpened" @click="createDirectory">新建目录</el-button>
        <el-upload class="store-batch-command" :show-file-list="false" :auto-upload="false" accept=".txt,.text,.md,.epub,.pdf,.umd" @change="uploadFile">
          <el-button :icon="Upload" :loading="uploading">上传</el-button>
        </el-upload>
        <el-button class="store-batch-command" :disabled="!importableCount || importing" :loading="importing" @click="importCurrentDirectory">
          导入当前目录 ({{ importableCount }})
        </el-button>
        <el-button class="store-batch-command" :disabled="!shownImportablePaths.length || importing" :loading="importing" @click="importFiltered">
          导入筛选 ({{ shownImportablePaths.length }})
        </el-button>
        <el-button class="store-batch-command" type="primary" :disabled="!selectedImportablePaths.length || importing" :loading="importing" @click="importSelected">
          导入选中 ({{ selectedImportablePaths.length }})
        </el-button>
      </div>
    </header>

    <section class="store-toolbar app-panel">
      <el-breadcrumb separator="/" class="store-breadcrumb">
        <el-breadcrumb-item>
          <button type="button" @click="goPath('')">localStore</button>
        </el-breadcrumb-item>
        <el-breadcrumb-item v-for="crumb in breadcrumbs" :key="crumb.path">
          <button type="button" @click="goPath(crumb.path)">{{ crumb.name }}</button>
        </el-breadcrumb-item>
      </el-breadcrumb>
      <el-input v-model="keyword" placeholder="搜索文件名" clearable>
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>
      <el-select v-model="extension" placeholder="全部格式" clearable>
        <el-option v-for="ext in extensions" :key="ext" :label="ext" :value="ext" />
      </el-select>
      <el-select v-model="targetCategoryIds" placeholder="导入到分组（可多选）" multiple clearable>
        <el-option v-for="category in bookshelf.categories" :key="category.id" :label="category.name" :value="String(category.id)" />
      </el-select>
      <el-switch v-model="recursiveScan" inline-prompt active-text="子目录" inactive-text="当前层" @change="load" />
    </section>

    <el-table
      :data="shownItems"
      row-key="path"
      stripe
      class="store-table desktop-store-table"
      @selection-change="onSelectionChange"
      @row-dblclick="openRow"
    >
      <el-table-column type="selection" width="42" />
      <el-table-column prop="name" label="文件名" min-width="260" show-overflow-tooltip>
        <template #default="{ row }">
          <button class="file-name" type="button" @click="openRow(row)">
            <el-icon><component :is="row.isDir ? FolderOpened : Document" /></el-icon>
            <span>{{ row.name }}</span>
          </button>
        </template>
      </el-table-column>
      <el-table-column prop="extension" label="格式" width="90" />
      <el-table-column label="大小" width="120">
        <template #default="{ row }">{{ row.isDir ? '-' : formatSize(row.size) }}</template>
      </el-table-column>
      <el-table-column prop="path" label="路径" min-width="260" show-overflow-tooltip />
      <el-table-column label="操作" width="250" fixed="right">
        <template #default="{ row }">
          <el-button v-if="row.importable" size="small" text type="primary" @click="importOne(row)">导入</el-button>
          <el-button v-else-if="row.isDir" size="small" text type="primary" @click="importDirectory(row)">导入目录</el-button>
          <el-button v-if="!row.isDir" size="small" text @click="downloadItem(row)">下载</el-button>
          <el-button size="small" text @click="renameItem(row)">重命名</el-button>
          <el-button size="small" text type="danger" @click="deleteItem(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div v-if="shownItems.length" class="mobile-file-select-actions app-panel">
      <span>已选 {{ selectedRows.length }} 个</span>
      <div>
        <el-button size="small" text @click="selectShownFiles">全选当前</el-button>
        <el-button size="small" text @click="clearSelection">清空</el-button>
      </div>
    </div>

    <div v-if="shownItems.length" class="mobile-file-list">
      <article v-for="row in shownItems" :key="row.path" class="mobile-file-card app-panel">
        <header>
          <button class="mobile-file-name" type="button" @click="openRow(row)">
            <el-icon><component :is="row.isDir ? FolderOpened : Document" /></el-icon>
            <span>{{ row.name }}</span>
          </button>
          <el-checkbox
            :model-value="isSelected(row)"
            @change="value => toggleSelectedRow(row, value)"
          />
        </header>
        <p>{{ row.path }}</p>
        <div class="mobile-file-meta">
          <el-tag size="small" effect="plain">{{ row.isDir ? '目录' : row.extension || '文件' }}</el-tag>
          <el-tag v-if="!row.isDir" size="small" effect="plain">{{ formatSize(row.size) }}</el-tag>
          <el-tag v-if="row.importable" size="small" type="success" effect="plain">可导入</el-tag>
        </div>
        <footer>
          <el-button v-if="row.importable" size="small" text type="primary" @click="importOne(row)">导入</el-button>
          <el-button v-else-if="row.isDir" size="small" text type="primary" @click="importDirectory(row)">导入目录</el-button>
          <el-button v-if="!row.isDir" size="small" text @click="downloadItem(row)">下载</el-button>
          <el-button size="small" text @click="renameItem(row)">重命名</el-button>
          <el-button size="small" text type="danger" @click="deleteItem(row)">删除</el-button>
        </footer>
      </article>
    </div>

    <el-empty v-if="!items.length && !loading" description="书仓为空，把文件放入 localStore 目录即可显示" />

    <div v-if="items.length" class="store-batch-footer app-panel">
      <span class="check-tip">已选择 {{ selectedRows.length }} 个</span>
      <el-button type="primary" plain :disabled="!selectedRows.length" @click="deleteSelected">批量删除</el-button>
      <el-button type="primary" :disabled="!selectedImportablePaths.length || importing" :loading="importing" @click="importSelected">
        批量加入书架 {{ selectedImportablePaths.length || '' }}
      </el-button>
      <el-upload :show-file-list="false" :auto-upload="false" accept=".txt,.text,.md,.epub,.pdf,.umd" @change="uploadFile">
        <el-button :loading="uploading">上传书籍</el-button>
      </el-upload>
      <el-button @click="clearSelection">取消</el-button>
    </div>

    <LocalBookImportPreviewDialog
      v-model="previewDialog"
      title="本地书仓导入预览"
      :items="previewItems"
      :categories="bookshelf.categories"
      :category-ids="targetCategoryIds"
      :loading="importing"
      @confirm="confirmPreviewImport"
    />

    <el-dialog v-model="resultDialog" title="导入结果" width="560px" :fullscreen="isMobileDialog">
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
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Document, FolderOpened, Refresh, Search, Upload } from '@element-plus/icons-vue'
import { createLocalStoreDirectory, deleteFromLocalStore, downloadFromLocalStore, importFromLocalStore, listLocalStore, previewLocalStoreImport, renameLocalStoreItem, uploadToLocalStore } from '../api/localStore'
import LocalBookImportPreviewDialog from '../components/LocalBookImportPreviewDialog.vue'
import { useBookshelfStore } from '../stores/bookshelf'
import { useReaderStore } from '../stores/reader'
import { currentViewportWidth, shouldUseMiniInterface } from '../utils/responsive'

defineProps({
  embedded: {
    type: Boolean,
    default: false,
  },
})

const bookshelf = useBookshelfStore()
const reader = useReaderStore()
const items = ref([])
const checkedRows = ref([])
const selectedRows = ref([])
const currentPath = ref('')
const keyword = ref('')
const extension = ref('')
const targetCategoryIds = ref([])
const recursiveScan = ref(true)
const loading = ref(false)
const importing = ref(false)
const uploading = ref(false)
const previewDialog = ref(false)
const previewItems = ref([])
const resultDialog = ref(false)
const importResults = ref([])
const windowWidth = ref(currentViewportWidth())

const extensions = computed(() => [...new Set(items.value.filter(item => item.importable).map(item => item.extension).filter(Boolean))].sort())
const importableCount = computed(() => items.value.filter(item => item.importable).length)
const breadcrumbs = computed(() => {
  if (!currentPath.value) return []
  const parts = currentPath.value.split(/[\\/]/).filter(Boolean)
  return parts.map((name, index) => ({ name, path: parts.slice(0, index + 1).join('/') }))
})
const shownItems = computed(() => {
  const value = keyword.value.trim().toLowerCase()
  return items.value.filter(item => {
    if (extension.value && !item.isDir && item.extension !== extension.value) return false
    if (extension.value && item.isDir) return true
    if (!value) return true
    return `${item.name || ''} ${item.path || ''}`.toLowerCase().includes(value)
  })
})
const shownImportablePaths = computed(() => shownItems.value.filter(item => item.importable).map(item => item.path))
const selectedImportablePaths = computed(() => selectedRows.value.filter(item => item.importable).map(item => item.path))
const isMobileDialog = computed(() => shouldUseMiniInterface(reader.pageMode, windowWidth.value))

onMounted(async () => {
  window.addEventListener('resize', handleResize)
  const [, categoriesResult] = await Promise.allSettled([load(), warmLocalStoreCategories()])
  if (categoriesResult.status === 'rejected') {
    ElMessage.warning(readError(categoriesResult.reason, '加载分组失败，导入时可能暂时无法选择分组'))
  }
})

onBeforeUnmount(() => window.removeEventListener('resize', handleResize))

function handleResize() {
  windowWidth.value = currentViewportWidth()
}

async function warmLocalStoreCategories() {
  return bookshelf.ensureCategoriesLoaded()
}

async function load() {
  loading.value = true
  try {
    const { data } = await listLocalStore(currentPath.value, recursiveScan.value)
    currentPath.value = data.path || ''
    items.value = data.items || []
    clearSelection()
  } catch (err) {
    ElMessage.error(readError(err, '加载书仓失败'))
  } finally {
    loading.value = false
  }
}

function formatSize(bytes) {
  if (!bytes) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

async function goPath(path) {
  currentPath.value = path
  await load()
}

function openRow(row) {
  if (row.isDir) {
    goPath(row.path)
  }
}

function onSelectionChange(rows) {
  selectedRows.value = rows
  checkedRows.value = rows.filter(row => row.importable).map(row => row.path)
}

function isSelected(row) {
  return selectedRows.value.some(item => item.path === row.path)
}

function toggleSelectedRow(row, checked) {
  if (checked) {
    if (!isSelected(row)) selectedRows.value.push(row)
    checkedRows.value = selectedImportablePaths.value
    return
  }
  selectedRows.value = selectedRows.value.filter(item => item.path !== row.path)
  checkedRows.value = selectedImportablePaths.value
}

function selectShownFiles() {
  selectedRows.value = [...shownItems.value]
  checkedRows.value = selectedImportablePaths.value
}

function clearSelection() {
  selectedRows.value = []
  checkedRows.value = []
}

async function uploadFile(data) {
  const file = data.raw
  if (!file) return
  uploading.value = true
  try {
    await uploadToLocalStore({ path: currentPath.value, file })
    ElMessage.success('文件已上传')
    await load()
  } catch (err) {
    ElMessage.error(readError(err, '上传失败'))
  } finally {
    uploading.value = false
  }
}

async function createDirectory() {
  try {
    const { value } = await ElMessageBox.prompt('输入目录名称', '新建目录', {
      inputValidator: value => !!value?.trim() || '目录名称不能为空',
    })
    await createLocalStoreDirectory({ path: currentPath.value, name: value.trim() })
    ElMessage.success('目录已创建')
    await load()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '创建目录失败'))
  }
}

async function importSelected() {
  if (!selectedImportablePaths.value.length) return
  importing.value = true
  try {
    await importPaths(selectedImportablePaths.value)
  } catch (err) {
    ElMessage.error(readError(err, '导入失败'))
  } finally {
    importing.value = false
  }
}

async function importCurrentDirectory() {
  if (!importableCount.value) return
  try {
    const label = currentPath.value || 'localStore'
    await ElMessageBox.confirm(`将递归导入“${label}”下的 ${importableCount.value} 个可导入文件，是否继续？`, '导入当前目录', { type: 'info' })
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    throw err
  }
  importing.value = true
  try {
    await importPaths([currentPath.value])
  } catch (err) {
    ElMessage.error(readError(err, '导入目录失败'))
  } finally {
    importing.value = false
  }
}

async function importFiltered() {
  if (!shownImportablePaths.value.length) return
  importing.value = true
  try {
    await importPaths(shownImportablePaths.value)
  } catch (err) {
    ElMessage.error(readError(err, '导入失败'))
  } finally {
    importing.value = false
  }
}

async function importOne(row) {
  if (!row.importable) return
  importing.value = true
  try {
    await importPaths([row.path])
  } catch (err) {
    ElMessage.error(readError(err, '导入失败'))
  } finally {
    importing.value = false
  }
}

async function importDirectory(row) {
  if (!row.isDir) return
  importing.value = true
  try {
    await importPaths([row.path])
  } catch (err) {
    ElMessage.error(readError(err, '导入目录失败'))
  } finally {
    importing.value = false
  }
}

async function downloadItem(row) {
  if (row.isDir) return
  try {
    const resp = await downloadFromLocalStore(row.path)
    downloadBlob(resp.data, row.name)
  } catch (err) {
    ElMessage.error(readError(err, '下载失败'))
  }
}

async function importPaths(paths) {
  const { data } = await previewLocalStoreImport(paths)
  previewItems.value = data.items || []
  if (!previewItems.value.some(item => item.book)) {
    ElMessage.warning('没有可导入的书籍')
    return
  }
  previewDialog.value = true
}

async function confirmPreviewImport({ items: selectedItems, categoryIds }) {
  importing.value = true
  try {
    const { data } = await importFromLocalStore(selectedItems, categoryIds)
    applyImportResults(data)
    previewDialog.value = false
    clearSelection()
    await load()
  } catch (err) {
    ElMessage.error(readError(err, '导入失败'))
  } finally {
    importing.value = false
  }
}

function applyImportResults(data) {
  importResults.value = data.imported || []
  importResults.value.forEach(item => {
    if (item.book) bookshelf.upsertBook(item.book)
  })
  const success = importResults.value.filter(item => item.book).length
  const failed = importResults.value.filter(item => item.error).length
  ElMessage.success(`导入 ${success} 本` + (failed ? `，${failed} 本失败` : ''))
  resultDialog.value = true
}

function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  URL.revokeObjectURL(url)
}

async function renameItem(row) {
  try {
    const { value } = await ElMessageBox.prompt('输入新的名称', '重命名', {
      inputValue: row.name,
      inputValidator: value => !!value?.trim() || '名称不能为空',
    })
    const nextName = value.trim()
    if (!nextName || nextName === row.name) return
    await renameLocalStoreItem({ path: row.path, name: nextName })
    ElMessage.success('已重命名')
    await load()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '重命名失败'))
  }
}

async function deleteItem(row) {
  try {
    await ElMessageBox.confirm(`确定删除“${row.name}”吗？`, '删除书仓项目', { type: 'warning' })
    await deleteFromLocalStore(row.path)
    ElMessage.success('已删除')
    await load()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除失败'))
  }
}

async function deleteSelected() {
  if (!selectedRows.value.length) return
  try {
    await ElMessageBox.confirm(`确定删除选中的 ${selectedRows.value.length} 个书仓项目吗？`, '批量删除书仓项目', { type: 'warning' })
    for (const row of selectedRows.value) {
      await deleteFromLocalStore(row.path)
    }
    ElMessage.success('已批量删除')
    await load()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '批量删除失败'))
  }
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.store-page {
  display: grid;
  min-width: 0;
  gap: 16px;
}

.store-page.embedded-store {
  width: 100%;
  max-width: none;
  margin: 0;
  padding: 0;
  overflow: visible;
}

.store-head,
.head-actions,
.store-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
}

.store-head {
  justify-content: space-between;
}

.embedded-store-title {
  display: grid;
  min-width: 0;
  gap: 4px;
}

.embedded-store-title strong {
  color: var(--app-text);
  font-size: 16px;
}

.embedded-store-title span {
  overflow: hidden;
  color: var(--app-text-muted);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.head-actions {
  flex-wrap: wrap;
  justify-content: flex-end;
}

.result-row small {
  color: var(--app-text-muted);
}

.store-toolbar {
  display: grid;
  min-width: 0;
  grid-template-columns: minmax(220px, 1fr) 160px 210px auto;
  align-items: center;
  padding: 12px;
}

.store-breadcrumb {
  grid-column: 1 / -1;
}

.store-breadcrumb button {
  padding: 0;
  color: var(--app-primary);
  background: transparent;
  border: 0;
  cursor: pointer;
}

.store-toolbar .el-input {
  min-width: 0;
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

.mobile-file-list {
  display: none;
}

.mobile-file-select-actions {
  display: none;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 10px 12px;
  color: var(--app-text-muted);
  font-weight: 700;
}

.store-batch-footer {
  display: none;
}

.mobile-file-select-actions div {
  display: flex;
  gap: 4px;
}

.mobile-file-card {
  display: grid;
  gap: 9px;
  padding: 12px;
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

.mobile-file-name {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 8px;
  padding: 0;
  color: var(--app-text);
  background: transparent;
  border: 0;
  cursor: pointer;
  font-weight: 700;
  text-align: left;
}

.mobile-file-name span,
.mobile-file-card p {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-file-card p {
  margin: 0;
  color: var(--app-text-muted);
  font-size: 12px;
}

.mobile-file-card footer {
  justify-content: flex-end;
}

.result-list {
  display: grid;
  gap: 8px;
}

.result-row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 6px 10px;
  align-items: center;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.result-row small {
  grid-column: 2;
}

@media (max-width: 750px) {
  .store-head,
  .store-toolbar {
    display: grid;
    grid-template-columns: 1fr;
  }

  .head-actions,
  .store-toolbar :deep(.el-input),
  .store-toolbar :deep(.el-select),
  .store-toolbar :deep(.el-button) {
    width: 100%;
  }

  .store-batch-command {
    display: none;
  }

  .desktop-store-table {
    display: none;
  }

  .mobile-file-list {
    display: grid;
    gap: 10px;
  }

  .mobile-file-select-actions {
    display: flex;
  }

  .store-batch-footer {
    position: sticky;
    z-index: 2;
    bottom: max(10px, env(safe-area-inset-bottom));
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
    padding: 10px;
    box-shadow: 0 -8px 22px rgba(15, 23, 42, 0.08);
  }

  .store-batch-footer .check-tip {
    grid-column: 1 / -1;
    color: var(--app-text-muted);
    font-size: 13px;
  }

  .store-batch-footer :deep(.el-button),
  .store-batch-footer :deep(.el-upload) {
    width: 100%;
    min-height: 38px;
    margin-left: 0;
  }

  .store-batch-footer :deep(.el-upload .el-button) {
    width: 100%;
  }
}
</style>
