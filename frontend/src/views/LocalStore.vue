<template>
  <section class="app-page store-page">
    <header class="store-head">
      <div>
        <p class="eyebrow">Local Store</p>
        <h1 class="app-page-title">本地书仓</h1>
        <p class="app-page-subtitle">扫描服务端 localStore 目录，批量导入 TXT / EPUB / PDF / UMD 等本地文件。</p>
      </div>
      <div class="head-actions">
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        <el-button :icon="FolderOpened" @click="createDirectory">新建目录</el-button>
        <el-upload :show-file-list="false" :auto-upload="false" accept=".txt,.text,.md,.epub,.pdf,.umd" @change="uploadFile">
          <el-button :icon="Upload" :loading="uploading">上传</el-button>
        </el-upload>
        <el-button type="primary" :disabled="!checkedRows.length || importing" :loading="importing" @click="importSelected">
          导入选中 ({{ checkedRows.length }})
        </el-button>
      </div>
    </header>

    <section class="store-summary">
      <article class="app-panel stat"><span>文件</span><strong>{{ items.length }}</strong></article>
      <article class="app-panel stat"><span>已选</span><strong>{{ checkedRows.length }}</strong></article>
      <article class="app-panel stat"><span>总大小</span><strong>{{ formatSize(totalSize) }}</strong></article>
      <article class="app-panel stat"><span>格式</span><strong>{{ extensions.length }}</strong></article>
    </section>

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
      <el-select v-model="targetCategoryId" placeholder="导入到分组（可选）" clearable>
        <el-option label="未分组" value="" />
        <el-option v-for="category in bookshelf.categories" :key="category.id" :label="category.name" :value="String(category.id)" />
      </el-select>
    </section>

    <el-alert type="success" :closable="false" show-icon title="已支持目录浏览、新建目录、上传、重命名、删除和批量导入。" />

    <el-table
      :data="shownItems"
      row-key="path"
      stripe
      class="store-table desktop-store-table"
      @selection-change="checkedRows = $event.filter(row => row.importable).map(row => row.path)"
      @row-dblclick="openRow"
    >
      <el-table-column type="selection" width="42" :selectable="row => row.importable" />
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
      <el-table-column label="操作" width="210" fixed="right">
        <template #default="{ row }">
          <el-button v-if="row.importable" size="small" text type="primary" @click="importOne(row)">导入</el-button>
          <el-button size="small" text @click="renameItem(row)">重命名</el-button>
          <el-button size="small" text type="danger" @click="deleteItem(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <div v-if="shownItems.length" class="mobile-file-list">
      <article v-for="row in shownItems" :key="row.path" class="mobile-file-card app-panel">
        <header>
          <button class="mobile-file-name" type="button" @click="openRow(row)">
            <el-icon><component :is="row.isDir ? FolderOpened : Document" /></el-icon>
            <span>{{ row.name }}</span>
          </button>
          <el-checkbox
            v-if="row.importable"
            :model-value="checkedRows.includes(row.path)"
            @change="value => toggleCheckedPath(row.path, value)"
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
          <el-button size="small" text @click="renameItem(row)">重命名</el-button>
          <el-button size="small" text type="danger" @click="deleteItem(row)">删除</el-button>
        </footer>
      </article>
    </div>

    <el-empty v-if="!items.length && !loading" description="书仓为空，把文件放入 localStore 目录即可显示" />

    <el-dialog v-model="resultDialog" title="导入结果" width="560px">
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
import { Document, FolderOpened, Refresh, Search, Upload } from '@element-plus/icons-vue'
import { createLocalStoreDirectory, deleteFromLocalStore, importFromLocalStore, listLocalStore, renameLocalStoreItem, uploadToLocalStore } from '../api/localStore'
import { useBookshelfStore } from '../stores/bookshelf'

const bookshelf = useBookshelfStore()
const items = ref([])
const checkedRows = ref([])
const currentPath = ref('')
const keyword = ref('')
const extension = ref('')
const targetCategoryId = ref('')
const loading = ref(false)
const importing = ref(false)
const uploading = ref(false)
const resultDialog = ref(false)
const importResults = ref([])

const totalSize = computed(() => items.value.filter(item => !item.isDir).reduce((sum, item) => sum + (item.size || 0), 0))
const extensions = computed(() => [...new Set(items.value.filter(item => item.importable).map(item => item.extension).filter(Boolean))].sort())
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

onMounted(async () => {
  await Promise.all([load(), bookshelf.loadCategories()])
})

async function load() {
  loading.value = true
  try {
    const { data } = await listLocalStore(currentPath.value)
    currentPath.value = data.path || ''
    items.value = data.items || []
    checkedRows.value = []
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

function toggleCheckedPath(path, checked) {
  if (checked) {
    if (!checkedRows.value.includes(path)) checkedRows.value.push(path)
    return
  }
  checkedRows.value = checkedRows.value.filter(item => item !== path)
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
  if (!checkedRows.value.length) return
  importing.value = true
  try {
    await importPaths(checkedRows.value)
    checkedRows.value = []
    await load()
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
    await load()
  } catch (err) {
    ElMessage.error(readError(err, '导入失败'))
  } finally {
    importing.value = false
  }
}

async function importPaths(paths) {
  const categoryId = targetCategoryId.value ? Number(targetCategoryId.value) : null
  const { data } = await importFromLocalStore(paths, categoryId)
  importResults.value = data.imported || []
  const success = importResults.value.filter(item => item.book).length
  const failed = importResults.value.filter(item => item.error).length
  ElMessage.success(`导入 ${success} 本` + (failed ? `，${failed} 本失败` : ''))
  resultDialog.value = true
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

.head-actions {
  flex-wrap: wrap;
  justify-content: flex-end;
}

.eyebrow {
  margin: 0 0 4px;
  color: var(--app-primary);
  font-size: 12px;
  font-weight: 800;
  text-transform: uppercase;
}

.store-summary {
  display: grid;
  min-width: 0;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

.stat {
  display: grid;
  gap: 6px;
  padding: 16px;
}

.stat span,
.result-row small {
  color: var(--app-text-muted);
}

.stat strong {
  font-size: 24px;
}

.store-toolbar {
  display: grid;
  min-width: 0;
  grid-template-columns: minmax(220px, 1fr) 180px 210px;
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

@media (max-width: 760px) {
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

  .store-summary {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .desktop-store-table {
    display: none;
  }

  .mobile-file-list {
    display: grid;
    gap: 10px;
  }
}
</style>
