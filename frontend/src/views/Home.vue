<template>
  <SearchWorkspace v-if="workspace.isSearchResult" @back-to-shelf="backToShelf" />
  <DiscoverWorkspace v-else-if="workspace.isExploreResult" @back-to-shelf="backToShelf" />
  <section v-else class="app-page shelf-page" :class="{ 'mobile-shelf': isMobileShelf }">
    <div class="shelf-title">
      <div class="shelf-title-main">
        <button v-if="isMobileShelf" class="mobile-menu-trigger" type="button" aria-label="打开侧边栏" @click.stop="toggleMobileNavigation">
          <el-icon><Menu /></el-icon>
        </button>
        <strong>书架 ({{ displayedBooks.length }})</strong>
      </div>
      <div class="title-actions">
        <button v-if="isNormalPage" type="button" @click="showBookEditButton = !showBookEditButton">
          {{ showBookEditButton ? '取消' : '编辑' }}
        </button>
        <button type="button" @click="refreshShelf">
          {{ refreshLoading ? '刷新中...' : '刷新' }}
        </button>
        <button v-if="isNormalPage" type="button" @click="overlay.openRSS()">RSS</button>
        <button v-if="isNormalPage" type="button" @click.stop="openExploreWorkspace">书海</button>
      </div>
    </div>

    <div v-if="showBookEditButton" class="shelf-search-wrapper">
      <el-input v-model="shelfKeyword" placeholder="搜索书名或作者" clearable>
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>
    </div>

    <div class="book-group-wrapper" role="tablist" aria-label="书架分组">
      <button
        v-for="item in groupItems"
        :key="item.key"
        class="group-chip"
        :class="{ active: selectedGroup === item.key }"
        type="button"
        role="tab"
        :aria-selected="selectedGroup === item.key"
        @click="selectedGroup = item.key"
      >
        <span>{{ item.name }}</span>
      </button>
    </div>

    <main class="shelf-main">
      <div
        v-loading="shelfLoading"
        class="books-wrapper"
        :element-loading-text="shelfLoadingText"
        :element-loading-background="shelfLoadingBackground"
      >
        <div class="book-list wrapper">
          <article
            v-for="book in displayedBooks"
            :key="book.id"
            class="book-row book"
            :class="{ editing: showBookEditButton }"
            role="button"
            tabindex="0"
            @click="handleBookRowClick(book)"
            @keyup.enter="handleBookRowClick(book)"
          >
            <span class="cover-img" @click.stop="openDetail(book)">
              <BookCover
                class="list-cover"
                :book="book"
              />
            </span>
            <span class="list-main info">
              <span class="book-operation">
                <button v-if="showBookEditButton" class="operation-icon danger" type="button" title="删除" @click.stop="deleteManagedBook(book)">
                  <el-icon><Close /></el-icon>
                </button>
                <button v-if="showBookEditButton" class="operation-icon" type="button" title="编辑" @click.stop="goEditBook(book)">
                  <el-icon><Edit /></el-icon>
                </button>
                <el-badge
                  v-if="!showBookEditButton && unreadCount(book) > 0"
                  class="unread-num-badge"
                  :max="99"
                  :value="unreadCount(book)"
                />
              </span>
              <strong class="name" :class="{ edit: showBookEditButton }">{{ book.title }}</strong>
              <span class="sub">
                <span class="author">{{ book.author || '' }}</span>
                <span v-if="book.chapterCount" class="dot">•</span>
                <span v-if="book.chapterCount" class="size">共{{ book.chapterCount }}章</span>
              </span>
              <span v-if="readChapterTitle(book)" class="dur-chapter">已读：{{ readChapterTitle(book) }}</span>
              <span v-if="latestChapterTitle(book)" class="last-chapter">{{ latestChapterLabel(book) }}：{{ latestChapterTitle(book) }}</span>
            </span>
          </article>
        </div>
      </div>
    </main>

  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Close, Edit, Menu, Search } from '@element-plus/icons-vue'
import { useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore } from '../stores/reader'
import { usePreferencesStore } from '../stores/preferences'
import { useIndexWorkspaceStore } from '../stores/indexWorkspace'
import SearchWorkspace from './Search.vue'
import DiscoverWorkspace from './Discover.vue'
import BookCover from '../components/BookCover.vue'
import { createBookCategoryNameResolver } from '../utils/bookCategory'
import { filterBooksByBookGroup, resolveBookGroupSelection, visibleBookGroups } from '../utils/bookGroups'
import { newestBookProgress, sortByShelfOrder } from '../utils/bookOrder'
import { readerRouteQueryFromBook } from '../utils/readerRoute'
import { currentViewportWidth, shouldUseMiniInterface } from '../utils/responsive'
import { filterShelfBooksByEditQuery, relativeShelfTimeLabel } from '../utils/shelfPresentation'

const router = useRouter()
const route = useRoute()
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const reader = useReaderStore()
const preferences = usePreferencesStore()
const workspace = useIndexWorkspaceStore()
const categoryName = createBookCategoryNameResolver(() => bookshelf.categories)

const selectedGroup = computed({
  get: () => resolveBookGroupSelection(bookshelf.bookGroups, bookshelf.books, preferences.shelf.groupKey),
  set: value => preferences.setShelfGroup(value),
})
const showBookEditButton = ref(false)
const shelfKeyword = ref('')
const refreshLoading = ref(false)
const windowWidth = ref(currentViewportWidth())

const groupItems = computed(() => visibleBookGroups(bookshelf.bookGroups, bookshelf.books))

const sortedBooks = computed(() => sortByShelfOrder(Array.isArray(bookshelf.books) ? bookshelf.books : [], reader.progressByBook))
const displayedBooks = computed(() => {
  const filtered = filterBooksByBookGroup(sortedBooks.value, selectedGroup.value)
  return filterShelfBooksByEditQuery(filtered, showBookEditButton.value ? shelfKeyword.value : '')
})

const isMobileShelf = computed(() => shouldUseMiniInterface(reader.pageMode, windowWidth.value))
const isNormalPage = computed(() => !['kindle', 'simple', 'Kindle'].includes(reader.pageType))
const shelfLoading = computed(() => bookshelf.loading || refreshLoading.value)
const shelfLoadingText = computed(() => refreshLoading.value ? '正在刷新书籍信息' : '正在获取书籍信息')
const shelfLoadingBackground = computed(() => reader.themeType === 'night' ? '#222' : '#fff')

onMounted(async () => {
  updateViewportFlags()
  window.addEventListener('resize', updateViewportFlags)
  window.addEventListener('orientationchange', updateViewportFlags)
  await warmHomeShelf()
})

watch(
  () => [route.name, route.query.workspace, route.query.q, route.query.mode, route.query.searchType, route.query.group, route.query.sourceId, route.query.concurrent],
  () => applyRouteWorkspaceIntent(),
  { immediate: true },
)

onBeforeUnmount(() => {
  window.removeEventListener('resize', updateViewportFlags)
  window.removeEventListener('orientationchange', updateViewportFlags)
})

watch(
  () => route.query.import,
  (value) => {
    if (value === '1') overlay.openImportBook()
  },
  { immediate: true },
)

watch(isNormalPage, (normal) => {
  if (!normal) showBookEditButton.value = false
})

async function deleteManagedBook(book) {
  try {
    await ElMessageBox.confirm(`确定删除《${book.title}》吗？阅读进度和书签也会一并删除。`, '删除书籍', { type: 'warning' })
    await bookshelf.removeBook(book.id)
    ElMessage.success('书籍已删除')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除失败'))
  }
}

async function refreshShelf() {
  refreshLoading.value = true
  try {
    const [categoryResult, bookGroupResult, booksResult] = await Promise.allSettled([
      bookshelf.loadCategories({ force: true }),
      bookshelf.loadBookGroups({ force: true }),
      bookshelf.loadBooks({ force: true, all: true, settleProgress: true }),
    ])
    if (booksResult.status === 'rejected') throw booksResult.reason
    if (categoryResult.status === 'rejected' || bookGroupResult.status === 'rejected') {
      const groupError = categoryResult.status === 'rejected' ? categoryResult.reason : bookGroupResult.reason
      ElMessage.warning(readError(groupError, '书架已刷新，分组刷新失败'))
    } else {
      ElMessage.success('书架已刷新')
    }
  } catch (err) {
    ElMessage.error(readError(err, '刷新书架失败'))
  } finally {
    refreshLoading.value = false
  }
}

function goEditBook(book) {
  overlay.openBookEdit(book)
}

function openDetail(book) {
  overlay.openBookInfo(book, {
    categoryName: categoryName(book),
    progress: (bookProgress(book)?.percent || 0),
  })
}

function continueRead(book) {
  router.push({ name: 'reader', params: { id: book.id }, query: readerRouteQuery(book) })
}

function handleBookRowClick(book) {
  continueRead(book)
}

function readChapterTitle(book) {
  const progress = bookProgress(book)
  if (progress?.chapterTitle) return progress.chapterTitle
  if (Number.isInteger(progress?.chapterIndex)) return `第 ${progress.chapterIndex + 1} 章`
  return ''
}

function unreadCount(book) {
  const progress = bookProgress(book)
  const chapterIndex = Number.isInteger(progress?.chapterIndex) ? progress.chapterIndex : -1
  const chapterCount = Number(book.chapterCount || 0)
  return Math.max(0, chapterCount - 1 - chapterIndex)
}

function bookProgress(book) {
  return newestBookProgress(book, reader.progressByBook)
}

function latestChapterTitle(book) {
  return book.lastChapter || book.latestChapterTitle || book.latestChapter || ''
}

function latestChapterLabel(book) {
  const rawTime = book.lastCheckTime
  return rawTime ? relativeShelfTimeLabel(rawTime) : '最新'
}

function readerRouteQuery(book) {
  return readerRouteQueryFromBook(book, bookProgress(book))
}

function updateViewportFlags() {
  windowWidth.value = currentViewportWidth()
}

function toggleMobileNavigation() {
  window.dispatchEvent(new CustomEvent('openreader:toggle-mobile-nav'))
}

function openExploreWorkspace() {
  workspace.requestExplore()
}

function backToShelf() {
  workspace.backToShelf()
  if (route.query.workspace === undefined) return
  const {
    workspace: _workspace,
    q: _query,
    mode: _mode,
    searchType: _searchType,
    group: _group,
    sourceId: _sourceId,
    concurrent: _concurrent,
    url: _url,
    name: _name,
    ...query
  } = route.query
  router.replace({ name: 'home', query })
}

function applyRouteWorkspaceIntent() {
  if (route.name !== 'home') return
  if (route.query.workspace === 'search') {
    workspace.beginSearch({
      keyword: route.query.q,
      mode: route.query.mode,
      searchType: route.query.searchType,
      group: route.query.group,
      sourceId: route.query.sourceId,
      concurrent: route.query.concurrent,
    })
    return
  }
  if (route.query.workspace === 'explore') {
    workspace.requestExplore({
      sourceId: route.query.sourceId,
      sourceGroup: route.query.group,
      url: route.query.url,
      name: route.query.name,
    })
    return
  }
  if (!workspace.showingResults) workspace.backToShelf()
}

async function warmHomeShelf() {
  const jobs = [
    ['categories', bookshelf.ensureCategoriesLoaded()],
    ['bookGroups', bookshelf.ensureBookGroupsLoaded()],
    ['books', bookshelf.ensureBooksLoaded({ all: true })],
  ]
  const results = await Promise.allSettled(jobs.map(([, job]) => job))
  results.forEach((result, index) => {
    if (result.status !== 'rejected') return
    const type = jobs[index][0]
    if (type === 'books') {
      ElMessage.error(readError(result.reason, '加载书架失败'))
    } else {
      ElMessage.warning(readError(result.reason, '分组加载失败，书架仍可使用'))
    }
  })
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style src="../styles/home-shelf.css"></style>
