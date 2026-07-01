<template>
  <main ref="shellEl" class="reader-shell" :class="[reader.mode, { 'mobile-chrome-visible': mobileChromeVisible }]" :style="readerStyle">
    <ReaderDesktopTools
      :remote-book="isRemoteBook"
      :auto-reading="autoReading"
      :tts-playing="tts.state.playing"
      :tts-supported="tts.state.supported"
      @action="handleDesktopToolAction"
    />

    <ReaderMobileChrome
      :visible="mobileChromeVisible"
      :book-title="book?.title || '阅读中'"
      :chapter-title="displayChapterTitle(chapter?.title) || chapterLabel"
      :book-progress-label="bookProgressLabel"
      :chapter-label="chapterLabel"
      :book-slider-value="mobileBookSliderValue"
      :book-slider-label="mobileBookProgressLabel"
      :previous-disabled="currentIndex <= 0"
      :next-disabled="currentIndex >= chapters.length - 1"
      @action="handleMobileChromeAction"
      @book-progress-input="handleMobileBookProgressInput"
      @book-progress-change="handleMobileBookProgressChange"
    />

    <section
      ref="pageEl"
      class="reader-page"
      :style="readerStyle"
      @touchstart.passive="handleReaderTouchStart"
      @touchmove="handleReaderTouchMove"
      @touchend.passive="handleReaderTouchEnd"
      @wheel="handleReaderWheel"
      @click="handleReaderContentClick"
    >
      <header class="reader-page-head">
        <span>{{ book?.title || '阅读中' }}</span>
        <span>{{ chapterLabel }}</span>
      </header>

      <article
        ref="contentEl"
        class="reader-content"
        :style="readerContentStyle"
        @scroll.passive="onScroll"
        @mouseup="handleReaderSelectionEnd"
      >
        <div ref="contentBody" class="reader-body" :style="bodyStyle">
          <ReaderChapterContent
            :blocks="displayedChapterBlocks"
            :error="chapterLoadError"
            :loaded="chapterLoaded"
            :loading="chapterLoading"
            :mode="reader.mode"
            @reload="reloadChapter"
          />
        </div>
      </article>
      <ReaderClickZones
        :mode="reader.mode"
        :show-overlay="showClickZoneOverlay"
        @tap="handleTapZone"
        @close-overlay="showClickZoneOverlay = false"
      />
    </section>

    <ReaderDesktopProgress
      :book-progress-label="bookProgressLabel"
      :chapter-slider-value="desktopChapterSliderValue"
      :chapter-progress-label="desktopChapterProgressLabel"
      :previous-disabled="currentIndex <= 0"
      :next-disabled="currentIndex >= chapters.length - 1"
      @previous="goChapter(currentIndex - 1)"
      @next="goChapter(currentIndex + 1)"
      @chapter-progress-input="handleDesktopProgressInput"
      @chapter-progress-change="handleDesktopProgressChange"
    />

    <!-- TTS 朗读条 -->
    <ReaderTTSBar
      v-if="tts.state.playing"
      :paused="tts.state.paused"
      :rate="tts.state.rate"
      :pitch="tts.state.pitch"
      :sleep-minutes="ttsSleepMinutes"
      :progress-text="ttsProgressLabel"
      @backward="tts.skipBackward"
      @pause="tts.pause"
      @resume="tts.resume"
      @forward="tts.skipForward"
      @stop="ttsStop"
      @rate-change="setTTSRate"
      @pitch-change="setTTSPitch"
      @sleep-change="setTTSSleepMinutes"
    />

    <!-- Toast -->
    <div v-if="toastMsg" class="reader-toast">{{ toastMsg }}</div>

    <!-- ===== 书架抽屉 ===== -->
    <el-drawer v-model="showShelfDrawer" title="书架" :direction="drawerDirection" :size="shelfDrawerSize" @opened="locateReaderShelfCurrentBook">
      <div class="reader-drawer-title">
        <span>书架({{ filteredShelfBooks.length }})</span>
        <button type="button" :disabled="shelfLoading" @click="refreshReaderShelf">
          {{ shelfLoading ? '刷新中...' : '刷新' }}
        </button>
      </div>
      <ReaderShelfPanel
        ref="shelfPanelRef"
        v-loading="shelfLoading"
        :books="filteredShelfBooks"
        :current-book-id="bookId"
        :progress-by-book="reader.progressByBook"
        :loading="shelfLoading"
        @select="changeBookFromShelf"
      />
    </el-drawer>

    <!-- ===== 目录抽屉 ===== -->
    <el-drawer v-model="showTocDrawer" title="目录" :direction="drawerDirection" :size="drawerSize" @opened="locateTocCurrentChapter">
      <div class="reader-drawer-title">
        <span>目录({{ chapters.length }})</span>
        <div class="reader-drawer-actions">
          <button v-if="chapters.length" type="button" @click="toggleTocReverse">{{ tocReverse ? '顺序' : '倒序' }}</button>
          <button v-if="chapters.length" type="button" @click="scrollTocTop">顶部</button>
          <button v-if="chapters.length" type="button" @click="scrollTocBottom">底部</button>
          <button v-if="canChangeLocalTocRule" type="button" :disabled="tocRefreshing" @click="changeReaderLocalTocRule">修改规则</button>
          <button type="button" :disabled="tocRefreshing" @click="refreshTocDrawer">{{ tocRefreshing ? '刷新中...' : '刷新' }}</button>
        </div>
      </div>
      <ReaderTocPanel
        ref="tocPanelRef"
        :chapters="chapters"
        :current-index="currentIndex"
        :reverse="tocReverse"
        :locate-key="tocLocateKey"
        :browser-cached-map="browserCachedChapters"
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
        @remove-many="removeBookmarks"
        @import="importBookmarks"
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
        @load-all="searchAllBookContent"
        @jump="jumpToBookSearchResult"
      />
    </el-drawer>

    <!-- ===== 书源抽屉 ===== -->
    <el-drawer v-model="showSourceDrawer" title="书源" :direction="drawerDirection" :size="drawerSize" @open="ensureSourceCandidates">
      <SourceSwitchPanel
        :book="book"
        :sources="sourceCandidates"
        :loading="loadingSources"
        :changing-source="changingSource"
        :current-source-name="currentSourceName"
        :group="sourceGroup"
        :groups="sourceGroups"
        :has-more="sourceHasMore"
        @refresh="refreshSourceCandidates"
        @load-more="loadMoreSourceCandidates"
        @group-change="changeSourceGroup"
        @change="changeSource"
      />
    </el-drawer>

    <!-- ===== 移动端更多 ===== -->
    <el-drawer v-model="showMobileMoreDrawer" title="阅读工具" direction="btt" size="72%" class="mobile-more-drawer">
      <ReaderMobileToolsPanel
        :remote-book="isRemoteBook"
        :auto-reading="autoReading"
        :tts-playing="tts.state.playing"
        :tts-supported="tts.state.supported"
        @action="handleMobileToolAction"
      />
    </el-drawer>

    <!-- ===== 缓存抽屉 ===== -->
    <el-drawer v-model="showCacheDrawer" title="缓存章节" :direction="drawerDirection" :size="drawerSize">
      <ReaderCachePanel
        :caching="isCachingContent"
        :status-text="cachingContentTip"
        @cache="cacheFollowingChapters"
        @cancel="cancelCachingContent"
      />
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
        :mini-interface="isMobileReader"
        @mode-change="onModeChange"
        @theme-change="setTheme"
        @pick-bg-image="pickBgImage"
        @clear-bg-image="clearBgImage"
        @pick-font-file="pickFontFile"
        @clear-font-file="clearFontFile"
        @tts-rate-change="setTTSRate"
        @tts-pitch-change="setTTSPitch"
        @tts-voice-change="setTTSVoice"
        @open-replace-rules="openReplaceRules"
        @show-click-zone="showClickZone"
      />
    </el-drawer>

    <ReaderBookmarkFormDialog
      v-model="showNoteDialog"
      v-model:note="noteText"
      dialog-title="添加笔记"
      width="360px"
      note-placeholder="写下当前阅读位置的笔记..."
      @save="saveNote"
    />

    <ReaderBookmarkFormDialog
      v-model="showBookmarkEditor"
      v-model:title="bookmarkDraft.title"
      v-model:excerpt="bookmarkDraft.excerpt"
      v-model:note="bookmarkDraft.note"
      dialog-title="编辑书签"
      show-details
      :saving="savingBookmark"
      @save="saveBookmarkEdit"
    />
  </main>
</template>

<script setup>
import { computed, h, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api/client'
import { refreshBook, refreshLocalBook } from '../api/books'
import { createReplaceRule } from '../api/replaceRules'
import { listSources } from '../api/sources'
import { deleteAsset, uploadAsset } from '../api/uploads'
import ReaderBookmarkFormDialog from '../components/reader/ReaderBookmarkFormDialog.vue'
import ReaderBookmarkPanel from '../components/reader/ReaderBookmarkPanel.vue'
import ReaderCachePanel from '../components/reader/ReaderCachePanel.vue'
import ReaderChapterContent from '../components/reader/ReaderChapterContent.vue'
import ReaderClickZones from '../components/reader/ReaderClickZones.vue'
import ReaderDesktopProgress from '../components/reader/ReaderDesktopProgress.vue'
import ReaderDesktopTools from '../components/reader/ReaderDesktopTools.vue'
import ReaderMobileChrome from '../components/reader/ReaderMobileChrome.vue'
import ReaderMobileToolsPanel from '../components/reader/ReaderMobileToolsPanel.vue'
import ReaderSearchPanel from '../components/reader/ReaderSearchPanel.vue'
import ReaderShelfPanel from '../components/reader/ReaderShelfPanel.vue'
import ReaderSettingsPanel from '../components/reader/ReaderSettingsPanel.vue'
import ReaderTTSBar from '../components/reader/ReaderTTSBar.vue'
import SourceSwitchPanel from '../components/reader/SourceSwitchPanel.vue'
import ReaderTocPanel from '../components/reader/ReaderTocPanel.vue'
import { mergeShelfBook, useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore, themePresets } from '../stores/reader'
import { useKeyboard } from '../composables/useKeyboard'
import { useGesture } from '../composables/useGesture'
import { useAutoReading } from '../composables/useAutoReading'
import { useBookBookmarks } from '../composables/useBookBookmarks'
import { useBookContentSearch } from '../composables/useBookContentSearch'
import { useBookSourceChange } from '../composables/useBookSourceChange'
import { useBookSourceCandidates } from '../composables/useBookSourceCandidates'
import { useReaderChapterCache } from '../composables/useReaderChapterCache'
import { useReaderProgressPersistence } from '../composables/useReaderProgressPersistence'
import { useReaderBookmarkActions } from '../composables/useReaderBookmarkActions'
import { useReaderSelection } from '../composables/useReaderSelection'
import { useReaderShelf } from '../composables/useReaderShelf'
import { useReaderToc } from '../composables/useReaderToc'
import { useReaderTTS } from '../composables/useReaderTTS'
import { useReaderViewportProgress } from '../composables/useReaderViewportProgress'
import { bookCategoryIds, createBookCategoryNameResolver } from '../utils/bookCategory'
import { chapterCacheBookKey, clearBookBrowserChapterCache, isValidChapterContentResponse, loadBrowserChapterContent } from '../utils/bookChapterCache'
import { cacheFirstRequest, networkFirstRequest } from '../utils/browserCache'
import { simplized, traditionalized } from '../utils/chinese'
import { epubTocRuleOptions, isEPUBLocalBook as checkEPUBLocalBook, isTextLocalBook as checkTextLocalBook } from '../utils/localBookToc'
import { readerFontOptions, readerFontStack, syncReaderFontFaces } from '../utils/readerFonts'
import {
  didReaderTouchMove,
  isReaderTouchTap,
  MOBILE_READER_TAP_MOVE_TOLERANCE,
  normalizedReaderWheelDelta,
  readerTapPointAction,
  readerTapZoneAction,
  shouldHandleReaderHorizontalSwipe,
  shouldPreventReaderTouchMove,
} from '../utils/readerInteraction'
import {
  clampReaderPercent,
  readerBookProgress,
  readerBookSeekTarget,
  readerFlipPageLayout,
  readerScrollBehaviorForDuration,
  readerScrollStep,
  readerVerticalPageLayout,
} from '../utils/readerPagination'
import {
  readerChapterBoundaryScrollTop,
  readerParagraphAtPosition,
  READER_CHAPTER_END_OFFSET,
  restoredReaderContinuousScrollTop,
  restoredReaderFlipPage,
  restoredReaderSingleChapterScrollTop,
} from '../utils/readerPosition'
import { savedBookChapterPercent } from '../utils/readerRoute'
import { parseReaderContentBlocks } from '../utils/readerContent'
import {
  adjacentReaderChapterIndex,
  nearbyReaderChapterIndexes,
  readerChapterWindowExtension,
  readerChapterWindowIndexes,
  readerChapterWindowPrunePlan,
} from '../utils/readerChapterWindow'
import {
  readerProgressBaseUpdatedAt,
  readerProgressPayload,
} from '../utils/readerProgressPersistence'
import { currentViewportWidth, shouldUseMiniInterface } from '../utils/responsive'
import { invalidateReaderDataCache as invalidateReaderCache, readerDataCacheKey as scopedReaderDataCacheKey, writeReaderDataCache as writeReaderCache } from '../utils/readerDataCache'
import { createMultiBookChapterMemoryCache } from '../utils/multiBookChapterMemoryCache'
import { sourceCandidateSourceName } from '../utils/sourceCandidate'

const route = useRoute()
const router = useRouter()
const reader = useReaderStore()
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const categoryName = createBookCategoryNameResolver(() => bookshelf.categories)
const bookId = computed(() => Number(route.params.id))

const book = ref(null)
const chapters = ref([])
const chapter = ref(null)
const {
  items: bookmarks,
  mutating: savingBookmark,
  load: loadBookmarks,
  create: addBookmark,
  update: updateBookmarkData,
  remove: removeBookmarkData,
  removeMany: removeBookmarkRows,
  importPayloads: importBookmarkPayloads,
  handleUpdated: handleBookmarksUpdated,
} = useBookBookmarks({
  bookId,
  onLoadError: error => ElMessage.error(readError(error, '加载书签失败')),
})
const {
  draft: bookmarkDraft,
  editorVisible: showBookmarkEditor,
  noteText,
  noteVisible: showNoteDialog,
  createCurrent: createBookmark,
  createFromSelectedText: createBookmarkFromSelectedText,
  importRows: importBookmarks,
  jump: jumpToBookmark,
  openEditor: openBookmarkEditor,
  openNote: openNoteDialog,
  removeMany: removeBookmarks,
  removeOne: removeBookmark,
  saveEdit: saveBookmarkEdit,
  saveNote,
} = useReaderBookmarkActions({
  chapter,
  currentIndex,
  getOffset: () => currentOffset(),
  getPercent: () => currentChapterPercent(),
  getExcerpt: currentVisibleExcerpt,
  create: addBookmark,
  update: updateBookmarkData,
  remove: removeBookmarkData,
  removeMany: removeBookmarkRows,
  importPayloads: importBookmarkPayloads,
  confirm: (...args) => ElMessageBox.confirm(...args),
  closeDrawer: () => {
    showBookmarkDrawer.value = false
  },
  reloadCurrent: ({ offset, percent }) => loadChapter(
    currentIndex.value,
    offset,
    { restorePercent: percent, saveAfterLoad: true },
  ),
  navigate: query => router.replace({
    name: 'reader',
    params: { id: bookId.value },
    query,
  }),
  onToast: message => showReaderToast(message),
  onSuccess: message => ElMessage.success(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})
const content = ref('')
const chapterBlocks = ref([])
const chapterLoading = ref(true)
const chapterLoadError = ref('')
const chapterLoaded = ref(false)
const contentEl = ref(null)
const contentBody = ref(null)
const {
  consumeSuppressedContentClick,
  schedule: scheduleSelectedTextOperation,
  suppressContentClick,
} = useReaderSelection({
  contentBody,
  getAction: () => reader.selectionAction,
  onOperate: operateSelectedText,
  onError: error => ElMessage.error(readError(error, '处理选中文字失败')),
})
const pageEl = ref(null)
const shellEl = ref(null)
const currentIndex = ref(Number(route.query.chapter || 0))
const page = ref(0)
const pageCount = ref(1)
const showSettingsDrawer = ref(false)
const showBookmarkDrawer = ref(false)
const showSearchDrawer = ref(false)
const showSourceDrawer = ref(false)
const showMobileMoreDrawer = ref(false)
const showCacheDrawer = ref(false)
const showClickZoneOverlay = ref(false)
const mobileBookSliderDraft = ref(null)
const sourceGroupOptions = ref([])
const {
  candidates: sourceCandidates,
  loading: loadingSources,
  group: sourceGroup,
  hasMore: sourceHasMore,
  groups: sourceGroups,
  ensure: ensureSourceCandidates,
  refresh: refreshSourceCandidates,
  loadMore: loadMoreSourceCandidates,
  changeGroup: changeSourceGroup,
  reset: resetSourceCandidates,
} = useBookSourceCandidates({
  bookId,
  groupSources: sourceGroupOptions,
  loadGroupSources: async () => {
    const { data } = await listSources()
    return (data || []).filter(source => source.enabled)
  },
  onError: error => ElMessage.error(readError(error, '搜索可用来源失败')),
  onInfo: message => ElMessage.info(message),
})
const {
  changingSource,
  change: changeSource,
} = useBookSourceChange({
  book,
  bookId,
  onChanged: applyReaderSourceChange,
  onSuccess: (_data, source) => ElMessage.success(`已切换到 ${sourceCandidateSourceName(source)}`),
  onError: error => ElMessage.error(readError(error, '换源失败')),
})
const {
  visible: showShelfDrawer,
  loading: shelfLoading,
  panelRef: shelfPanelRef,
  books: filteredShelfBooks,
  open: openShelfPanel,
  locateCurrentBook: locateReaderShelfCurrentBook,
  select: changeBookFromShelf,
  refresh: refreshReaderShelf,
} = useReaderShelf({
  bookshelf,
  reader,
  currentBookId: bookId,
  currentChapterCount: () => chapters.value.length,
  router,
  beforeOpen: () => {
    mobileChromeVisible.value = false
  },
  saveProgress: () => saveCurrentProgress({ force: true }),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})
const {
  keyword: contentSearch,
  results: bookSearchResults,
  loading: bookSearching,
  searched: searchedBookContent,
  hasMore: bookSearchHasMore,
  status: bookSearchStatus,
  reset: resetContentSearchState,
  search: searchBookContent,
  loadMore: loadMoreBookContent,
  loadAll: searchAllBookContent,
} = useBookContentSearch({
  bookId,
  book,
  chapters,
  onError: error => ElMessage.error(readError(error, '搜索正文失败')),
})
const toastMsg = ref('')
const progressVersion = ref(0)
const customBg = ref('')
const sliderLineHeight = ref(2.12)
const pageHeight = ref(600)
const pageWidth = ref(600)
const windowWidth = ref(currentViewportWidth())
let chapterLoadingTimer
let restoringPosition = false
const chapterContentCache = createMultiBookChapterMemoryCache(3)
let readerTouchStart = null
let readerTouchMoved = false
let readerTouchMove = { x: 0, y: 0 }
let handledTouchTapAt = 0
let lastLocalProgressKey = ''
let lastWheelPageAt = 0
let extendingShowChapters = false

const fontOptions = readerFontOptions
const SHOW_PREV_CHAPTER_SIZE = 1
const SHOW_NEXT_CHAPTER_SIZE = 2

const currentSourceName = computed(() => {
  if (!book.value?.sourceId) return '本地书籍'
  return sourceGroupOptions.value.find(source => Number(source.id) === Number(book.value.sourceId))?.name || '当前来源'
})
const isRemoteBook = computed(() => Number(book.value?.sourceId || 0) > 0)
const isTextLocalBook = computed(() => checkTextLocalBook(book.value))
const isEPUBLocalBook = computed(() => checkEPUBLocalBook(book.value))
const canChangeLocalTocRule = computed(() => isTextLocalBook.value || isEPUBLocalBook.value)
const {
  visible: showTocDrawer,
  panelRef: tocPanelRef,
  locateKey: tocLocateKey,
  reverse: tocReverse,
  refreshing: tocRefreshing,
  open: openTocDrawer,
  locateCurrentChapter: locateTocCurrentChapter,
  toggleReverse: toggleTocReverse,
  scrollTop: scrollTocTop,
  scrollBottom: scrollTocBottom,
  jump: jumpFromToc,
  refresh: refreshTocDrawer,
  runRefreshing: runTocRefreshing,
} = useReaderToc({
  chapters,
  isRemoteBook,
  beforeOpen: () => {
    mobileChromeVisible.value = false
  },
  refreshCachedChapters: computeBrowserCachedChapters,
  syncCurrentChapter: updateCurrentChapterFromScroll,
  goChapter,
  refreshRemoteCatalog: refreshReaderBookCatalog,
  refreshLocalCatalog: loadChapters,
})
const {
  cachedChapters: browserCachedChapters,
  caching: isCachingContent,
  statusText: cachingContentTip,
  refresh: computeBrowserCachedChapters,
  markCached: markBrowserChapterCached,
  reset: resetBrowserCachedChapters,
  cacheFollowing: cacheFollowingChapters,
  cancel: cancelCachingContent,
  clearBrowserCache: clearCurrentBookBrowserCache,
} = useReaderChapterCache({
  book,
  bookId,
  chapters,
  currentIndex,
  isRemoteBook,
  afterCache: loadChapters,
  onClearMemory: () => chapterContentCache.clearBook(currentChapterCacheBookKey()),
  notify: showChapterCacheMessage,
  onNoTargets: () => ElMessage.error('不需要缓存'),
  onError: error => ElMessage.error(readError(error, '缓存章节失败')),
})

const chapterParagraphs = computed(() => {
  return makeParagraphs(content.value, chapter.value?.title)
})
const lines = computed(() => chapterParagraphs.value.filter(item => item.type === 'text').map(item => item.text))
const chapterTextLength = computed(() => {
  return chapterBlockTextLength({ paragraphs: chapterParagraphs.value })
})
const isVerticalPagedRead = computed(() => reader.mode === 'page')
const isScrollRead = computed(() => reader.mode === 'scroll' || reader.mode === 'scroll2')
const isVerticalRead = computed(() => isVerticalPagedRead.value || isScrollRead.value)
const isContinuousScrollRead = computed(() => reader.mode === 'scroll' || reader.mode === 'scroll2')
const displayedChapterBlocks = computed(() => {
  if (isContinuousScrollRead.value && chapterBlocks.value.length) return chapterBlocks.value
  return [makeChapterBlock(currentIndex.value, chapter.value, content.value)]
})
const {
  activeChapterElement,
  captureReaderScrollAnchor,
  currentChapterPercent,
  currentChapterPosition,
  currentOffset,
  currentVisibleParagraph,
  restoreReaderScrollAnchor,
  visibleChapterProgressSnapshot,
} = useReaderViewportProgress({
  contentEl,
  contentBody,
  chapterBlocks,
  displayedChapterBlocks,
  chapters,
  currentIndex,
  chapter,
  content,
  chapterTextLength,
  progressVersion,
  page,
  pageCount,
  isContinuousScrollRead,
  getMode: () => reader.mode,
  makeChapterBlock,
  chapterBlockTextLength,
  nextFrame,
})

const fontStack = computed(() => {
  return readerFontStack(reader.fontFamily, reader.customFontsMap)
})

const readerStyle = computed(() => ({
  '--reader-font-family': fontStack.value,
  '--reader-font-size': `${reader.fontSize}px`,
  '--reader-heading-size': `${Math.round(reader.fontSize * 1.36)}px`,
  '--reader-body-bg': reader.customBodyColor || '#d9c27f',
  '--reader-popup-bg': reader.customPopupColor || 'rgba(255, 252, 239, 0.94)',
  '--reader-bg': reader.currentTheme.bg,
  '--reader-text': reader.fontColor || reader.currentTheme.text,
  '--reader-font-weight': reader.fontWeight,
  '--reader-brightness': `${reader.brightness}%`,
  '--reader-line-height': reader.lineHeight,
  '--reader-paragraph-space': `${reader.paragraphSpace}em`,
  '--reader-read-width': `${reader.columnWidth}px`,
  '--reader-bg-image': reader.customBgImage ? `url(${reader.customBgImage})` : '',
  '--reader-animate-duration': `${reader.animateDuration}ms`,
}))

const readerContentStyle = computed(() => ({
  fontFamily: fontStack.value,
  fontSize: `${reader.fontSize}px`,
  lineHeight: reader.lineHeight,
}))

const bodyStyle = computed(() => {
  const baseStyle = {
    fontFamily: fontStack.value,
    fontSize: `${reader.fontSize}px`,
    lineHeight: reader.lineHeight,
    fontWeight: reader.fontWeight,
  }
  if (reader.mode === 'flip') {
    return {
      ...baseStyle,
      '--reader-page-width': `${pageWidth.value}px`,
      transform: `translateX(-${page.value * pageWidth.value}px)`,
    }
  }
  return baseStyle
})

const chapterLabel = computed(() => `${currentIndex.value + 1} / ${chapters.value.length || 1}`)
const isMobileReader = computed(() => shouldUseMiniInterface(reader.pageMode, windowWidth.value))
const drawerDirection = computed(() => isMobileReader.value ? 'btt' : 'rtl')
const drawerSize = computed(() => isMobileReader.value ? '88%' : '360px')
const shelfDrawerSize = computed(() => isMobileReader.value ? '88%' : 'min(900px, calc(100vw - 80px))')
const bookProgress = computed(() => {
  return readerBookProgress({
    chapterIndex: currentIndex.value,
    chapterPercent: currentChapterPercent(),
    totalChapters: chapters.value.length,
  })
})
const bookProgressLabel = computed(() => `${Math.round(bookProgress.value * 100)}%`)
const mobileBookSliderValue = computed(() => {
  if (mobileBookSliderDraft.value !== null) return mobileBookSliderDraft.value
  return Math.round(bookProgress.value * 1000)
})
const mobileBookProgressLabel = computed(() => `${Math.round(Number(mobileBookSliderValue.value || 0) / 10)}%`)
const desktopChapterSliderValue = computed(() => {
  progressVersion.value
  return Math.round(Math.max(0, Math.min(1, currentChapterPercent())) * 1000)
})
const desktopChapterProgressLabel = computed(() => `${Math.round(desktopChapterSliderValue.value / 10)}%`)
const mobileChromeVisible = ref(false)
const NEARBY_PRELOAD_RADIUS = 2

const isOverlayOpen = computed(() => (
  showTocDrawer.value ||
  showSettingsDrawer.value ||
  showBookmarkDrawer.value ||
  showSearchDrawer.value ||
  showShelfDrawer.value ||
  showSourceDrawer.value ||
  showMobileMoreDrawer.value ||
  showCacheDrawer.value ||
  showNoteDialog.value ||
  showBookmarkEditor.value
))

const {
  active: autoReading,
  stop: stopAutoReading,
  toggle: toggleAutoReading,
} = useAutoReading({
  contentEl,
  contentBody,
  isVerticalRead,
  shouldPause: () => isOverlayOpen.value || mobileChromeVisible.value,
  settings: () => ({
    method: reader.autoReadingMethod,
    pixel: reader.autoReadingPixel,
    interval: reader.autoReadingLineTime,
    fontSize: reader.fontSize,
    lineHeight: reader.lineHeight,
  }),
  currentVisibleParagraph,
  scrollBehavior: readerScrollBehavior,
  advancePage: advanceAutoReadingPage,
  onProgress: recordAutoReadingProgress,
  onNotify: showAutoReadingMessage,
})

const {
  cancelScheduled: cancelProgressSave,
  isBusy: isProgressSaveBusy,
  key: progressSaveKey,
  markSaved: markProgressSaved,
  save: saveCurrentProgress,
  schedule: scheduleProgressSave,
} = useReaderProgressPersistence({
  minimumInterval: 1200,
  getPayload: () => chapter.value ? currentProgressPayload() : null,
  getBaseUpdatedAt: progressServerBaseUpdatedAt,
  applyLocal: applyLocalProgressSnapshot,
  saveRemote: payload => reader.saveProgress(payload),
  onSaved: progress => upsertReaderBookProgress(progress, { replace: true }),
  getMode: () => reader.mode,
  getStoredProgress: targetBookId => reader.progressByBook[targetBookId],
  ensureClientId: () => reader.ensureClientId(),
})

const {
  tts,
  voices: ttsVoices,
  sleepMinutes: ttsSleepMinutes,
  progressLabel: ttsProgressLabel,
  setRate: setTTSRate,
  setPitch: setTTSPitch,
  setVoice: setTTSVoice,
  setSleepMinutes: setTTSSleepMinutes,
  toggle: toggleTTS,
  stop: ttsStop,
} = useReaderTTS({
  reader,
  content,
  contentBody,
  currentIndex,
  chapters,
  goChapter,
  notify: showTTSMessage,
})

function onModeChange(mode) {
  reader.setMode(mode)
}

onMounted(async () => {
  reader.normalizeSettings()
  syncReaderFontFaces(reader.customFontsMap)
  try {
    await loadReaderBook()
  } catch (err) {
    chapterLoadError.value = readError(err, '章节加载失败')
    chapterLoading.value = false
  }
  window.addEventListener('resize', handleResize)
  window.addEventListener('wheel', handleReaderWheel, { passive: false })
  window.addEventListener('pagehide', handleReaderPageHide)
  document.addEventListener('visibilitychange', handleReaderVisibilityChange)
  window.addEventListener('openreader:progress-updated', handleProgressUpdated)
  window.addEventListener('openreader:reader-book-data-updated', handleReaderBookDataUpdated)
  window.addEventListener('openreader:replace-rules-updated', handleReplaceRulesUpdated)
  window.addEventListener('openreader:bookmarks-updated', handleBookmarksUpdated)
  customBg.value = reader.customBgColor
  sliderLineHeight.value = reader.lineHeight
})

onBeforeUnmount(() => {
  cancelProgressSave()
  clearTimeout(chapterLoadingTimer)
  stopAutoReading()
  saveCurrentProgress({ force: true, background: true })
  window.removeEventListener('resize', handleResize)
  window.removeEventListener('wheel', handleReaderWheel)
  window.removeEventListener('pagehide', handleReaderPageHide)
  document.removeEventListener('visibilitychange', handleReaderVisibilityChange)
  window.removeEventListener('openreader:progress-updated', handleProgressUpdated)
  window.removeEventListener('openreader:reader-book-data-updated', handleReaderBookDataUpdated)
  window.removeEventListener('openreader:replace-rules-updated', handleReplaceRulesUpdated)
  window.removeEventListener('openreader:bookmarks-updated', handleBookmarksUpdated)
})

onBeforeRouteLeave(() => {
  saveCurrentProgress({ force: true, background: true })
})

watch(bookId, async () => {
  chapterLoadError.value = ''
  try {
    await loadReaderBook()
  } catch (err) {
    chapterLoadError.value = readError(err, '章节加载失败')
    chapterLoading.value = false
  }
})

watch(() => [route.query.chapter, route.query.offset, route.query.percent], async ([q, offset, percent]) => {
  const idx = Number(q || 0)
  const nextOffset = Number(offset || 0)
  const restorePercent = parseRoutePercent(percent)
  if (idx !== currentIndex.value || offset !== undefined || restorePercent !== null) {
    await loadChapter(idx, nextOffset, { restorePercent, saveAfterLoad: idx !== currentIndex.value || offset !== undefined || restorePercent !== null })
  }
  await jumpToRouteLine()
})

watch(() => [route.query.line, route.query.match, route.query.q], async () => {
  await jumpToRouteLine()
})

watch(() => reader.mode, async () => {
  const offset = currentOffset()
  page.value = 0
  if (isContinuousScrollRead.value) {
    chapterLoading.value = true
    try {
      await computeShowChapterList()
    } finally {
      chapterLoading.value = false
    }
  } else {
    chapterBlocks.value = [makeChapterBlock(currentIndex.value, chapter.value, content.value)]
  }
  await nextTick()
  updateFlipLayout()
  await restoreReadingPosition(offset, { saveAfterLoad: false })
  saveCurrentProgress()
})

watch(isMobileReader, (mobile) => {
  if (!mobile && reader.mode === 'flip') {
    reader.setMode('page')
  }
}, { immediate: true })

watch(() => [reader.fontFamily, reader.chineseFont, reader.fontSize, reader.fontWeight, reader.lineHeight, reader.paragraphSpace, reader.columnWidth], async () => {
  const offset = currentOffset()
  const restorePercent = currentChapterPercent()
  restoringPosition = true
  try {
    await nextTick()
    updateFlipLayout()
    await restoreReadingPosition(offset, { restorePercent, saveAfterLoad: false })
  } finally {
    restoringPosition = false
  }
  progressVersion.value += 1
  scheduleProgressSave(300)
})

watch(() => reader.customFontsMap, (customFontsMap) => {
  syncReaderFontFaces(customFontsMap)
}, { deep: true })

function makeParagraphs(value, heading = '') {
  return parseReaderContentBlocks(value, heading, formatChineseText)
}

function formatChineseText(text) {
  if (!text) return ''
  return reader.chineseFont === '繁体' ? traditionalized(String(text)) : simplized(String(text))
}

function displayChapterTitle(title) {
  return formatChineseText(title || '')
}

function makeChapterBlock(index, chapterRow, text) {
  const fallback = chapters.value[index] || {}
  const title = chapterRow?.title || fallback.title || `第 ${index + 1} 章`
  const paragraphs = makeParagraphs(text, title)
  return {
    index,
    id: chapterRow?.id || fallback.id,
    title: displayChapterTitle(title),
    content: String(text || ''),
    paragraphs,
    imageUrls: paragraphs.filter(item => item.type === 'image').map(item => item.src),
  }
}

function chapterBlockTextLength(block) {
  const paragraphs = Array.isArray(block?.paragraphs) ? block.paragraphs : []
  if (!paragraphs.length) return 0
  const last = paragraphs[paragraphs.length - 1]
  return Number(last.endPos || last.pos || 0)
}

async function loadReaderBook() {
  cancelProgressSave()
  const targetBookId = bookId.value
  const bookmarksRequest = loadBookmarks(targetBookId).catch(() => [])
  const progressRequest = reader.loadProgress(targetBookId, { preferLocal: true }).catch(() => null)
  const cachedProgress = reader.cachedProgress(targetBookId)
  const [bookRes, chRes] = await Promise.all([
    cacheFirstRequest(
      () => api.get(`/books/${targetBookId}`),
      readerDataCacheKey(`book:${targetBookId}`),
      { validate: data => Boolean(data?.id) },
    ),
    cacheFirstRequest(
      () => api.get(`/books/${targetBookId}/chapters`),
      readerDataCacheKey(`chapters:${targetBookId}`),
      { validate: data => Array.isArray(data) },
    ),
  ])
  if (bookId.value !== targetBookId) return
  const saved = cachedProgress?.bookId ? cachedProgress : await progressRequest
  if (bookId.value !== targetBookId) return
  book.value = mergeLoadedBook(bookRes.data)
  chapters.value = chRes.data
  if (book.value?.progress?.bookId) {
    reader.applyServerProgress(book.value.progress)
    bookshelf.applyBookProgress(book.value.progress)
  }
  if (saved?.bookId) {
    book.value = mergeShelfBook(book.value, { id: book.value.id, progress: saved })
  }
  resetSourceCandidates()
  if (saved?.bookId) bookshelf.applyBookProgress(saved)
  const resumeFromProgress = route.query.resume === '1'
  const hasExplicitChapter = route.query.chapter !== undefined
  const shouldUseSavedPosition = resumeFromProgress || !hasExplicitChapter
  if (shouldUseSavedPosition && saved?.chapterIndex !== undefined) {
    currentIndex.value = saved.chapterIndex
  } else {
    currentIndex.value = Number(route.query.chapter || 0)
  }
  const hasRouteOffset = !resumeFromProgress && route.query.offset !== undefined
  const initialOffset = hasRouteOffset
    ? Number(route.query.offset || 0)
    : (shouldUseSavedPosition ? Number(saved?.offset || 0) : 0)
  const routePercent = resumeFromProgress ? null : parseRoutePercent(route.query.percent)
  const savedPercent = shouldUseSavedPosition ? savedBookChapterPercent(saved, chapters.value.length) : null
  await loadChapter(currentIndex.value, initialOffset, {
    restorePercent: routePercent ?? (hasRouteOffset ? null : savedPercent),
    saveAfterLoad: false,
  })
  const initialProgressKey = progressSaveKey(currentProgressPayload())
  progressRequest.then(serverSaved => {
    reconcileInitialServerProgress(serverSaved, {
      baseline: saved,
      baselineKey: initialProgressKey,
      resumeFromProgress,
      hasRouteOffset,
      routePercent,
    }).catch(() => {})
  })
  if (bookRes.fromCache || chRes.fromCache) {
    refreshReaderBookCaches({ book: Boolean(bookRes.fromCache), chapters: Boolean(chRes.fromCache) }).catch(() => {})
  }
  bookmarksRequest.then(data => {
    if (bookId.value === targetBookId) bookmarks.value = data
  }).catch(() => {})
  await jumpToRouteLine()
}

async function reconcileInitialServerProgress(serverSaved, options = {}) {
  if (!serverSaved?.bookId || Number(serverSaved.bookId) !== Number(bookId.value)) return
  const canFollowServer = options.resumeFromProgress || route.query.chapter === undefined
  if (!canFollowServer || options.hasRouteOffset || options.routePercent !== null) return
  if (options.baseline?.bookId && progressUpdatedAtMs(serverSaved) <= progressUpdatedAtMs(options.baseline)) return
  if (progressSaveKey(currentProgressPayload()) !== options.baselineKey) return
  const targetIndex = Math.max(0, Math.min(Number(serverSaved.chapterIndex || 0), Math.max(chapters.value.length - 1, 0)))
  const targetOffset = Math.max(0, Math.floor(Number(serverSaved.offset || 0)))
  const restorePercent = Number.isFinite(Number(serverSaved.chapterPercent))
    ? Math.max(0, Math.min(1, Number(serverSaved.chapterPercent)))
    : savedBookChapterPercent(serverSaved, chapters.value.length)
  await router.replace({
    name: 'reader',
    params: { id: bookId.value },
    query: {
      resume: '1',
      chapter: targetIndex,
      ...(targetOffset ? { offset: targetOffset } : {}),
      ...(restorePercent !== null ? { percent: Number(restorePercent.toFixed(6)) } : {}),
    },
  })
  await loadChapter(targetIndex, targetOffset, {
    restorePercent,
    saveAfterLoad: false,
  })
  markProgressSaved(currentProgressPayload())
}

function mergeLoadedBook(incoming) {
  if (!incoming?.id) return incoming
  const current = bookshelf.books.find(item => Number(item.id) === Number(incoming.id)) ||
    (Number(book.value?.id) === Number(incoming.id) ? book.value : null)
  return mergeShelfBook(current, incoming)
}

async function refreshReaderBookCaches(options = {}) {
  const targetBookId = bookId.value
  const requests = []
  if (options.book) {
    requests.push(networkFirstRequest(
      () => api.get(`/books/${targetBookId}`),
      readerDataCacheKey(`book:${targetBookId}`),
      { validate: data => Boolean(data?.id) },
    ).then(res => ({ key: 'book', data: res.data })))
  }
  if (options.chapters) {
    requests.push(networkFirstRequest(
      () => api.get(`/books/${targetBookId}/chapters`),
      readerDataCacheKey(`chapters:${targetBookId}`),
      { validate: data => Array.isArray(data) },
    ).then(res => ({ key: 'chapters', data: res.data })))
  }
  const rows = await Promise.all(requests)
  if (bookId.value !== targetBookId) return
  rows.forEach(row => {
    if (row.key === 'book' && row.data?.id) book.value = mergeLoadedBook(row.data)
    if (row.key === 'chapters' && Array.isArray(row.data)) chapters.value = row.data
  })
}

function readerDataCacheKey(key) {
  const [type, targetBookId] = String(key || '').split(':')
  return scopedReaderDataCacheKey(targetBookId || bookId.value, type || key)
}

async function invalidateReaderDataCache(options = {}) {
  const targetBookId = options.bookId || bookId.value
  await invalidateReaderCache(targetBookId, options)
}

async function writeReaderDataCache(options = {}) {
  const targetBookId = options.bookId || bookId.value
  await writeReaderCache(targetBookId, options)
}

async function resetReaderChapterCaches(options = {}) {
  const targetBook = options.book || book.value
  const targetBookId = targetBook?.id || bookId.value
  chapterContentCache.clearBook(currentChapterCacheBookKey(targetBook, targetBookId))
  resetBrowserCachedChapters()
  if (!options.clearBrowser) return 0
  try {
    return await clearBookBrowserChapterCache(targetBook, targetBookId)
  } catch {
    return 0
  }
}

async function loadChapter(index, offset = 0, options = {}) {
  currentIndex.value = Math.max(0, Math.min(index, Math.max(chapters.value.length - 1, 0)))
  mobileChromeVisible.value = false
  restoringPosition = true
  chapterLoaded.value = false
  chapterLoadError.value = ''
  cancelProgressSave()
  clearTimeout(chapterLoadingTimer)
  const cachedBeforeLoad = !options.refresh && getChapterContentFromMemory(currentIndex.value)
  chapterLoading.value = !cachedBeforeLoad
  if (cachedBeforeLoad) {
    chapterLoadingTimer = null
  } else {
    chapterLoadingTimer = setTimeout(() => {
      chapterLoading.value = true
    }, 120)
  }
  try {
    const data = await loadChapterContent(currentIndex.value, { refresh: Boolean(options.refresh) })
    chapter.value = data.chapter
    content.value = data.content || ''
    page.value = 0
    chapterBlocks.value = [makeChapterBlock(currentIndex.value, chapter.value, content.value)]
    chapterLoading.value = false
    await nextTick()
    updateFlipLayout()
    await restoreReadingPosition(offset, options)
    progressVersion.value += 1
    preloadNearbyChapters(currentIndex.value)
    if (options.saveAfterLoad) {
      await saveCurrentProgress({ force: true })
    } else {
      markProgressSaved(currentProgressPayload())
    }
    chapterLoaded.value = true
    if (isContinuousScrollRead.value) {
      computeShowChapterList({ anchorIndex: currentIndex.value }).catch(() => {})
    }
  } catch (err) {
    chapterLoadError.value = readError(err, '章节加载失败，请检查书源或网络后重试')
  } finally {
    clearTimeout(chapterLoadingTimer)
    await nextFrame()
    restoringPosition = false
    chapterLoading.value = false
  }
}

async function computeShowChapterList(options = {}) {
  if (!chapters.value.length) {
    chapterBlocks.value = []
    return
  }
  const anchorIndex = Number.isInteger(options.anchorIndex) ? options.anchorIndex : currentIndex.value
  const indexes = readerChapterWindowIndexes({
    mode: reader.mode,
    anchorIndex,
    totalChapters: chapters.value.length,
    previousSize: SHOW_PREV_CHAPTER_SIZE,
    nextSize: isContinuousScrollRead.value ? SHOW_NEXT_CHAPTER_SIZE : 0,
  })
  const rows = await Promise.all(indexes.map(async index => {
    try {
      const data = await loadChapterContent(index)
      return makeChapterBlock(index, data.chapter || chapters.value[index], data.content || '')
    } catch {
      return null
    }
  }))
  if (currentIndex.value !== anchorIndex) return
  const blocks = rows.filter(Boolean)
  if (blocks.some(block => block.index === anchorIndex)) {
    const scrollAnchor = captureReaderScrollAnchor()
    chapterBlocks.value = blocks
    await restoreReaderScrollAnchor(scrollAnchor)
  }
}

async function appendNextShowChapter() {
  if (!isContinuousScrollRead.value || !chapterBlocks.value.length) return
  const nextIndex = adjacentReaderChapterIndex({
    blocks: chapterBlocks.value,
    direction: 'next',
    totalChapters: chapters.value.length,
  })
  if (nextIndex === null) return
  if (chapterBlocks.value.some(block => block.index === nextIndex)) return
  const data = await loadChapterContent(nextIndex)
  chapterBlocks.value = [
    ...chapterBlocks.value,
    makeChapterBlock(nextIndex, data.chapter || chapters.value[nextIndex], data.content || ''),
  ]
}

async function prependPreviousShowChapter() {
  if (reader.mode !== 'scroll2' || !chapterBlocks.value.length || !contentEl.value) return
  const previousIndex = adjacentReaderChapterIndex({
    blocks: chapterBlocks.value,
    direction: 'previous',
    totalChapters: chapters.value.length,
  })
  if (previousIndex === null) return
  if (chapterBlocks.value.some(block => block.index === previousIndex)) return
  const beforeHeight = contentEl.value.scrollHeight
  const beforeTop = contentEl.value.scrollTop
  const data = await loadChapterContent(previousIndex)
  chapterBlocks.value = [
    makeChapterBlock(previousIndex, data.chapter || chapters.value[previousIndex], data.content || ''),
    ...chapterBlocks.value,
  ]
  await nextTick()
  await nextFrame()
  const heightDelta = Math.max(0, contentEl.value.scrollHeight - beforeHeight)
  contentEl.value.scrollTop = beforeTop + heightDelta
}

async function loadChapterContent(index, options = {}) {
  const targetBook = { ...(book.value || {}) }
  const targetBookId = bookId.value
  const cacheBookKey = currentChapterCacheBookKey(targetBook, targetBookId)
  if (!options.refresh) {
    const cached = getChapterContentFromMemory(index, cacheBookKey)
    if (cached) return cached
  }
  const data = await loadBrowserChapterContent(targetBook, targetBookId, index, { refresh: Boolean(options.refresh) })
  addChapterContentToMemory(index, data, cacheBookKey)
  if (
    isValidChapterContentResponse(data)
    && Number(bookId.value) === Number(targetBookId)
    && currentChapterCacheBookKey() === cacheBookKey
  ) {
    markBrowserChapterCached(index)
  }
  return data
}

function preloadNearbyChapters(index) {
  if (!book.value || !chapters.value.length) return
  nearbyReaderChapterIndexes({
    chapterIndex: index,
    totalChapters: chapters.value.length,
    radius: NEARBY_PRELOAD_RADIUS,
  })
    .forEach(target => {
      if (getChapterContentFromMemory(target)) return
      loadChapterContent(target).catch(() => {})
    })
}

function getChapterContentFromMemory(index, cacheBookKey = currentChapterCacheBookKey()) {
  const cached = chapterContentCache.get(cacheBookKey, index)
  return isValidChapterContentResponse(cached) ? cached : null
}

function addChapterContentToMemory(index, data, cacheBookKey = currentChapterCacheBookKey()) {
  if (!isValidChapterContentResponse(data)) return
  chapterContentCache.set(cacheBookKey, index, data)
}

function currentChapterCacheBookKey(targetBook = book.value, fallbackBookId = bookId.value) {
  return chapterCacheBookKey(targetBook, fallbackBookId)
}

async function restoreReadingPosition(offset = 0, options = {}) {
  const restorePercent = Number(options.restorePercent)
  const hasRestorePercent = Number.isFinite(restorePercent)
  await nextTick()
  await nextFrame()
  updateFlipLayout()
  const chapterOffset = Number(offset || 0)
  if (reader.mode === 'flip') {
    page.value = restoredReaderFlipPage({
      offset: chapterOffset,
      percent: hasRestorePercent ? restorePercent : null,
      pageCount: pageCount.value,
    })
    return
  }
  if (!contentEl.value) return
  if (isContinuousScrollRead.value) {
    restoreScroll2ChapterPosition(chapterOffset, hasRestorePercent ? restorePercent : null)
    return
  }
  if (!hasRestorePercent && chapterOffset > 0 && restoreByChapterPosition(chapterOffset)) {
    return
  }
  const applyScroll = () => {
    if (!contentEl.value) return
    contentEl.value.scrollTop = restoredReaderSingleChapterScrollTop({
      offset: chapterOffset,
      percent: hasRestorePercent ? restorePercent : null,
      scrollHeight: contentEl.value.scrollHeight,
      clientHeight: contentEl.value.clientHeight,
    })
  }
  applyScroll()
  await nextFrame()
  applyScroll()
}

function restoreScroll2ChapterPosition(chapterOffset, restorePercent = null) {
  const el = contentEl.value
  const activeChapter = contentBody.value?.querySelector(`.chapter-content[data-index="${currentIndex.value}"]`)
  if (!el || !activeChapter) return
  const scrollTop = restoredReaderContinuousScrollTop({
    offset: chapterOffset,
    percent: restorePercent,
    chapterTop: activeChapter.offsetTop,
    chapterHeight: activeChapter.offsetHeight,
    clientHeight: el.clientHeight,
  })
  if (scrollTop !== null) {
    el.scrollTop = scrollTop
    return
  }
  if (chapterOffset > 0 && restoreByChapterPosition(chapterOffset)) return
  el.scrollTop = Math.max(0, activeChapter.offsetTop)
}

function restoreByChapterPosition(position) {
  if (!contentBody.value || !Number.isFinite(position) || position <= 0) return false
  const activeChapter = contentBody.value.querySelector(`.chapter-content[data-index="${currentIndex.value}"]`) || contentBody.value
  const target = paragraphByChapterPosition(activeChapter, position)
  if (!target) return false
  jumpToParagraph(target, { save: false, flash: false })
  return true
}

function paragraphByChapterPosition(chapterEl, position) {
  if (!chapterEl || !Number.isFinite(position) || position <= 0) return null
  const nodes = [...chapterEl.querySelectorAll('h1[data-pos], [data-reader-block][data-pos]')]
  return readerParagraphAtPosition(nodes, position)
}

function nextFrame() {
  return new Promise(resolve => requestAnimationFrame(() => resolve()))
}

async function handleReplaceRulesUpdated() {
  if (!book.value?.id || !chapter.value) return
  const restorePercent = currentChapterPercent()
  try {
    await loadChapter(currentIndex.value, currentOffset(), { restorePercent, refresh: true })
    ElMessage.success('已按最新替换规则刷新当前章节')
  } catch (err) {
    ElMessage.error(readError(err, '刷新当前章节失败'))
  }
}

function setTheme(theme) { reader.setTheme(theme) }

async function pickBgImage(data) {
  const file = data.raw || data.file
  if (!file) return
  try {
    const { data: result } = await uploadAsset({ file, type: 'background' })
    if (!result?.url) throw new Error('上传结果缺少背景图地址')
    reader.addCustomBgImage(result.url)
    ElMessage.success('阅读背景图已上传')
  } catch (err) {
    ElMessage.error(readError(err, '上传背景图失败'))
  }
}

async function clearBgImage(image) {
  if (!image) return
  try {
    await deleteAsset(image)
    reader.removeCustomBgImage(image)
    ElMessage.success('已删除阅读背景图')
  } catch (err) {
    ElMessage.error(readError(err, '删除阅读背景图失败'))
  }
}

async function pickFontFile({ file, font }) {
  const rawFile = file?.raw || file?.file || file
  if (!rawFile || !font?.value) return
  try {
    const { data } = await uploadAsset({ file: rawFile, type: 'font' })
    if (!data?.url) throw new Error('上传结果缺少字体地址')
    reader.setCustomFont(font.value, data.url)
    reader.setFontFamily(font.value)
    syncReaderFontFaces(reader.customFontsMap)
    ElMessage.success(`已上传${font.label}字体`)
  } catch (err) {
    ElMessage.error(readError(err, '上传字体失败'))
  }
}

async function clearFontFile(font) {
  const url = reader.customFontsMap?.[font?.value]
  if (!url || !font?.value) return
  try {
    await deleteAsset(url)
    reader.clearCustomFont(font.value)
    syncReaderFontFaces(reader.customFontsMap)
    ElMessage.success(`已恢复默认${font.label}字体`)
  } catch (err) {
    ElMessage.error(readError(err, '恢复默认字体失败'))
  }
}

async function goChapter(index, offset = 0) {
  const targetIndex = Math.max(0, Math.min(Number(index), Math.max(chapters.value.length - 1, 0)))
  if (targetIndex === currentIndex.value) {
    showTocDrawer.value = false
    jumpWithinCurrentChapter(offset)
    return
  }
  if (isContinuousScrollRead.value && jumpToLoadedChapter(targetIndex, offset)) {
    showTocDrawer.value = false
    return
  }
  const query = { chapter: targetIndex }
  if (offset) query.offset = offset
  await router.replace({ name: 'reader', params: { id: bookId.value }, query })
}

function jumpWithinCurrentChapter(offset = 0) {
  if (reader.mode === 'flip') {
    page.value = restoredReaderFlipPage({
      offset: Number(offset) === READER_CHAPTER_END_OFFSET ? READER_CHAPTER_END_OFFSET : 0,
      percent: null,
      pageCount: pageCount.value,
    })
    progressVersion.value += 1
    saveCurrentProgress()
    return
  }
  if (jumpToLoadedChapter(currentIndex.value, offset)) return
  if (!contentEl.value) return
  contentEl.value.scrollTo({
    top: restoredReaderSingleChapterScrollTop({
      offset: Number(offset) === READER_CHAPTER_END_OFFSET ? READER_CHAPTER_END_OFFSET : 0,
      percent: null,
      scrollHeight: contentEl.value.scrollHeight,
      clientHeight: contentEl.value.clientHeight,
    }),
    behavior: readerScrollBehavior(),
  })
  progressVersion.value += 1
  saveCurrentProgress()
}

function jumpToLoadedChapter(index, offset = 0) {
  if (!contentEl.value || !contentBody.value) return false
  const targetIndex = Math.max(0, Math.min(Number(index), Math.max(chapters.value.length - 1, 0)))
  const chapterEl = contentBody.value.querySelector(`.chapter-content[data-index="${targetIndex}"]`)
  if (!chapterEl) return false
  const block = chapterBlocks.value.find(item => item.index === targetIndex)
  currentIndex.value = targetIndex
  chapter.value = chapters.value[targetIndex] || (block?.id ? { id: block.id, title: block.title, index: targetIndex } : chapter.value)
  content.value = block?.content || content.value
  if (Number(offset) === READER_CHAPTER_END_OFFSET) {
    contentEl.value.scrollTo({
      top: readerChapterBoundaryScrollTop({
        chapterTop: chapterEl.offsetTop,
        chapterHeight: chapterEl.offsetHeight,
        clientHeight: contentEl.value.clientHeight,
        end: true,
      }),
      behavior: readerScrollBehavior(),
    })
  } else if (offset > 0) {
    const target = paragraphByChapterPosition(chapterEl, offset)
    if (target) {
      jumpToParagraph(target, { save: false, flash: false })
    } else {
      contentEl.value.scrollTo({
        top: readerChapterBoundaryScrollTop({
          chapterTop: chapterEl.offsetTop,
          chapterHeight: chapterEl.offsetHeight,
          clientHeight: contentEl.value.clientHeight,
          end: false,
        }),
        behavior: readerScrollBehavior(),
      })
    }
  } else {
    contentEl.value.scrollTo({
      top: readerChapterBoundaryScrollTop({
        chapterTop: chapterEl.offsetTop,
        chapterHeight: chapterEl.offsetHeight,
        clientHeight: contentEl.value.clientHeight,
        end: false,
      }),
      behavior: readerScrollBehavior(),
    })
  }
  progressVersion.value += 1
  scheduleProgressSave(Math.max(300, reader.animateDuration + 80))
  return true
}

async function changeReaderLocalTocRule() {
  if (!book.value || !canChangeLocalTocRule.value) return
  const tocRule = await chooseReaderLocalTocRule()
  if (tocRule === null) return
  try {
    await runTocRefreshing(async () => {
      const { data } = await refreshLocalBook(book.value.id, { tocRule })
      await invalidateReaderDataCache({ chapters: true, book: true })
      await resetReaderChapterCaches({ clearBrowser: true })
      const updated = data?.book || data
      if (updated?.id) {
        book.value = mergeLoadedBook(updated)
        bookshelf.upsertBook(book.value)
        if (overlay.bookInfoBook?.id === updated.id) overlay.bookInfoBook = book.value
        await writeReaderDataCache({ bookData: book.value })
      }
      await loadChapters()
      const nextIndex = Math.min(currentIndex.value, Math.max(chapters.value.length - 1, 0))
      await loadChapter(nextIndex, 0, { refresh: true, saveAfterLoad: true })
      await computeBrowserCachedChapters()
      locateTocCurrentChapter()
      toastMsg.value = `目录规则已更新，共 ${data?.chapterCount || chapters.value.length} 章`
      setTimeout(() => { toastMsg.value = '' }, 1600)
    })
  } catch (err) {
    ElMessage.error(readError(err, '更新目录规则失败'))
  }
}

async function chooseReaderLocalTocRule() {
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

function openSettingsDrawer() {
  mobileChromeVisible.value = false
  customBg.value = reader.customBgColor
  sliderLineHeight.value = reader.lineHeight
  showSettingsDrawer.value = true
}

function showClickZone() {
  showSettingsDrawer.value = false
  showMobileMoreDrawer.value = false
  mobileChromeVisible.value = false
  showClickZoneOverlay.value = true
}

function openCacheDrawer() {
  if (!isRemoteBook.value) return
  mobileChromeVisible.value = false
  computeBrowserCachedChapters()
  showCacheDrawer.value = true
}

async function goBookDetail() {
  saveCurrentProgress({ force: true, background: true })
  await router.push({ name: 'book-detail', params: { id: bookId.value } })
}

async function goShelf() {
  mobileChromeVisible.value = false
  saveCurrentProgress({ force: true, background: true })
  await router.push({ name: 'home' })
}
function openReaderBookInfo() {
  if (!book.value) return
  const hasRemoteSource = isRemoteBook.value
  const actions = [
    { label: '目录', plain: true, handler: openInfoToc },
    { label: '书签', plain: true, handler: openInfoBookmarks },
    { label: '搜正文', plain: true, handler: openInfoSearch },
    hasRemoteSource ? { label: '书源', plain: true, handler: openInfoSources } : null,
    { label: '分组', plain: true, handler: openInfoGroup },
    hasRemoteSource ? { label: '刷新目录', plain: true, handler: refreshReaderBookCatalog } : null,
    hasRemoteSource ? { label: '缓存章节', plain: true, handler: openCacheDrawer } : null,
    hasRemoteSource ? { label: '清缓存', plain: true, handler: clearCurrentBookCache } : null,
    { label: '设置', plain: true, handler: openInfoSettings },
    { label: '完整详情', type: 'primary', handler: () => { overlay.closeBookInfo(); goBookDetail() } },
  ].filter(Boolean)
  overlay.openBookInfo(book.value, {
    statusLabel: `阅读中 · ${bookProgressLabel.value}`,
    statusType: 'success',
    progress: bookProgress.value,
    actions,
  })
}

function closeInfoAndMobileChrome() {
  overlay.closeBookInfo()
  mobileChromeVisible.value = false
}

function openInfoToc() {
  closeInfoAndMobileChrome()
  openTocDrawer()
}

function openInfoBookmarks() {
  closeInfoAndMobileChrome()
  openBookmarkDrawer()
}

function openInfoSearch() {
  closeInfoAndMobileChrome()
  openContentSearch()
}

function openInfoSources() {
  if (!isRemoteBook.value) return
  closeInfoAndMobileChrome()
  showSourceDrawer.value = true
}

function openInfoSettings() {
  closeInfoAndMobileChrome()
  openSettingsDrawer()
}

async function openInfoGroup() {
  if (!book.value) return
  closeInfoAndMobileChrome()
  try {
    await bookshelf.ensureCategoriesLoaded()
  } catch {
    // 分组弹层仍可打开，失败提示由保存时处理。
  }
  overlay.openBookGroup('set', book.value, {
    categoryName: categoryName(book.value),
    progress: bookProgress.value,
    statusLabel: `阅读中 · ${bookProgressLabel.value}`,
    statusType: 'success',
  })
}

async function refreshReaderBookCatalog() {
  if (!book.value?.id || Number(book.value.sourceId || 0) <= 0) return
  try {
    const restoreOffset = currentOffset()
    const restorePercent = currentChapterPercent()
    const { data } = await refreshBook(book.value.id)
    await invalidateReaderDataCache({ book: true, chapters: true })
    await resetReaderChapterCaches({ clearBrowser: true })
    const updated = data?.book || data
    if (updated?.id) {
      book.value = mergeLoadedBook(updated)
      bookshelf.upsertBook(book.value)
      await writeReaderDataCache({ bookData: book.value })
    }
    await loadChapters()
    await loadChapter(currentIndex.value, restoreOffset, { restorePercent, refresh: true })
    overlay.bookInfoBook = book.value
    toastMsg.value = '目录已刷新'
    setTimeout(() => { toastMsg.value = '' }, 1400)
  } catch (err) {
    ElMessage.error(readError(err, '刷新目录失败'))
  }
}

async function loadChapters() {
  const targetBookId = bookId.value
  const { data } = await api.get(`/books/${targetBookId}/chapters`)
  if (bookId.value !== targetBookId) return chapters.value
  chapters.value = Array.isArray(data) ? data : []
  currentIndex.value = Math.max(0, Math.min(currentIndex.value, Math.max(chapters.value.length - 1, 0)))
  await writeReaderDataCache({ bookId: targetBookId, chaptersData: chapters.value })
  return chapters.value
}

function goSourcePanel() {
  if (!isRemoteBook.value) return
  mobileChromeVisible.value = false
  showSourceDrawer.value = true
}

function openBookmarkDrawer() {
  mobileChromeVisible.value = false
  showBookmarkDrawer.value = true
}

function runMobileAction(action) {
  showMobileMoreDrawer.value = false
  mobileChromeVisible.value = false
  action?.()
}

function handleMobileToolAction(action) {
  runMobileAction(readerToolAction(action))
}

function handleMobileChromeAction(action) {
  if (action === 'previous') {
    goChapter(currentIndex.value - 1)
    return
  }
  if (action === 'next') {
    goChapter(currentIndex.value + 1)
    return
  }
  if (action === 'toggle') {
    toggleReaderChrome()
    return
  }
  if (action === 'more') {
    openMobileTool(() => { showMobileMoreDrawer.value = true })
    return
  }
  openMobileTool(readerToolAction(action))
}

function handleDesktopToolAction(action) {
  readerToolAction(action)?.()
}

function readerToolAction(action) {
  return {
    home: goShelf,
    shelf: openShelfPanel,
    source: goSourcePanel,
    toc: openTocDrawer,
    settings: openSettingsDrawer,
    bookmarks: openBookmarkDrawer,
    search: openContentSearch,
    info: openReaderBookInfo,
    note: openNoteDialog,
    cache: openCacheDrawer,
    'clear-cache': clearCurrentBookCache,
    reload: reloadChapter,
    'auto-read': toggleAutoReading,
    tts: toggleTTS,
    night: toggleNight,
    top: scrollToTop,
    bottom: scrollToBottom,
  }[action]
}

function openMobileTool(action) {
  mobileChromeVisible.value = false
  action?.()
}

function openReplaceRules() {
  showSettingsDrawer.value = false
  overlay.openReplaceRules()
}

async function applyReaderSourceChange({ book: updatedBook, previousBook }) {
  await invalidateReaderDataCache({ book: true, chapters: true })
  await resetReaderChapterCaches({ clearBrowser: true, book: previousBook })
  book.value = mergeLoadedBook(updatedBook)
  bookshelf.upsertBook(book.value)
  const chRes = await api.get(`/books/${bookId.value}/chapters`)
  chapters.value = Array.isArray(chRes.data) ? chRes.data : []
  await writeReaderDataCache({ bookData: book.value, chaptersData: chapters.value })
  currentIndex.value = Math.min(currentIndex.value, Math.max(chapters.value.length - 1, 0))
  await loadChapter(currentIndex.value, 0)
  resetContentSearchState()
  await refreshSourceCandidates()
  showSourceDrawer.value = false
}

function openContentSearch() {
  mobileChromeVisible.value = false
  showSearchDrawer.value = true
  nextTick(() => {
    const input = document.querySelector('.content-search-row input')
    input?.focus()
  })
}

async function reloadChapter() {
  await loadChapter(currentIndex.value, currentOffset(), { refresh: true })
  toastMsg.value = '章节已重新载入'
  setTimeout(() => { toastMsg.value = '' }, 1600)
}

async function clearCurrentBookCache() {
  if (!isRemoteBook.value) return
  try {
    const data = await bookshelf.batchClearCache([bookId.value])
    const localCleared = await clearCurrentBookBrowserCache()
    await loadChapters()
    toastMsg.value = `已清理服务器 ${data.cleared || 0} 章，本地 ${localCleared} 章`
    setTimeout(() => { toastMsg.value = '' }, 1600)
  } catch (err) {
    ElMessage.error(readError(err, '清理缓存失败'))
  }
}

async function advanceAutoReadingPage() {
  const beforeChapter = currentIndex.value
  const beforePage = page.value
  await nextPage()
  return beforeChapter !== currentIndex.value || beforePage !== page.value
}

function recordAutoReadingProgress() {
  progressVersion.value += 1
  saveCurrentProgress()
}

function showAutoReadingMessage(message) {
  toastMsg.value = message
  setTimeout(() => {
    if (toastMsg.value === message) toastMsg.value = ''
  }, 1200)
}

function showTTSMessage(message, duration = 0) {
  toastMsg.value = message
  if (duration <= 0) return
  setTimeout(() => {
    if (toastMsg.value === message) toastMsg.value = ''
  }, duration)
}

function showChapterCacheMessage(message) {
  toastMsg.value = message
  setTimeout(() => {
    if (toastMsg.value === message) toastMsg.value = ''
  }, 1600)
}

function showReaderToast(message, duration = 1600) {
  toastMsg.value = message
  setTimeout(() => {
    if (toastMsg.value === message) toastMsg.value = ''
  }, duration)
}

function toggleNight() {
  reader.setTheme(reader.theme === 'dark' || reader.theme === 'black' ? 'parchment' : 'dark')
}

async function previousPage() {
  if (reader.mode === 'flip' && page.value > 0) {
    page.value -= 1
    progressVersion.value += 1
    saveCurrentProgress()
    return
  }
  if (isVerticalRead.value && contentEl.value) {
    const el = contentEl.value
    if (el.scrollTop > 8) {
      el.scrollBy({ top: -scrollStep(), behavior: readerScrollBehavior() })
      scheduleProgressSave(reader.animateDuration + 60)
      return
    }
  }
  if (currentIndex.value > 0) await goChapter(currentIndex.value - 1, READER_CHAPTER_END_OFFSET)
}

async function nextPage() {
  if (reader.mode === 'flip' && page.value < pageCount.value - 1) {
    page.value += 1
    progressVersion.value += 1
    saveCurrentProgress()
    return
  }
  if (isVerticalRead.value && contentEl.value) {
    const el = contentEl.value
    const bottom = el.scrollHeight - el.clientHeight
    if (el.scrollTop < bottom - 8) {
      el.scrollBy({ top: scrollStep(), behavior: readerScrollBehavior() })
      scheduleProgressSave(reader.animateDuration + 60)
      return
    }
  }
  if (currentIndex.value < chapters.value.length - 1) await goChapter(currentIndex.value + 1)
}

function scrollStep() {
  const viewportHeight = contentEl.value?.clientHeight || window.innerHeight || readableViewportSize().height
  return readerScrollStep({
    viewportHeight,
    fontSize: reader.fontSize,
    lineHeight: reader.lineHeight,
    paragraphSpace: reader.paragraphSpace,
  })
}

function readerScrollBehavior() {
  return readerScrollBehaviorForDuration(reader.animateDuration)
}

function handleDesktopProgressInput(event) {
  seekCurrentChapterPercent(Number(event.target.value || 0) / 1000, { save: false })
}

function handleDesktopProgressChange(event) {
  seekCurrentChapterPercent(Number(event.target.value || 0) / 1000, { save: true })
}

function handleMobileBookProgressInput(event) {
  mobileBookSliderDraft.value = Number(event.target.value || 0)
}

async function handleMobileBookProgressChange(event) {
  const value = Number(event.target.value || 0)
  mobileBookSliderDraft.value = value
  try {
    await seekBookProgress(value / 1000)
  } finally {
    mobileBookSliderDraft.value = null
  }
}

async function seekBookProgress(percent) {
  const target = readerBookSeekTarget(percent, chapters.value.length)
  const targetIndex = target.chapterIndex
  const chapterPercent = target.chapterPercent
  if (targetIndex === currentIndex.value) {
    seekCurrentChapterPercent(chapterPercent, { save: true })
    return
  }
  await router.replace({
    name: 'reader',
    params: { id: bookId.value },
    query: { chapter: targetIndex, percent: chapterPercent },
  })
}

function seekCurrentChapterPercent(percent, options = {}) {
  const value = clampReaderPercent(percent)
  if (reader.mode === 'flip') {
    page.value = Math.round(value * Math.max(0, pageCount.value - 1))
    progressVersion.value += 1
    if (options.save !== false) saveCurrentProgress()
    return
  }
  if (!contentEl.value) return
  if (isContinuousScrollRead.value) {
    const chapterEl = contentBody.value?.querySelector(`.chapter-content[data-index="${currentIndex.value}"]`)
    if (chapterEl) {
      const room = Math.max(chapterEl.offsetHeight - contentEl.value.clientHeight, 0)
      contentEl.value.scrollTop = Math.max(0, chapterEl.offsetTop + Math.round(value * room))
    }
  } else {
    const bottom = Math.max(contentEl.value.scrollHeight - contentEl.value.clientHeight, 0)
    contentEl.value.scrollTop = Math.round(value * bottom)
  }
  progressVersion.value += 1
  applyLocalProgressSnapshot()
  if (options.save === false) {
    scheduleProgressSave(500)
  } else {
    saveCurrentProgress()
  }
}

function handleTapZone(zone) {
  if (isOverlayOpen.value) return
  applyReaderTapAction(readerTapZoneAction({
    zone,
    clickMethod: reader.clickMethod,
    mode: reader.mode,
    autoReading: autoReading.value,
  }), {
    mobile: true,
    hideChrome: reader.clickMethod === 'next',
  })
}

function handleReaderContentClick(event) {
  if (isOverlayOpen.value || !pageEl.value) return
  if (Date.now() - handledTouchTapAt < 450) return
  if (consumeSuppressedContentClick()) return
  if (event.defaultPrevented || event.button !== 0) return
  const target = event.target
  if (target?.closest?.('button, a, input, textarea, select, [role="button"]')) return
  const rect = pageEl.value.getBoundingClientRect()
  const point = {
    rect,
    relX: event.clientX - rect.left,
    relY: event.clientY - rect.top,
    clientX: event.clientX,
    clientY: event.clientY,
  }
  if (isMobileReader.value) {
    handleTapPoint(point)
  } else {
    handleDesktopTapPoint(point)
  }
}

function handleReaderTouchStart(event) {
  if (!isMobileReader.value || event.touches?.length !== 1) return
  const touch = event.touches[0]
  readerTouchStart = { x: touch.clientX, y: touch.clientY, at: Date.now() }
  readerTouchMoved = false
  readerTouchMove = { x: 0, y: 0 }
}

function handleReaderTouchMove(event) {
  if (!isMobileReader.value || !readerTouchStart || event.touches?.length !== 1) return
  const touch = event.touches[0]
  const moveX = touch.clientX - readerTouchStart.x
  const moveY = touch.clientY - readerTouchStart.y
  readerTouchMove = { x: moveX, y: moveY }
  if (didReaderTouchMove(readerTouchMove, MOBILE_READER_TAP_MOVE_TOLERANCE)) {
    readerTouchMoved = true
  }
  if (shouldPreventReaderTouchMove({ mode: reader.mode, moveX, moveY })) {
    event.preventDefault()
    event.stopPropagation()
  }
}

function handleReaderTouchEnd(event) {
  if (!isMobileReader.value) return
  const touch = event.changedTouches?.[0]
  if (scheduleSelectedTextOperation(200)) {
    suppressContentClick()
    readerTouchStart = null
    readerTouchMoved = false
    readerTouchMove = { x: 0, y: 0 }
    return
  }
  const elapsed = readerTouchStart ? Date.now() - readerTouchStart.at : 0
  const isTap = isReaderTouchTap({
    move: readerTouchMove,
    elapsed,
    hasTouch: touch,
    tolerance: MOBILE_READER_TAP_MOVE_TOLERANCE,
  })
  if (touch) suppressContentClick(360)
  if (isTap) handledTouchTapAt = Date.now()
  if (readerTouchMoved && !isOverlayOpen.value && shouldHandleReaderHorizontalSwipe({
    mode: reader.mode,
    move: readerTouchMove,
  })) {
    if (readerTouchMove.x > 0) previousPage()
    else nextPage()
  } else if (!readerTouchMoved && !isOverlayOpen.value && pageEl.value) {
    if (touch) {
      const rect = pageEl.value.getBoundingClientRect()
      handleTapPoint({
        rect,
        relX: touch.clientX - rect.left,
        relY: touch.clientY - rect.top,
        clientX: touch.clientX,
        clientY: touch.clientY,
      })
    }
  }
  readerTouchStart = null
  readerTouchMoved = false
  readerTouchMove = { x: 0, y: 0 }
}

function handleTapPoint(point) {
  if (isOverlayOpen.value || !point?.rect) return
  if (scheduleSelectedTextOperation(0)) {
    suppressContentClick()
    return
  }
  const viewportWidth = window.innerWidth || point.rect.width
  const viewportHeight = window.innerHeight || point.rect.height
  const pointX = Number.isFinite(point.clientX) ? point.clientX : point.relX
  const pointY = Number.isFinite(point.clientY) ? point.clientY : point.relY
  applyReaderTapAction(readerTapPointAction({
    mobile: true,
    pointX,
    pointY,
    viewportWidth,
    viewportHeight,
    clickMethod: reader.clickMethod,
    mode: reader.mode,
    autoReading: autoReading.value,
  }), { mobile: true, hideChrome: true })
}

function handleDesktopTapPoint(point) {
  if (isOverlayOpen.value || !point?.rect) return
  if (scheduleSelectedTextOperation(0)) {
    suppressContentClick()
    return
  }
  const viewportWidth = window.innerWidth || point.rect.width
  const viewportHeight = window.innerHeight || point.rect.height
  const pointX = Number.isFinite(point.clientX) ? point.clientX : point.relX
  const pointY = Number.isFinite(point.clientY) ? point.clientY : point.relY
  applyReaderTapAction(readerTapPointAction({
    mobile: false,
    pointX,
    pointY,
    viewportWidth,
    viewportHeight,
    clickMethod: reader.clickMethod,
    mode: reader.mode,
    autoReading: autoReading.value,
  }))
}

function applyReaderTapAction(action, options = {}) {
  if (!action) return
  if (action === 'toggle-chrome') {
    if (options.mobile) toggleMobileReaderChrome()
    else toggleReaderChrome()
    return
  }
  if (options.hideChrome) mobileChromeVisible.value = false
  if (action === 'next') nextPage()
  if (action === 'previous') previousPage()
}

function handleReaderWheel(event) {
  if (event._openReaderWheelHandled) return
  event._openReaderWheelHandled = true
  if (isOverlayOpen.value) return
  if (!shellEl.value?.contains(event.target)) return
  const target = event.target
  if (target?.closest?.('a, input, textarea, select, .el-drawer, .el-dialog')) return
  const delta = normalizedReaderWheelDelta({
    deltaX: event.deltaX,
    deltaY: event.deltaY,
    deltaMode: event.deltaMode,
    fontSize: reader.fontSize,
    lineHeight: reader.lineHeight,
    pageHeight: contentEl.value?.clientHeight || window.innerHeight || 800,
  })
  if (Math.abs(delta) < 4) return
  if (isScrollRead.value) {
    if (!contentEl.value) return
    event.preventDefault()
    scrollReaderByWheel(delta)
    return
  }
  event.preventDefault()
  const now = Date.now()
  if (now - lastWheelPageAt < Math.max(140, reader.animateDuration + 40)) return
  lastWheelPageAt = now
  if (delta > 0) {
    nextPage()
  } else {
    previousPage()
  }
}

function scrollReaderByWheel(delta) {
  const el = contentEl.value
  if (!el) return
  const bottom = Math.max(0, el.scrollHeight - el.clientHeight)
  const atTop = el.scrollTop <= 2
  const atBottom = el.scrollTop >= bottom - 2
  if (delta < 0 && atTop) {
    previousPage()
    return
  }
  if (delta > 0 && atBottom) {
    nextPage()
    return
  }
  el.scrollTop = Math.max(0, Math.min(bottom, el.scrollTop + delta))
}

function toggleReaderChrome() {
  if (isMobileReader.value) {
    mobileChromeVisible.value = !mobileChromeVisible.value
    return
  }
  if (showTocDrawer.value) {
    showTocDrawer.value = false
  } else {
    openTocDrawer()
  }
  showSettingsDrawer.value = false
}

function toggleMobileReaderChrome() {
  if (isMobileReader.value) toggleReaderChrome()
}

function updateFlipLayout() {
  if (!contentEl.value || !contentBody.value) return
  const viewport = readableViewportSize()
  if (reader.mode === 'flip') {
    const layout = readerFlipPageLayout({
      viewportWidth: viewport.width,
      viewportHeight: viewport.height,
      scrollWidth: contentBody.value.scrollWidth,
      currentPage: page.value,
    })
    pageWidth.value = layout.pageWidth
    pageHeight.value = layout.pageHeight
    pageCount.value = layout.pageCount
    page.value = layout.page
    return
  }
  if (reader.mode === 'page') {
    const layout = readerVerticalPageLayout({
      scrollHeight: contentEl.value.scrollHeight,
      clientHeight: contentEl.value.clientHeight,
      scrollTop: contentEl.value.scrollTop,
      pageHeight: scrollStep(),
    })
    pageHeight.value = layout.pageHeight
    pageCount.value = layout.pageCount
    page.value = layout.page
    return
  }
  // 滚动模式
  pageCount.value = 1
  page.value = 0
}

function readableViewportSize() {
  const el = contentEl.value
  if (!el) {
    return { width: window.innerWidth, height: window.innerHeight }
  }
  const style = window.getComputedStyle(el)
  const horizontalPadding = parseFloat(style.paddingLeft || '0') + parseFloat(style.paddingRight || '0')
  const verticalPadding = parseFloat(style.paddingTop || '0') + parseFloat(style.paddingBottom || '0')
  return {
    width: Math.max(1, el.clientWidth - horizontalPadding),
    height: Math.max(1, el.clientHeight - verticalPadding),
  }
}

function handleResize() {
  windowWidth.value = currentViewportWidth()
  updateFlipLayout()
}

function handleReaderPageHide() {
  saveCurrentProgress({ force: true, background: true })
}

function handleReaderVisibilityChange() {
  if (document.hidden) saveCurrentProgress({ force: true, background: true })
}

async function handleProgressUpdated(event) {
  const progress = event?.detail?.progress
  if (!progress?.bookId || Number(progress.bookId) !== Number(bookId.value)) return
  if (!chapter.value || restoringPosition || isProgressSaveBusy()) return
  const localKey = progressSaveKey(currentProgressPayload())
  const remoteKey = progressSaveKey({
    bookId: progress.bookId,
    chapterId: progress.chapterId,
    chapterIndex: progress.chapterIndex,
    offset: progress.offset,
    percent: progress.percent,
    chapterPercent: progress.chapterPercent,
  })
  if (!remoteKey || remoteKey === localKey) return
  const targetIndex = Math.max(0, Math.min(Number(progress.chapterIndex || 0), Math.max(chapters.value.length - 1, 0)))
  const targetOffset = Math.max(0, Math.floor(Number(progress.offset || 0)))
  const restorePercent = Number.isFinite(Number(progress.chapterPercent))
    ? Math.max(0, Math.min(1, Number(progress.chapterPercent)))
    : null
  cancelProgressSave()
  try {
    await router.replace({
      name: 'reader',
      params: { id: bookId.value },
      query: {
        chapter: targetIndex,
        ...(targetOffset ? { offset: targetOffset } : {}),
        ...(restorePercent !== null ? { percent: Number(restorePercent.toFixed(6)) } : {}),
      },
    })
    await loadChapter(targetIndex, targetOffset, { restorePercent, saveAfterLoad: false })
    markProgressSaved(currentProgressPayload())
  } catch {
    // If the chapter cannot be applied immediately, the stored progress will be used on the next open.
  }
}

async function handleReaderBookDataUpdated(event) {
  const detail = event?.detail || {}
  if (!detail.bookId || Number(detail.bookId) !== Number(bookId.value)) return
  if (detail.book?.id) book.value = detail.book
  if (!Array.isArray(detail.chapters)) return
  const restoreOffset = currentOffset()
  const restorePercent = currentChapterPercent()
  const targetIndex = Math.max(0, Math.min(currentIndex.value, Math.max(detail.chapters.length - 1, 0)))
  chapters.value = detail.chapters
  currentIndex.value = targetIndex
  chapterContentCache.clearBook(currentChapterCacheBookKey())
  resetBrowserCachedChapters()
  resetContentSearchState()
  await computeBrowserCachedChapters()
  await loadChapter(targetIndex, restoreOffset, { restorePercent, refresh: true, saveAfterLoad: false })
}

function onScroll() {
  if (!isVerticalRead.value) return
  if (restoringPosition || chapterLoading.value) return
  updateCurrentChapterFromScroll()
  maybeExtendShowChapters()
  updateFlipLayout()
  progressVersion.value += 1
  applyLocalProgressSnapshot()
  scheduleProgressSave(500)
}

function updateCurrentChapterFromScroll() {
  if (!isContinuousScrollRead.value) return
  const snapshot = visibleChapterProgressSnapshot()
  const nextIndex = Number(snapshot?.chapterIndex)
  if (!Number.isInteger(nextIndex) || nextIndex === currentIndex.value) return
  const block = chapterBlocks.value.find(item => item.index === nextIndex)
  currentIndex.value = nextIndex
  chapter.value = snapshot?.chapter || chapters.value[nextIndex] || (block?.id ? { id: block.id, title: block.title, index: nextIndex } : chapter.value)
  content.value = block?.content || content.value
  pruneScroll2ChapterWindow()
}

function maybeExtendShowChapters() {
  if (!isContinuousScrollRead.value || extendingShowChapters || !contentEl.value) return
  const el = contentEl.value
  const extension = readerChapterWindowExtension({
    mode: reader.mode,
    scrollTop: el.scrollTop,
    clientHeight: el.clientHeight,
    scrollHeight: el.scrollHeight,
  })
  if (!extension.previous && !extension.next) return
  extendingShowChapters = true
  Promise.all([
    extension.previous ? prependPreviousShowChapter() : Promise.resolve(),
    extension.next ? appendNextShowChapter() : Promise.resolve(),
  ])
    .catch(() => {})
    .finally(() => {
      extendingShowChapters = false
    })
}

function pruneScroll2ChapterWindow() {
  if (reader.mode !== 'scroll2' || !contentEl.value || !chapterBlocks.value.length) return
  const currentBlocks = chapterBlocks.value
  const plan = readerChapterWindowPrunePlan({
    blocks: currentBlocks,
    currentIndex: currentIndex.value,
    totalChapters: chapters.value.length,
    previousSize: SHOW_PREV_CHAPTER_SIZE,
    nextSize: SHOW_NEXT_CHAPTER_SIZE,
  })
  if (!plan.changed) return
  const removedBeforeHeight = plan.removedBeforeIndexes
    .reduce((sum, index) => {
      const element = contentBody.value?.querySelector(`.chapter-content[data-index="${index}"]`)
      return sum + (element?.getBoundingClientRect?.().height || 0)
    }, 0)
  const beforeTop = contentEl.value.scrollTop
  chapterBlocks.value = plan.blocks
  if (removedBeforeHeight > 0) {
    nextTick(() => {
      if (!contentEl.value) return
      contentEl.value.scrollTop = Math.max(0, beforeTop - removedBeforeHeight)
    })
  }
}

function currentVisibleExcerpt() {
  const paragraph = currentVisibleParagraph()
  const text = paragraph?.textContent?.replace(/\s+/g, ' ').trim()
  if (text) return text.slice(0, 140)
  return lines.value.slice(0, 2).join(' ').slice(0, 140)
}

function handleReaderSelectionEnd() {
  scheduleSelectedTextOperation(180)
}

async function operateSelectedText(text) {
  const action = await ElMessageBox.confirm('请选择对选中文字执行的操作。', '选择文字', {
    confirmButtonText: '添加过滤规则',
    cancelButtonText: '添加书签',
    distinguishCancelAndClose: true,
    closeOnClickModal: false,
    closeOnPressEscape: false,
    type: 'info',
  }).catch(result => result)
  if (action === 'close') return
  if (action === 'cancel') {
    await createBookmarkFromSelectedText(text)
    return
  }
  await createReplaceRuleFromSelectedText(text)
}

async function createReplaceRuleFromSelectedText(text) {
  const prompt = await ElMessageBox.prompt('替换为留空时表示直接过滤该文字。', '添加过滤规则', {
    confirmButtonText: '保存',
    cancelButtonText: '取消',
    inputValue: '',
    inputPlaceholder: '替换为',
  }).catch(() => null)
  if (!prompt) return
  const cleanText = String(text || '').trim()
  if (!cleanText) return
  const name = cleanText.length > 24 ? `${cleanText.slice(0, 24)}...` : cleanText
  await createReplaceRule({
    name,
    pattern: cleanText,
    replacement: String(prompt.value || ''),
    scope: `${book.value?.title || ''};${book.value?.url || ''}`,
    isRegex: false,
    enabled: true,
  })
  window.dispatchEvent(new CustomEvent('openreader:replace-rules-updated'))
  ElMessage.success('过滤规则已添加')
}

function currentProgressPayload() {
  const snapshot = visibleChapterProgressSnapshot()
  return readerProgressPayload({
    bookId: bookId.value,
    visibleSnapshot: snapshot,
    currentChapter: chapter.value,
    currentChapterIndex: currentIndex.value,
    currentOffset: snapshot ? 0 : currentOffset(),
    currentChapterPercent: snapshot ? 0 : currentChapterPercent(),
    totalChapters: chapters.value.length,
  })
}

function applyLocalProgressSnapshot(payload = currentProgressPayload(), options = {}) {
  if (!payload?.bookId || !chapter.value) return
  const nextPayload = {
    ...payload,
    baseUpdatedAt: payload.baseUpdatedAt || progressServerBaseUpdatedAt(payload.bookId),
  }
  const key = progressSaveKey(nextPayload)
  if (key === lastLocalProgressKey && !options.force) return
  lastLocalProgressKey = key
  reader.applyProgress({
    ...nextPayload,
    mode: reader.mode,
    updatedAt: new Date().toISOString(),
    pendingSync: true,
  })
  upsertReaderBookProgress(reader.progressByBook[nextPayload.bookId])
}

function upsertReaderBookProgress(progress, options = {}) {
  if (!progress?.bookId) return
  if (book.value?.id && Number(book.value.id) === Number(progress.bookId)) {
    const nextBook = mergeShelfBook(book.value, {
      id: book.value.id,
      progress,
      shelfOrderAt: progress.updatedAt,
    })
    book.value = nextBook
    bookshelf.upsertBook(nextBook)
    return
  }
  bookshelf.applyBookProgress(progress, options)
}

function progressServerBaseUpdatedAt(targetBookId = bookId.value) {
  return readerProgressBaseUpdatedAt(reader.progressByBook[targetBookId])
}

function progressUpdatedAtMs(progress) {
  const time = Date.parse(progress?.updatedAt || '')
  return Number.isFinite(time) ? time : 0
}

function parseRoutePercent(value) {
  if (value === undefined || value === null || value === '') return null
  const percent = Number(value)
  return Number.isFinite(percent) ? Math.max(0, Math.min(1, percent)) : null
}

async function jumpToBookSearchResult(result) {
  showSearchDrawer.value = false
  const targetIndex = Number(result.chapterIndex || 0)
  const restorePercent = Number.isFinite(Number(result.percent)) ? Number(result.percent) : null
  if (targetIndex === currentIndex.value) {
    await loadChapter(targetIndex, 0, { restorePercent, saveAfterLoad: true })
  } else {
    await router.replace({ name: 'reader', params: { id: bookId.value }, query: { chapter: targetIndex, percent: restorePercent ?? undefined } })
    await loadChapter(targetIndex, 0, { restorePercent, saveAfterLoad: true })
  }
  await nextTick()
  if (jumpToSearchMatch(result)) {
    return
  }
  if (Number.isInteger(result.lineIndex)) {
    jumpToLine(result.lineIndex)
  } else {
    jumpToFirstSearchMatch()
  }
}

function jumpToFirstSearchMatch() {
  const keyword = contentSearch.value.trim().toLowerCase()
  if (!keyword || !contentBody.value) return
  const scope = contentBody.value.querySelector(`.chapter-content[data-index="${currentIndex.value}"]`) || contentBody.value
  const paragraphList = [...scope.querySelectorAll('p')]
  const index = paragraphList.findIndex(item => item.textContent.toLowerCase().includes(keyword))
  if (index >= 0) jumpToLine(index)
}

function jumpToSearchMatch(result) {
  const keyword = String(result?.query || contentSearch.value || route.query.q || '').trim()
  if (!keyword || !contentBody.value) return false
  const targetIndex = Number.isInteger(result?.resultCountWithinChapter)
    ? result.resultCountWithinChapter
    : Number(result?.resultCountWithinChapter ?? route.query.match ?? 0)
  const expectedIndex = Number.isFinite(targetIndex) ? Math.max(0, Math.floor(targetIndex)) : 0
  const scope = contentBody.value.querySelector(`.chapter-content[data-index="${currentIndex.value}"]`) || contentBody.value
  const paragraphs = [...scope.querySelectorAll('p')]
  let matchCount = 0
  for (let index = 0; index < paragraphs.length; index += 1) {
    const text = paragraphs[index].textContent || ''
    const exactMatches = countTextMatches(text, keyword)
    if (matchCount + exactMatches > expectedIndex) {
      jumpToParagraph(paragraphs[index])
      return true
    }
    matchCount += exactMatches
  }
  const normalizedKeyword = normalizeSearchText(keyword)
  if (!normalizedKeyword) return false
  matchCount = 0
  for (let index = 0; index < paragraphs.length; index += 1) {
    const text = normalizeSearchText(paragraphs[index].textContent || '')
    const matches = countTextMatches(text, normalizedKeyword)
    if (matchCount + matches > expectedIndex) {
      jumpToParagraph(paragraphs[index])
      return true
    }
    matchCount += matches
  }
  return false
}

function countTextMatches(text, keyword) {
  const haystack = String(text || '').toLowerCase()
  const needle = String(keyword || '').toLowerCase()
  if (!haystack || !needle) return 0
  let count = 0
  for (let offset = 0; offset < haystack.length;) {
    const position = haystack.indexOf(needle, offset)
    if (position < 0) break
    count += 1
    offset = position + Math.max(needle.length, 1)
  }
  return count
}

function normalizeSearchText(value) {
  return String(value || '').toLowerCase().replace(/[\s\p{P}\p{S}]+/gu, '')
}

function jumpToLine(index) {
  const scope = contentBody.value?.querySelector(`.chapter-content[data-index="${currentIndex.value}"]`) || contentBody.value
  const lineEl = scope?.querySelectorAll('p')?.[index]
  if (!lineEl) return
  jumpToParagraph(lineEl)
}

function jumpToParagraph(lineEl, options = {}) {
  if (!lineEl) return
  showSearchDrawer.value = false
  const chapterEl = lineEl.closest?.('.chapter-content')
  const chapterIndex = Number(chapterEl?.dataset?.index)
  if (Number.isInteger(chapterIndex) && chapterIndex !== currentIndex.value) {
    currentIndex.value = chapterIndex
    const block = chapterBlocks.value.find(item => item.index === chapterIndex)
    chapter.value = chapters.value[chapterIndex] || (block?.id ? { id: block.id, title: block.title, index: chapterIndex } : chapter.value)
    content.value = block?.content || content.value
  }
  if (reader.mode === 'flip') {
    page.value = Math.min(pageCount.value - 1, Math.floor(lineEl.offsetLeft / Math.max(pageWidth.value, 1)))
  } else if (contentEl.value) {
    contentEl.value.scrollTop = Math.max(0, lineEl.offsetTop - 80)
  }
  if (options.flash !== false) flashParagraph(lineEl)
  if (options.save !== false) saveCurrentProgress()
}

async function jumpToRouteLine() {
  if (route.query.q !== undefined && route.query.match !== undefined) {
    await nextTick()
    if (jumpToSearchMatch({
      query: route.query.q,
      resultCountWithinChapter: Number(route.query.match),
      lineIndex: Number(route.query.line),
    })) {
      return
    }
  }
  if (route.query.line === undefined) return
  const index = Number(route.query.line)
  if (!Number.isFinite(index)) return
  await nextTick()
  jumpToLine(Math.max(0, Math.floor(index)))
}

function flashParagraph(lineEl) {
  lineEl.classList.remove('reader-search-active')
  requestAnimationFrame(() => {
    lineEl.classList.add('reader-search-active')
    window.setTimeout(() => lineEl.classList.remove('reader-search-active'), 1800)
  })
}

function scrollToTop() {
  if (reader.mode === 'flip') {
    page.value = 0
    progressVersion.value += 1
    saveCurrentProgress()
    return
  }
  if (contentEl.value) {
    contentEl.value.scrollTop = 0
    progressVersion.value += 1
    saveCurrentProgress()
  }
}

function scrollToBottom() {
  if (reader.mode === 'flip') {
    page.value = Math.max(0, pageCount.value - 1)
    progressVersion.value += 1
    saveCurrentProgress()
    return
  }
  if (contentEl.value) {
    contentEl.value.scrollTop = Math.max(0, contentEl.value.scrollHeight - contentEl.value.clientHeight)
    progressVersion.value += 1
    saveCurrentProgress()
  }
}

// ---- Keyboard ----
useKeyboard({
  onPageUp: () => previousPage(),
  onPageDown: () => nextPage(),
  onArrowLeft: () => {
    mobileChromeVisible.value = false
    if (reader.mode === 'flip') previousPage()
    else if (currentIndex.value > 0) goChapter(currentIndex.value - 1, READER_CHAPTER_END_OFFSET)
  },
  onArrowRight: () => {
    mobileChromeVisible.value = false
    if (reader.mode === 'flip') nextPage()
    else if (currentIndex.value < chapters.value.length - 1) goChapter(currentIndex.value + 1)
  },
  onArrowUp: () => {
    mobileChromeVisible.value = false
    if (reader.mode === 'page' || isScrollRead.value) previousPage()
  },
  onArrowDown: () => {
    mobileChromeVisible.value = false
    if (reader.mode === 'page' || isScrollRead.value) nextPage()
  },
  onHome: () => scrollToTop(),
  onEnd: () => scrollToBottom(),
  onSpace: () => nextPage(),
  onEscape: () => {
    if (showTocDrawer.value || showSettingsDrawer.value) {
      showTocDrawer.value = false; showSettingsDrawer.value = false
    } else {
      mobileChromeVisible.value = false
      goShelf()
    }
  },
})

useGesture(pageEl, {
  onPinchOut: () => reader.setFontSize(reader.fontSize + 2),
  onPinchIn: () => reader.setFontSize(reader.fontSize - 2),
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
    var(--reader-body-bg);
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
  height: 100dvh; line-height: var(--reader-line-height);
  overflow-y: auto; overflow-x: hidden;
  padding: 44px 65px 180px;
  width: 100%;
  box-sizing: border-box;
  scroll-padding-bottom: 180px;
}
.reader-body { transition: transform var(--reader-animate-duration, 180ms) ease; }
.reader-shell.scroll .reader-body::after,
.reader-shell.scroll2 .reader-body::after {
  content: "";
  display: block;
  height: min(40vh, 280px);
}
/* 翻页模式 */
.reader-shell.flip .reader-content {
  overflow: hidden;
}
.reader-shell.flip .reader-body {
  height: 100%;
  column-width: var(--reader-page-width);
  column-gap: 0;
  column-fill: auto;
}
.reader-shell.flip .reader-body {
  transition: transform var(--reader-animate-duration, 180ms) ease;
}

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
  gap: 14px;
  margin: -2px 0 14px;
}

.reader-shell :deep(.el-drawer) {
  color: var(--reader-text);
  background: var(--reader-popup-bg);
}

.reader-shell :deep(.el-drawer__header) {
  color: var(--reader-text);
  margin-bottom: 14px;
}

.reader-shell :deep(.el-drawer__body) {
  background: var(--reader-popup-bg);
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
.reader-drawer-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 14px;
}
/* ---- 响应式 ---- */
@media (max-width: 750px) {
  .reader-shell {
    --reader-frame-width: 100%;
    --reader-content-width: calc(100% - 44px);
    min-height: 100dvh;
    width: 100%;
    max-width: 100%;
    min-width: 0;
    box-sizing: border-box;
    overflow: hidden;
    padding: 0;
  }
  .reader-page {
    height: 100dvh;
    border: 0;
    width: 100%;
    max-width: 100%;
    min-width: 0;
    box-sizing: border-box;
  }
  .reader-page-head { display: none; }
  .reader-content {
    box-sizing: border-box;
    width: 100%;
    max-width: 100%;
    min-width: 0;
    font-size: var(--reader-font-size);
    padding: 42px 22px calc(42px + env(safe-area-inset-bottom));
    scroll-padding-bottom: calc(42px + env(safe-area-inset-bottom));
    touch-action: pan-y pinch-zoom;
  }
  .reader-shell.scroll .reader-content,
  .reader-shell.scroll2 .reader-content {
    scrollbar-width: none;
    -ms-overflow-style: none;
  }
  .reader-shell.scroll .reader-content::-webkit-scrollbar,
  .reader-shell.scroll2 .reader-content::-webkit-scrollbar {
    display: none;
    width: 0;
    height: 0;
  }
  .reader-shell.mobile-chrome-visible .reader-content {
    padding-bottom: calc(250px + env(safe-area-inset-bottom));
    scroll-padding-bottom: calc(250px + env(safe-area-inset-bottom));
  }
}
</style>
