<template>
  <section class="app-page detail-page">
    <button class="back-link" type="button" @click="router.push({ name: 'home' })">
      <el-icon><ArrowLeft /></el-icon>
      <span>返回书架</span>
    </button>

    <div v-loading="loading">
      <template v-if="book">
        <section class="book-hero app-panel">
          <BookInfoPanel
            :book="book"
            :source-name="currentSource?.name || ''"
            :category-name="categoryName(book)"
            :chapters="chapters"
            :progress="bookProgress?.percent || 0"
            :browser-cache-count="book.id ? browserCacheCount : -1"
            :status-label="book.sourceId ? '远程书籍' : '本地书籍'"
            :status-type="book.sourceId ? 'success' : 'info'"
            :cover-editable="true"
            :cover-uploading="uploadingCover"
            :show-update-switch="book.sourceId > 0"
            :can-update="book.canUpdate !== false"
            :update-switch-loading="updatingBook"
            :show-category-action="true"
            @cover-upload="uploadBookCoverFromPanel"
            @can-update-change="toggleBookCanUpdate"
            @category-action="openBookGroupSetter"
          >
            <div class="hero-actions">
              <el-button type="primary" @click="startRead">开始阅读</el-button>
              <el-button @click="openBookEditor">编辑</el-button>
              <el-button v-if="book.sourceId > 0" :loading="refreshingBook" @click="refreshCurrentBook">刷新目录</el-button>
              <el-button v-else :loading="refreshingBook" @click="refreshCurrentLocalBook">刷新本地书</el-button>
              <el-button v-if="canChangeLocalTocRule" :loading="refreshingBook" @click="changeLocalTocRule">修改目录规则</el-button>
              <el-button v-if="book.sourceId > 0" :icon="Switch" :loading="loadingSourceCandidates" @click="openChangeSource">换源</el-button>
              <el-button :loading="cachingLocalBook" @click="cacheCurrentBookLocal">缓存到浏览器</el-button>
              <el-button v-if="book.sourceId > 0" :loading="cachingBook" @click="cacheCurrentBook">缓存到服务器</el-button>
              <el-button :loading="clearingLocalCache" @click="clearCurrentBookLocalCache">清浏览器缓存</el-button>
              <el-button v-if="book.sourceId > 0" :loading="clearingCache" @click="clearCurrentBookCache">清服务器缓存</el-button>
              <el-button type="danger" plain @click="deleteCurrentBook">删除</el-button>
            </div>
          </BookInfoPanel>
        </section>

        <el-tabs v-model="activeTab" class="detail-tabs">
          <el-tab-pane label="目录" name="toc">
            <section class="app-panel tab-panel">
              <div class="tab-toolbar">
                <el-switch v-model="tocReverse" active-text="倒序" inactive-text="正序" />
                <span class="toc-cache-summary">浏览器缓存 {{ browserCacheCount }} 章</span>
              </div>
              <ReaderTocPanel
                ref="tocPanelRef"
                v-model="tocKeyword"
                :chapters="chapters"
                :current-index="detailCurrentIndex"
                :reverse="tocReverse"
                :show-meta="true"
                searchable
                :locate-key="tocLocateKey"
                :browser-cached-map="browserCachedChapters"
                @jump="goChapter"
              />
            </section>
          </el-tab-pane>

          <el-tab-pane label="书签" name="bookmarks">
            <section class="app-panel tab-panel">
              <div class="tab-toolbar">
                <el-button @click="openGlobalBookmark">管理书签</el-button>
              </div>
              <ReaderBookmarkPanel
                :bookmarks="bookmarks"
                :show-add="false"
                :show-edit="false"
                @jump="goBookmark"
                @remove="deleteBookmarkItem"
              />
            </section>
          </el-tab-pane>

          <el-tab-pane label="来源" name="sources">
            <section class="app-panel tab-panel">
              <SourceSwitchPanel
                :book="book"
                :sources="sourceCandidates"
                :loading="loadingSourceCandidates"
                :changing-source="changingSource"
                :current-source-name="currentSource?.name || ''"
                :group="sourceGroup"
                :groups="sourceGroups"
                :has-more="sourceHasMore"
                @refresh="loadSourceCandidates"
                @load-more="loadMoreSourceCandidates"
                @group-change="changeSourceGroup"
                @change="changeSource"
              />
              <p v-if="changeMessage" :class="changeError ? 'msg-error' : 'msg-success'">{{ changeMessage }}</p>
            </section>
          </el-tab-pane>

          <el-tab-pane label="详情" name="info">
            <section class="app-panel tab-panel">
              <dl class="info-list">
                <div><dt>书籍 ID</dt><dd>{{ book.id }}</dd></div>
                <div><dt>来源 ID</dt><dd>{{ book.sourceId || '本地' }}</dd></div>
                <div><dt>原始文件</dt><dd>{{ book.originalFile || '-' }}</dd></div>
                <div><dt>书库路径</dt><dd>{{ book.libraryPath || '-' }}</dd></div>
                <div><dt>创建时间</dt><dd>{{ formatDate(book.createdAt) }}</dd></div>
                <div><dt>更新时间</dt><dd>{{ formatDate(book.updatedAt) }}</dd></div>
              </dl>
            </section>
          </el-tab-pane>
        </el-tabs>
      </template>
    </div>

    <BookEditDialog v-model="showBookEditor" :book="book" :saving="savingBook" @save="saveBookEdit" />
  </section>
</template>

<script setup>
import { computed, h, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowLeft, Switch } from '@element-plus/icons-vue'
import { cacheBookContent, changeBookSource, deleteBookmark, listBookmarks, listBookSourceCandidates, refreshBook, refreshLocalBook, updateBook } from '../api/books'
import api from '../api/client'
import { uploadAsset } from '../api/uploads'
import BookEditDialog from '../components/BookEditDialog.vue'
import BookInfoPanel from '../components/BookInfoPanel.vue'
import ReaderBookmarkPanel from '../components/reader/ReaderBookmarkPanel.vue'
import ReaderTocPanel from '../components/reader/ReaderTocPanel.vue'
import SourceSwitchPanel from '../components/reader/SourceSwitchPanel.vue'
import { bookCategoryIds, mergeShelfBook, useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore } from '../stores/reader'
import { cacheBookChaptersToBrowser, clearBookBrowserChapterCache, listBookBrowserCachedChapters } from '../utils/bookChapterCache'
import { newestBookProgress } from '../utils/bookOrder'
import { readerRouteQueryFromBook } from '../utils/readerRoute'
import { invalidateReaderDataCache, writeReaderDataCache } from '../utils/readerDataCache'
import { epubTocRuleOptions, isEPUBLocalBook as checkEPUBLocalBook, isTextLocalBook as checkTextLocalBook } from '../utils/localBookToc'
import {
  sourceCandidateAuthor,
  sourceCandidateBookUrl,
  sourceCandidateCover,
  sourceCandidateIntro,
  sourceCandidateKey,
  sourceCandidateSourceId,
  sourceCandidateTitle,
} from '../utils/sourceCandidate'

const route = useRoute()
const router = useRouter()
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const reader = useReaderStore()

const loading = ref(true)
const book = ref(null)
const chapters = ref([])
const bookmarks = ref([])
const availableSources = ref([])
const sourceCandidates = ref([])
const loadingSourceCandidates = ref(false)
const sourceGroup = ref('')
const sourceOffset = ref(0)
const sourceHasMore = ref(true)
const activeTab = ref('toc')
const tocPanelRef = ref(null)
const tocKeyword = ref('')
const tocLocateKey = ref(0)
const tocReverse = ref(false)
const browserCachedChapters = ref({})
const showBookEditor = ref(false)
const savingBook = ref(false)
const uploadingCover = ref(false)
const updatingBook = ref(false)
const refreshingBook = ref(false)
const cachingBook = ref(false)
const cachingLocalBook = ref(false)
const clearingCache = ref(false)
const clearingLocalCache = ref(false)
const changingSource = ref(null)
const changeMessage = ref('')
const changeError = ref(false)

const currentSource = computed(() => availableSources.value.find(source => Number(source.id) === Number(book.value?.sourceId)))
const sourceGroups = computed(() => {
  const groups = availableSources.value.map(source => source.group).filter(Boolean)
  return [...new Set(groups)].sort()
})
const bookProgress = computed(() => newestBookProgress(book.value, reader.progressByBook))
const isTextLocalBook = computed(() => checkTextLocalBook(book.value))
const isEPUBLocalBook = computed(() => checkEPUBLocalBook(book.value))
const canChangeLocalTocRule = computed(() => isTextLocalBook.value || isEPUBLocalBook.value)
const detailCurrentIndex = computed(() => {
  const progress = bookProgress.value
  const index = Number(progress?.chapterIndex || 0)
  return Number.isFinite(index) ? Math.max(0, Math.min(chapters.value.length - 1, index)) : 0
})
const browserCacheCount = computed(() => Object.keys(browserCachedChapters.value).length)

onMounted(() => {
  window.addEventListener('openreader:book-info-updated', handleExternalBookUpdate)
  load()
})
onBeforeUnmount(() => {
  window.removeEventListener('openreader:book-info-updated', handleExternalBookUpdate)
})

watch(activeTab, async (tab) => {
  if (tab !== 'toc') return
  tocKeyword.value = ''
  await refreshBrowserCacheMap()
  nextTick(() => {
    tocLocateKey.value += 1
    tocPanelRef.value?.locateCurrentChapter?.()
  })
})

async function load() {
  loading.value = true
  try {
    const id = route.params.id
    const [bookRes, chapterRes] = await Promise.all([
      api.get(`/books/${id}`),
      api.get(`/books/${id}/chapters`),
    ])
    book.value = mergeBookUpdate(bookRes.data)
    chapters.value = Array.isArray(chapterRes.data) ? chapterRes.data : []

    const [categoryRes, bookmarkRes, sourceRes, progressRes] = await Promise.allSettled([
      warmDetailCategories(),
      listBookmarks(id),
      api.get('/sources'),
      reader.loadProgress(id),
    ])
    if (categoryRes.status === 'rejected') {
      ElMessage.warning(readError(categoryRes.reason, '分组加载失败，详情仍可阅读'))
    }
    bookmarks.value = bookmarkRes.status === 'fulfilled' && Array.isArray(bookmarkRes.value.data) ? bookmarkRes.value.data : []
    if (bookmarkRes.status === 'rejected') {
      ElMessage.warning(readError(bookmarkRes.reason, '书签加载失败，详情仍可阅读'))
    }
    availableSources.value = sourceRes.status === 'fulfilled' && Array.isArray(sourceRes.value.data)
      ? sourceRes.value.data.filter(source => source.enabled)
      : []
    if (sourceRes.status === 'rejected') {
      ElMessage.warning(readError(sourceRes.reason, '书源加载失败，详情仍可阅读'))
    }
    const progress = progressRes.status === 'fulfilled' ? progressRes.value : null
    if (book.value?.progress?.bookId) {
      reader.applyServerProgress(book.value.progress)
      bookshelf.applyBookProgress(book.value.progress)
    }
    if (progress?.bookId) {
      book.value = mergeShelfBook(book.value, { id: book.value.id, progress })
    }
    sourceCandidates.value = []
    sourceOffset.value = 0
    sourceHasMore.value = true
    await refreshBrowserCacheMap()
    if (book.value?.sourceId) await loadSourceCandidates({ silent: true })
  } catch (err) {
    ElMessage.error(readError(err, '加载书籍失败'))
  } finally {
    loading.value = false
  }
}

async function warmDetailCategories() {
  return bookshelf.ensureCategoriesLoaded()
}

function startRead() {
  router.push({ name: 'reader', params: { id: book.value.id }, query: readerRouteQuery(book.value) })
}

function goChapter(index) {
  router.push({ name: 'reader', params: { id: book.value.id }, query: { chapter: index } })
}

function goBookmark(bookmark) {
  router.push({
    name: 'reader',
    params: { id: book.value.id },
    query: {
      chapter: bookmark.chapterIndex,
      offset: bookmark.offset,
      percent: Number.isFinite(Number(bookmark.percent)) ? Number(bookmark.percent) : undefined,
    },
  })
}

function readerRouteQuery(targetBook) {
  const progress = newestBookProgress(targetBook, reader.progressByBook)
  return readerRouteQueryFromBook(targetBook, progress, targetBook?.chapterCount || chapters.value.length)
}

function openGlobalBookmark() {
  overlay.openBookmark(book.value)
}

async function deleteBookmarkItem(bookmark) {
  try {
    await deleteBookmark(bookmark.id)
    bookmarks.value = bookmarks.value.filter(item => item.id !== bookmark.id)
    ElMessage.success('书签已删除')
  } catch (err) {
    ElMessage.error(readError(err, '删除书签失败'))
  }
}

async function openBookGroupSetter() {
  if (!book.value?.id) return
  try {
    await warmDetailCategories()
  } catch (err) {
    ElMessage.warning(readError(err, '分组加载失败，仍可尝试打开设置'))
  }
  overlay.openBookGroup('set', book.value, {
    categoryName: categoryName(book.value),
    progress: bookProgress.value?.percent || 0,
    statusLabel: book.value.sourceId ? '远程书籍' : '本地书籍',
    statusType: book.value.sourceId ? 'success' : 'info',
  })
}

async function deleteCurrentBook() {
  if (!book.value) return
  try {
    await ElMessageBox.confirm(`确定删除《${book.value.title}》吗？阅读进度和书签也会一并删除。`, '删除书籍', { type: 'warning' })
    await bookshelf.removeBook(book.value.id)
    ElMessage.success('书籍已删除')
    router.push({ name: 'home' })
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '删除失败'))
  }
}

function openBookEditor() {
  if (book.value) showBookEditor.value = true
}

async function saveBookEdit(payload) {
  if (!book.value) return
  savingBook.value = true
  try {
    const { data } = await updateBook(book.value.id, {
      ...payload,
      categoryIds: bookCategoryIds(book.value),
      canUpdate: book.value.canUpdate !== false,
    })
    await applyBookUpdate(data)
    showBookEditor.value = false
    ElMessage.success('书籍已更新')
  } catch (err) {
    ElMessage.error(readError(err, '更新书籍失败'))
  } finally {
    savingBook.value = false
  }
}

async function uploadBookCoverFromPanel(file) {
  if (!book.value || !file) return
  uploadingCover.value = true
  try {
    const { data: result } = await uploadAsset({ file, type: 'cover' })
    const { data } = await updateBook(book.value.id, {
      title: book.value.title,
      author: book.value.author || '',
      customCoverUrl: result.url,
      intro: book.value.intro || '',
      categoryIds: bookCategoryIds(book.value),
      canUpdate: book.value.canUpdate !== false,
    })
    await applyBookUpdate(data)
    ElMessage.success('封面已更新')
  } catch (err) {
    ElMessage.error(readError(err, '上传封面失败'))
  } finally {
    uploadingCover.value = false
  }
}

async function toggleBookCanUpdate(value) {
  if (!book.value?.id || !book.value.sourceId) return
  updatingBook.value = true
  try {
    const { data } = await updateBook(book.value.id, {
      title: book.value.title,
      author: book.value.author || '',
      coverUrl: book.value.coverUrl || '',
      intro: book.value.intro || '',
      categoryIds: bookCategoryIds(book.value),
      canUpdate: value,
    })
    await applyBookUpdate(data)
    ElMessage.success(value ? '已开启追更' : '已关闭追更')
  } catch (err) {
    ElMessage.error(readError(err, '更新追更状态失败'))
  } finally {
    updatingBook.value = false
  }
}

async function refreshCurrentBook() {
  if (!book.value) return
  refreshingBook.value = true
  try {
    const previousBook = book.value
    const { data } = await refreshBook(book.value.id)
    const updatedBook = mergeBookUpdate(data?.book || data)
    await invalidateBookReaderCaches(previousBook, { clearBrowser: true })
    const nextChapters = await loadBookChapters(updatedBook)
    await applyBookUpdate(updatedBook, { chapters: nextChapters })
    ElMessage.success(data.added ? `新增 ${data.added} 章` : '目录已刷新')
  } catch (err) {
    ElMessage.error(readError(err, '刷新目录失败'))
  } finally {
    refreshingBook.value = false
  }
}

async function refreshCurrentLocalBook() {
  if (!book.value) return
  refreshingBook.value = true
  try {
    const previousBook = book.value
    const { data } = await refreshLocalBook(book.value.id)
    const updatedBook = mergeBookUpdate(data?.book || data)
    await invalidateBookReaderCaches(previousBook, { clearBrowser: true })
    const nextChapters = await loadBookChapters(updatedBook)
    await applyBookUpdate(updatedBook, { chapters: nextChapters })
    ElMessage.success(`本地书已刷新，共 ${data?.chapterCount || book.value?.chapterCount || chapters.value.length} 章`)
  } catch (err) {
    ElMessage.error(readError(err, '刷新本地书失败'))
  } finally {
    refreshingBook.value = false
  }
}

async function changeLocalTocRule() {
  if (!book.value || !canChangeLocalTocRule.value) return
  const tocRule = await chooseLocalTocRule()
  if (tocRule === null) return
  refreshingBook.value = true
  try {
    const previousBook = book.value
    const { data } = await refreshLocalBook(book.value.id, { tocRule })
    const updatedBook = mergeBookUpdate(data?.book || data)
    await invalidateBookReaderCaches(previousBook, { clearBrowser: true })
    const nextChapters = await loadBookChapters(updatedBook)
    await applyBookUpdate(updatedBook, { chapters: nextChapters })
    ElMessage.success(`目录规则已更新，共 ${data?.chapterCount || book.value?.chapterCount || chapters.value.length} 章`)
  } catch (err) {
    ElMessage.error(readError(err, '更新目录规则失败'))
  } finally {
    refreshingBook.value = false
  }
}

async function chooseLocalTocRule() {
  if (!isEPUBLocalBook.value) {
    const result = await ElMessageBox.prompt('填写 TXT 目录行正则，留空则使用默认目录规则。', '修改目录规则', {
      confirmButtonText: '刷新目录',
      cancelButtonText: '取消',
      inputType: 'textarea',
      inputValue: book.value?.tocRule || '',
      inputPlaceholder: '^第.+章.*$',
    }).catch(() => null)
    return result ? (result.value || '') : null
  }
  const selected = ref(book.value?.tocRule || 'spin+toc')
  const selector = h('select', {
    value: selected.value,
    style: 'width:100%;min-height:38px;padding:0 10px;border:1px solid var(--el-border-color);border-radius:4px;background:var(--el-bg-color);color:var(--el-text-color-primary)',
    onChange: event => { selected.value = event.target.value },
  }, epubTocRuleOptions.map(rule => h('option', { value: rule.value }, rule.label)))
  const confirmed = await ElMessageBox.confirm(selector, '修改 EPUB 目录规则', {
    confirmButtonText: '刷新目录',
    cancelButtonText: '取消',
  }).catch(() => false)
  return confirmed ? selected.value : null
}

async function cacheCurrentBook() {
  if (!book.value) return
  cachingBook.value = true
  try {
    const { data } = await cacheBookContent(book.value.id, { all: true, count: 20, chapterIndex: cacheStartChapterIndex() })
    await reloadChapters()
    ElMessage.success(`已缓存 ${data.cached || 0}/${data.requested || 0} 章`)
  } catch (err) {
    ElMessage.error(readError(err, '缓存失败'))
  } finally {
    cachingBook.value = false
  }
}

async function cacheCurrentBookLocal() {
  if (!book.value) return
  cachingLocalBook.value = true
  try {
    if (!chapters.value.length) await reloadChapters()
    const result = await cacheBookChaptersToBrowser(book.value, book.value.id, chapters.value, {
      startIndex: cacheStartChapterIndex(),
      count: 100,
    })
    await refreshBrowserCacheMap()
    ElMessage.success(`已缓存到浏览器 ${result.cached}/${result.requested} 章`)
  } catch (err) {
    ElMessage.error(readError(err, '缓存到浏览器失败'))
  } finally {
    cachingLocalBook.value = false
  }
}

function cacheStartChapterIndex() {
  const progress = bookProgress.value
  const chapterIndex = Number(progress?.chapterIndex)
  return Number.isInteger(chapterIndex) && chapterIndex > 0 ? chapterIndex : 0
}

async function clearCurrentBookCache() {
  if (!book.value) return
  try {
    await ElMessageBox.confirm(`确定清理《${book.value.title}》的章节缓存吗？`, '清理缓存', { type: 'warning' })
    clearingCache.value = true
    const data = await bookshelf.batchClearCache([book.value.id])
    await reloadChapters()
    ElMessage.success(`已清理 ${data.cleared || 0} 个章节缓存`)
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '清理缓存失败'))
  } finally {
    clearingCache.value = false
  }
}

async function clearCurrentBookLocalCache() {
  if (!book.value) return
  try {
    await ElMessageBox.confirm(`确定清理浏览器中《${book.value.title}》的章节缓存吗？`, '清理浏览器缓存', { type: 'warning' })
    clearingLocalCache.value = true
    const removed = await clearBookBrowserChapterCache(book.value, book.value.id)
    browserCachedChapters.value = {}
    ElMessage.success(`已清理浏览器缓存 ${removed} 章`)
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '清理浏览器缓存失败'))
  } finally {
    clearingLocalCache.value = false
  }
}

async function reloadChapters() {
  if (!book.value) return
  chapters.value = await loadBookChapters(book.value)
  await refreshBrowserCacheMap()
}

async function loadBookChapters(targetBook = book.value) {
  if (!targetBook?.id) return []
  const chaptersRes = await api.get(`/books/${targetBook.id}/chapters`)
  return Array.isArray(chaptersRes.data) ? chaptersRes.data : []
}

function mergeBookUpdate(incoming) {
  if (!incoming?.id) return incoming
  const current = bookshelf.books.find(item => Number(item.id) === Number(incoming.id)) ||
    (Number(book.value?.id) === Number(incoming.id) ? book.value : null)
  return mergeShelfBook(current, incoming)
}

async function applyBookUpdate(incoming, options = {}) {
  if (!incoming?.id) return incoming
  const nextBook = mergeBookUpdate(incoming)
  book.value = nextBook
  bookshelf.upsertBook(nextBook)
  const nextChapters = Array.isArray(options.chapters) ? options.chapters : null
  if (nextChapters) chapters.value = nextChapters
  await writeReaderDataCache(nextBook.id, {
    bookData: nextBook,
    ...(nextChapters ? { chaptersData: nextChapters } : {}),
  })
  window.dispatchEvent(new CustomEvent('openreader:reader-book-data-updated', {
    detail: { bookId: nextBook.id, book: nextBook, chapters: nextChapters },
  }))
  return nextBook
}

async function handleExternalBookUpdate(event) {
  const updatedBook = event?.detail?.book
  if (!updatedBook?.id || Number(updatedBook.id) !== Number(book.value?.id)) return
  await applyBookUpdate(updatedBook)
}

async function invalidateBookReaderCaches(targetBook, options = {}) {
  if (!targetBook?.id) return
  await invalidateReaderDataCache(targetBook.id, { book: true, chapters: true })
  if (options.clearBrowser) {
    await clearBookBrowserChapterCache(targetBook, targetBook.id).catch(() => 0)
    browserCachedChapters.value = {}
  }
}

async function refreshBrowserCacheMap() {
  if (!book.value?.id) {
    browserCachedChapters.value = {}
    return
  }
  try {
    browserCachedChapters.value = await listBookBrowserCachedChapters(book.value, book.value.id)
  } catch {
    browserCachedChapters.value = {}
  }
}

async function loadSourceCandidates({ append = false, silent = false } = {}) {
  if (!book.value) return
  loadingSourceCandidates.value = true
  try {
    if (!append) {
      sourceOffset.value = 0
      sourceHasMore.value = true
    }
    const { data } = await listBookSourceCandidates(book.value.id, {
      group: sourceGroup.value || undefined,
      offset: sourceOffset.value,
      limit: 10,
      paged: 1,
    })
    const rows = Array.isArray(data) ? data : (data?.list || [])
    sourceCandidates.value = append ? mergeSourceCandidates(sourceCandidates.value, rows) : rows
    sourceOffset.value = Number.isInteger(data?.nextOffset) ? data.nextOffset : sourceOffset.value + 10
    sourceHasMore.value = typeof data?.hasMore === 'boolean' ? data.hasMore : rows.length >= 10
  } catch (err) {
    if (!silent) ElMessage.error(readError(err, '搜索可用来源失败'))
  } finally {
    loadingSourceCandidates.value = false
  }
}

function loadMoreSourceCandidates() {
  if (!sourceHasMore.value) {
    ElMessage.info('没有更多啦')
    return undefined
  }
  return loadSourceCandidates({ append: true })
}

function changeSourceGroup(value) {
  sourceGroup.value = value || ''
  sourceHasMore.value = true
  loadSourceCandidates()
}

function mergeSourceCandidates(existing, incoming) {
  const seen = new Set(existing.map(item => sourceCandidateKey(item)))
  return existing.concat(incoming.filter(item => {
    const key = sourceCandidateKey(item)
    if (seen.has(key)) return false
    seen.add(key)
    return true
  }))
}

async function openChangeSource() {
  activeTab.value = 'sources'
  if (!sourceCandidates.value.length) {
    await loadSourceCandidates()
  }
}

async function changeSource(source) {
  if (!book.value || source.current) return
  const nextSourceId = sourceCandidateSourceId(source)
  changingSource.value = nextSourceId
  changeMessage.value = ''
  changeError.value = false
  try {
    const previousBook = book.value
    const { data } = await changeBookSource(book.value.id, {
      sourceId: nextSourceId,
      bookUrl: sourceCandidateBookUrl(source),
      title: sourceCandidateTitle(source, book.value.title),
      author: sourceCandidateAuthor(source),
      coverUrl: sourceCandidateCover(source),
      intro: sourceCandidateIntro(source),
    })
    const updatedBook = mergeBookUpdate(data)
    await invalidateBookReaderCaches(previousBook, { clearBrowser: true })
    const nextChapters = await loadBookChapters(updatedBook)
    await applyBookUpdate(updatedBook, { chapters: nextChapters })
    changeMessage.value = `已切换，共 ${updatedBook.chapterCount || chapters.value.length} 章`
    sourceHasMore.value = true
    await loadSourceCandidates()
    ElMessage.success('换源成功')
  } catch (err) {
    changeError.value = true
    changeMessage.value = readError(err, '换源失败')
  } finally {
    changingSource.value = null
  }
}

function categoryName(bookOrId) {
  const ids = typeof bookOrId === 'object' ? bookCategoryIds(bookOrId) : (bookOrId ? [Number(bookOrId)] : [])
  if (!ids.length) return '未分组'
  const names = ids
    .map(id => bookshelf.categories.find(category => String(category.id) === String(id))?.name)
    .filter(Boolean)
  return names.length ? names.join('、') : '未分组'
}

function formatDate(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.detail-page {
  display: grid;
  gap: 16px;
}

.back-link {
  display: inline-flex;
  width: fit-content;
  align-items: center;
  gap: 6px;
  color: var(--app-text-muted);
  background: transparent;
  border: 0;
  cursor: pointer;
}

.back-link:hover {
  color: var(--app-primary);
}

.book-hero {
  position: relative;
  overflow: hidden;
  padding: 24px;
}

.cover-wrap {
  position: relative;
  display: grid;
  place-items: center;
}

.cover-shadow {
  position: absolute;
  inset: 8px;
  opacity: 0.18;
  background-position: center;
  background-size: cover;
  filter: blur(18px);
}

.book-cover {
  position: relative;
  display: grid;
  width: 118px;
  height: 160px;
  place-items: center;
  border-radius: 5px;
  box-shadow: 0 16px 36px rgba(58, 41, 10, 0.18);
  font-size: 44px;
  font-weight: 900;
}

.book-main {
  display: grid;
  align-content: start;
  gap: 12px;
  min-width: 0;
}

.book-title-line,
.hero-actions,
.tab-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
}

.book-title-line {
  justify-content: space-between;
}

.book-title-line h1 {
  margin: 0;
  font-size: 28px;
}

.book-meta,
.book-intro {
  margin: 0;
  color: var(--app-text-muted);
}

.book-intro {
  display: -webkit-box;
  overflow: hidden;
  line-height: 1.7;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
}

.book-facts {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.book-facts span {
  padding: 5px 9px;
  color: var(--app-text-muted);
  background: var(--app-bg-soft);
  border: 1px solid var(--app-border);
  border-radius: 999px;
  font-size: 12px;
}

.detail-tabs {
  min-width: 0;
}

.tab-panel {
  padding: 16px;
}

.tab-toolbar {
  justify-content: space-between;
  margin-bottom: 12px;
}

.tab-toolbar .el-input {
  max-width: 360px;
}

.toc-cache-summary {
  color: var(--app-text-muted);
  font-size: 13px;
}

.info-list {
  display: grid;
  gap: 8px;
}

.info-list {
  margin: 0;
}

.info-list div {
  display: grid;
  grid-template-columns: 100px minmax(0, 1fr);
  gap: 12px;
  padding: 10px 0;
  border-bottom: 1px solid var(--app-border);
}

.info-list dt {
  color: var(--app-text-muted);
}

.info-list dd {
  min-width: 0;
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.msg-success {
  color: #67c23a;
}

.msg-error {
  color: #f56c6c;
}

@media (max-width: 750px) {
  .detail-page {
    gap: 12px;
  }

  .back-link {
    padding: 4px 0;
  }

  .book-hero {
    padding: 14px;
  }

  .book-title-line,
  .hero-actions,
  .tab-toolbar {
    display: grid;
  }

  .hero-actions {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    align-items: stretch;
    gap: 8px;
  }

  .hero-actions :deep(.el-button),
  .hero-actions :deep(.el-select) {
    width: 100%;
  }

  .hero-actions :deep(.el-button + .el-button) {
    margin-left: 0;
  }

  .tab-toolbar .el-input {
    max-width: none;
    width: 100%;
  }

  .tab-panel {
    padding: 12px;
  }

  .detail-tabs :deep(.el-tabs__item) {
    padding: 0 10px;
  }

  .info-list div {
    grid-template-columns: 78px minmax(0, 1fr);
    gap: 8px;
  }

  .detail-page :deep(.el-dialog) {
    width: 94vw !important;
  }

  .cover-upload-row {
    display: grid;
  }
}
</style>
