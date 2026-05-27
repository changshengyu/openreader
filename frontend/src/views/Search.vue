<template>
  <section class="app-page search-page">
    <header class="search-head">
      <div>
        <p class="eyebrow">Search</p>
        <h1 class="app-page-title">{{ searchMode === 'local' ? '搜索本地书籍' : '搜索书源书籍' }}</h1>
        <p class="app-page-subtitle">{{ searchMode === 'local' ? '搜索服务端 localStore 里的本地书文件，和远程书源搜索分开处理。' : '按上游阅读器的逻辑，选择书源范围后并发搜索并加入书架。' }}</p>
      </div>
      <el-button :icon="searchMode === 'local' ? FolderOpened : Connection" @click="searchMode === 'local' ? router.push({ name: 'local-store' }) : router.push({ name: 'sources' })">
        {{ searchMode === 'local' ? '本地书仓' : '书源管理' }}
      </el-button>
    </header>

    <section class="search-console app-panel">
      <el-radio-group v-model="searchMode" size="large" class="mode-switch" @change="switchSearchMode">
        <el-radio-button value="remote">书源搜索</el-radio-button>
        <el-radio-button value="local">本地书籍</el-radio-button>
      </el-radio-group>

      <el-input v-model="keyword" :placeholder="searchMode === 'local' ? '输入本地文件名或路径，留空显示全部可导入文件' : '输入书名或作者'" size="large" clearable @keyup.enter="doSearch">
        <template #prefix><el-icon><SearchIcon /></el-icon></template>
      </el-input>
      <el-button type="primary" size="large" :loading="searching" @click="doSearch">搜索</el-button>

      <div v-if="searchMode === 'remote'" class="search-options">
        <el-radio-group v-model="searchType" size="small" @change="syncSelection">
          <el-radio-button value="all">全部书源</el-radio-button>
          <el-radio-button value="group">按分组</el-radio-button>
          <el-radio-button value="single">单个书源</el-radio-button>
          <el-radio-button value="custom">自选</el-radio-button>
        </el-radio-group>

        <el-select v-if="searchType === 'group'" v-model="selectedGroup" placeholder="选择分组" size="small" @change="syncSelection">
          <el-option v-for="group in sourceGroups" :key="group.value" :label="`${group.label} (${group.count})`" :value="group.value" />
        </el-select>

        <el-select v-if="searchType === 'single'" v-model="singleSourceId" placeholder="选择书源" filterable size="small" @change="syncSelection">
          <el-option v-for="source in enabledSources" :key="source.id" :label="source.name" :value="source.id" />
        </el-select>

        <el-select v-model="targetCategoryId" placeholder="加入书架分组（可选）" clearable size="small">
          <el-option label="未分组" value="" />
          <el-option v-for="category in bookshelf.categories" :key="category.id" :label="category.name" :value="String(category.id)" />
        </el-select>
      </div>

      <div v-else class="search-options local-search-options">
        <el-select v-model="targetCategoryId" placeholder="导入到书架分组（可选）" clearable size="small">
          <el-option label="未分组" value="" />
          <el-option v-for="category in bookshelf.categories" :key="category.id" :label="category.name" :value="String(category.id)" />
        </el-select>
        <el-switch v-model="localRecursiveScan" inline-prompt active-text="子目录" inactive-text="当前层" />
        <el-button size="small" :disabled="!checkedLocalPaths.length || importingLocal" :loading="importingLocal" @click="importSelectedLocal">
          导入选中 {{ checkedLocalPaths.length }}
        </el-button>
        <el-button size="small" :disabled="!shownLocalImportablePaths.length || importingLocal" :loading="importingLocal" @click="importShownLocal">
          导入命中 {{ shownLocalImportablePaths.length }}
        </el-button>
      </div>

      <el-collapse v-if="searchMode === 'remote' && searchType === 'custom'" class="source-collapse">
        <el-collapse-item :title="`自选书源（${selectedIds.length}/${enabledSources.length}）`">
          <el-checkbox :model-value="allSelected" @change="toggleAll">全选</el-checkbox>
          <el-checkbox-group v-model="selectedIds" class="source-checks">
            <el-checkbox v-for="source in enabledSources" :key="source.id" :value="source.id" :label="source.name" />
          </el-checkbox-group>
        </el-collapse-item>
      </el-collapse>
    </section>

    <section v-if="searchMode === 'remote'" class="search-status">
      <el-tag effect="plain">启用书源 {{ enabledSources.length }}</el-tag>
      <el-tag effect="plain">本次搜索 {{ selectedIds.length }}</el-tag>
      <el-tag v-if="searched" :type="results.length ? 'success' : 'info'" effect="plain">结果 {{ results.length }}</el-tag>
    </section>
      <section v-else class="search-status">
      <el-tag effect="plain">本地书架 {{ localShelfBooks.length }}</el-tag>
      <el-tag effect="plain">本地书仓 {{ localItems.length }}</el-tag>
      <el-tag effect="plain">可导入文件 {{ localImportableCount }}</el-tag>
      <el-tag effect="plain">已选 {{ checkedLocalPaths.length }}</el-tag>
      <el-tag v-if="searched" :type="shownLocalResults.length ? 'success' : 'info'" effect="plain">命中 {{ shownLocalResults.length }}</el-tag>
    </section>

    <div v-loading="searching" class="result-area">
      <div v-if="searchMode === 'remote' && groupedResults.length" class="source-result-list">
        <section v-for="group in groupedResults" :key="group.sourceId" class="source-result-group">
          <header class="source-result-head">
            <h2>{{ group.sourceName }}</h2>
            <el-tag effect="plain">{{ group.items.length }} 条</el-tag>
          </header>
          <div class="result-list">
            <article v-for="item in group.items" :key="item.bookUrl + item.sourceId" class="result-card app-panel" @click="openPreview(item)">
              <BookCover :book="item" />
              <div class="result-main">
                <div class="result-title">
                  <h3>{{ item.title }}</h3>
                  <el-tag size="small" effect="plain">{{ item.sourceName }}</el-tag>
                </div>
                <p>{{ item.author || '未知作者' }}</p>
                <p v-if="item.latestChapter" class="latest-chapter">{{ item.latestChapter }}</p>
                <p class="result-intro">{{ item.intro || '暂无简介' }}</p>
              </div>
              <div class="result-actions" @click.stop>
                <el-button type="primary" size="small" @click="openPreview(item)">查看信息</el-button>
              </div>
            </article>
          </div>
        </section>
      </div>

      <div v-else-if="searchMode === 'local' && shownLocalResults.length" class="local-result-list">
        <article
          v-for="item in shownLocalResults"
          :key="localResultKey(item)"
          class="local-result-card app-panel"
          :class="{ selected: item.importable && checkedLocalPaths.includes(item.path) }"
        >
          <el-checkbox
            v-if="item.importable"
            :model-value="checkedLocalPaths.includes(item.path)"
            @change="value => toggleLocalPath(item.path, value)"
          />
          <span v-else class="local-result-spacer" />
          <el-icon class="local-file-icon"><Document /></el-icon>
          <div class="result-main">
            <div class="result-title">
              <h3>{{ localBookTitle(item) }}</h3>
              <el-tag size="small" :type="item.book ? 'success' : 'info'" effect="plain">{{ item.book ? '已在书架' : (item.extension || '文件') }}</el-tag>
            </div>
            <p>{{ localBookSubline(item) }}</p>
            <p class="latest-chapter">{{ localBookMeta(item) }}</p>
          </div>
          <div class="result-actions" @click.stop>
            <template v-if="item.book">
              <el-button type="primary" size="small" @click="readLocalShelfBook(item.book)">阅读</el-button>
              <el-button size="small" @click="openLocalShelfDetail(item.book)">详情</el-button>
            </template>
            <el-button v-else type="primary" size="small" :loading="importingLocal" @click="importLocalOne(item)">导入书架</el-button>
          </div>
        </article>
      </div>

      <el-empty v-else-if="searched && !searching" :description="searchMode === 'local' ? '没有找到本地书籍文件' : '没有找到相关书籍'" />
      <el-empty v-else :description="searchMode === 'local' ? '输入关键词搜索本地书仓，或直接搜索显示全部可导入文件' : '输入关键词后开始搜索书源'" />
    </div>

  </section>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Connection, Document, FolderOpened, Search as SearchIcon } from '@element-plus/icons-vue'
import { createRemoteBook } from '../api/books'
import { importFromLocalStore, listLocalStore } from '../api/localStore'
import api from '../api/client'
import BookCover from '../components/BookCover.vue'
import { useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { readerRouteQueryFromBook } from '../utils/readerRoute'

const route = useRoute()
const router = useRouter()
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()

const keyword = ref('')
const searchMode = ref(route.query.mode === 'local' ? 'local' : 'remote')
const sources = ref([])
const selectedIds = ref([])
const selectedGroup = ref('')
const singleSourceId = ref(null)
const targetCategoryId = ref('')
const searchType = ref('all')
const results = ref([])
const searching = ref(false)
const searched = ref(false)
const addingBook = ref(null)
const localItems = ref([])
const checkedLocalPaths = ref([])
const localRecursiveScan = ref(true)
const importingLocal = ref(false)

const enabledSources = computed(() => sources.value.filter(source => source.enabled))
const allSelected = computed(() => enabledSources.value.length > 0 && selectedIds.value.length === enabledSources.value.length)
const groupedResults = computed(() => {
  const groups = new Map()
  for (const item of results.value) {
    const key = item.sourceId || item.sourceName || 'unknown'
    if (!groups.has(key)) {
      groups.set(key, {
        sourceId: key,
        sourceName: item.sourceName || '未知书源',
        items: [],
      })
    }
    groups.get(key).items.push(item)
  }
  return [...groups.values()]
})

const sourceGroups = computed(() => {
  const groups = new Map()
  for (const source of enabledSources.value) {
    const name = source.group || '默认分组'
    groups.set(name, (groups.get(name) || 0) + 1)
  }
  return [...groups.entries()].map(([label, count]) => ({ label, value: label, count }))
})

const localImportableCount = computed(() => localItems.value.filter(item => item.importable).length)
const localShelfBooks = computed(() => (bookshelf.books || []).filter(book => Number(book.sourceId || 0) === 0))
const shownLocalResults = computed(() => {
  if (!searched.value || searchMode.value !== 'local') return []
  const value = normalizeLocalSearch(keyword.value)
  const shelfResults = localShelfBooks.value
    .filter(book => !value || localShelfSearchText(book).includes(value))
    .map(book => ({
      type: 'shelf',
      book,
      name: book.title,
      path: book.originalFile || book.libraryPath || book.url || '',
      extension: fileExtension(book.originalFile || book.libraryPath || book.title),
      importable: false,
    }))
  const storeResults = localItems.value
    .filter(item => {
      if (!item.importable) return false
      if (!value) return true
      return localFileSearchText(item).includes(value)
    })
    .map(item => ({ ...item, type: 'file' }))
  return [...shelfResults, ...storeResults]
})
const shownLocalImportablePaths = computed(() => shownLocalResults.value.filter(item => item.importable).map(item => item.path))

onMounted(async () => {
  await Promise.all([loadSources(), bookshelf.loadCategories(), bookshelf.loadBooks()])
  keyword.value = route.query.q || ''
  syncSelection()
  if (keyword.value || searchMode.value === 'local') doSearch()
})

watch(searchType, syncSelection)
watch(() => route.query.mode, (mode) => {
  const nextMode = mode === 'local' ? 'local' : 'remote'
  if (nextMode !== searchMode.value) switchSearchMode(nextMode, false)
})

async function loadSources() {
  const { data } = await api.get('/sources')
  sources.value = data
  if (!selectedGroup.value && sourceGroups.value.length) selectedGroup.value = sourceGroups.value[0].value
  if (!singleSourceId.value && enabledSources.value.length) singleSourceId.value = enabledSources.value[0].id
}

function syncSelection() {
  if (searchType.value === 'all') {
    selectedIds.value = enabledSources.value.map(source => source.id)
  } else if (searchType.value === 'group') {
    selectedIds.value = enabledSources.value
      .filter(source => (source.group || '默认分组') === selectedGroup.value)
      .map(source => source.id)
  } else if (searchType.value === 'single') {
    selectedIds.value = singleSourceId.value ? [singleSourceId.value] : []
  }
}

function toggleAll() {
  selectedIds.value = allSelected.value ? [] : enabledSources.value.map(source => source.id)
}

function switchSearchMode(mode, updateRoute = true) {
  searchMode.value = mode
  searched.value = false
  results.value = []
  checkedLocalPaths.value = []
  if (mode === 'remote') syncSelection()
  if (updateRoute) {
    router.replace({
      name: 'search',
      query: {
        ...route.query,
        mode: mode === 'local' ? 'local' : undefined,
      },
    })
  }
}

async function doSearch() {
  if (searchMode.value === 'local') {
    await searchLocalBooks()
    return
  }
  const value = keyword.value.trim()
  if (!value) return
  if (!selectedIds.value.length) {
    ElMessage.warning('请至少选择一个书源')
    return
  }
  searching.value = true
  searched.value = false
  results.value = []
  try {
    const { data } = await api.post('/search', { keyword: value, sourceIds: selectedIds.value })
    results.value = data
    searched.value = true
    ElMessage.success(data.length ? `找到 ${data.length} 条结果` : '没有找到相关书籍')
  } catch (err) {
    ElMessage.error(readError(err, '搜索失败'))
  } finally {
    searching.value = false
  }
}

async function searchLocalBooks() {
  searching.value = true
  searched.value = false
  results.value = []
  checkedLocalPaths.value = []
  try {
    const { data } = await listLocalStore('', localRecursiveScan.value)
    localItems.value = data.items || []
    searched.value = true
    ElMessage.success(shownLocalResults.value.length ? `找到 ${shownLocalResults.value.length} 条本地结果` : '没有找到本地书籍')
  } catch (err) {
    ElMessage.error(readError(err, '搜索本地书仓失败'))
  } finally {
    searching.value = false
  }
}

function toggleLocalPath(path, checked) {
  if (checked) {
    if (!checkedLocalPaths.value.includes(path)) checkedLocalPaths.value.push(path)
    return
  }
  checkedLocalPaths.value = checkedLocalPaths.value.filter(item => item !== path)
}

async function importSelectedLocal() {
  if (!checkedLocalPaths.value.length) return
  await importLocalPaths(checkedLocalPaths.value)
}

async function importShownLocal() {
  if (!shownLocalImportablePaths.value.length) return
  await importLocalPaths(shownLocalImportablePaths.value)
}

async function importLocalOne(item) {
  if (!item?.importable) return
  await importLocalPaths([item.path])
}

async function importLocalPaths(paths) {
  importingLocal.value = true
  try {
    const categoryId = targetCategoryId.value ? Number(targetCategoryId.value) : null
    const { data } = await importFromLocalStore(paths, categoryId)
    const imported = data.imported || []
    imported.forEach(item => {
      if (item.book) bookshelf.upsertBook(item.book)
    })
    checkedLocalPaths.value = checkedLocalPaths.value.filter(path => !paths.includes(path))
    const success = imported.filter(item => item.book).length
    const failed = imported.filter(item => item.error).length
    ElMessage.success(`导入 ${success} 本` + (failed ? `，${failed} 本失败` : ''))
  } catch (err) {
    ElMessage.error(readError(err, '导入本地书失败'))
  } finally {
    importingLocal.value = false
  }
}

function localBookTitle(item) {
  if (item?.book) return item.book.title || '未命名本地书'
  return String(item?.name || '未命名本地书').replace(/\.[^.]+$/, '')
}

function localBookSubline(item) {
  if (item?.book) {
    const parts = []
    if (item.book.author) parts.push(item.book.author)
    if (item.book.chapterCount) parts.push(`共${item.book.chapterCount}章`)
    return parts.join(' · ') || item.path || '本地书籍'
  }
  return item?.path || ''
}

function localBookMeta(item) {
  if (item?.book) {
    if (item.book.lastChapter) return `最新：${item.book.lastChapter}`
    if (item.path) return `来源：${item.path}`
    return '已导入书架'
  }
  return `大小：${formatSize(item?.size)}`
}

function localResultKey(item) {
  return item?.book ? `shelf-${item.book.id}` : `file-${item.path}`
}

function localShelfSearchText(book) {
  return normalizeLocalSearch([
    book.title,
    book.author,
    book.lastChapter,
    book.latestChapter,
    book.latestChapterTitle,
    book.originalFile,
    book.libraryPath,
    book.tocFile,
    book.sourceFile,
    book.url,
    localBookSubline({ book }),
    localBookMeta({ book }),
  ].filter(Boolean).join(' '))
}

function localFileSearchText(item) {
  return normalizeLocalSearch([
    item.name,
    item.path,
    item.extension,
    item.mimeType,
  ].filter(Boolean).join(' '))
}

function normalizeLocalSearch(value) {
  return String(value || '')
    .toLowerCase()
    .replace(/[\s·•._\-—–:：，,。.!！?？()[\]【】《》"'“”‘’/\\]+/g, '')
}

function fileExtension(value) {
  const match = String(value || '').match(/\.([^.\\/]+)$/)
  return match ? match[1].toUpperCase() : '本地'
}

function readLocalShelfBook(book) {
  router.push({ name: 'reader', params: { id: book.id }, query: readerRouteQueryForLocalBook(book) })
}

function readerRouteQueryForLocalBook(book) {
  return readerRouteQueryFromBook(book, readerProgressForBook(book))
}

function readerProgressForBook(book) {
  return bookshelf.books.find(item => item.id === book?.id)?.progress || book?.progress || null
}

function openLocalShelfDetail(book) {
  overlay.openBookInfo(book, {
    statusLabel: '本地书籍',
    statusType: 'info',
  })
}

function formatSize(bytes) {
  if (!bytes) return '0 B'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

async function addRemoteBook(item, shouldRead) {
  addingBook.value = item.bookUrl
  try {
    const payload = {
      title: item.title,
      author: item.author,
      coverUrl: item.coverUrl,
      intro: item.intro,
      bookUrl: item.bookUrl,
      sourceId: item.sourceId,
      sourceName: item.sourceName,
      categoryId: targetCategoryId.value ? Number(targetCategoryId.value) : null,
    }
    const { data } = await createRemoteBook(payload)
    bookshelf.upsertBook(data)
    ElMessage.success(`已加入书架：《${item.title}》`)
    if (shouldRead) {
      overlay.closeBookInfo()
      router.push({ name: 'reader', params: { id: data.id } })
      return
    }
    overlay.openBookInfo(data, {
      sourceName: item.sourceName,
      statusLabel: '已加入书架',
      statusType: 'success',
      progress: 0,
      actions: [
        { label: '完整详情', plain: true, handler: () => openExistingDetail(data) },
        { label: '开始阅读', type: 'primary', handler: () => openExistingReader(data) },
      ],
    })
  } catch (err) {
    ElMessage.error(readError(err, '加入失败'))
  } finally {
    addingBook.value = null
  }
}

function openPreview(item) {
  const existing = findExistingBook(item)
  overlay.openBookInfo(item, {
    sourceName: item.sourceName,
    statusLabel: existing ? '已在书架' : '搜索结果',
    statusType: existing ? 'warning' : 'success',
    progress: existing?.progress?.percent || 0,
    actions: existing
      ? [
          { label: '查看详情', plain: true, handler: () => openExistingInfo(existing, item.sourceName) },
          { label: '继续阅读', type: 'primary', handler: () => openExistingReader(existing) },
        ]
      : [
          { label: '加入书架', plain: true, loading: addingBook.value === item.bookUrl, handler: () => addRemoteBook(item, false) },
          { label: '加入并阅读', type: 'primary', loading: addingBook.value === item.bookUrl, handler: () => addRemoteBook(item, true) },
        ],
  })
}

function findExistingBook(item) {
  return bookshelf.books.find(book => (
    Number(book.sourceId || 0) === Number(item.sourceId || 0)
    && String(book.url || book.bookUrl || '') === String(item.bookUrl || '')
  )) || null
}

function openExistingDetail(book) {
  overlay.closeBookInfo()
  router.push({ name: 'book-detail', params: { id: book.id } })
}

function openExistingInfo(book, sourceName = '') {
  overlay.openBookInfo(book, {
    sourceName,
    statusLabel: '已在书架',
    statusType: 'warning',
    progress: book.progress?.percent || 0,
    actions: [
      { label: '完整详情', plain: true, handler: () => openExistingDetail(book) },
      { label: '继续阅读', type: 'primary', handler: () => openExistingReader(book) },
    ],
  })
}

function openExistingReader(book) {
  overlay.closeBookInfo()
  router.push({ name: 'reader', params: { id: book.id }, query: readerRouteQueryForLocalBook(book) })
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.search-page {
  display: grid;
  min-width: 0;
  gap: 16px;
}

.search-head,
.search-console,
.search-options,
.search-status,
.result-card,
.result-title,
.result-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.search-head,
.result-title {
  justify-content: space-between;
}

.eyebrow {
  margin: 0 0 4px;
  color: var(--app-primary);
  font-size: 12px;
  font-weight: 800;
  text-transform: uppercase;
}

.search-console {
  min-width: 0;
  flex-wrap: wrap;
  padding: 14px;
}

.mode-switch {
  max-width: 100%;
}

.search-console > .el-input {
  min-width: min(260px, 100%);
  flex: 1;
}

.search-options {
  min-width: 0;
  width: 100%;
  flex-wrap: wrap;
}

.search-options :deep(.el-select),
.search-options :deep(.el-radio-group) {
  max-width: 100%;
}

.source-collapse {
  width: 100%;
}

.source-checks {
  display: flex;
  flex-wrap: wrap;
  gap: 10px 16px;
}

.search-status {
  flex-wrap: wrap;
}

.source-result-list,
.result-list,
.local-result-list {
  display: grid;
  min-width: 0;
  gap: 12px;
}

.source-result-group {
  display: grid;
  gap: 10px;
}

.source-result-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.source-result-head h2 {
  margin: 0;
  color: var(--app-text);
  font-size: 16px;
}

.result-card,
.local-result-card {
  padding: 14px;
  align-items: start;
  cursor: pointer;
}

.result-card:hover,
.local-result-card:hover,
.local-result-card.selected {
  border-color: var(--app-primary);
}

.local-result-card {
  display: flex;
  gap: 12px;
}

.local-file-icon {
  display: grid;
  width: 42px;
  height: 54px;
  place-items: center;
  flex: 0 0 42px;
  color: var(--app-primary-strong);
  background: var(--app-primary-soft);
  border-radius: 5px;
  font-size: 24px;
}

.result-main {
  display: grid;
  min-width: 0;
  flex: 1;
  gap: 6px;
}

.result-main h3,
.result-main p {
  margin: 0;
}

.result-main h3 {
  font-size: 17px;
}

.result-main p {
  color: var(--app-text-muted);
  font-size: 13px;
}

.result-intro {
  display: -webkit-box;
  overflow: hidden;
  line-height: 1.6;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.latest-chapter {
  color: var(--app-primary-strong) !important;
}

.result-actions {
  flex-wrap: wrap;
  justify-content: flex-end;
}

.preview-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 6px;
}

.preview-actions .el-select {
  min-width: 180px;
  flex: 1;
}

@media (max-width: 860px), (hover: none) and (pointer: coarse) {
  .search-page {
    gap: 8px;
    padding-bottom: 14px;
  }

  .search-head,
  .search-console,
  .search-options,
  .result-card,
  .result-actions {
    display: grid;
  }

  .search-head {
    gap: 6px;
  }

  .search-head .app-page-subtitle {
    display: none;
  }

  .search-head :deep(.el-button),
  .search-console :deep(.el-button) {
    min-height: 38px;
  }

  .search-console {
    gap: 8px;
    padding: 8px;
  }

  .search-console > .el-input,
  .search-console > :deep(.el-button),
  .mode-switch,
  .search-options :deep(.el-select),
  .search-options :deep(.el-radio-group) {
    width: 100%;
  }

  .search-options,
  .local-search-options {
    gap: 8px;
  }

  .mode-switch,
  .search-options :deep(.el-radio-group) {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .mode-switch :deep(.el-radio-button__inner),
  .search-options :deep(.el-radio-button__inner) {
    display: block;
    min-height: 36px;
    overflow: hidden;
    padding: 8px 6px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .source-checks {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
  }

  .source-checks :deep(.el-checkbox) {
    min-width: 0;
    margin-right: 0;
  }

  .source-checks :deep(.el-checkbox__label) {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .search-status {
    gap: 6px;
  }

  .source-result-list,
  .result-list,
  .local-result-list {
    gap: 8px;
  }

  .source-result-head {
    align-items: flex-start;
    display: grid;
    gap: 4px;
  }

  .result-actions {
    justify-content: stretch;
  }

  .result-actions :deep(.el-button) {
    width: 100%;
    min-height: 36px;
    margin-left: 0;
  }

  .result-card,
  .local-result-card {
    grid-template-columns: 42px minmax(0, 1fr);
    gap: 10px;
    padding: 10px;
  }

  .local-result-card {
    display: grid;
    grid-template-columns: auto 34px minmax(0, 1fr);
  }

  .local-file-icon {
    width: 34px;
    height: 46px;
    font-size: 20px;
  }

  .result-title {
    display: grid;
    gap: 4px;
  }

  .result-main h3 {
    overflow: hidden;
    font-size: 16px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .result-main p {
    min-width: 0;
    font-size: 12px;
  }

  .result-intro {
    -webkit-line-clamp: 2;
  }
}
</style>
