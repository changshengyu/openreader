<template>
  <main class="reader-shell" :class="[reader.mode, { 'mobile-chrome-visible': mobileChromeVisible }]">
    <aside class="reader-left-rail">
      <button class="rail-item rail-home" type="button" title="返回首页" @click="goShelf">
        <el-icon :size="18"><ArrowLeft /></el-icon>
        <span>首页</span>
      </button>
      <button class="rail-item" type="button" title="书架" @click="openShelfPanel">
        <el-icon :size="18"><Notebook /></el-icon>
        <span>书架</span>
      </button>
      <button class="rail-item" type="button" title="书源" @click="goSourcePanel">
        <el-icon :size="18"><Grid /></el-icon>
        <span>书源</span>
      </button>
      <button class="rail-item" type="button" title="目录" @click="showTocDrawer = true">
        <el-icon :size="18"><List /></el-icon>
        <span>目录</span>
      </button>
      <button class="rail-item" type="button" title="设置" @click="showSettingsDrawer = true">
        <el-icon :size="18"><Setting /></el-icon>
        <span>设置</span>
      </button>
      <button class="rail-item" type="button" title="回到顶部" @click="scrollToTop">
        <el-icon :size="18"><ArrowUpBold /></el-icon>
        <span>顶部</span>
      </button>
      <button class="rail-item" type="button" title="跳到底部" @click="scrollToBottom">
        <el-icon :size="18"><ArrowDownBold /></el-icon>
        <span>底部</span>
      </button>
    </aside>

    <aside class="reader-right-rail">
      <button class="round-tool" type="button" title="书签" @click="showBookmarkDrawer = true">
        <el-icon :size="18"><CollectionTag /></el-icon>
      </button>
      <button class="round-tool" type="button" title="搜索正文" @click="openContentSearch">
        <el-icon :size="18"><Search /></el-icon>
      </button>
      <button class="round-tool" type="button" title="书籍信息" @click="openReaderBookInfo">
        <el-icon :size="18"><InfoFilled /></el-icon>
      </button>
      <button class="round-tool" type="button" title="添加笔记" @click="openNoteDialog">
        <el-icon :size="18"><EditPen /></el-icon>
      </button>
      <button class="round-tool" type="button" title="缓存本章" @click="cacheCurrentChapter">
        <el-icon :size="18"><Download /></el-icon>
      </button>
      <button class="round-tool" type="button" title="重新载入章节" @click="reloadChapter">
        <el-icon :size="18"><RefreshRight /></el-icon>
      </button>
      <button class="round-tool" type="button" :class="{ active: autoReading }" title="自动阅读" @click="toggleAutoReading">
        <el-icon :size="18"><VideoPlay /></el-icon>
      </button>
      <button class="round-tool" type="button" title="阅读设置" @click="showSettingsDrawer = true">
        <el-icon :size="18"><View /></el-icon>
      </button>
      <button class="round-tool" type="button" :class="{ active: tts.state.playing }" :disabled="!tts.state.supported" :title="tts.state.supported ? '朗读' : '当前浏览器不支持朗读'" @click="toggleTTS">
        <el-icon :size="18"><Headset /></el-icon>
      </button>
      <button class="round-tool" type="button" title="夜间模式" @click="toggleNight">
        <el-icon :size="18"><Moon /></el-icon>
      </button>
    </aside>

    <header class="reader-mobile-top">
      <button class="mobile-tool-button" type="button" aria-label="返回首页" @click="goShelf">
        <el-icon :size="20"><ArrowLeft /></el-icon>
      </button>
      <div class="mobile-reader-title">
        <strong>{{ book?.title || '阅读中' }}</strong>
        <span>{{ chapter?.title || chapterLabel }}</span>
      </div>
      <span class="mobile-reader-progress">{{ bookProgressLabel }}</span>
    </header>

    <section ref="pageEl" class="reader-page" :style="readerStyle">
      <header class="reader-page-head">
        <span>{{ book?.title || '阅读中' }}</span>
        <span>{{ chapterLabel }}</span>
      </header>

      <article ref="contentEl" class="reader-content" @scroll.passive="onScroll">
        <div ref="contentBody" class="reader-body" :style="bodyStyle">
          <h1>{{ chapter?.title || '正文' }}</h1>
          <p v-for="(line, index) in lines" :key="index">{{ line }}</p>
          <p v-if="lines.length === 0" class="empty-hint">当前章节暂无缓存内容</p>
        </div>
      </article>
    </section>

    <footer class="reader-page-control">
      <div class="progress-box">{{ bookProgressLabel }}</div>
      <button class="page-step" type="button" title="上一页" @click="previousPage">
        <el-icon :size="24"><ArrowLeft /></el-icon>
      </button>
      <button class="page-step" type="button" title="下一页" @click="nextPage">
        <el-icon :size="24"><ArrowRight /></el-icon>
      </button>
    </footer>

    <footer class="reader-mobile-bottom">
      <button class="mobile-tool-button" type="button" @click="openMobileTool(openTocSearch)">
        <el-icon :size="20"><List /></el-icon>
        <span>目录</span>
      </button>
      <button class="mobile-tool-button" type="button" @click="openMobileTool(() => { showBookmarkDrawer = true })">
        <el-icon :size="20"><CollectionTag /></el-icon>
        <span>书签</span>
      </button>
      <button class="mobile-tool-button" type="button" @click="openMobileTool(openContentSearch)">
        <el-icon :size="20"><Search /></el-icon>
        <span>搜索</span>
      </button>
      <button class="mobile-tool-button" type="button" @click="openMobileTool(() => { showSettingsDrawer = true })">
        <el-icon :size="20"><Setting /></el-icon>
        <span>设置</span>
      </button>
      <button class="mobile-tool-button" type="button" @click="openMobileTool(() => { showMobileMoreDrawer = true })">
        <el-icon :size="20"><MoreFilled /></el-icon>
        <span>更多</span>
      </button>
    </footer>

    <!-- TTS 朗读条 -->
    <div v-if="tts.state.playing" class="tts-bar">
      <el-button text class="tts-btn" @click="tts.skipBackward">‹</el-button>
      <el-button text class="tts-btn" @click="tts.state.paused ? tts.resume() : tts.pause()">
        {{ tts.state.paused ? '▶' : '⏸' }}
      </el-button>
      <el-button text class="tts-btn" @click="tts.skipForward">›</el-button>
      <el-button text class="tts-btn" @click="ttsStop">⏹</el-button>
      <span class="tts-label">语速</span>
      <input :value="tts.state.rate" max="3" min="0.5" step="0.1" type="range" class="tts-slider" @input="setTTSRate($event.target.value)" />
      <span class="tts-label">音调</span>
      <input :value="tts.state.pitch" max="2" min="0.5" step="0.1" type="range" class="tts-slider" @input="setTTSPitch($event.target.value)" />
    </div>

    <!-- Toast -->
    <div v-if="toastMsg" class="reader-toast">{{ toastMsg }}</div>

    <!-- ===== 书架抽屉 ===== -->
    <el-drawer v-model="showShelfDrawer" title="书架" :direction="drawerDirection" :size="drawerSize">
      <div class="reader-drawer-title">
        <span>书架({{ filteredShelfBooks.length }})</span>
        <button type="button" :disabled="shelfLoading" @click="refreshReaderShelf">
          {{ shelfLoading ? '刷新中...' : '刷新' }}
        </button>
      </div>
      <el-input v-model="shelfKeyword" placeholder="搜索书名或作者..." clearable size="small" class="shelf-search" />
      <div v-loading="shelfLoading" class="reader-shelf-list">
        <button
          v-for="item in filteredShelfBooks"
          :key="item.id"
          class="reader-shelf-card"
          :class="{ active: item.id === bookId }"
          type="button"
          @click="changeBookFromShelf(item)"
        >
          <span class="reader-shelf-cover" :style="shelfCoverStyle(item)">{{ shelfCoverInitial(item) }}</span>
          <span class="reader-shelf-main">
            <span class="reader-shelf-title-line">
              <strong>{{ item.title }}</strong>
              <em v-if="unreadCount(item)">{{ unreadCount(item) }}</em>
            </span>
            <small>{{ item.author || '未知作者' }} · {{ shelfProgressLabel(item) }}</small>
            <small>{{ readChapterTitle(item) || '尚未阅读' }}</small>
            <small v-if="item.lastChapter">最新：{{ item.lastChapter }}</small>
          </span>
        </button>
        <el-empty v-if="!shelfLoading && !filteredShelfBooks.length" description="书架里没有匹配书籍" />
      </div>
    </el-drawer>

    <!-- ===== 目录抽屉 ===== -->
    <el-drawer v-model="showTocDrawer" title="目录" :direction="drawerDirection" :size="drawerSize">
      <ReaderTocPanel
        v-model="tocFilter"
        :chapters="chapters"
        :current-index="currentIndex"
        @jump="jumpFromToc"
      />
    </el-drawer>

    <!-- ===== 书签抽屉 ===== -->
    <el-drawer v-model="showBookmarkDrawer" title="书签" :direction="drawerDirection" :size="drawerSize">
      <ReaderBookmarkPanel
        :bookmarks="bookmarks"
        @add="createBookmark"
        @jump="jumpToBookmark"
        @edit="openBookmarkEditor"
        @remove="removeBookmark"
      />
    </el-drawer>

    <!-- ===== 正文搜索抽屉 ===== -->
    <el-drawer v-model="showSearchDrawer" title="搜索正文" :direction="drawerDirection" :size="drawerSize">
      <ReaderSearchPanel
        v-model="contentSearch"
        :results="bookSearchResults"
        :loading="bookSearching"
        :searched="searchedBookContent"
        :has-more="bookSearchHasMore"
        :status-text="bookSearchStatus"
        @search="searchBookContent"
        @load-more="loadMoreBookContent"
        @jump="jumpToBookSearchResult"
      />
    </el-drawer>

    <!-- ===== 书源抽屉 ===== -->
    <el-drawer v-model="showSourceDrawer" title="书源" :direction="drawerDirection" :size="drawerSize" @open="loadSourceCandidates">
      <SourceSwitchPanel
        :book="book"
        :sources="switchableSourceCandidates"
        :loading="loadingSources"
        :changing-source="changingSource"
        :current-source-name="currentSourceName"
        :group="sourceGroup"
        :groups="sourceGroups"
        @refresh="refreshSourceCandidates"
        @load-more="loadMoreSourceCandidates"
        @group-change="changeSourceGroup"
        @show-info="openReaderBookInfo"
        @change="changeSource"
      />
    </el-drawer>

    <!-- ===== 移动端更多 ===== -->
    <el-drawer v-model="showMobileMoreDrawer" title="阅读工具" direction="btt" size="72%" class="mobile-more-drawer">
      <div class="mobile-more-grid">
        <button type="button" class="mobile-more-item" @click="runMobileAction(openShelfPanel)">
          <el-icon :size="22"><Notebook /></el-icon>
          <span>书架</span>
        </button>
        <button type="button" class="mobile-more-item" @click="runMobileAction(goSourcePanel)">
          <el-icon :size="22"><Grid /></el-icon>
          <span>书源</span>
        </button>
        <button type="button" class="mobile-more-item" @click="runMobileAction(openReaderBookInfo)">
          <el-icon :size="22"><InfoFilled /></el-icon>
          <span>信息</span>
        </button>
        <button type="button" class="mobile-more-item" @click="runMobileAction(openNoteDialog)">
          <el-icon :size="22"><EditPen /></el-icon>
          <span>笔记</span>
        </button>
        <button type="button" class="mobile-more-item" @click="runMobileAction(cacheCurrentChapter)">
          <el-icon :size="22"><Download /></el-icon>
          <span>缓存</span>
        </button>
        <button type="button" class="mobile-more-item" @click="runMobileAction(reloadChapter)">
          <el-icon :size="22"><RefreshRight /></el-icon>
          <span>刷新</span>
        </button>
        <button type="button" class="mobile-more-item" :class="{ active: autoReading }" @click="runMobileAction(toggleAutoReading)">
          <el-icon :size="22"><VideoPlay /></el-icon>
          <span>自动</span>
        </button>
        <button type="button" class="mobile-more-item" :class="{ active: tts.state.playing }" :disabled="!tts.state.supported" @click="runMobileAction(toggleTTS)">
          <el-icon :size="22"><Headset /></el-icon>
          <span>听书</span>
        </button>
        <button type="button" class="mobile-more-item" @click="runMobileAction(toggleNight)">
          <el-icon :size="22"><Moon /></el-icon>
          <span>夜间</span>
        </button>
        <button type="button" class="mobile-more-item" @click="runMobileAction(scrollToTop)">
          <el-icon :size="22"><ArrowUpBold /></el-icon>
          <span>顶部</span>
        </button>
        <button type="button" class="mobile-more-item" @click="runMobileAction(scrollToBottom)">
          <el-icon :size="22"><ArrowDownBold /></el-icon>
          <span>底部</span>
        </button>
      </div>
      <p v-if="!tts.state.supported" class="mobile-more-hint">当前浏览器不支持系统朗读，听书入口已禁用。</p>
    </el-drawer>

    <!-- ===== 设置抽屉 ===== -->
    <el-drawer v-model="showSettingsDrawer" title="阅读设置" :direction="drawerDirection" :size="drawerSize">
      <ReaderSettingsPanel
        v-model:custom-bg="customBg"
        v-model:line-height="sliderLineHeight"
        :reader="reader"
        :tts="tts"
        :tts-voices="ttsVoices"
        :font-options="fontOptions"
        :theme-presets="themePresets"
        @mode-change="onModeChange"
        @theme-change="setTheme"
        @pick-bg-image="pickBgImage"
        @tts-rate-change="setTTSRate"
        @tts-pitch-change="setTTSPitch"
        @tts-voice-change="setTTSVoice"
        @open-replace-rules="openReplaceRules"
      />
    </el-drawer>

    <el-dialog v-model="showNoteDialog" title="添加笔记" width="360px">
      <el-input v-model="noteText" type="textarea" :rows="4" placeholder="写下当前阅读位置的笔记..." />
      <template #footer>
        <el-button @click="showNoteDialog = false">取消</el-button>
        <el-button type="primary" @click="saveNote">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showBookmarkEditor" title="编辑书签" width="380px">
      <div class="bookmark-editor">
        <el-input v-model="bookmarkDraft.title" placeholder="标题" />
        <el-input v-model="bookmarkDraft.excerpt" type="textarea" :rows="3" placeholder="摘录" />
        <el-input v-model="bookmarkDraft.note" type="textarea" :rows="4" placeholder="笔记" />
      </div>
      <template #footer>
        <el-button @click="showBookmarkEditor = false">取消</el-button>
        <el-button type="primary" :loading="savingBookmark" @click="saveBookmarkEdit">保存</el-button>
      </template>
    </el-dialog>
  </main>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import {
  ArrowDownBold,
  ArrowLeft,
  ArrowRight,
  ArrowUpBold,
  CollectionTag,
  Download,
  EditPen,
  Grid,
  Headset,
  InfoFilled,
  List,
  MoreFilled,
  Moon,
  Notebook,
  RefreshRight,
  Search,
  Setting,
  VideoPlay,
  View,
} from '@element-plus/icons-vue'
import api from '../api/client'
import { cacheBookContent, changeBookSource, listBookSourceCandidates, searchBookContent as searchBookContentApi } from '../api/books'
import { listSources } from '../api/sources'
import ReaderBookmarkPanel from '../components/reader/ReaderBookmarkPanel.vue'
import ReaderSearchPanel from '../components/reader/ReaderSearchPanel.vue'
import ReaderSettingsPanel from '../components/reader/ReaderSettingsPanel.vue'
import SourceSwitchPanel from '../components/reader/SourceSwitchPanel.vue'
import ReaderTocPanel from '../components/reader/ReaderTocPanel.vue'
import { useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore, themePresets } from '../stores/reader'
import { useKeyboard } from '../composables/useKeyboard'
import { useGesture } from '../composables/useGesture'
import { useTTS } from '../composables/useTTS'

const route = useRoute()
const router = useRouter()
const reader = useReaderStore()
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const bookId = computed(() => Number(route.params.id))

const book = ref(null)
const chapters = ref([])
const chapter = ref(null)
const bookmarks = ref([])
const content = ref('')
const contentEl = ref(null)
const contentBody = ref(null)
const pageEl = ref(null)
const currentIndex = ref(Number(route.query.chapter || 0))
const page = ref(0)
const pageCount = ref(1)
const showTocDrawer = ref(false)
const showSettingsDrawer = ref(false)
const showBookmarkDrawer = ref(false)
const showSearchDrawer = ref(false)
const showShelfDrawer = ref(false)
const showSourceDrawer = ref(false)
const showMobileMoreDrawer = ref(false)
const showNoteDialog = ref(false)
const showBookmarkEditor = ref(false)
const sourceCandidates = ref([])
const sourceGroupOptions = ref([])
const loadingSources = ref(false)
const changingSource = ref(null)
const sourceGroup = ref('')
const sourceOffset = ref(0)
const sourceCandidatesLoadedKey = ref('')
const shelfKeyword = ref('')
const shelfLoading = ref(false)
const tocFilter = ref('')
const contentSearch = ref('')
const bookSearchResults = ref([])
const bookSearching = ref(false)
const searchedBookContent = ref(false)
const bookSearchLastIndex = ref(-1)
const bookSearchHasMore = ref(false)
const bookSearchTotal = ref(0)
const noteText = ref('')
const editingBookmark = ref(null)
const savingBookmark = ref(false)
const bookmarkDraft = reactive({ title: '', excerpt: '', note: '' })
const toastMsg = ref('')
const progressVersion = ref(0)
const autoReading = ref(false)
const customBg = ref('')
const sliderLineHeight = ref(2.12)
const pageHeight = ref(600)   // 可视区高度（翻页/分页模式用）
const windowWidth = ref(window.innerWidth)

let saveTimer
let autoReadTimer

const fontOptions = [
  { label: '系统黑体', value: 'system' },
  { label: '宋体', value: 'serif' },
  { label: '楷体', value: 'kai' },
  { label: '仿宋', value: 'mono' },
]

const filteredShelfBooks = computed(() => {
  const value = shelfKeyword.value.trim().toLowerCase()
  const books = Array.isArray(bookshelf.books) ? bookshelf.books : []
  const values = value
    ? books.filter(item => `${item.title || ''} ${item.author || ''}`.toLowerCase().includes(value))
    : books
  return [...values].sort(compareByReadingOrder)
})
const sourceGroups = computed(() => {
  const sourceRows = sourceGroupOptions.value.length ? sourceGroupOptions.value : sourceCandidates.value
  const groups = sourceRows.map(item => item.group).filter(Boolean)
  return [...new Set(groups)].sort()
})
const currentSourceName = computed(() => {
  if (!book.value?.sourceId) return '本地书籍'
  return sourceGroupOptions.value.find(source => Number(source.id) === Number(book.value.sourceId))?.name || '当前来源'
})
const switchableSourceCandidates = computed(() => sourceCandidates.value.filter(source => !source.current))

const lines = computed(() => content.value.split('\n').map(l => l.trim()).filter(Boolean))

const fontStack = computed(() => {
  const stacks = {
    system: '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
    serif: '"Songti SC", "STSong", "Noto Serif CJK SC", serif',
    kai: '"Kaiti SC", "STKaiti", "KaiTi", serif',
    mono: '"STFangsong", "FangSong", "FangSong_GB2312", serif',
  }
  return stacks[reader.fontFamily] || stacks.system
})

const readerStyle = computed(() => ({
  '--reader-font-family': fontStack.value,
  '--reader-font-size': `${reader.fontSize}px`,
  '--reader-heading-size': `${Math.round(reader.fontSize * 1.36)}px`,
  '--reader-bg': reader.currentTheme.bg,
  '--reader-text': reader.currentTheme.text,
  '--reader-font-weight': reader.fontWeight,
  '--reader-brightness': `${reader.brightness}%`,
  '--reader-line-height': reader.lineHeight,
  '--reader-paragraph-space': `${reader.paragraphSpace}em`,
  '--reader-read-width': `${reader.columnWidth}px`,
  '--reader-bg-image': reader.customBgImage ? `url(${reader.customBgImage})` : '',
}))

const bodyStyle = computed(() => {
  // 翻页/分页模式：translateY 垂直切页，一页 = 一屏
  if (reader.mode === 'flip' || reader.mode === 'page') {
    return { transform: `translateY(-${page.value * pageHeight.value}px)` }
  }
  return {}
})

const chapterLabel = computed(() => `${currentIndex.value + 1} / ${chapters.value.length || 1}`)
const isMobileReader = computed(() => windowWidth.value <= 680)
const drawerDirection = computed(() => windowWidth.value <= 680 ? 'btt' : 'rtl')
const drawerSize = computed(() => windowWidth.value <= 680 ? '82%' : '360px')
const bookProgress = computed(() => {
  const total = Math.max(chapters.value.length, 1)
  return Math.min(1, Math.max(0, (currentIndex.value + currentChapterPercent()) / total))
})
const bookProgressLabel = computed(() => `${Math.round(bookProgress.value * 100)}%`)
const bookSearchStatus = computed(() => {
  if (!searchedBookContent.value) return ''
  const scanned = bookSearchLastIndex.value >= 0 ? bookSearchLastIndex.value + 1 : 0
  const total = bookSearchTotal.value || chapters.value.length || 0
  if (!total) return `${bookSearchResults.value.length} 条结果`
  return `已搜索 ${Math.min(scanned, total)} / ${total} 章，${bookSearchResults.value.length} 条结果`
})
const mobileChromeVisible = ref(false)

function onModeChange(mode) {
  reader.setMode(mode)
}

onMounted(async () => {
  reader.normalizeSettings()
  await loadReaderBook()
  window.addEventListener('resize', handleResize)
  sliderLineHeight.value = reader.lineHeight
})

onBeforeUnmount(() => {
  clearTimeout(saveTimer)
  stopAutoReading()
  saveCurrentProgress()
  window.removeEventListener('resize', handleResize)
})

watch(bookId, async () => {
  await loadReaderBook()
})

watch(() => route.query.chapter, async (q) => {
  const idx = Number(q || 0)
  if (idx !== currentIndex.value) await loadChapter(idx, Number(route.query.offset || 0))
  await jumpToRouteLine()
})

watch(() => route.query.line, async () => {
  await jumpToRouteLine()
})

watch(() => reader.mode, async () => {
  page.value = 0
  await nextTick(); updateFlipLayout(); saveCurrentProgress()
})

watch(() => [reader.fontFamily, reader.fontSize, reader.fontWeight, reader.lineHeight, reader.paragraphSpace, reader.columnWidth], async () => {
  await nextTick(); updateFlipLayout(); progressVersion.value += 1
})

watch(contentSearch, () => {
  bookSearchLastIndex.value = -1
  bookSearchHasMore.value = false
  bookSearchTotal.value = 0
  searchedBookContent.value = false
  bookSearchResults.value = []
})

async function loadReaderBook() {
  clearTimeout(saveTimer)
  const [bookRes, chRes, bmRes, saved] = await Promise.all([
    api.get(`/books/${bookId.value}`),
    api.get(`/books/${bookId.value}/chapters`),
    api.get(`/books/${bookId.value}/bookmarks`),
    reader.loadProgress(bookId.value),
  ])
  book.value = bookRes.data
  chapters.value = chRes.data
  bookmarks.value = bmRes.data
  if (route.query.chapter === undefined && saved?.chapterIndex !== undefined) {
    currentIndex.value = saved.chapterIndex
  } else {
    currentIndex.value = Number(route.query.chapter || 0)
  }
  await loadChapter(currentIndex.value, Number(route.query.offset || saved?.offset || 0))
  await jumpToRouteLine()
}

async function loadChapter(index, offset = 0) {
  currentIndex.value = Math.max(0, Math.min(index, Math.max(chapters.value.length - 1, 0)))
  const { data } = await api.get(`/books/${bookId.value}/chapters/${currentIndex.value}/content`)
  chapter.value = data.chapter
  content.value = data.content || ''
  page.value = 0
  await nextTick()
  updateFlipLayout()
  if (reader.mode === 'flip' || reader.mode === 'page') {
    page.value = Math.min(Math.max(offset, 0), pageCount.value - 1)
  } else if (contentEl.value) {
    contentEl.value.scrollTop = Math.max(offset, 0)
  }
  saveCurrentProgress()
}

function setTheme(theme) { reader.setTheme(theme) }

function pickBgImage(data) {
  const file = data.raw || data.file
  if (!file) return
  const fr = new FileReader()
  fr.onload = (e) => reader.setCustomBgImage(e.target.result)
  fr.readAsDataURL(file)
}

async function goChapter(index) {
  if (index === currentIndex.value) { showTocDrawer.value = false; return }
  await router.replace({ name: 'reader', params: { id: bookId.value }, query: { chapter: index } })
}

async function jumpFromToc(index) {
  showTocDrawer.value = false
  await goChapter(index)
}

function goBookDetail() { router.push({ name: 'book-detail', params: { id: bookId.value } }) }
function goShelf() { router.push({ name: 'home' }) }
async function openShelfPanel() {
  showShelfDrawer.value = true
  if (bookshelf.books.length) return
  shelfLoading.value = true
  try {
    await bookshelf.loadBooks()
  } catch (err) {
    ElMessage.error(readError(err, '加载书架失败'))
  } finally {
    shelfLoading.value = false
  }
}

async function changeBookFromShelf(item) {
  showShelfDrawer.value = false
  if (item.id === bookId.value) return
  await router.push({ name: 'reader', params: { id: item.id } })
}

function readChapterTitle(item) {
  const progress = reader.progressByBook[item.id] || item.progress
  return progress?.chapterTitle || item.durChapterTitle || ''
}

function unreadCount(item) {
  const progress = reader.progressByBook[item.id] || item.progress
  const chapterIndex = Number.isInteger(progress?.chapterIndex) ? progress.chapterIndex : -1
  const total = Number(item.chapterCount || item.totalChapterNum || 0)
  return Math.max(0, total - 1 - chapterIndex)
}

function shelfProgressLabel(item) {
  const progress = reader.progressByBook[item.id] || item.progress
  return `${Math.round(Math.max(0, Math.min(1, progress?.percent || 0)) * 100)}%`
}

function shelfCoverInitial(item) {
  return (item.title || '?').slice(0, 1)
}

function shelfCoverStyle(item) {
  if (item.coverUrl) {
    return {
      backgroundImage: `url(${item.coverUrl})`,
      backgroundSize: 'cover',
      backgroundPosition: 'center',
      color: 'transparent',
    }
  }
  const palettes = [
    ['#6b4f18', '#f3dfab'],
    ['#235d58', '#d7ece8'],
    ['#734533', '#f0d8cb'],
    ['#4f4b82', '#dddaf0'],
  ]
  const [fg, bg] = palettes[Number(item.id || 1) % palettes.length]
  return { color: fg, background: bg }
}

async function refreshReaderShelf() {
  shelfLoading.value = true
  try {
    await bookshelf.loadBooks()
  } catch (err) {
    ElMessage.error(readError(err, '刷新书架失败'))
  } finally {
    shelfLoading.value = false
  }
}

function compareByReadingOrder(a, b) {
  const aProgress = reader.progressByBook[a.id] || a.progress
  const bProgress = reader.progressByBook[b.id] || b.progress
  const aReadAt = new Date(aProgress?.updatedAt || 0).getTime()
  const bReadAt = new Date(bProgress?.updatedAt || 0).getTime()
  if (aReadAt !== bReadAt) return bReadAt - aReadAt
  return new Date(b.updatedAt || 0).getTime() - new Date(a.updatedAt || 0).getTime()
}

function openReaderBookInfo() {
  if (!book.value) return
  overlay.openBookInfo(book.value, {
    statusLabel: `阅读中 · ${bookProgressLabel.value}`,
    statusType: 'success',
    progress: bookProgress.value,
    actions: [
      { label: '目录', plain: true, handler: () => { showTocDrawer.value = true; overlay.closeBookInfo() } },
      { label: '书源', plain: true, handler: () => { showSourceDrawer.value = true; overlay.closeBookInfo() } },
      { label: '完整详情', type: 'primary', handler: () => { overlay.closeBookInfo(); goBookDetail() } },
    ],
  })
}

function goSourcePanel() {
  showSourceDrawer.value = true
}

function runMobileAction(action) {
  showMobileMoreDrawer.value = false
  mobileChromeVisible.value = false
  action?.()
}

function openMobileTool(action) {
  mobileChromeVisible.value = false
  action?.()
}

function openReplaceRules() {
  showSettingsDrawer.value = false
  overlay.openReplaceRules()
}

async function loadSourceCandidates({ append = false, force = false } = {}) {
  const key = `${bookId.value}:${sourceGroup.value || 'all'}`
  if (!append && !force && sourceCandidatesLoadedKey.value === key && sourceCandidates.value.length) return
  loadingSources.value = true
  try {
    if (!sourceGroupOptions.value.length) {
      await loadSourceGroups()
    }
    if (!append) sourceOffset.value = 0
    const { data } = await listBookSourceCandidates(bookId.value, {
      group: sourceGroup.value || undefined,
      offset: sourceOffset.value,
      limit: 10,
    })
    const rows = data || []
    sourceCandidates.value = append ? mergeSourceCandidates(sourceCandidates.value, rows) : rows
    sourceOffset.value += 10
    sourceCandidatesLoadedKey.value = key
  } catch (err) {
    ElMessage.error(readError(err, '搜索可用来源失败'))
  } finally {
    loadingSources.value = false
  }
}

function refreshSourceCandidates() {
  sourceCandidatesLoadedKey.value = ''
  return loadSourceCandidates({ force: true })
}

async function loadSourceGroups() {
  try {
    const { data } = await listSources()
    sourceGroupOptions.value = (data || []).filter(source => source.enabled)
  } catch (err) {
    sourceGroupOptions.value = []
  }
}

function loadMoreSourceCandidates() {
  return loadSourceCandidates({ append: true })
}

function changeSourceGroup(value) {
  sourceGroup.value = value || ''
  sourceCandidatesLoadedKey.value = ''
  loadSourceCandidates({ force: true })
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

async function changeSource(source) {
  if (!book.value || source.current) return
  changingSource.value = source.sourceId
  try {
    const { data } = await changeBookSource(bookId.value, {
      sourceId: source.sourceId,
      bookUrl: source.bookUrl,
      title: source.title,
      author: source.author,
      coverUrl: source.coverUrl,
      intro: source.intro,
    })
    book.value = data
    const chRes = await api.get(`/books/${bookId.value}/chapters`)
    chapters.value = chRes.data
    currentIndex.value = Math.min(currentIndex.value, Math.max(chapters.value.length - 1, 0))
    await loadChapter(currentIndex.value, 0)
    sourceCandidatesLoadedKey.value = ''
    await loadSourceCandidates({ force: true })
    ElMessage.success(`已切换到 ${source.sourceName}`)
  } catch (err) {
    ElMessage.error(readError(err, '换源失败'))
  } finally {
    changingSource.value = null
  }
}

function openTocSearch() {
  showTocDrawer.value = true
  nextTick(() => {
    const input = document.querySelector('.toc-search input')
    input?.focus()
  })
}

function openContentSearch() {
  showSearchDrawer.value = true
  nextTick(() => {
    const input = document.querySelector('.content-search-row input')
    input?.focus()
  })
}

async function searchBookContent() {
  return runBookContentSearch({ append: false })
}

async function loadMoreBookContent() {
  return runBookContentSearch({ append: true })
}

async function runBookContentSearch({ append = false } = {}) {
  const keyword = contentSearch.value.trim()
  if (!keyword) return
  bookSearching.value = true
  searchedBookContent.value = true
  try {
    const params = append
      ? {
          paged: 1,
          lastIndex: bookSearchLastIndex.value,
          chapterLimit: 80,
          matchLimit: 200,
        }
      : {}
    const { data } = await searchBookContentApi(bookId.value, keyword, params)
    const rows = Array.isArray(data) ? data : (data?.list || [])
    bookSearchResults.value = append ? bookSearchResults.value.concat(rows) : rows
    bookSearchLastIndex.value = Number.isInteger(data?.lastIndex) ? data.lastIndex : -1
    bookSearchHasMore.value = Boolean(data?.hasMore)
    bookSearchTotal.value = Number(data?.total || 0)
  } catch (err) {
    ElMessage.error(readError(err, '搜索正文失败'))
  } finally {
    bookSearching.value = false
  }
}

function openNoteDialog() {
  noteText.value = ''
  showNoteDialog.value = true
}

async function reloadChapter() {
  await loadChapter(currentIndex.value, currentOffset())
  toastMsg.value = '章节已重新载入'
  setTimeout(() => { toastMsg.value = '' }, 1600)
}

async function cacheCurrentChapter() {
  try {
    const { data } = await cacheBookContent(bookId.value, { chapterIndex: currentIndex.value })
    await loadChapters()
    toastMsg.value = data.cached ? '本章已缓存' : '本章暂无可缓存内容'
    setTimeout(() => { toastMsg.value = '' }, 1600)
  } catch (err) {
    ElMessage.error(readError(err, '缓存章节失败'))
  }
}

function toggleAutoReading() {
  if (autoReading.value) {
    stopAutoReading()
    toastMsg.value = '自动阅读已停止'
    setTimeout(() => { toastMsg.value = '' }, 1200)
    return
  }
  autoReading.value = true
  autoReadTimer = setInterval(() => {
    if (reader.mode === 'scroll' && contentEl.value) {
      const el = contentEl.value
      const atBottom = el.scrollTop + el.clientHeight >= el.scrollHeight - 4
      if (atBottom) {
        if (currentIndex.value < chapters.value.length - 1) nextPage()
        else stopAutoReading()
      } else {
        el.scrollTop += reader.autoReadSpeed
      }
      return
    }
    nextPage()
  }, 260)
  toastMsg.value = '自动阅读已开始'
  setTimeout(() => { toastMsg.value = '' }, 1200)
}

function stopAutoReading() {
  autoReading.value = false
  clearInterval(autoReadTimer)
  autoReadTimer = null
}

function toggleNight() {
  reader.setTheme(reader.theme === 'dark' || reader.theme === 'black' ? 'parchment' : 'dark')
}

function previousPage() {
  // 翻页/分页：在本章内垂直切上一页
  if ((reader.mode === 'flip' || reader.mode === 'page') && page.value > 0) {
    page.value -= 1
    progressVersion.value += 1
    saveCurrentProgress()
    return
  }
  // 已到章首：切上一章
  if (currentIndex.value > 0) goChapter(currentIndex.value - 1)
}

function nextPage() {
  // 翻页/分页：在本章内垂直切下一页
  if ((reader.mode === 'flip' || reader.mode === 'page') && page.value < pageCount.value - 1) {
    page.value += 1
    progressVersion.value += 1
    saveCurrentProgress()
    return
  }
  // 已到章尾：切下一章
  if (currentIndex.value < chapters.value.length - 1) goChapter(currentIndex.value + 1)
}

function updateFlipLayout() {
  if (!contentEl.value || !contentBody.value) return
  // 翻页/分页模式：按可视区高度分页
  if (reader.mode === 'flip' || reader.mode === 'page') {
    pageHeight.value = contentEl.value.clientHeight
    pageCount.value = Math.max(1, Math.ceil(contentBody.value.scrollHeight / pageHeight.value))
    page.value = Math.min(page.value, pageCount.value - 1)
    return
  }
  // 滚动模式
  pageCount.value = 1
  page.value = 0
}

function handleResize() {
  windowWidth.value = window.innerWidth
  updateFlipLayout()
}

function onScroll() {
  if (reader.mode !== 'scroll') return
  progressVersion.value += 1
  clearTimeout(saveTimer)
  saveTimer = setTimeout(saveCurrentProgress, 500)
}

function currentChapterPercent() {
  progressVersion.value
  // 翻页/分页：以页为单位
  if (reader.mode === 'flip' || reader.mode === 'page') {
    return pageCount.value <= 1 ? 0 : page.value / (pageCount.value - 1)
  }
  // 滚动：以滚动位置为单位
  const el = contentEl.value
  if (!el) return 0
  return el.scrollTop / Math.max(el.scrollHeight - el.clientHeight, 1)
}

function currentOffset() {
  // 翻页/分页：保存页码
  if (reader.mode === 'flip' || reader.mode === 'page') return page.value
  return Math.round(contentEl.value?.scrollTop || 0)
}

async function saveCurrentProgress() {
  if (!chapter.value) return
  await reader.saveProgress({
    bookId: bookId.value, chapterId: chapter.value.id,
    chapterIndex: currentIndex.value, offset: currentOffset(), percent: bookProgress.value,
  })
}

async function createBookmark() {
  if (!chapter.value) return
  const excerpt = lines.value.slice(0, 2).join(' ').slice(0, 140)
  const { data } = await api.post(`/books/${bookId.value}/bookmarks`, {
    chapterId: chapter.value.id, chapterIndex: currentIndex.value,
    offset: currentOffset(), percent: currentChapterPercent(),
    title: chapter.value.title, excerpt,
  })
  bookmarks.value = [data, ...bookmarks.value]
  toastMsg.value = '书签已创建'
  setTimeout(() => { toastMsg.value = '' }, 1600)
}

async function saveNote() {
  if (!chapter.value) return
  const note = noteText.value.trim()
  if (!note) return
  const excerpt = lines.value.slice(0, 2).join(' ').slice(0, 140)
  const { data } = await api.post(`/books/${bookId.value}/bookmarks`, {
    chapterId: chapter.value.id, chapterIndex: currentIndex.value,
    offset: currentOffset(), percent: currentChapterPercent(),
    title: chapter.value.title, excerpt, note,
  })
  bookmarks.value = [data, ...bookmarks.value]
  showNoteDialog.value = false
  toastMsg.value = '笔记已保存'
  setTimeout(() => { toastMsg.value = '' }, 1600)
}

async function removeBookmark(bookmark) {
  await api.delete(`/bookmarks/${bookmark.id}`)
  bookmarks.value = bookmarks.value.filter(item => item.id !== bookmark.id)
}

function openBookmarkEditor(bookmark) {
  editingBookmark.value = bookmark
  Object.assign(bookmarkDraft, {
    title: bookmark.title || '',
    excerpt: bookmark.excerpt || '',
    note: bookmark.note || '',
  })
  showBookmarkEditor.value = true
}

async function saveBookmarkEdit() {
  if (!editingBookmark.value) return
  savingBookmark.value = true
  try {
    const { data } = await api.put(`/bookmarks/${editingBookmark.value.id}`, {
      title: bookmarkDraft.title,
      excerpt: bookmarkDraft.excerpt,
      note: bookmarkDraft.note,
    })
    const index = bookmarks.value.findIndex(item => item.id === data.id)
    if (index >= 0) bookmarks.value[index] = data
    showBookmarkEditor.value = false
    toastMsg.value = '书签已更新'
    setTimeout(() => { toastMsg.value = '' }, 1600)
  } catch (err) {
    ElMessage.error(readError(err, '更新书签失败'))
  } finally {
    savingBookmark.value = false
  }
}

async function jumpToBookmark(bookmark) {
  showBookmarkDrawer.value = false
  const query = { chapter: bookmark.chapterIndex, offset: bookmark.offset || 0 }
  if (bookmark.chapterIndex === currentIndex.value) {
    await loadChapter(currentIndex.value, query.offset)
    return
  }
  await router.replace({ name: 'reader', params: { id: bookId.value }, query })
}

async function jumpToBookSearchResult(result) {
  showSearchDrawer.value = false
  const targetIndex = Number(result.chapterIndex || 0)
  if (targetIndex === currentIndex.value) {
    await loadChapter(targetIndex, 0)
  } else {
    await router.replace({ name: 'reader', params: { id: bookId.value }, query: { chapter: targetIndex } })
    await loadChapter(targetIndex, 0)
  }
  await nextTick()
  if (Number.isInteger(result.lineIndex)) {
    jumpToLine(result.lineIndex)
  } else {
    jumpToFirstSearchMatch()
  }
}

function jumpToFirstSearchMatch() {
  const keyword = contentSearch.value.trim().toLowerCase()
  if (!keyword || !contentBody.value) return
  const paragraphList = [...contentBody.value.querySelectorAll('p')]
  const index = paragraphList.findIndex(item => item.textContent.toLowerCase().includes(keyword))
  if (index >= 0) jumpToLine(index)
}

function jumpToLine(index) {
  const lineEl = contentBody.value?.querySelectorAll('p')?.[index]
  if (!lineEl) return
  showSearchDrawer.value = false
  if (reader.mode === 'flip' || reader.mode === 'page') {
    page.value = Math.min(pageCount.value - 1, Math.floor(lineEl.offsetTop / Math.max(pageHeight.value, 1)))
  } else if (contentEl.value) {
    contentEl.value.scrollTop = Math.max(0, lineEl.offsetTop - 80)
  }
  saveCurrentProgress()
}

async function jumpToRouteLine() {
  if (route.query.line === undefined) return
  const index = Number(route.query.line)
  if (!Number.isFinite(index)) return
  await nextTick()
  jumpToLine(Math.max(0, Math.floor(index)))
}

function scrollToTop() {
  if (reader.mode === 'flip' || reader.mode === 'page') { page.value = 0; return }
  if (contentEl.value) contentEl.value.scrollTop = 0
}

function scrollToBottom() {
  if (reader.mode === 'flip' || reader.mode === 'page') { page.value = Math.max(0, pageCount.value - 1); return }
  if (contentEl.value) contentEl.value.scrollTop = contentEl.value.scrollHeight
}

// ---- Keyboard ----
useKeyboard({
  onPageUp: () => previousPage(),
  onPageDown: () => nextPage(),
  onHome: () => scrollToTop(),
  onEnd: () => scrollToBottom(),
  onSpace: () => nextPage(),
  onEscape: () => {
    if (showTocDrawer.value || showSettingsDrawer.value) {
      showTocDrawer.value = false; showSettingsDrawer.value = false
    } else {
      router.push({ name: 'book-detail', params: { id: bookId.value } })
    }
  },
})

useGesture(pageEl, {
  onSwipeLeft: () => nextPage(),
  onSwipeRight: () => previousPage(),
  onCenterTap: () => {
    if (isMobileReader.value) {
      mobileChromeVisible.value = !mobileChromeVisible.value
      return
    }
    showTocDrawer.value = !showTocDrawer.value
    showSettingsDrawer.value = false
  },
  onEdgeLeftTap: () => previousPage(),
  onEdgeRightTap: () => nextPage(),
  onPinchOut: () => reader.setFontSize(reader.fontSize + 2),
  onPinchIn: () => reader.setFontSize(reader.fontSize - 2),
})

// ---- TTS ----
const tts = useTTS()
const ttsVoices = computed(() => tts.voices.value)
tts.setRate(reader.ttsRate)
tts.setPitch(reader.ttsPitch)
tts.setVoice(reader.ttsVoiceURI)

function setTTSRate(value) {
  reader.setTTSRate(value)
  tts.setRate(reader.ttsRate)
}

function setTTSPitch(value) {
  reader.setTTSPitch(value)
  tts.setPitch(reader.ttsPitch)
}

function setTTSVoice(value) {
  reader.setTTSVoice(value)
  tts.setVoice(reader.ttsVoiceURI)
}

function toggleTTS() {
  if (!tts.state.supported) {
    toastMsg.value = '当前浏览器不支持朗读'
    return
  }
  if (tts.state.playing) {
    tts.stop()
  } else {
    tts.speak(content.value, () => {
      if (currentIndex.value < chapters.value.length - 1) {
        goChapter(currentIndex.value + 1).then(() => setTimeout(() => tts.speak(content.value), 500))
      }
    })
  }
}
function ttsStop() { tts.stop() }

watch(() => tts.currentIndex.value, (idx) => {
  if (idx < 0 || !contentBody.value) return
  const ps = contentBody.value.querySelectorAll('p')
  ps.forEach(p => p.classList.remove('tts-active'))
  const t = ps[idx]
  if (t) { t.classList.add('tts-active'); t.scrollIntoView({ behavior: 'smooth', block: 'center' }) }
})

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
/* ---- 阅读器壳 — 羊皮纸 ---- */
.reader-shell {
  --reader-frame-width: min(var(--reader-read-width, 800px), calc(100vw - 150px));
  --reader-content-width: calc(var(--reader-frame-width) - 130px);
  --reader-left-x: calc(50vw - var(--reader-frame-width) / 2 - 68px);
  --reader-right-x: calc(50vw + var(--reader-frame-width) / 2 + 10px);
  --paper-texture:
    radial-gradient(circle at 16% 10%, rgba(255, 255, 255, 0.34), transparent 30%),
    radial-gradient(circle at 74% 30%, rgba(126, 95, 38, 0.06), transparent 34%),
    repeating-linear-gradient(90deg, rgba(118, 90, 36, 0.026) 0 1px, transparent 1px 7px);
  min-height: 100vh;
  display: grid;
  justify-content: center;
  background:
    linear-gradient(90deg, rgba(124, 99, 43, 0.16), transparent 18%, transparent 82%, rgba(124, 99, 43, 0.16)),
    repeating-linear-gradient(0deg, rgba(105, 83, 35, 0.035) 0 1px, transparent 1px 6px),
    #d9c27f;
}

/* ---- 左侧工具栏 ---- */
.reader-left-rail {
  position: fixed;
  top: 0;
  bottom: 0;
  left: max(8px, var(--reader-left-x));
  z-index: 4;
  width: 58px;
  display: grid;
  align-content: start;
  background: rgba(255, 250, 226, 0.5);
  border-left: 1px solid rgba(148, 132, 87, 0.26);
  border-right: 1px solid rgba(148, 132, 87, 0.38);
  backdrop-filter: blur(2px);
}

.rail-item {
  display: grid;
  width: 100%;
  height: 60px;
  place-items: center;
  align-content: center;
  gap: 2px;
  padding: 7px 0 5px;
  color: rgba(36, 33, 27, 0.62);
  background: rgba(255, 253, 240, 0.46);
  border: 0;
  border-bottom: 1px solid rgba(148, 132, 87, 0.35);
  cursor: pointer;
  font-size: 16px;
}

.rail-item span {
  font-size: 12px;
  line-height: 1;
}

.rail-item:hover {
  color: #1e1f22;
  background: rgba(255, 253, 240, 0.78);
}

.rail-home {
  height: 60px;
  color: #111;
}

/* ---- 右侧浮动工具 ---- */
.reader-right-rail {
  position: fixed;
  right: auto;
  left: var(--reader-right-x);
  bottom: 155px;
  z-index: 4;
  display: grid;
  align-content: start;
  grid-template-columns: 36px 36px;
  grid-auto-flow: column;
  grid-template-rows: repeat(5, 36px);
  gap: 20px 10px;
  overflow-y: auto;
  padding-right: 2px;
  scrollbar-width: none;
}

.reader-right-rail::-webkit-scrollbar {
  display: none;
}

.round-tool {
  display: grid;
  width: 36px;
  height: 36px;
  place-items: center;
  color: #121212;
  background: rgba(255, 249, 226, 0.9);
  border: 1px solid rgba(255, 255, 255, 0.7);
  border-radius: 999px;
  box-shadow: 0 4px 10px rgba(80, 62, 28, 0.08);
  cursor: pointer;
}

.round-tool:hover,
.round-tool.active {
  color: #0f5451;
  background: #fff9df;
  box-shadow: 0 12px 26px rgba(80, 62, 28, 0.14);
}

.round-tool:disabled {
  cursor: not-allowed;
  opacity: 0.42;
}

/* ---- 正文 ---- */
.reader-page {
  background-color: var(--reader-bg);
  background-image: var(--reader-bg-image, var(--paper-texture));
  background-size: cover; background-position: center;
  filter: brightness(var(--reader-brightness));
  color: var(--reader-text);
  border-left: 1px solid rgba(109,95,55,0.28);
  border-right: 1px solid rgba(109,95,55,0.28);
  box-shadow:
    inset 24px 0 44px rgba(90, 71, 28, 0.05),
    inset -24px 0 44px rgba(90, 71, 28, 0.05);
  height: 100vh;
  overflow: hidden;
  position: relative;
  width: var(--reader-frame-width);
}
.reader-page-head {
  align-items: center; color: rgba(36,40,44,0.45);
  display: flex; font-size: 14px; justify-content: space-between;
  padding: 10px 65px 0; pointer-events: none;
  position: absolute; left: 0; right: 0; top: 0; z-index: 1;
}
.reader-content {
  font-family: var(--reader-font-family);
  font-size: var(--reader-font-size);
  height: 100vh; line-height: var(--reader-line-height);
  overflow-y: auto; overflow-x: hidden;
  padding: 44px 65px 72px;
  width: var(--reader-content-width);
  box-sizing: content-box;
}
.reader-body { transition: transform 180ms ease; }
.reader-content h1 {
  font-size: var(--reader-heading-size);
  line-height: 1.35;
  margin: 0 0 76px;
  text-align: center;
}
.reader-content p {
  margin: 0 0 var(--reader-paragraph-space);
  font-weight: var(--reader-font-weight);
  text-indent: 2em;
}

/* 翻页 & 分页模式：隐藏滚动条，用 translateY 切页 */
.reader-shell.flip .reader-content,
.reader-shell.page .reader-content {
  overflow: hidden;
}
.reader-shell.flip .reader-body,
.reader-shell.page .reader-body {
  transition: transform 250ms ease;
}

/* ---- 右下翻页控制 ---- */
.reader-page-control {
  position: fixed;
  right: auto;
  left: calc(50vw + var(--reader-frame-width) / 2 + 52px);
  bottom: 0;
  z-index: 4;
  display: grid;
  width: 42px;
  background: rgba(255, 250, 226, 0.72);
  border: 1px solid rgba(148, 132, 87, 0.38);
  border-bottom: 0;
}

.progress-box,
.page-step {
  display: grid;
  height: 43px;
  place-items: center;
  color: #121212;
  background: rgba(255, 253, 240, 0.44);
  border: 0;
  border-bottom: 1px solid rgba(148, 132, 87, 0.32);
  font-size: 16px;
}

.page-step {
  cursor: pointer;
}

.page-step:hover {
  background: rgba(255, 253, 240, 0.82);
}

.reader-mobile-bottom {
  display: none;
}

.reader-mobile-top {
  display: none;
}

/* ---- TTS ---- */
.tts-bar {
  align-items: center; background: rgba(64,158,255,0.9);
  border-radius: 10px; bottom: 16px; color: #fff;
  display: flex; gap: 8px; left: 50%; padding: 10px 18px;
  position: fixed; transform: translateX(-50%); z-index: 6;
}
.tts-btn { color: #fff !important; font-size: 18px; }
.tts-label { color: rgba(255,255,255,0.7); font-size: 12px; }
.tts-slider { width: 60px; accent-color: #fff; }

/* ---- Toast ---- */
.reader-toast {
  background: rgba(30, 41, 59, 0.92); border-radius: 8px; bottom: 96px; color: #fff;
  left: 50%; padding: 10px 18px; position: fixed;
  transform: translateX(-50%); z-index: 5; font-size: 14px;
}

.reader-drawer-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin: -2px 0 14px;
}
.reader-drawer-title span {
  color: #ed4259;
  border-bottom: 1px solid #ed4259;
  font-size: 18px;
}
.reader-drawer-title button {
  padding: 0;
  color: #ed4259;
  background: transparent;
  border: 0;
  cursor: pointer;
  font-size: 14px;
}
.reader-drawer-title button:disabled {
  color: #8c8c8c;
  cursor: default;
}
.shelf-search { margin-bottom: 12px; }
.reader-shelf-list { display: grid; }
.reader-shelf-card {
  display: grid;
  grid-template-columns: 42px minmax(0, 1fr);
  gap: 10px;
  align-items: start;
  width: 100%;
  padding: 10px 0;
  color: #24282c;
  background: transparent;
  border: 0;
  border-bottom: 1px solid rgba(160, 139, 91, 0.22);
  cursor: pointer;
  text-align: left;
}
.reader-shelf-cover {
  display: grid;
  width: 42px;
  height: 56px;
  place-items: center;
  overflow: hidden;
  border-radius: 4px;
  font-size: 18px;
  font-weight: 800;
}
.reader-shelf-card:hover,
.reader-shelf-card.active {
  color: #ed4259;
  background: transparent;
}
.reader-shelf-main {
  display: grid;
  min-width: 0;
  gap: 6px;
}
.reader-shelf-title-line {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.reader-shelf-title-line strong,
.reader-shelf-main small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.reader-shelf-title-line strong {
  min-width: 0;
  font-size: 16px;
  font-weight: 500;
}
.reader-shelf-title-line em {
  flex: 0 0 auto;
  color: #ed4259;
  font-size: 12px;
  font-style: normal;
}
.reader-shelf-main small {
  color: #7b715e;
  font-size: 12px;
}
/* ---- 编辑弹层 ---- */
.bookmark-editor {
  display: grid;
  gap: 10px;
}

.empty-hint { color: #999; text-align: center; padding-top: 40px; text-indent: 0; }

/* ---- 响应式 ---- */
@media (max-width: 680px) {
  .reader-shell {
    --reader-frame-width: 100vw;
    --reader-content-width: calc(100vw - 44px);
    overflow: hidden;
    padding: 0;
  }
  .reader-page { border: 0; width: 100vw; }
  .reader-page-head { display: none; }
  .reader-content {
    box-sizing: border-box;
    width: 100vw;
    font-size: var(--reader-font-size);
    padding: 42px 22px 58px;
  }
  .reader-content h1 { font-size: var(--reader-heading-size); margin-bottom: 28px; }
  .reader-left-rail,
  .reader-right-rail,
  .reader-page-control {
    display: none;
  }
  .reader-mobile-top {
    position: fixed;
    top: 0;
    right: 0;
    left: 0;
    z-index: 8;
    display: grid;
    grid-template-columns: 44px minmax(0, 1fr) 52px;
    align-items: center;
    gap: 8px;
    min-height: 58px;
    padding: max(8px, env(safe-area-inset-top)) 12px 8px;
    background: rgba(255, 252, 239, 0.94);
    border-bottom: 1px solid rgba(148, 132, 87, 0.28);
    box-shadow: 0 8px 24px rgba(73, 57, 27, 0.08);
    transform: translateY(-110%);
    transition: transform 180ms ease;
  }
  .mobile-reader-title {
    display: grid;
    min-width: 0;
    gap: 2px;
    color: #25282c;
  }
  .mobile-reader-title strong,
  .mobile-reader-title span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .mobile-reader-title strong {
    font-size: 14px;
  }
  .mobile-reader-title span,
  .mobile-reader-progress {
    color: #756c5a;
    font-size: 12px;
  }
  .mobile-reader-progress {
    text-align: right;
  }
  .reader-mobile-bottom {
    position: fixed;
    right: 0;
    bottom: 0;
    left: 0;
    z-index: 8;
    display: grid;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    align-items: center;
    gap: 4px;
    padding: 8px 10px max(8px, env(safe-area-inset-bottom));
    background: rgba(255, 252, 239, 0.92);
    border-top: 1px solid rgba(148, 132, 87, 0.35);
    box-shadow: 0 -8px 24px rgba(73, 57, 27, 0.08);
    transform: translateY(110%);
    transition: transform 180ms ease;
  }
  .reader-shell.mobile-chrome-visible .reader-mobile-top,
  .reader-shell.mobile-chrome-visible .reader-mobile-bottom {
    transform: translateY(0);
  }
  .mobile-tool-button {
    display: grid;
    min-width: 0;
    min-height: 44px;
    place-items: center;
    gap: 3px;
    padding: 6px 4px;
    color: #111;
    background: transparent;
    border: 0;
    border-radius: 6px;
    font-size: 12px;
  }
  .mobile-tool-button:active,
  .mobile-more-item:active {
    background: rgba(114, 91, 43, 0.1);
  }
  .mobile-more-grid {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 10px;
    padding: 4px 0 10px;
  }
  .mobile-more-item {
    display: grid;
    min-height: 72px;
    place-items: center;
    align-content: center;
    gap: 7px;
    color: #232323;
    background: #fffaf0;
    border: 1px solid #eee4c9;
    border-radius: 8px;
    font-size: 13px;
  }
  .mobile-more-item.active {
    color: #0f5451;
    border-color: #0f5451;
    background: #fff7dc;
  }
  .mobile-more-item:disabled {
    cursor: not-allowed;
    opacity: 0.42;
  }
  .mobile-more-hint {
    margin: 4px 0 0;
    color: #8a8171;
    font-size: 12px;
    line-height: 1.6;
  }
  .tts-bar {
    right: 10px;
    bottom: max(74px, calc(env(safe-area-inset-bottom) + 74px));
    left: 10px;
    justify-content: center;
    overflow-x: auto;
    transform: none;
  }
}
</style>
