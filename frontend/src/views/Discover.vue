<template>
  <section class="app-page discover-page">
    <header class="discover-head">
      <div>
        <p class="eyebrow">Explore</p>
        <h1 class="app-page-title">书海探索</h1>
        <p class="app-page-subtitle">仅显示配置了 exploreUrl 的书源，按书源真实发现页加载书籍。</p>
      </div>
      <el-button :icon="Refresh" :loading="loadingSources" @click="loadSources">刷新书源</el-button>
    </header>

    <section class="discover-toolbar app-panel">
      <el-select v-model="selectedSourceId" placeholder="选择探索书源" filterable @change="loadBooks">
        <el-option v-for="source in sources" :key="source.id" :label="source.name" :value="source.id" />
      </el-select>
      <el-button type="primary" :loading="loadingBooks" :disabled="!selectedSourceId" @click="loadBooks">加载</el-button>
      <span v-if="activeSource" class="source-status">{{ activeSource.group || '默认分组' }} · 第 {{ page }} 页</span>
    </section>

    <div v-loading="loadingBooks" class="discover-results">
      <article v-for="book in books" :key="book.sourceId + book.bookUrl" class="discover-card app-panel" @click="openPreview(book)">
        <BookCover :book="book" />
        <div>
          <h2>{{ book.title }}</h2>
          <p>{{ book.author || '未知作者' }} · {{ book.sourceName }}</p>
          <p v-if="book.latestChapter" class="latest">最新：{{ book.latestChapter }}</p>
          <p class="intro">{{ book.intro || '无简介' }}</p>
        </div>
      </article>
      <el-empty v-if="!loadingBooks && !books.length" :description="sources.length ? '当前书源没有返回发现结果' : '没有配置 exploreUrl 的书源'" />
    </div>

    <div v-if="books.length" class="load-more-row">
      <el-button :loading="loadingMore" :disabled="!hasMore" @click="loadMoreBooks">
        {{ hasMore ? '加载更多' : '没有更多结果' }}
      </el-button>
    </div>

    <BookInfoDialog
      v-model="previewDialog"
      :book="selectedBook"
      :source-name="selectedBook?.sourceName"
      status-label="探索结果"
      status-type="success"
    >
      <div v-if="selectedBook" class="preview-actions">
        <el-select v-model="targetCategoryId" placeholder="加入书架分组（可选）" clearable>
          <el-option label="未分组" value="" />
          <el-option v-for="category in bookshelf.categories" :key="category.id" :label="category.name" :value="String(category.id)" />
        </el-select>
        <el-button plain :loading="addingBook === selectedBook.bookUrl" @click="addRemoteBook(selectedBook, false)">加入书架</el-button>
        <el-button type="primary" :loading="addingBook === selectedBook.bookUrl" @click="addRemoteBook(selectedBook, true)">加入并阅读</el-button>
      </div>
    </BookInfoDialog>
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { createRemoteBook } from '../api/books'
import { exploreBooks, listExploreSources } from '../api/explore'
import BookCover from '../components/BookCover.vue'
import BookInfoDialog from '../components/BookInfoDialog.vue'
import { useBookshelfStore } from '../stores/bookshelf'

const router = useRouter()
const bookshelf = useBookshelfStore()
const sources = ref([])
const books = ref([])
const selectedSourceId = ref('')
const targetCategoryId = ref('')
const loadingSources = ref(false)
const loadingBooks = ref(false)
const previewDialog = ref(false)
const selectedBook = ref(null)
const addingBook = ref(null)
const page = ref(1)
const hasMore = ref(false)
const loadingMore = ref(false)

const activeSource = computed(() => sources.value.find(source => source.id === selectedSourceId.value))

onMounted(async () => {
  await Promise.all([loadSources(), bookshelf.loadCategories()])
  if (selectedSourceId.value) await loadBooks()
})

async function loadSources() {
  loadingSources.value = true
  try {
    const { data } = await listExploreSources()
    sources.value = data || []
    if (!selectedSourceId.value && sources.value.length) selectedSourceId.value = sources.value[0].id
  } catch (err) {
    ElMessage.error(readError(err, '加载探索书源失败'))
  } finally {
    loadingSources.value = false
  }
}

async function loadBooks() {
  if (!selectedSourceId.value) return
  loadingBooks.value = true
  try {
    page.value = 1
    const { data } = await exploreBooks(selectedSourceId.value, { page: page.value })
    const result = normalizeExploreResult(data, page.value)
    books.value = result.items
    hasMore.value = result.hasMore
  } catch (err) {
    ElMessage.error(readError(err, '加载探索结果失败'))
  } finally {
    loadingBooks.value = false
  }
}

async function loadMoreBooks() {
  if (!selectedSourceId.value || loadingMore.value || !hasMore.value) return
  loadingMore.value = true
  try {
    const nextPage = page.value + 1
    const { data } = await exploreBooks(selectedSourceId.value, { page: nextPage })
    const result = normalizeExploreResult(data, nextPage)
    const known = new Set(books.value.map(book => `${book.sourceId}-${book.bookUrl}`))
    const nextItems = result.items.filter(book => !known.has(`${book.sourceId}-${book.bookUrl}`))
    books.value = [...books.value, ...nextItems]
    page.value = result.page || nextPage
    hasMore.value = result.hasMore && nextItems.length > 0
  } catch (err) {
    ElMessage.error(readError(err, '加载更多失败'))
  } finally {
    loadingMore.value = false
  }
}

function normalizeExploreResult(data, fallbackPage) {
  if (Array.isArray(data)) {
    return { items: data, page: fallbackPage, hasMore: false }
  }
  return {
    items: Array.isArray(data?.items) ? data.items : [],
    page: Number(data?.page || fallbackPage),
    hasMore: !!data?.hasMore,
  }
}

function openPreview(book) {
  selectedBook.value = book
  previewDialog.value = true
}

async function addRemoteBook(book, shouldRead) {
  addingBook.value = book.bookUrl
  try {
    const { data } = await createRemoteBook({
      title: book.title,
      author: book.author,
      coverUrl: book.coverUrl,
      intro: book.intro,
      bookUrl: book.bookUrl,
      sourceId: book.sourceId,
      sourceName: book.sourceName,
      categoryId: targetCategoryId.value ? Number(targetCategoryId.value) : null,
    })
    ElMessage.success(`已加入书架：《${book.title}》`)
    previewDialog.value = false
    router.push({ name: shouldRead ? 'reader' : 'book-detail', params: { id: data.id } })
  } catch (err) {
    ElMessage.error(readError(err, '加入书架失败'))
  } finally {
    addingBook.value = null
  }
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.discover-page {
  display: grid;
  gap: 16px;
}

.discover-head,
.discover-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  justify-content: space-between;
}

.discover-toolbar {
  justify-content: flex-start;
  padding: 12px;
}

.discover-toolbar .el-select {
  min-width: 280px;
}

.source-status {
  color: var(--app-text-muted);
  font-size: 13px;
}

.discover-results {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}

.discover-card {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 14px;
  align-items: center;
  padding: 14px;
  cursor: pointer;
}

.discover-card h2 {
  margin: 0 0 6px;
  font-size: 18px;
}

.discover-card p {
  margin: 0;
  color: var(--app-text-muted);
}

.discover-card .intro {
  display: -webkit-box;
  margin-top: 8px;
  overflow: hidden;
  line-height: 1.6;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.discover-card .latest {
  margin-top: 4px;
  color: var(--app-accent);
  font-size: 13px;
}

.load-more-row {
  display: flex;
  justify-content: center;
}

.preview-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  justify-content: center;
}

.preview-actions .el-select {
  min-width: 180px;
  flex: 1;
}

@media (max-width: 760px) {
  .discover-head,
  .discover-toolbar {
    display: grid;
    justify-content: stretch;
  }
}
</style>
