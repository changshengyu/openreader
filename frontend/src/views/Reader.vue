<template>
  <main ref="shellEl" class="reader-shell" :class="[effectiveReaderMode, { 'mobile-chrome-visible': mobileChromeVisible }]" :style="readerStyle">
    <ReaderDesktopTools
      :auto-reading="autoReading"
      :auto-reading-supported="!isAudioChapter"
      :tts-playing="tts.state.playing"
      :tts-supported="ttsSupportedForChapter"
      :active-panel="desktopWorkspacePanel"
      :is-night="isNightTheme"
      @action="handleDesktopToolAction"
    />

    <ReaderDesktopWorkspacePanel
      v-if="!isMobileReader && desktopWorkspacePanel"
      :panel="desktopWorkspacePanel"
      :title="desktopWorkspaceTitle"
      @close="closeDesktopWorkspace"
    >
      <template #actions>
        <template v-if="desktopWorkspacePanel === 'shelf'">
          <button type="button" :disabled="shelfLoading" @click="refreshReaderShelf">
            {{ shelfLoading ? '刷新中...' : '刷新' }}
          </button>
        </template>
        <template v-else-if="desktopWorkspacePanel === 'toc'">
          <button v-if="chapters.length" type="button" @click="toggleTocReverse">{{ tocReverse ? '顺序' : '倒序' }}</button>
          <button v-if="chapters.length" type="button" @click="scrollTocTop">顶部</button>
          <button v-if="chapters.length" type="button" @click="scrollTocBottom">底部</button>
          <button v-if="canChangeLocalTocRule" type="button" :disabled="tocRefreshing" @click="changeReaderLocalTocRule">修改规则</button>
          <button type="button" :disabled="tocRefreshing" @click="refreshTocDrawer">{{ tocRefreshing ? '刷新中...' : '刷新' }}</button>
        </template>
      </template>

      <ReaderShelfPanel
        v-if="desktopWorkspacePanel === 'shelf'"
        ref="shelfPanelRef"
        v-loading="shelfLoading"
        :books="filteredShelfBooks"
        :current-book-id="bookId"
        :progress-by-book="reader.progressByBook"
        :loading="shelfLoading"
        @select="changeBookFromShelf"
      />
      <SourceSwitchPanel
        v-else-if="desktopWorkspacePanel === 'source'"
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
      <ReaderTocPanel
        v-else-if="desktopWorkspacePanel === 'toc'"
        ref="tocPanelRef"
        :chapters="chapters"
        :current-index="currentIndex"
        :reverse="tocReverse"
        :locate-key="tocLocateKey"
        :browser-cached-map="browserCachedChapters"
        desktop-grid
        @jump="jumpFromToc"
      />
      <ReaderSettingsPanel
        v-else-if="desktopWorkspacePanel === 'settings'"
        v-model:custom-bg="customBg"
        v-model:line-height="sliderLineHeight"
        :reader="reader"
        :tts="tts"
        :tts-voices="ttsVoices"
        :font-options="fontOptions"
        :theme-presets="themePresets"
        :mini-interface="false"
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
    </ReaderDesktopWorkspacePanel>

    <ReaderMobileChrome
      :visible="mobileChromeVisible"
      :auto-reading="autoReading"
      :auto-reading-supported="!isAudioChapter"
      :tts-playing="tts.state.playing"
      :tts-supported="ttsSupportedForChapter"
      :is-night="isNightTheme"
      :book-progress-label="bookProgressLabel"
      :page-slider-visible="!isAudioChapter"
      :page-slider-value="mobilePageSliderValue"
      :page-slider-max="mobilePageSliderMax"
      :page-slider-label="mobilePageProgressLabel"
      :cache-visible="showCacheContentZone"
      :caching="isCachingContent"
      :cache-status-text="cachingContentTip"
      :previous-disabled="currentIndex <= 0"
      :next-disabled="currentIndex >= chapters.length - 1"
      @action="handleMobileChromeAction"
      @cache="cacheFollowingChapters"
      @cache-cancel="cancelCachingContent"
      @page-progress-input="handleMobilePageProgressInput"
      @page-progress-change="handleMobilePageProgressChange"
    />

    <button
      v-if="isMobileReader && isReaderPrimaryPanelOpen()"
      class="reader-mobile-primary-dismiss"
      type="button"
      aria-label="关闭阅读主面板"
      @click.stop="closeReaderPrimaryPanels"
      @touchstart.stop
      @touchmove.stop
      @touchend.stop
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
            :mode="effectiveReaderMode"
            :epub-resource="epubResource"
            :audio-resource="audioResource"
            :audio-initial-time="audioInitialTime"
            :audio-title="chapter?.title || book?.title || ''"
            :audio-cover-url="book?.customCoverUrl || book?.coverUrl || ''"
            :audio-autoplay="audioAutoplay"
            :previous-disabled="currentIndex <= 0"
            :next-disabled="currentIndex >= chapters.length - 1"
            :epub-style="epubStyleText"
            :viewport-height="readerViewportHeight"
            @reload="reloadChapter"
            @epub-load="handleEpubLoad"
            @epub-height="handleEpubHeight"
            @epub-click="handleEpubClick"
            @epub-hash="handleEpubHash"
            @epub-navigate="handleEpubNavigate"
            @epub-keydown="handleEpubKeydown"
            @epub-preview="handleEpubPreview"
            @epub-error="handleEpubError"
            @audio-loaded="handleAudioLoaded"
            @audio-progress="handleAudioProgress"
            @audio-ended="handleAudioEnded"
            @audio-error="handleAudioError"
            @audio-previous="goAudioChapter(currentIndex - 1)"
            @audio-next="goAudioChapter(currentIndex + 1)"
            @image-load="handleReaderImageLoad"
            @retry-block="retryContinuousChapter"
          />
        </div>
      </article>
      <ReaderClickZones
        v-if="chapterFormat !== 'epub' && !isAudioChapter"
        :mode="effectiveReaderMode"
        :show-overlay="showClickZoneOverlay"
        @tap="handleTapZone"
        @close-overlay="showClickZoneOverlay = false"
      />
    </section>

    <ReaderDesktopProgress
      :book-progress-label="bookProgressLabel"
      :cache-visible="showCacheContentZone"
      :caching="isCachingContent"
      :cache-status-text="cachingContentTip"
      :previous-disabled="currentIndex <= 0"
      :next-disabled="currentIndex >= chapters.length - 1"
      @cache-toggle="toggleCacheContentZone"
      @cache="cacheFollowingChapters"
      @cache-cancel="cancelCachingContent"
      @previous="goChapter(currentIndex - 1)"
      @next="goChapter(currentIndex + 1)"
    />

    <!-- TTS 朗读条 -->
    <ReaderTTSBar
      v-if="ttsBarShown"
      :playing="tts.state.playing"
      :paused="tts.state.paused"
      :rate="tts.state.rate"
      :pitch="tts.state.pitch"
      :voices="ttsVoices"
      :voice-uri="reader.ttsVoiceURI"
      :config-expanded="ttsConfigExpanded"
      :sleep-minutes="ttsSleepMinutes"
      :progress-text="ttsProgressLabel"
      @backward="ttsPrevious"
      @play="toggleTTS"
      @pause="tts.pause"
      @resume="tts.resume"
      @forward="ttsNext"
      @close="closeTTSBar"
      @toggle-config="ttsConfigExpanded = !ttsConfigExpanded"
      @voice-change="setTTSVoice"
      @rate-change="setTTSRate"
      @pitch-change="setTTSPitch"
      @sleep-change="setTTSSleepMinutes"
    />

    <!-- Toast -->
    <div v-if="toastMsg" class="reader-toast">{{ toastMsg }}</div>

    <!-- ===== 移动端书架面板 ===== -->
    <ReaderMobileWorkspacePanel
      v-if="isMobileReader && showShelfDrawer"
      primary
      :show-header="false"
      :title="`书架(${filteredShelfBooks.length})`"
      @close="showShelfDrawer = false"
    >
      <div class="reader-mobile-primary-popover-body reader-mobile-primary-shelf">
        <div class="reader-mobile-primary-title-zone">
          <div class="reader-mobile-primary-title">书架({{ filteredShelfBooks.length }})</div>
          <div class="reader-mobile-primary-actions">
            <button type="button" :disabled="shelfLoading" @click="refreshReaderShelf">
              {{ shelfLoading ? '刷新中...' : '刷新' }}
            </button>
          </div>
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
      </div>
    </ReaderMobileWorkspacePanel>

    <!-- ===== 移动端目录面板 ===== -->
    <ReaderMobileWorkspacePanel
      v-if="isMobileReader && showTocDrawer"
      primary
      :show-header="false"
      :title="`目录(${chapters.length})`"
      @close="showTocDrawer = false"
    >
      <div class="reader-mobile-primary-popover-body reader-mobile-primary-toc">
        <div class="reader-mobile-primary-title-zone">
          <div class="reader-mobile-primary-title">目录<span v-if="chapters.length">({{ chapters.length }})</span></div>
          <div class="reader-mobile-primary-actions">
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
      </div>
    </ReaderMobileWorkspacePanel>

    <!-- ===== 移动端书源面板 ===== -->
    <ReaderMobileWorkspacePanel
      v-if="isMobileReader && showSourceDrawer"
      primary
      :show-header="false"
      title="书源"
      @close="showSourceDrawer = false"
    >
      <div class="reader-mobile-primary-popover-body reader-mobile-primary-source">
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
      </div>
    </ReaderMobileWorkspacePanel>

    <!-- ===== 移动端设置面板 ===== -->
    <ReaderMobileWorkspacePanel
      v-if="isMobileReader && showSettingsDrawer"
      primary
      title="设置"
      :show-header="false"
      @close="showSettingsDrawer = false"
    >
      <div class="reader-mobile-primary-popover-body reader-mobile-primary-settings">
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
      </div>
    </ReaderMobileWorkspacePanel>

    <el-image-viewer
      v-if="epubPreviewVisible"
      :url-list="epubPreviewImages"
      :initial-index="epubPreviewIndex"
      @close="epubPreviewVisible = false"
    />
  </main>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api/client'
import { refreshBook, refreshLocalBook } from '../api/books'
import { getRemoteReaderChapterContent, getRemoteReaderSession } from '../api/remoteReader'
import { listSources } from '../api/sources'
import { deleteAsset, uploadAsset } from '../api/uploads'
import ReaderChapterContent from '../components/reader/ReaderChapterContent.vue'
import ReaderClickZones from '../components/reader/ReaderClickZones.vue'
import ReaderDesktopWorkspacePanel from '../components/reader/ReaderDesktopWorkspacePanel.vue'
import ReaderDesktopProgress from '../components/reader/ReaderDesktopProgress.vue'
import ReaderDesktopTools from '../components/reader/ReaderDesktopTools.vue'
import ReaderMobileWorkspacePanel from '../components/reader/ReaderMobileWorkspacePanel.vue'
import ReaderMobileChrome from '../components/reader/ReaderMobileChrome.vue'
import ReaderShelfPanel from '../components/reader/ReaderShelfPanel.vue'
import ReaderSettingsPanel from '../components/reader/ReaderSettingsPanel.vue'
import ReaderTTSBar from '../components/reader/ReaderTTSBar.vue'
import SourceSwitchPanel from '../components/reader/SourceSwitchPanel.vue'
import ReaderTocPanel from '../components/reader/ReaderTocPanel.vue'
import { mergeShelfBook, useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore, themePresets } from '../stores/reader'
import { useGesture } from '../composables/useGesture'
import { useReaderAppearanceAssets } from '../composables/useReaderAppearanceAssets'
import { useReaderAutoReading } from '../composables/useReaderAutoReading'
import { useReaderBookLoad } from '../composables/useReaderBookLoad'
import { useReaderBookState } from '../composables/useReaderBookState'
import { useReaderCatalogActions } from '../composables/useReaderCatalogActions'
import { useBookSourceChange } from '../composables/useBookSourceChange'
import { useBookSourceCandidates } from '../composables/useBookSourceCandidates'
import { useReaderChapterCache } from '../composables/useReaderChapterCache'
import { useReaderChapterContent } from '../composables/useReaderChapterContent'
import { useReaderChapterLoader } from '../composables/useReaderChapterLoader'
import { useReaderChapterMaintenance } from '../composables/useReaderChapterMaintenance'
import { isCBZBook, useReaderChapterPresentation } from '../composables/useReaderChapterPresentation'
import { useReaderChapterWindow } from '../composables/useReaderChapterWindow'
import { useReaderChrome } from '../composables/useReaderChrome'
import { useReaderExternalUpdates } from '../composables/useReaderExternalUpdates'
import { epubChapterIndexForResourceURL } from '../composables/useReaderEpubFrame'
import { useReaderLayout } from '../composables/useReaderLayout'
import { useReaderKeyboard } from '../composables/useReaderKeyboard'
import { useReaderLocalTocRulePicker } from '../composables/useReaderLocalTocRulePicker'
import { useReaderLocalProgress } from '../composables/useReaderLocalProgress'
import { useReaderProgressPersistence } from '../composables/useReaderProgressPersistence'
import { useReaderProgressControls } from '../composables/useReaderProgressControls'
import { useReaderBookmarkActions } from '../composables/useReaderBookmarkActions'
import { useReaderNavigation } from '../composables/useReaderNavigation'
import { readerEffectiveMode, useReaderMode } from '../composables/useReaderMode'
import { useReaderPageLifecycle } from '../composables/useReaderPageLifecycle'
import { useReaderPanels } from '../composables/useReaderPanels'
import { useReaderPrimaryPanels } from '../composables/useReaderPrimaryPanels'
import { useReaderPositionRestore } from '../composables/useReaderPositionRestore'
import { useReaderPointer } from '../composables/useReaderPointer'
import { useReaderRouteSync } from '../composables/useReaderRouteSync'
import { useReaderScrollSync } from '../composables/useReaderScrollSync'
import { useReaderSelectedTextActions } from '../composables/useReaderSelectedTextActions'
import { useReaderSelection } from '../composables/useReaderSelection'
import { useReaderSearchNavigation } from '../composables/useReaderSearchNavigation'
import { useReaderShelf } from '../composables/useReaderShelf'
import { useReaderToc } from '../composables/useReaderToc'
import { useReaderToast } from '../composables/useReaderToast'
import { useReaderTools } from '../composables/useReaderTools'
import { useReaderTTS } from '../composables/useReaderTTS'
import { useReaderTypographySync } from '../composables/useReaderTypographySync'
import { useReaderViewportProgress } from '../composables/useReaderViewportProgress'
import { useReaderWheel } from '../composables/useReaderWheel'
import { bookCategoryIds, createBookCategoryNameResolver } from '../utils/bookCategory'
import { clearBookBrowserChapterCache, loadBrowserChapterContent } from '../utils/bookChapterCache'
import { cacheFirstRequest, networkFirstRequest } from '../utils/browserCache'
import { isEPUBLocalBook as checkEPUBLocalBook, isTextLocalBook as checkTextLocalBook } from '../utils/localBookToc'
import { readerFontOptions, readerFontStack, syncReaderFontFaces } from '../utils/readerFonts'
import {
  readerBookmarkText,
  selectedTextBookmarkContext,
} from '../utils/readerBookmarkContext'
import { readerTextProgress, selectVisibleReaderBlock } from '../utils/readerVisibility'
import { readerTTSBarVisible } from '../utils/readerTTS'
import { createReaderScrollAnimator } from '../utils/readerAnimation'
import {
  readerScrollBehaviorForDuration,
  readerScrollStep,
} from '../utils/readerPagination'
import { currentViewportWidth, shouldUseMiniInterface } from '../utils/responsive'
import { createMultiBookChapterMemoryCache } from '../utils/multiBookChapterMemoryCache'
import { sourceCandidateSourceName } from '../utils/sourceCandidate'

const route = useRoute()
const router = useRouter()
const reader = useReaderStore()
const bookshelf = useBookshelfStore()
const overlay = useOverlayStore()
const readerScrollAnimator = createReaderScrollAnimator()
const categoryName = createBookCategoryNameResolver(() => bookshelf.categories)
const remoteSessionId = computed(() => (
  route.name === 'remote-reader' ? String(route.params.sessionId || '') : ''
))
const isTemporaryRemoteReader = computed(() => Boolean(remoteSessionId.value))
const bookId = computed(() => (
  isTemporaryRemoteReader.value ? `remote:${remoteSessionId.value}` : Number(route.params.id)
))

function readerRouteLocation(query = {}) {
  return isTemporaryRemoteReader.value
    ? { name: 'remote-reader', params: { sessionId: remoteSessionId.value }, query }
    : { name: 'reader', params: { id: bookId.value }, query }
}

function temporaryReaderUnavailable() {
  showReaderToast('临时阅读请先加入书架后使用此功能', 2200)
}
const {
  clearBgImage,
  clearFontFile,
  pickBgImage,
  pickFontFile,
  setTheme,
  toggleNight,
} = useReaderAppearanceAssets({
  reader,
  upload: uploadAsset,
  removeAsset: deleteAsset,
  saveSettings: () => reader.saveReaderSettings(),
  syncFonts: syncReaderFontFaces,
  onSuccess: message => ElMessage.success(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})

const book = ref(null)
const chapters = ref([])
const chapter = ref(null)
const currentIndex = ref(Number(route.query.chapter || 0))
const isNightTheme = computed(() => reader.themeType === 'night')
const {
  cacheKey: readerDataCacheKey,
  invalidate: invalidateReaderDataCache,
  mergeLoadedBook,
  write: writeReaderDataCache,
} = useReaderBookState({
  book,
  bookId,
  bookshelf,
  mergeBook: mergeShelfBook,
})
const {
  createFromSelectedText: createBookmarkFromSelectedText,
  currentDraft: currentBookmarkDraft,
  openNote: openNoteDialog,
} = useReaderBookmarkActions({
  book,
  chapter,
  currentIndex,
  getOffset: () => currentOffset(),
  getPercent: () => currentChapterPercent(),
  getCurrentContext: currentBookmarkParagraphContext,
  getSelectedTextContext: selectedBookmarkContextFromText,
  onSelectedTextNotFound: () => ElMessage.error('选择1-2段整段文字才能定位段落'),
  openForm: (...args) => overlay.openBookmarkForm(...args),
})
const {
  operate: operateSelectedText,
} = useReaderSelectedTextActions({
  getBook: () => book.value,
  confirm: (...args) => ElMessageBox.confirm(...args),
  createBookmark: createBookmarkFromSelectedText,
  openReplaceRuleEditor: draft => overlay.openReplaceRuleEditor(draft),
})
const content = ref('')
const chapterFormat = ref('text')
const epubResource = ref(null)
const audioResource = ref(null)
const audioInitialTime = ref(0)
const audioCurrentTime = ref(0)
const audioDuration = ref(0)
const audioAutoplay = ref(false)
const epubPendingRestore = ref(null)
const epubPreviewVisible = ref(false)
const epubPreviewImages = ref([])
const epubPreviewIndex = ref(0)
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
const handleReaderSelectionEnd = () => scheduleSelectedTextOperation(180)
const pageEl = ref(null)
const shellEl = ref(null)
const page = ref(0)
const pageCount = ref(1)
const showSettingsDrawer = ref(false)
const showSourceDrawer = ref(false)
const showCacheContentZone = ref(false)
const showClickZoneOverlay = ref(false)
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
  onChanged: (...args) => applyReaderSourceChange(...args),
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
  saveProgress: () => saveCurrentProgress({ force: true }),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})
const {
  message: toastMsg,
  show: showReaderToast,
} = useReaderToast()
const progressVersion = ref(0)
const customBg = ref('')
const sliderLineHeight = ref(2.12)
const pageHeight = ref(600)
const pageWidth = ref(600)
const windowWidth = ref(currentViewportWidth())
const restoringPosition = ref(false)
const chapterContentCache = createMultiBookChapterMemoryCache(3)

const fontOptions = readerFontOptions
const NEARBY_PRELOAD_RADIUS = 2

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
  isTemporaryReader: isTemporaryRemoteReader,
  onUnavailable: temporaryReaderUnavailable,
  refreshCachedChapters: (...args) => computeBrowserCachedChapters(...args),
  syncCurrentChapter: (...args) => updateCurrentChapterFromScroll(...args),
  goChapter: (...args) => goChapter(...args),
  refreshRemoteCatalog: (...args) => refreshReaderBookCatalog(...args),
  refreshLocalCatalog: (...args) => loadChapters(...args),
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
  isTemporaryReader: isTemporaryRemoteReader,
  afterCache: (...args) => loadChapters(...args),
  onClearMemory: () => clearChapterContentMemory(),
  notify: message => showReaderToast(message, 1600),
  onNoTargets: () => ElMessage.error('不需要缓存'),
  onUnavailable: temporaryReaderUnavailable,
  onError: error => ElMessage.error(readError(error, '缓存章节失败')),
})
const {
  clear: clearChapterContentMemory,
  get: getChapterContentFromMemory,
  load: loadChapterContent,
  preload: preloadNearbyChapters,
} = useReaderChapterContent({
  book,
  bookId,
  chapters,
  memoryCache: chapterContentCache,
  markCached: markBrowserChapterCached,
  preloadRadius: NEARBY_PRELOAD_RADIUS,
  shouldCache: () => !isTemporaryRemoteReader.value,
  loadBrowserContent: async (targetBook, targetBookId, index, options) => {
    if (isTemporaryRemoteReader.value && targetBookId === bookId.value) {
      const { data } = await getRemoteReaderChapterContent(remoteSessionId.value, index)
      return data
    }
    return loadBrowserChapterContent(targetBook, targetBookId, index, options)
  },
})
const {
  clearCurrentBookCache,
  loadChapters,
  reloadChapter,
  resetCaches: resetReaderChapterCaches,
} = useReaderChapterMaintenance({
  book,
  bookId,
  chapters,
  currentIndex,
  isRemoteBook,
  isTemporaryReader: isTemporaryRemoteReader,
  onUnavailable: temporaryReaderUnavailable,
  fetchChapters: async targetBookId => {
    const { data } = await api.get(`/books/${targetBookId}/chapters`)
    return data
  },
  writeDataCache: writeReaderDataCache,
  clearMemory: clearChapterContentMemory,
  resetBrowserState: resetBrowserCachedChapters,
  clearBrowserCache: clearBookBrowserChapterCache,
  loadChapter: (...args) => loadChapter(...args),
  getCurrentOffset: () => currentOffset(),
  clearServerCache: ids => bookshelf.batchClearCache(ids),
  clearCurrentBrowserCache: clearCurrentBookBrowserCache,
  notify: message => showReaderToast(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})
const {
  choose: chooseReaderLocalTocRule,
} = useReaderLocalTocRulePicker({
  book,
  isEPUBLocalBook,
  prompt: (...args) => ElMessageBox.prompt(...args),
  confirm: (...args) => ElMessageBox.confirm(...args),
})
const {
  applySourceChange: applyReaderSourceChange,
  changeLocalTocRule: changeReaderLocalTocRule,
  refreshRemoteCatalog: refreshReaderBookCatalog,
} = useReaderCatalogActions({
  book,
  bookId,
  chapters,
  currentIndex,
  canChangeLocalTocRule,
  chooseLocalTocRule: chooseReaderLocalTocRule,
  runTocRefreshing,
  refreshLocalBook: async (...args) => {
    const { data } = await refreshLocalBook(...args)
    return data
  },
  refreshRemoteBook: async (...args) => {
    const { data } = await refreshBook(...args)
    return data
  },
  invalidateDataCache: invalidateReaderDataCache,
  resetChapterCaches: resetReaderChapterCaches,
  mergeLoadedBook,
  upsertBook: row => bookshelf.upsertBook(row),
  getOverlayBook: () => overlay.bookInfoBook,
  setOverlayBook: row => {
    overlay.bookInfoBook = row
  },
  writeDataCache: writeReaderDataCache,
  loadChapters,
  loadChapter: (...args) => loadChapter(...args),
  refreshBrowserCachedChapters: computeBrowserCachedChapters,
  locateCurrentTocChapter: locateTocCurrentChapter,
  getCurrentOffset: () => currentOffset(),
  getCurrentChapterPercent: () => currentChapterPercent(),
  fetchChapters: async targetBookId => {
    const { data } = await api.get(`/books/${targetBookId}/chapters`)
    return data
  },
  refreshSourceCandidates,
  closeSourceDrawer: () => {
    showSourceDrawer.value = false
  },
  notify: (...args) => showReaderToast(...args),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})
const {
  chapterBlockTextLength,
  displayChapterTitle,
  makeChapterBlock,
  makeParagraphs,
} = useReaderChapterPresentation({
  reader,
  book,
  chapters,
})

const chapterParagraphs = computed(() => {
  return makeParagraphs(content.value, chapter.value?.title)
})
const lines = computed(() => chapterParagraphs.value.filter(item => item.type === 'text').map(item => item.text))
const chapterTextLength = computed(() => {
  return chapterBlockTextLength({ paragraphs: chapterParagraphs.value })
})
const isAudioChapter = computed(() => chapterFormat.value === 'audio')
const ttsBarRequested = ref(false)
const ttsConfigExpanded = ref(true)
const isComicChapter = computed(() => (
  makeChapterBlock(currentIndex.value, chapter.value, content.value).isComic === true
))
const isOrdinaryImageComicChapter = computed(() => (
  isComicChapter.value && !isCBZBook(book.value)
))
const ttsReadBarLayoutActive = computed(() => (
  ttsBarRequested.value
    && chapterFormat.value !== 'epub'
    && !isAudioChapter.value
    && !isComicChapter.value
))
const effectiveReaderMode = computed(() => (
  readerEffectiveMode(
    reader.mode,
    chapterFormat.value === 'epub',
    isAudioChapter.value,
    ttsReadBarLayoutActive.value,
    isOrdinaryImageComicChapter.value,
  )
))
const effectiveReaderState = {
  get mode() {
    return effectiveReaderMode.value
  },
  get clickMethod() {
    return reader.clickMethod
  },
  get fontSize() {
    return reader.fontSize
  },
  get lineHeight() {
    return reader.lineHeight
  },
  get animateDuration() {
    return reader.animateDuration
  },
}
const isVerticalPagedRead = computed(() => !isAudioChapter.value && effectiveReaderMode.value === 'page')
const isScrollRead = computed(() => (
  !isAudioChapter.value && (effectiveReaderMode.value === 'scroll' || effectiveReaderMode.value === 'scroll2')
))
const isVerticalRead = computed(() => isVerticalPagedRead.value || isScrollRead.value)
const isContinuousScrollRead = computed(() => (
  !isAudioChapter.value && (effectiveReaderMode.value === 'scroll' || effectiveReaderMode.value === 'scroll2')
))
const displayedChapterBlocks = computed(() => {
  if (chapterFormat.value === 'epub' || isAudioChapter.value) return []
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
  isEPUB: computed(() => chapterFormat.value === 'epub' || isAudioChapter.value),
  getMode: () => effectiveReaderMode.value,
  makeChapterBlock,
  chapterBlockTextLength,
  nextFrame,
})
const {
  apply: applyLocalProgressSnapshot,
  currentPayload: currentProgressPayload,
  serverBaseUpdatedAt: progressServerBaseUpdatedAt,
  upsert: upsertReaderBookProgress,
} = useReaderLocalProgress({
  reader,
  bookshelf,
  bookId,
  book,
  chapter,
  chapters,
  currentIndex,
  getVisibleSnapshot: visibleChapterProgressSnapshot,
  getCurrentPayload: currentAudioProgressPayload,
  getCurrentOffset: currentOffset,
  getCurrentPercent: currentChapterPercent,
  mergeBook: mergeShelfBook,
  isTemporaryReader: isTemporaryRemoteReader,
})
const {
  compute: computeShowChapterList,
  maybeExtend: maybeExtendShowChapters,
  retry: retryContinuousChapter,
  syncCurrentChapter: updateCurrentChapterFromScroll,
} = useReaderChapterWindow({
  reader,
  contentEl,
  contentBody,
  chapters,
  currentIndex,
  chapter,
  content,
  chapterBlocks,
  isContinuousScrollRead,
  loadContent: loadChapterContent,
  makeChapterBlock,
  captureScrollAnchor: captureReaderScrollAnchor,
  restoreScrollAnchor: restoreReaderScrollAnchor,
  visibleProgressSnapshot: visibleChapterProgressSnapshot,
  nextFrame,
  nextSize: 1,
  formatError: error => readError(error, '章节加载失败，请检查书源或网络后重试'),
})
const {
  readableViewportSize,
  resize: handleResize,
  update: updateFlipLayout,
} = useReaderLayout({
  reader: effectiveReaderState,
  contentEl,
  contentBody,
  page,
  pageCount,
  pageWidth,
  pageHeight,
  windowWidth,
  getScrollStep: scrollStep,
  getViewportWidth: currentViewportWidth,
})
const {
  jumpToFirstSearchMatch,
  jumpToLine,
  jumpToMatch: jumpToSearchMatch,
  jumpToParagraph,
  jumpToRouteLine,
} = useReaderSearchNavigation({
  keyword: computed(() => String(route.query.q || '')),
  contentEl,
  contentBody,
  currentIndex,
  chapterBlocks,
  chapters,
  chapter,
  content,
  page,
  pageCount,
  pageWidth,
  getMode: () => effectiveReaderMode.value,
  getRouteQuery: () => route.query,
  navigate: query => router.replace(readerRouteLocation(query)),
  loadChapter: (index, loadOptions) => loadChapter(index, 0, loadOptions),
  canMatchBookmark: () => chapterFormat.value === 'text',
  onBookmarkNotFound: () => ElMessage.error('无法定位内容所在段落'),
  flashParagraph,
  saveProgress: () => saveCurrentProgress(),
})
const {
  cancelPageAnimation,
  goChapter,
  jumpToLoadedChapter,
  jumpWithinCurrentChapter,
  nextPage,
  paragraphByChapterPosition,
  previousPage,
  scrollToBottom,
  scrollToTop,
} = useReaderNavigation({
  scrollAnimator: readerScrollAnimator,
  contentEl,
  contentBody,
  chapterBlocks,
  chapters,
  currentIndex,
  chapter,
  content,
  page,
  pageCount,
  progressVersion,
  isContinuousScrollRead,
  isVerticalRead,
  getMode: () => effectiveReaderMode.value,
  getAnimateDuration: () => reader.animateDuration,
  scrollStep,
  jumpToParagraph,
  rebuildContinuousWindow: index => computeShowChapterList({
    anchorIndex: index,
    activate: true,
  }),
  closeToc: () => {
    showTocDrawer.value = false
  },
  navigate: query => router.replace(readerRouteLocation(query)),
  saveProgress: () => saveCurrentProgress(),
  scheduleProgressSave: delay => scheduleProgressSave(delay),
})
onBeforeUnmount(cancelPageAnimation)
const {
  restore: restoreReadingPosition,
} = useReaderPositionRestore({
  reader,
  contentEl,
  contentBody,
  currentIndex,
  page,
  pageCount,
  isContinuousScrollRead,
  paragraphByChapterPosition,
  jumpToParagraph,
  updateLayout: updateFlipLayout,
  nextFrame,
})
const {
  bookProgress,
  bookProgressLabel,
  mobilePageProgressLabel,
  mobilePageSliderMax,
  mobilePageSliderValue,
  handleMobilePageProgressChange,
  handleMobilePageProgressInput,
} = useReaderProgressControls({
  scrollAnimator: readerScrollAnimator,
  contentEl,
  contentBody,
  chapters,
  currentIndex,
  page,
  pageCount,
  progressVersion,
  isContinuousScrollRead,
  getMode: () => effectiveReaderMode.value,
  getAnimateDuration: () => reader.animateDuration,
  getCurrentChapterPercent: currentChapterPercent,
  navigate: query => router.replace(readerRouteLocation(query)),
  applyLocalProgress: () => applyLocalProgressSnapshot(),
  saveProgress: () => saveCurrentProgress(),
  scheduleProgressSave: delay => scheduleProgressSave(delay),
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
  '--reader-tts-bottom-space': ttsReadBarLayoutActive.value
    ? `${ttsConfigExpanded.value ? 280 : 80}px`
    : '0px',
  '--reader-content-bottom-space': ttsReadBarLayoutActive.value
    ? `${ttsConfigExpanded.value ? 280 : 80}px`
    : '180px',
  '--reader-mobile-content-bottom-space': ttsReadBarLayoutActive.value
    ? `${ttsConfigExpanded.value ? 280 : 80}px`
    : '15px',
}))

const readerContentStyle = computed(() => ({
  fontFamily: fontStack.value,
  fontSize: `${reader.fontSize}px`,
  lineHeight: reader.lineHeight,
}))

const readerViewportHeight = computed(() => (
  contentEl.value?.clientHeight ||
  pageHeight.value ||
  (typeof window === 'undefined' ? 0 : window.innerHeight)
))

const epubStyleText = computed(() => `
  *::-webkit-scrollbar {
    display: none;
    width: 0 !important;
    height: 0 !important;
  }
  *:focus {
    outline: none !important;
  }
  html {
    min-height: 100%;
    color: ${reader.fontColor || reader.currentTheme.text};
    background: transparent;
    font-family: ${fontStack.value};
    font-size: ${reader.fontSize}px;
    font-weight: ${reader.fontWeight};
  }
  body {
    min-height: 100%;
    margin: 0 !important;
    color: inherit;
    background: transparent !important;
    font: inherit;
  }
  body p {
    margin-top: ${reader.paragraphSpace}em !important;
    margin-bottom: ${reader.paragraphSpace}em !important;
    color: inherit !important;
    font-family: ${fontStack.value} !important;
    font-size: ${reader.fontSize}px !important;
    font-weight: ${reader.fontWeight} !important;
    line-height: ${reader.lineHeight} !important;
  }
  img {
    display: block;
    max-width: 100% !important;
    height: auto !important;
  }
`)

const bodyStyle = computed(() => {
  const baseStyle = {
    fontFamily: fontStack.value,
    fontSize: `${reader.fontSize}px`,
    lineHeight: reader.lineHeight,
    fontWeight: reader.fontWeight,
  }
  if (effectiveReaderMode.value === 'flip') {
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
const desktopWorkspacePanel = computed(() => {
  if (isMobileReader.value) return ''
  if (showShelfDrawer.value) return 'shelf'
  if (showSourceDrawer.value) return 'source'
  if (showTocDrawer.value) return 'toc'
  if (showSettingsDrawer.value) return 'settings'
  return ''
})
const desktopWorkspaceTitle = computed(() => {
  if (desktopWorkspacePanel.value === 'shelf') {
    return `书架 (${filteredShelfBooks.value.length})`
  }
  if (desktopWorkspacePanel.value === 'source') return ''
  if (desktopWorkspacePanel.value === 'toc') {
    return `目录 (${chapters.value.length})`
  }
  return ''
})

function closeDesktopWorkspace() {
  showShelfDrawer.value = false
  showSourceDrawer.value = false
  showTocDrawer.value = false
  showSettingsDrawer.value = false
}

function openDesktopToolPanel(panel, open) {
  if (!isMobileReader.value && desktopWorkspacePanel.value === panel) {
    closeDesktopWorkspace()
    return
  }
  if (!isMobileReader.value) closeDesktopWorkspace()
  open()
}

function runWithDesktopWorkspaceClosed(action) {
  if (!isMobileReader.value) closeDesktopWorkspace()
  return action?.()
}

watch(
  [showShelfDrawer, showSourceDrawer, showTocDrawer, showSettingsDrawer],
  (values, previous = []) => {
    if (isMobileReader.value) return
    const opened = values.findIndex((value, index) => (
      value && !previous[index]
    ))
    if (opened < 0) return
    const refs = [
      showShelfDrawer,
      showSourceDrawer,
      showTocDrawer,
      showSettingsDrawer,
    ]
    refs.forEach((state, index) => {
      if (index !== opened) state.value = false
    })
  },
)

watch(showSourceDrawer, (visible) => {
  if (visible) ensureSourceCandidates()
})
const {
  change: onModeChange,
} = useReaderMode({
  reader,
  isMobileReader,
  isContinuousScrollRead,
  isEPUB: computed(() => chapterFormat.value === 'epub'),
  isAudio: isAudioChapter,
  page,
  chapterLoading,
  chapterBlocks,
  currentIndex,
  chapter,
  content,
  getCurrentOffset: currentOffset,
  computeChapterWindow: computeShowChapterList,
  makeChapterBlock,
  updateLayout: updateFlipLayout,
  restorePosition: restoreReadingPosition,
  saveProgress: () => saveCurrentProgress(),
})
const mobileChromeVisible = ref(true)
const {
  close: closeReaderPrimaryPanels,
  isOpen: isReaderPrimaryPanelOpen,
  toggle: toggleReaderPrimaryPanel,
} = useReaderPrimaryPanels({
  panels: {
    shelf: showShelfDrawer,
    source: showSourceDrawer,
    toc: showTocDrawer,
    settings: showSettingsDrawer,
  },
})
const {
  toggle: toggleReaderChrome,
} = useReaderChrome({
  isMobileReader,
  mobileChromeVisible,
  tocVisible: showTocDrawer,
  settingsVisible: showSettingsDrawer,
  openToc: openTocDrawer,
})

const isOverlayOpen = computed(() => (
  showTocDrawer.value ||
  showSettingsDrawer.value ||
  showShelfDrawer.value ||
  showSourceDrawer.value ||
  overlay.bookmarkFormVisible
))
const {
  handle: handleReaderWheel,
} = useReaderWheel({
  reader: effectiveReaderState,
  shellEl,
  contentEl,
  isOverlayOpen,
  isVerticalRead,
  cancelPageAnimation,
  nextPage,
  previousPage,
})

const {
  active: autoReading,
  stop: stopAutoReading,
  toggle: toggleAutoReading,
} = useReaderAutoReading({
  reader,
  contentEl,
  contentBody,
  isVerticalRead,
  isOverlayOpen,
  mobileChromeVisible,
  currentIndex,
  page,
  progressVersion,
  currentVisibleParagraph,
  scrollBehavior: readerScrollBehavior,
  nextPage,
  saveProgress: () => saveCurrentProgress(),
  notify: showReaderToast,
})
const {
  handleContentClick: handleReaderContentClick,
  handleTapZone,
  handleTouchEnd: handleReaderTouchEnd,
  handleTouchMove: handleReaderTouchMove,
  handleTouchStart: handleReaderTouchStart,
  tapPoint: handleReaderTapPoint,
} = useReaderPointer({
  reader: effectiveReaderState,
  pageEl,
  isMobileReader,
  isOverlayOpen,
  isAudio: isAudioChapter,
  autoReading,
  mobileChromeVisible,
  ttsBarVisible: ttsBarRequested,
  scheduleSelectedTextOperation,
  suppressContentClick,
  consumeSuppressedContentClick,
  cancelPageAnimation,
  nextPage,
  previousPage,
  toggleChrome: toggleReaderChrome,
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
  getMode: () => effectiveReaderMode.value,
  getStoredProgress: targetBookId => reader.progressByBook[targetBookId],
  ensureClientId: () => reader.ensureClientId(),
})
const {
  goShelf,
  openBookInfo: openReaderBookInfo,
  openBookmarks: openBookmarkDialog,
  openCache: toggleCacheContentZone,
  openContentSearch,
  openReplaceRules,
  openSettings: openSettingsDrawer,
  openSource: goSourcePanel,
  showClickZone,
} = useReaderPanels({
  book,
  bookId,
  isRemoteBook,
  bookProgress,
  bookProgressLabel,
  mobileChromeVisible,
  settingsVisible: showSettingsDrawer,
  sourceVisible: showSourceDrawer,
  cacheVisible: showCacheContentZone,
  clickZoneVisible: showClickZoneOverlay,
  customBg,
  sliderLineHeight,
  getCustomBgColor: () => reader.customBgColor,
  getLineHeight: () => reader.lineHeight,
  refreshBrowserCachedChapters: computeBrowserCachedChapters,
  saveProgress: saveCurrentProgress,
  navigate: routeLocation => router.push(routeLocation),
  openBookmarksOverlay: currentBook => overlay.openBookmark(currentBook, {
    createDraft: currentBookmarkDraft(),
  }),
  openContentSearchOverlay: currentBook => overlay.openSearchBookContent(currentBook),
  closeBookInfo: () => overlay.closeBookInfo(),
  openBookInfoOverlay: (...args) => overlay.openBookInfo(...args),
  openReplaceRulesOverlay: () => overlay.openReplaceRules(),
  isTemporaryReader: isTemporaryRemoteReader,
  onUnavailable: temporaryReaderUnavailable,
  openToc: openTocDrawer,
  ensureCategoriesLoaded: () => bookshelf.ensureCategoriesLoaded(),
  openBookGroup: (...args) => overlay.openBookGroup(...args),
  getCategoryName: row => categoryName(row),
  refreshCatalog: refreshReaderBookCatalog,
  clearCache: clearCurrentBookCache,
})

const {
  clearLoadingTimer: clearChapterLoadingTimer,
  load: loadChapter,
} = useReaderChapterLoader({
  chapters,
  currentIndex,
  mobileChromeVisible,
  restoringPosition,
  chapterLoaded,
  chapterLoadError,
  chapterLoading,
  chapter,
  content,
  chapterFormat,
  epubResource,
  audioResource,
  page,
  chapterBlocks,
  progressVersion,
  isContinuousScrollRead,
  cancelProgressSave,
  getMemoryContent: getChapterContentFromMemory,
  loadContent: loadChapterContent,
  makeChapterBlock,
  updateLayout: updateFlipLayout,
  restorePosition: restoreReadingPosition,
  preloadNearby: preloadNearbyChapters,
  saveProgress: saveCurrentProgress,
  markProgressSaved,
  getCurrentProgress: currentProgressPayload,
  computeChapterWindow: computeShowChapterList,
  formatError: error => readError(error, '章节加载失败，请检查书源或网络后重试'),
  nextFrame,
  onEpubPrepared: pending => {
    epubPendingRestore.value = pending
  },
  onAudioPrepared: pending => {
    audioInitialTime.value = Math.max(0, Number(pending.offset) || 0)
    audioCurrentTime.value = audioInitialTime.value
    audioDuration.value = 0
  },
})
const {
  handle: onScroll,
} = useReaderScrollSync({
  isVerticalRead,
  restoringPosition,
  chapterLoading,
  progressVersion,
  syncCurrentChapter: updateCurrentChapterFromScroll,
  maybeExtendChapterWindow: maybeExtendShowChapters,
  updateLayout: updateFlipLayout,
  applyLocalProgress: applyLocalProgressSnapshot,
  scheduleProgressSave,
})
const {
  load: loadReaderBook,
} = useReaderBookLoad({
  reader,
  bookshelf,
  bookId,
  book,
  chapters,
  currentIndex,
  getRouteQuery: () => route.query,
  isTemporaryReader: isTemporaryRemoteReader,
  loadTemporaryReader: async () => {
    const { data } = await getRemoteReaderSession(remoteSessionId.value)
    return data
  },
  cancelProgressSave,
  getShelfBook: targetBookId => bookshelf.books.find(
    item => Number(item.id) === Number(targetBookId),
  ),
  loadCachedBook: targetBookId => cacheFirstRequest(
    () => api.get(`/books/${targetBookId}`),
    readerDataCacheKey(`book:${targetBookId}`),
    { validate: data => Boolean(data?.id) },
  ),
  loadCachedChapters: targetBookId => cacheFirstRequest(
    () => api.get(`/books/${targetBookId}/chapters`),
    readerDataCacheKey(`chapters:${targetBookId}`),
    { validate: data => Array.isArray(data) },
  ),
  refreshBook: targetBookId => networkFirstRequest(
    () => api.get(`/books/${targetBookId}`),
    readerDataCacheKey(`book:${targetBookId}`),
    { validate: data => Boolean(data?.id) },
  ),
  refreshChapters: targetBookId => networkFirstRequest(
    () => api.get(`/books/${targetBookId}/chapters`),
    readerDataCacheKey(`chapters:${targetBookId}`),
    { validate: data => Array.isArray(data) },
  ),
  mergeLoadedBook,
  mergeBookProgress: (loadedBook, progress) => mergeShelfBook(
    loadedBook,
    { id: loadedBook.id, progress },
  ),
  resetSourceCandidates,
  loadChapter,
  progressKey: progressSaveKey,
  getCurrentProgress: currentProgressPayload,
  navigate: query => router.replace(readerRouteLocation(query)),
  markProgressSaved,
  jumpToRouteLine,
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
  previous: ttsPrevious,
  next: ttsNext,
  stop: ttsStop,
} = useReaderTTS({
  reader,
  content,
  contentBody,
  currentIndex,
  chapters,
  goChapter,
  notify: showReaderToast,
  isSlideRead: () => effectiveReaderMode.value === 'flip',
})
const ttsSupportedForChapter = computed(() => (
  tts.state.supported
    && chapterFormat.value !== 'epub'
    && !isAudioChapter.value
    && !isComicChapter.value
))
const ttsBarShown = computed(() => readerTTSBarVisible({
  requested: ttsBarRequested.value,
  supported: tts.state.supported,
  chapterFormat: chapterFormat.value,
  audio: isAudioChapter.value,
  comic: isComicChapter.value,
}))
function toggleTTSBar() {
  if (!ttsSupportedForChapter.value) return
  ttsBarRequested.value = !ttsBarRequested.value
  if (ttsBarRequested.value && isMobileReader.value) {
    mobileChromeVisible.value = false
  }
}
function closeTTSBar() {
  ttsBarRequested.value = false
  ttsStop()
}

function openReaderPrimaryTool(name, open) {
  if (isMobileReader.value) return toggleReaderPrimaryPanel(name, open)
  return openDesktopToolPanel(name, open)
}

watch([chapterFormat, isComicChapter], ([format, comic]) => {
  if (format === 'epub' || format === 'audio' || comic) {
    ttsBarRequested.value = false
    ttsStop()
    if (autoReading.value) stopAutoReading()
  }
})
const {
  handleDesktopToolAction,
  handleMobileChromeAction,
} = useReaderTools({
  currentIndex,
  mobileChromeVisible,
  goChapter,
  toggleChrome: toggleReaderChrome,
  actions: {
    home: () => runWithDesktopWorkspaceClosed(goShelf),
    shelf: () => openReaderPrimaryTool('shelf', openShelfPanel),
    source: () => openReaderPrimaryTool('source', goSourcePanel),
    toc: () => openReaderPrimaryTool('toc', openTocDrawer),
    settings: () => openReaderPrimaryTool('settings', openSettingsDrawer),
    bookmarks: openBookmarkDialog,
    search: openContentSearch,
    info: openReaderBookInfo,
    note: () => {
      if (isTemporaryRemoteReader.value) return temporaryReaderUnavailable()
      return openNoteDialog()
    },
    cache: toggleCacheContentZone,
    'clear-cache': () => runWithDesktopWorkspaceClosed(clearCurrentBookCache),
    reload: () => runWithDesktopWorkspaceClosed(reloadChapter),
    'auto-read': () => {
      if (isAudioChapter.value) return
      return runWithDesktopWorkspaceClosed(toggleAutoReading)
    },
    tts: () => {
      if (!ttsSupportedForChapter.value) return
      return runWithDesktopWorkspaceClosed(toggleTTSBar)
    },
    night: () => runWithDesktopWorkspaceClosed(toggleNight),
    top: () => runWithDesktopWorkspaceClosed(scrollToTop),
    bottom: () => runWithDesktopWorkspaceClosed(scrollToBottom),
  },
})

useReaderRouteSync({
  bookId,
  currentIndex,
  positionQuery: () => [route.query.chapter, route.query.offset, route.query.percent],
  searchQuery: () => [route.query.line, route.query.match, route.query.q, route.query.bookmark],
  loadBook: () => loadReaderBook(),
  loadChapter: (index, offset, options) => loadChapter(index, offset, options),
  jumpToRouteLine,
  onBookLoadStart: () => {
    chapterLoadError.value = ''
  },
  onBookLoadError: error => {
    chapterLoadError.value = readError(error, '章节加载失败')
    chapterLoading.value = false
  },
})

useReaderTypographySync({
  reader,
  progressVersion,
  getCurrentOffset: currentOffset,
  getCurrentPercent: currentChapterPercent,
  setRestoring: value => {
    restoringPosition.value = value
  },
  updateLayout: updateFlipLayout,
  restorePosition: restoreReadingPosition,
  scheduleProgressSave,
  syncFonts: syncReaderFontFaces,
})

const {
  handleBookDataUpdated: handleReaderBookDataUpdated,
  handleProgressUpdated,
  handleReplaceRulesUpdated,
} = useReaderExternalUpdates({
  bookId,
  book,
  chapter,
  chapters,
  currentIndex,
  isRestoring: () => restoringPosition.value,
  isProgressSaveBusy,
  progressKey: progressSaveKey,
  getCurrentProgress: currentProgressPayload,
  cancelProgressSave,
  navigate: query => router.replace(readerRouteLocation(query)),
  loadChapter,
  markProgressSaved,
  getCurrentOffset: currentOffset,
  getCurrentPercent: currentChapterPercent,
  clearChapterCache: () => clearChapterContentMemory(),
  resetCachedChapters: resetBrowserCachedChapters,
  refreshCachedChapters: computeBrowserCachedChapters,
  onReplaceSuccess: () => ElMessage.success('已按最新替换规则刷新当前章节'),
  onReplaceError: error => ElMessage.error(readError(error, '刷新当前章节失败')),
})

useReaderPageLifecycle({
  reader,
  customBg,
  sliderLineHeight,
  syncFonts: syncReaderFontFaces,
  loadBook: loadReaderBook,
  onBookLoadError: error => {
    chapterLoadError.value = readError(error, '章节加载失败')
    chapterLoading.value = false
  },
  cancelProgressSave,
  clearChapterLoadingTimer,
  stopAutoReading,
  saveProgress: saveCurrentProgress,
  onResize: handleResize,
  onWheel: handleReaderWheel,
  onPageHide: handleReaderPageHide,
  onVisibilityChange: handleReaderVisibilityChange,
  onProgressUpdated: handleProgressUpdated,
  onBookDataUpdated: handleReaderBookDataUpdated,
  onReplaceRulesUpdated: handleReplaceRulesUpdated,
  onBookmarksUpdated: () => {},
})

onBeforeRouteLeave(() => {
  saveCurrentProgress({ force: true, background: true })
})

function nextFrame() {
  return new Promise(resolve => requestAnimationFrame(() => resolve()))
}

async function handleEpubLoad(location) {
  const resourceLocation = location?.href || location?.path || ''
  if (navigateEpubChapterFromLocation(resourceLocation)) return

  const pending = epubPendingRestore.value
  await nextTick()
  updateFlipLayout()
  if (pending && pending.chapterIndex === currentIndex.value) {
    await restoreReadingPosition(pending.offset, pending.restoreOptions)
    epubPendingRestore.value = null
  }
  chapterLoaded.value = true
  progressVersion.value += 1
  scheduleProgressSave(120)
}

function navigateEpubChapterFromLocation(resourceLocation) {
  const navigatedIndex = epubChapterIndexForResourceURL(resourceLocation, chapters.value)
  if (navigatedIndex < 0 || navigatedIndex === currentIndex.value) return false

  // Fragment navigation must enter through the regular Reader transaction.
  // It refreshes the signed iframe resource URL as well as the chapter state;
  // merely loading cached text leaves the previous XHTML slice visible.
  void loadChapter(navigatedIndex, 0, { restorePercent: 0, saveAfterLoad: false })
  return true
}

function handleEpubNavigate(location) {
  const resourceLocation = location?.href || location?.path || ''
  navigateEpubChapterFromLocation(resourceLocation)
}

function handleEpubHeight() {
  updateFlipLayout()
  progressVersion.value += 1
}

function handleEpubClick(point) {
  const frame = contentBody.value?.querySelector('.epub-iframe')
  const page = pageEl.value
  if (!frame || !page || !point) return
  const frameRect = frame.getBoundingClientRect()
  const pageRect = page.getBoundingClientRect()
  const clientX = frameRect.left + (Number(point.clientX) || 0)
  const clientY = frameRect.top + (Number(point.clientY) || 0)
  handleReaderTapPoint({
    rect: pageRect,
    relX: clientX - pageRect.left,
    relY: clientY - pageRect.top,
    clientX,
    clientY,
  }, isMobileReader.value)
}

function handleEpubHash(rect) {
  const viewport = contentEl.value
  const frame = contentBody.value?.querySelector('.epub-iframe')
  if (!viewport || !frame || !Number.isFinite(Number(rect?.top))) return
  const viewportRect = viewport.getBoundingClientRect()
  const frameRect = frame.getBoundingClientRect()
  viewport.scrollTop = Math.max(
    0,
    viewport.scrollTop + frameRect.top - viewportRect.top + Number(rect.top),
  )
  scheduleProgressSave(120)
}

function handleEpubKeydown(event) {
  const key = String(event?.key || '')
  if (!key) return
  window.dispatchEvent(new KeyboardEvent('keydown', {
    key,
    code: key,
    bubbles: true,
    cancelable: true,
  }))
}

function handleEpubPreview(payload) {
  const images = Array.isArray(payload?.imageList)
    ? payload.imageList.filter(Boolean)
    : []
  if (!images.length) return
  epubPreviewImages.value = images
  epubPreviewIndex.value = Math.max(
    0,
    Math.min(Number(payload.imageIndex) || 0, images.length - 1),
  )
  epubPreviewVisible.value = true
}

function handleEpubError(error) {
  chapterLoadError.value = error?.message || 'EPUB 正文加载失败，请重试'
  chapterLoaded.value = false
}

function handleReaderImageLoad() {
  updateFlipLayout()
  progressVersion.value += 1
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

function handleReaderPageHide() {
  saveCurrentProgress({ force: true, background: true })
}

function handleReaderVisibilityChange() {
  if (document.hidden) saveCurrentProgress({ force: true, background: true })
}

function currentBookmarkParagraphContext() {
  if (isAudioChapter.value) return null
  const paragraph = chapterFormat.value === 'epub'
    ? currentEpubBookmarkParagraph()
    : currentVisibleParagraph()
  if (!paragraph || paragraph.closest?.('.chapter-inline-error')) return null
  const excerpt = readerBookmarkText(paragraph)
  if (!excerpt) return null

  if (chapterFormat.value === 'epub') {
    const activeChapter = chapter.value
    if (!activeChapter) return null
    return {
      chapterId: activeChapter.id,
      chapterIndex: currentIndex.value,
      offset: currentOffset(),
      percent: currentChapterPercent(),
      title: activeChapter.title,
      excerpt,
    }
  }

  if (String(paragraph.tagName || '').toUpperCase() !== 'P') return null
  const chapterIndex = Number(paragraph.closest?.('.chapter-content')?.dataset?.index)
  if (!Number.isInteger(chapterIndex)) return null
  const activeChapter = chapters.value[chapterIndex]
    || (chapterIndex === currentIndex.value ? chapter.value : null)
  if (!activeChapter) return null
  const paragraphOffset = Number(paragraph.dataset?.pos)
  const offset = Number.isFinite(paragraphOffset) ? Math.max(0, paragraphOffset) : currentOffset()
  const block = displayedChapterBlocks.value.find(item => item.index === chapterIndex)
    || chapterBlocks.value.find(item => item.index === chapterIndex)
  const textLength = block ? chapterBlockTextLength(block) : chapterTextLength.value
  return {
    chapterId: activeChapter.id,
    chapterIndex,
    offset,
    percent: readerTextProgress(offset, textLength),
    title: activeChapter.title,
    excerpt,
  }
}

function currentEpubBookmarkParagraph() {
  const viewport = contentEl.value?.getBoundingClientRect()
  const frame = contentBody.value?.querySelector('.epub-iframe')
  const documentRoot = frame?.contentDocument
  if (!viewport || !frame || !documentRoot) return null
  const frameRect = frame.getBoundingClientRect()
  const candidates = [...documentRoot.querySelectorAll('p, li, blockquote')]
    .filter(node => readerBookmarkText(node))
  return selectVisibleReaderBlock(candidates.map(node => {
    const rect = node.getBoundingClientRect()
    return {
      node,
      rect: {
        top: frameRect.top + rect.top,
        bottom: frameRect.top + rect.bottom,
        left: frameRect.left + rect.left,
        right: frameRect.left + rect.right,
        width: rect.width,
        height: rect.height,
      },
    }
  }), viewport)
}

function readerBookmarkParagraphs() {
  const root = contentBody.value
    ?.querySelector(`.chapter-content[data-index="${currentIndex.value}"]`)
    || contentBody.value
  return [...(root?.querySelectorAll('h3, p') || [])].map(element => ({
    element,
    text: element.innerText || element.textContent || '',
  }))
}

function selectedBookmarkContextFromText(text) {
  return selectedTextBookmarkContext({
    selectedText: text,
    paragraphs: readerBookmarkParagraphs(),
  })
}

function currentAudioProgressPayload() {
  if (isTemporaryRemoteReader.value || !isAudioChapter.value || !chapter.value) return null
  const totalChapters = Math.max(chapters.value.length || 0, 1)
  const currentSecond = Math.max(0, Math.floor(Number(audioCurrentTime.value) || 0))
  const duration = Math.max(0, Number(audioDuration.value) || 0)
  const chapterPercent = duration > 0
    ? Math.min(1, Math.max(0, currentSecond / duration))
    : 0
  return {
    bookId: bookId.value,
    chapterId: chapter.value.id,
    chapterIndex: currentIndex.value,
    offset: currentSecond,
    percent: Math.min(1, Math.max(0, (currentIndex.value + chapterPercent) / totalChapters)),
    chapterPercent,
    chapterTitle: chapter.value.title || '',
  }
}

function handleAudioLoaded(event) {
  audioCurrentTime.value = Math.max(0, Number(event?.currentTime) || audioCurrentTime.value || 0)
  audioDuration.value = Math.max(0, Number(event?.duration) || 0)
  audioAutoplay.value = false
  markProgressSaved(currentAudioProgressPayload())
}

function handleAudioProgress(event) {
  audioCurrentTime.value = Math.max(0, Number(event?.currentTime) || 0)
  audioDuration.value = Math.max(0, Number(event?.duration) || audioDuration.value || 0)
  scheduleProgressSave(1200)
}

function goAudioChapter(index) {
  const target = Math.max(0, Math.min(Number(index), chapters.value.length - 1))
  if (target === currentIndex.value) return
  // reader-dev marks both manual previous/next actions as autoplay requests
  // before changing the chapter. The destination audio element receives that
  // intent through its autoplay prop and clears it once metadata is available.
  audioAutoplay.value = true
  return goChapter(target)
}

function handleAudioEnded() {
  audioCurrentTime.value = Math.max(0, Number(audioDuration.value) || audioCurrentTime.value || 0)
  saveCurrentProgress({ force: true }).catch(() => {})
  if (currentIndex.value < chapters.value.length - 1) {
    audioAutoplay.value = true
    goChapter(currentIndex.value + 1)
  }
}

function handleAudioError() {
  showReaderToast('音频加载失败，请检查书源或网络后重试')
}

function flashParagraph(lineEl) {
  lineEl.classList.remove('reader-search-active')
  requestAnimationFrame(() => {
    lineEl.classList.add('reader-search-active')
    window.setTimeout(() => lineEl.classList.remove('reader-search-active'), 1800)
  })
}

useReaderKeyboard({
  reader: effectiveReaderState,
  currentIndex,
  chapters,
  isScrollRead,
  isAudio: isAudioChapter,
  mobileChromeVisible,
  primaryPanelOpen: computed(() => isReaderPrimaryPanelOpen()),
  tocVisible: showTocDrawer,
  settingsVisible: showSettingsDrawer,
  previousPage,
  nextPage,
  goChapter,
  scrollToTop,
  scrollToBottom,
  goShelf,
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
  box-sizing: content-box;
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
  padding: 44px 65px var(--reader-content-bottom-space);
  width: 100%;
  box-sizing: border-box;
  scroll-padding-bottom: var(--reader-content-bottom-space);
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

@media (min-width: 751px) {
  .reader-body {
    text-align: left;
  }
}

/* ---- Toast ---- */
.reader-toast {
  background: rgba(30, 41, 59, 0.92); border-radius: 8px; bottom: 96px; color: #fff;
  left: 50%; padding: 10px 18px; position: fixed;
  transform: translateX(-50%); z-index: 5; font-size: 14px;
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
    width: 100vw;
    max-width: 100%;
    min-width: 0;
    box-sizing: border-box;
    padding: 0 16px;
    text-align: justify;
  }
  .reader-page-head { display: none; }
  .reader-content {
    box-sizing: border-box;
    width: 100%;
    max-width: 100%;
    min-width: 0;
    font-size: var(--reader-font-size);
    padding: 0;
    scroll-padding-bottom: calc(var(--reader-mobile-content-bottom-space) + env(safe-area-inset-bottom));
    touch-action: pan-y pinch-zoom;
  }
  .reader-body {
    margin-top: calc(30px + env(safe-area-inset-top));
    padding-top: 15px;
    padding-bottom: calc(var(--reader-mobile-content-bottom-space) + env(safe-area-inset-bottom));
    text-align: justify;
  }
  .reader-shell.flip .reader-page {
    padding: 0;
  }
  .reader-shell.flip .reader-content {
    position: absolute;
    top: calc(30px + env(safe-area-inset-top));
    right: 0;
    bottom: 24px;
    left: 0;
    width: 100%;
    height: auto;
    padding: 0;
    overflow: hidden;
    scroll-padding-bottom: 0;
  }
  .reader-shell.flip .reader-body {
    height: 100%;
    margin: 0 16px;
    padding: 0;
    text-align: justify;
    column-width: calc(100vw - 16px);
    column-gap: 16px;
    column-fill: auto;
  }
  .reader-mobile-primary-popover-body {
    box-sizing: border-box;
    width: 100%;
    height: auto;
    min-height: 0;
    padding: calc(24px + env(safe-area-inset-top)) 24px calc(24px + env(safe-area-inset-bottom));
    color: var(--reader-text);
  }
  .reader-mobile-primary-shelf,
  .reader-mobile-primary-toc,
  .reader-mobile-primary-source {
    display: grid;
    grid-template-rows: auto minmax(0, 1fr);
    gap: 0;
    overflow: visible;
  }
  .reader-mobile-primary-settings {
    max-height: none;
    overflow: visible;
  }
  .reader-mobile-primary-title-zone {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    width: 100%;
    margin: 0 0 20px;
  }
  .reader-mobile-primary-title {
    width: fit-content;
    color: #ed4259;
    border-bottom: 1px solid #ed4259;
    font-family: -apple-system, "Noto Sans", "Helvetica Neue", Helvetica, Arial, "PingFang SC", "Microsoft YaHei", sans-serif;
    font-size: 18px;
    font-weight: 400;
  }
  .reader-mobile-primary-actions {
    display: flex;
    flex: 1;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 0 15px;
    min-width: 0;
    color: #ed4259;
    font-size: 14px;
    line-height: 26px;
  }
  .reader-mobile-primary-actions button {
    padding: 0;
    color: inherit;
    background: transparent;
    border: 0;
    cursor: pointer;
    font: inherit;
    line-height: inherit;
  }
  .reader-mobile-primary-actions button:disabled {
    color: #606266;
    cursor: default;
  }
  .reader-mobile-primary-shelf :deep(.reader-shelf-list),
  .reader-mobile-primary-toc :deep(.toc-list),
  .reader-mobile-primary-source :deep(.source-switch-list) {
    height: 300px;
    max-height: 300px;
    min-height: 0;
    padding-bottom: 0;
  }
  .reader-mobile-primary-source :deep(.title-zone) {
    margin-bottom: 20px;
  }
  .reader-mobile-primary-dismiss {
    position: fixed;
    inset: 0;
    z-index: 9;
    width: 100vw;
    height: 100dvh;
    margin: 0;
    padding: 0;
    cursor: default;
    background: transparent;
    border: 0;
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
}
</style>
