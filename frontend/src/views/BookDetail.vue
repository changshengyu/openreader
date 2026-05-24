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
            :category-name="categoryName(book.categoryId)"
            :chapters="chapters"
            :progress="reader.progressByBook[book.id]?.percent || 0"
            :status-label="book.sourceId ? '远程书籍' : '本地书籍'"
            :status-type="book.sourceId ? 'success' : 'info'"
            :cover-editable="true"
            :cover-uploading="uploadingCover"
            :show-update-switch="book.sourceId > 0"
            :can-update="book.canUpdate !== false"
            :update-switch-loading="updatingBook"
            @cover-upload="uploadBookCoverFromPanel"
            @can-update-change="toggleBookCanUpdate"
          >
            <div class="hero-actions">
              <el-button type="primary" @click="startRead">开始阅读</el-button>
	              <el-button @click="openBookEditor">编辑</el-button>
	              <el-button v-if="book.sourceId > 0" :loading="refreshingBook" @click="refreshCurrentBook">刷新目录</el-button>
	              <el-button v-if="book.sourceId > 0" :icon="Switch" :loading="loadingSourceCandidates" @click="openChangeSource">换源</el-button>
	              <el-button v-if="book.sourceId > 0" :loading="cachingBook" @click="cacheCurrentBook">缓存后续20章</el-button>
	              <el-button v-if="book.sourceId > 0" :loading="clearingCache" @click="clearCurrentBookCache">清缓存</el-button>
	              <el-button type="danger" plain @click="deleteCurrentBook">删除</el-button>
              <el-select v-model="categoryDraft" placeholder="设置分组" clearable size="default" class="category-select" @change="changeCategory">
                <el-option label="未分组" value="" />
                <el-option v-for="category in bookshelf.categories" :key="category.id" :label="category.name" :value="String(category.id)" />
              </el-select>
            </div>
          </BookInfoPanel>
        </section>

        <el-tabs v-model="activeTab" class="detail-tabs">
          <el-tab-pane label="目录" name="toc">
            <section class="app-panel tab-panel">
              <div class="tab-toolbar">
                <el-switch v-model="tocReverse" active-text="倒序" inactive-text="正序" />
              </div>
              <ReaderTocPanel
                v-model="tocKeyword"
                :chapters="chapters"
                :current-index="detailCurrentIndex"
                :reverse="tocReverse"
                :show-meta="true"
                :locate-key="tocLocateKey"
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
                :stats="sourceStats"
                :show-info-button="false"
                @refresh="loadSourceCandidates"
                @load-more="loadMoreSourceCandidates"
                @group-change="changeSourceGroup"
                @change="changeSource"
              />
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

    <el-dialog v-model="showChangeSource" title="换源" width="460px" :fullscreen="isMobileDialog">
      <SourceSwitchPanel
        :book="book"
        :sources="sourceCandidates"
        :loading="loadingSourceCandidates"
        :changing-source="changingSource"
        :current-source-name="currentSource?.name || ''"
        :group="sourceGroup"
        :groups="sourceGroups"
        :has-more="sourceHasMore"
        :stats="sourceStats"
        :show-info-button="false"
        @refresh="loadSourceCandidates"
        @load-more="loadMoreSourceCandidates"
        @group-change="changeSourceGroup"
        @change="changeSource"
      />
      <p v-if="changeMessage" :class="changeError ? 'msg-error' : 'msg-success'">{{ changeMessage }}</p>
    </el-dialog>

    <el-dialog v-model="showBookEditor" title="编辑书籍" width="540px" :fullscreen="isMobileDialog">
      <el-form label-position="top" class="book-editor">
        <el-form-item label="书名"><el-input v-model="bookDraft.title" /></el-form-item>
        <el-form-item label="作者"><el-input v-model="bookDraft.author" /></el-form-item>
        <el-form-item label="封面">
          <div class="cover-upload-row">
            <el-input v-model="bookDraft.coverUrl" placeholder="封面地址或上传本地图片" />
            <el-upload accept="image/*" :show-file-list="false" :auto-upload="false" @change="uploadBookCover">
              <el-button :loading="uploadingCover">上传</el-button>
            </el-upload>
          </div>
        </el-form-item>
        <el-form-item label="简介"><el-input v-model="bookDraft.intro" type="textarea" :rows="5" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showBookEditor = false">取消</el-button>
        <el-button type="primary" :loading="savingBook" @click="saveBookEdit">保存</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowLeft, Switch } from '@element-plus/icons-vue'
import { cacheBookContent, changeBookSource, deleteBookmark, listBookmarks, listBookSourceCandidates, refreshBook, updateBook, updateBookCategory } from '../api/books'
import api from '../api/client'
import { uploadAsset } from '../api/uploads'
import BookInfoPanel from '../components/BookInfoPanel.vue'
import ReaderBookmarkPanel from '../components/reader/ReaderBookmarkPanel.vue'
import ReaderTocPanel from '../components/reader/ReaderTocPanel.vue'
import SourceSwitchPanel from '../components/reader/SourceSwitchPanel.vue'
import { useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore } from '../stores/reader'

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
const sourceHasMore = ref(false)
const sourceStats = ref(null)
const activeTab = ref('toc')
const tocKeyword = ref('')
const tocLocateKey = ref(0)
const tocReverse = ref(false)
const categoryDraft = ref('')
const showChangeSource = ref(false)
const showBookEditor = ref(false)
const savingBook = ref(false)
const uploadingCover = ref(false)
const updatingBook = ref(false)
const refreshingBook = ref(false)
const cachingBook = ref(false)
const clearingCache = ref(false)
const changingSource = ref(null)
const changeMessage = ref('')
const changeError = ref(false)
const bookDraft = reactive({ title: '', author: '', coverUrl: '', intro: '' })
const windowWidth = ref(typeof window === 'undefined' ? 1280 : window.innerWidth)
const coarsePointer = ref(typeof window === 'undefined' ? false : window.matchMedia?.('(hover: none) and (pointer: coarse)').matches || false)

const currentSource = computed(() => availableSources.value.find(source => Number(source.id) === Number(book.value?.sourceId)))
const isMobileDialog = computed(() => windowWidth.value <= 860 || coarsePointer.value)
const sourceGroups = computed(() => {
  const groups = availableSources.value.map(source => source.group).filter(Boolean)
  return [...new Set(groups)].sort()
})
const detailCurrentIndex = computed(() => {
  const id = book.value?.id
  const progress = id ? reader.progressByBook[id] || book.value?.progress : null
  const index = Number(progress?.chapterIndex || 0)
  return Number.isFinite(index) ? Math.max(0, Math.min(chapters.value.length - 1, index)) : 0
})

onMounted(() => {
  window.addEventListener('resize', updateWindowWidth, { passive: true })
  load()
})
onBeforeUnmount(() => window.removeEventListener('resize', updateWindowWidth))

watch(activeTab, (tab) => {
  if (tab !== 'toc') return
  tocKeyword.value = ''
  nextTick(() => { tocLocateKey.value += 1 })
})

function updateWindowWidth() {
  windowWidth.value = window.innerWidth
  coarsePointer.value = window.matchMedia?.('(hover: none) and (pointer: coarse)').matches || false
}

async function load() {
  loading.value = true
  try {
    const id = route.params.id
    await bookshelf.loadCategories()
    const [bookRes, chapterRes, bookmarkRes, sourceRes] = await Promise.all([
      api.get(`/books/${id}`),
      api.get(`/books/${id}/chapters`),
      listBookmarks(id),
      api.get('/sources'),
      reader.loadProgress(id).catch(() => null),
    ])
    book.value = bookRes.data
    chapters.value = chapterRes.data
    bookmarks.value = bookmarkRes.data
    availableSources.value = sourceRes.data.filter(source => source.enabled)
    await loadSourceCandidates()
    categoryDraft.value = book.value.categoryId ? String(book.value.categoryId) : ''
  } catch (err) {
    ElMessage.error(readError(err, '加载书籍失败'))
  } finally {
    loading.value = false
  }
}

function startRead() {
  router.push({ name: 'reader', params: { id: book.value.id } })
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

async function changeCategory(value) {
  try {
    const categoryId = value ? Number(value) : null
    const { data } = await updateBookCategory(book.value.id, categoryId)
    book.value = data
    bookshelf.upsertBook(data)
    ElMessage.success('分组已更新')
  } catch (err) {
    ElMessage.error(readError(err, '更新分组失败'))
    categoryDraft.value = book.value.categoryId ? String(book.value.categoryId) : ''
  }
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
  if (!book.value) return
  Object.assign(bookDraft, {
    title: book.value.title || '',
    author: book.value.author || '',
    coverUrl: book.value.coverUrl || '',
    intro: book.value.intro || '',
  })
  showBookEditor.value = true
}

async function saveBookEdit() {
  if (!book.value) return
  if (!bookDraft.title.trim()) {
    ElMessage.warning('书名不能为空')
    return
  }
  savingBook.value = true
  try {
    const { data } = await updateBook(book.value.id, {
      title: bookDraft.title,
      author: bookDraft.author,
      coverUrl: bookDraft.coverUrl,
      intro: bookDraft.intro,
      categoryId: book.value.categoryId || null,
      canUpdate: book.value.canUpdate !== false,
    })
    book.value = data
    bookshelf.upsertBook(data)
    showBookEditor.value = false
    ElMessage.success('书籍已更新')
  } catch (err) {
    ElMessage.error(readError(err, '更新书籍失败'))
  } finally {
    savingBook.value = false
  }
}

async function uploadBookCover(data) {
  const file = data.raw || data.file
  if (!file) return
  uploadingCover.value = true
  try {
    const { data: result } = await uploadAsset({ file, type: 'cover' })
    bookDraft.coverUrl = result.url
    ElMessage.success('封面已上传')
  } catch (err) {
    ElMessage.error(readError(err, '上传封面失败'))
  } finally {
    uploadingCover.value = false
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
      coverUrl: result.url,
      intro: book.value.intro || '',
      categoryId: book.value.categoryId || null,
      canUpdate: book.value.canUpdate !== false,
    })
    book.value = data
    bookshelf.upsertBook(data)
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
      categoryId: book.value.categoryId || null,
      canUpdate: value,
    })
    book.value = data
    bookshelf.upsertBook(data)
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
    const { data } = await refreshBook(book.value.id)
    book.value = data?.book || data
    if (book.value?.id) bookshelf.upsertBook(book.value)
    const chaptersRes = await api.get(`/books/${book.value.id}/chapters`)
    chapters.value = chaptersRes.data
    ElMessage.success(data.added ? `新增 ${data.added} 章` : '目录已刷新')
  } catch (err) {
    ElMessage.error(readError(err, '刷新目录失败'))
  } finally {
    refreshingBook.value = false
  }
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

function cacheStartChapterIndex() {
  const progress = reader.progressByBook[book.value?.id] || book.value?.progress
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

async function reloadChapters() {
  if (!book.value) return
  const chaptersRes = await api.get(`/books/${book.value.id}/chapters`)
  chapters.value = chaptersRes.data
}

async function loadSourceCandidates({ append = false } = {}) {
  if (!book.value) return
  loadingSourceCandidates.value = true
  try {
    if (!append) {
      sourceOffset.value = 0
      sourceHasMore.value = false
      sourceStats.value = null
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
    sourceHasMore.value = Boolean(data?.hasMore)
    sourceStats.value = Array.isArray(data)
      ? null
      : {
          searched: data?.searched || 0,
          matched: data?.matched || 0,
          failed: data?.failed || 0,
          empty: data?.empty || 0,
        }
  } catch (err) {
    ElMessage.error(readError(err, '搜索可用来源失败'))
  } finally {
    loadingSourceCandidates.value = false
  }
}

function loadMoreSourceCandidates() {
  return loadSourceCandidates({ append: true })
}

function changeSourceGroup(value) {
  sourceGroup.value = value || ''
  sourceStats.value = null
  loadSourceCandidates()
}

function mergeSourceCandidates(existing, incoming) {
  const seen = new Set(existing.map(item => `${item.sourceId}-${item.bookUrl}`))
  return existing.concat(incoming.filter(item => {
    const key = `${item.sourceId}-${item.bookUrl}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  }))
}

async function openChangeSource() {
  showChangeSource.value = true
  if (!sourceCandidates.value.length) {
    await loadSourceCandidates()
  }
}

async function changeSource(source) {
  if (!book.value || source.current) return
  changingSource.value = source.sourceId
  changeMessage.value = ''
  changeError.value = false
  try {
    const { data } = await changeBookSource(book.value.id, {
      sourceId: source.sourceId,
      bookUrl: source.bookUrl,
      title: source.title,
      author: source.author,
      coverUrl: source.coverUrl,
      intro: source.intro,
    })
    book.value = data
    bookshelf.upsertBook(data)
    const chaptersRes = await api.get(`/books/${book.value.id}/chapters`)
    chapters.value = chaptersRes.data
    changeMessage.value = `已切换，共 ${data.chapterCount || chapters.value.length} 章`
    await loadSourceCandidates()
    ElMessage.success('换源成功')
  } catch (err) {
    changeError.value = true
    changeMessage.value = readError(err, '换源失败')
  } finally {
    changingSource.value = null
  }
}

function categoryName(id) {
  if (!id) return '未分组'
  return bookshelf.categories.find(category => String(category.id) === String(id))?.name || '未分组'
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

.category-select {
  width: 160px;
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

.book-editor {
  display: grid;
}

.cover-upload-row {
  display: flex;
  gap: 8px;
}

.cover-upload-row .el-input {
  flex: 1;
}

.msg-success {
  color: #67c23a;
}

.msg-error {
  color: #f56c6c;
}

@media (max-width: 860px), (hover: none) and (pointer: coarse) {
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

  .tab-toolbar .el-input,
  .category-select {
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
