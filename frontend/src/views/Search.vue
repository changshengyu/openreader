<template>
  <section class="app-page search-page">
    <header class="search-head">
      <div>
        <p class="eyebrow">Search</p>
        <h1 class="app-page-title">搜索书籍</h1>
        <p class="app-page-subtitle">按上游阅读器的逻辑，选择书源范围后并发搜索并加入书架。</p>
      </div>
      <el-button :icon="Connection" @click="router.push({ name: 'sources' })">书源管理</el-button>
    </header>

    <section class="search-console app-panel">
      <el-input v-model="keyword" placeholder="输入书名或作者" size="large" clearable @keyup.enter="doSearch">
        <template #prefix><el-icon><SearchIcon /></el-icon></template>
      </el-input>
      <el-button type="primary" size="large" :loading="searching" @click="doSearch">搜索</el-button>

      <div class="search-options">
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

      <el-collapse v-if="searchType === 'custom'" class="source-collapse">
        <el-collapse-item :title="`自选书源（${selectedIds.length}/${enabledSources.length}）`">
          <el-checkbox :model-value="allSelected" @change="toggleAll">全选</el-checkbox>
          <el-checkbox-group v-model="selectedIds" class="source-checks">
            <el-checkbox v-for="source in enabledSources" :key="source.id" :value="source.id" :label="source.name" />
          </el-checkbox-group>
        </el-collapse-item>
      </el-collapse>
    </section>

    <section class="search-status">
      <el-tag effect="plain">启用书源 {{ enabledSources.length }}</el-tag>
      <el-tag effect="plain">本次搜索 {{ selectedIds.length }}</el-tag>
      <el-tag v-if="searched" :type="results.length ? 'success' : 'info'" effect="plain">结果 {{ results.length }}</el-tag>
    </section>

    <div v-loading="searching" class="result-area">
      <div v-if="groupedResults.length" class="source-result-list">
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

      <el-empty v-else-if="searched && !searching" description="没有找到相关书籍" />
      <el-empty v-else description="输入关键词后开始搜索" />
    </div>

    <BookInfoDialog
      v-model="previewDialog"
      :book="selectedResult"
      :source-name="selectedResult?.sourceName"
      status-label="搜索结果"
      status-type="success"
    >
      <div v-if="selectedResult" class="preview-actions">
        <el-select v-model="targetCategoryId" placeholder="加入书架分组（可选）" clearable>
          <el-option label="未分组" value="" />
          <el-option v-for="category in bookshelf.categories" :key="category.id" :label="category.name" :value="String(category.id)" />
        </el-select>
        <el-button plain :loading="addingBook === selectedResult.bookUrl" @click="addRemoteBook(selectedResult, false)">加入书架</el-button>
        <el-button type="primary" :loading="addingBook === selectedResult.bookUrl" @click="addRemoteBook(selectedResult, true)">加入并阅读</el-button>
      </div>
    </BookInfoDialog>
  </section>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Connection, Search as SearchIcon } from '@element-plus/icons-vue'
import { createRemoteBook } from '../api/books'
import api from '../api/client'
import BookCover from '../components/BookCover.vue'
import BookInfoDialog from '../components/BookInfoDialog.vue'
import { useBookshelfStore } from '../stores/bookshelf'

const route = useRoute()
const router = useRouter()
const bookshelf = useBookshelfStore()

const keyword = ref('')
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
const previewDialog = ref(false)
const selectedResult = ref(null)

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

onMounted(async () => {
  await Promise.all([loadSources(), bookshelf.loadCategories()])
  keyword.value = route.query.q || ''
  syncSelection()
  if (keyword.value) doSearch()
})

watch(searchType, syncSelection)

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

async function doSearch() {
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
    ElMessage.success(`已加入书架：《${item.title}》`)
    previewDialog.value = false
    router.push({ name: shouldRead ? 'reader' : 'book-detail', params: { id: data.id } })
  } catch (err) {
    ElMessage.error(readError(err, '加入失败'))
  } finally {
    addingBook.value = null
  }
}

function openPreview(item) {
  selectedResult.value = item
  previewDialog.value = true
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.search-page {
  display: grid;
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
  flex-wrap: wrap;
  padding: 14px;
}

.search-console > .el-input {
  min-width: 260px;
  flex: 1;
}

.search-options {
  width: 100%;
  flex-wrap: wrap;
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
.result-list {
  display: grid;
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

.result-card {
  padding: 14px;
  align-items: start;
  cursor: pointer;
}

.result-card:hover {
  border-color: var(--app-primary);
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

@media (max-width: 760px) {
  .search-head,
  .result-card,
  .result-actions {
    display: grid;
  }

  .result-actions {
    justify-content: stretch;
  }
}
</style>
