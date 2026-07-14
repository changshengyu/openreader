<template>
  <section class="app-page store-page" :class="{ 'embedded-store': embedded }">
    <header v-if="!embedded" class="store-head">
      <h1 class="app-page-title">本地书仓</h1>
    </header>

    <el-table
      v-loading="loading"
      :data="tableItems"
      row-key="path"
      stripe
      class="store-table"
      :selectable="row => !row.toParent && !row.loadMore"
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
      <el-table-column label="大小" width="118">
        <template #default="{ row }">{{ row.isDir || row.toParent || row.loadMore ? '-' : formatSize(row.size) }}</template>
      </el-table-column>
      <el-table-column label="修改时间" width="156">
        <template #default="{ row }">{{ formatDate(row.lastModified) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="170" fixed="right">
        <template #header>
          <div class="operation-header">
            <span>操作</span>
            <el-input v-model="keyword" size="small" clearable placeholder="输入关键字搜索" />
          </div>
        </template>
        <template #default="{ row }">
          <el-button
            v-if="canImport(row)"
            text
            type="primary"
            :loading="storageImportPending"
            @click="importOne(row)"
          >
            加入书架
          </el-button>
          <el-button
            v-if="!row.toParent && !row.loadMore"
            text
            type="danger"
            @click="deleteItem(row)"
          >
            删除
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-empty v-if="!items.length && !loading" description="书仓为空，把文件放入 localStore 目录即可显示" />

    <footer v-if="items.length" class="store-batch-footer app-panel">
      <span class="check-tip">已选择 {{ selectedRows.length }} 个</span>
      <el-button type="primary" plain :disabled="!selectedRows.length" @click="deleteSelected">批量删除</el-button>
      <el-button
        type="primary"
        :disabled="!selectedImportablePaths.length || storageImportPending"
        :loading="storageImportPending"
        @click="importSelected"
      >
        批量加入书架 {{ selectedImportablePaths.length || '' }}
      </el-button>
      <input ref="uploadInput" class="hidden-upload" type="file" multiple @change="uploadFiles">
      <el-button :loading="uploading" @click="chooseFiles">上传书籍</el-button>
      <el-button @click="clearSelection">取消</el-button>
    </footer>
  </section>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Document, FolderOpened } from '@element-plus/icons-vue'
import { deleteFromLocalStore, listLocalStore, uploadToLocalStore } from '../api/localStore'
import { useOverlayStore } from '../stores/overlay'
import { filterLocalStoreItems, visibleLocalStoreItems } from '../utils/localStoreItems'
import { isLocalStoreImportable } from '../utils/storageImportable'

defineProps({
  embedded: {
    type: Boolean,
    default: false,
  },
})

const overlay = useOverlayStore()
const items = ref([])
const selectedRows = ref([])
const currentPath = ref('')
const keyword = ref('')
const loading = ref(false)
const uploading = ref(false)
const showAllItems = ref(false)
const uploadInput = ref(null)

const filteredItems = computed(() => filterLocalStoreItems(items.value, { keyword: keyword.value }))
const visibleItems = computed(() => visibleLocalStoreItems(filteredItems.value, showAllItems.value))
const remainingItemCount = computed(() => Math.max(0, filteredItems.value.length - visibleItems.value.length))
const tableItems = computed(() => {
  const rows = [...visibleItems.value]
  if (currentPath.value) {
    rows.unshift({
      name: '..',
      path: parentPath(currentPath.value),
      isDir: true,
      toParent: true,
    })
  }
  if (remainingItemCount.value > 0) {
    rows.push({
      name: `加载更多 ${remainingItemCount.value} 项`,
      path: '__load-more__',
      loadMore: true,
    })
  }
  return rows
})
const selectedImportablePaths = computed(() => selectedRows.value.filter(canImport).map(item => item.path))
const storageImportPending = computed(() => overlay.storageImportVisible)

watch(keyword, () => {
  showAllItems.value = false
})

onMounted(load)

async function load() {
  loading.value = true
  try {
    const { data } = await listLocalStore(currentPath.value)
    currentPath.value = data.path || ''
    items.value = data.items || []
    showAllItems.value = false
    clearSelection()
  } catch (err) {
    ElMessage.error(readError(err, '加载书仓失败'))
  } finally {
    loading.value = false
  }
}

function canImport(row) {
  return !row?.isDir && !row?.toParent && !row?.loadMore && isLocalStoreImportable(row.path || row.name)
}

function onSelectionChange(rows) {
  selectedRows.value = rows.filter(row => !row.toParent && !row.loadMore)
}

async function openRow(row) {
  if (row.loadMore) {
    showMoreItems()
    return
  }
  if (row.isDir) await goPath(row.path)
}

async function goPath(path) {
  currentPath.value = path
  await load()
}

function showMoreItems() {
  showAllItems.value = true
}

function clearSelection() {
  selectedRows.value = []
}

function chooseFiles() {
  uploadInput.value?.click()
}

async function uploadFiles(event) {
  const files = Array.from(event.target?.files || [])
  if (!files.length) return
  uploading.value = true
  try {
    await uploadToLocalStore({ path: currentPath.value, files })
    ElMessage.success('文件已上传')
    await load()
  } catch (err) {
    ElMessage.error(readError(err, '上传失败'))
  } finally {
    uploading.value = false
    if (event.target) event.target.value = ''
  }
}

function importOne(row) {
  if (canImport(row)) importPaths([row.path])
}

function importSelected() {
  if (selectedImportablePaths.value.length) importPaths(selectedImportablePaths.value)
}

function importPaths(paths) {
  overlay.openStorageImport('local-store', paths)
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
    for (const row of selectedRows.value) await deleteFromLocalStore(row.path)
    ElMessage.success('已批量删除')
    await load()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '批量删除失败'))
  }
}

function parentPath(path) {
  const parts = String(path || '').split('/').filter(Boolean)
  parts.pop()
  return parts.join('/')
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

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.store-page {
  display: grid;
  min-width: 0;
  gap: 14px;
}

.store-page.embedded-store {
  width: 100%;
  max-width: none;
  margin: 0;
  padding: 0;
}

.store-head {
  display: flex;
  align-items: center;
}

.store-table {
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

.operation-header {
  display: grid;
  gap: 6px;
}

.store-batch-footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
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
  .store-table {
    overflow-x: auto;
  }

  .store-batch-footer {
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

  .store-batch-footer :deep(.el-button) {
    width: 100%;
    min-height: 38px;
    margin-left: 0;
  }
}
</style>
