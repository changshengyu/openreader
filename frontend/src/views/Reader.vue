<template>
  <main ref="shellEl" class="reader-shell" :class="[reader.mode, { 'mobile-chrome-visible': mobileChromeVisible }]" :style="readerStyle">
    <aside class="reader-left-rail">
      <button class="rail-item rail-home" type="button" title="返回首页" @click="goShelf">
        <el-icon :size="18"><ArrowLeft /></el-icon>
        <span>首页</span>
      </button>
      <button class="rail-item" type="button" title="书架" @click="openShelfPanel">
        <el-icon :size="18"><Notebook /></el-icon>
        <span>书架</span>
      </button>
      <button class="rail-item" type="button" :disabled="!isRemoteBook" :title="isRemoteBook ? '书源' : '本地书无可切换书源'" @click="goSourcePanel">
        <el-icon :size="18"><Grid /></el-icon>
        <span>书源</span>
      </button>
      <button class="rail-item" type="button" title="目录" @click="openTocDrawer">
        <el-icon :size="18"><List /></el-icon>
        <span>目录</span>
      </button>
      <button class="rail-item" type="button" title="设置" @click="openSettingsDrawer">
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
      <button class="round-tool" type="button" title="书签" @click="openBookmarkDrawer">
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
      <button class="round-tool" type="button" :disabled="!isRemoteBook" :title="isRemoteBook ? '缓存章节' : '本地书无需章节缓存'" @click="openCacheDrawer">
        <el-icon :size="18"><Download /></el-icon>
      </button>
      <button class="round-tool" type="button" title="重新载入章节" @click="reloadChapter">
        <el-icon :size="18"><RefreshRight /></el-icon>
      </button>
      <button class="round-tool" type="button" :class="{ active: autoReading }" title="自动阅读" @click="toggleAutoReading">
        <el-icon :size="18"><VideoPlay /></el-icon>
      </button>
      <button class="round-tool" type="button" title="阅读设置" @click="openSettingsDrawer">
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
        <span>{{ displayChapterTitle(chapter?.title) || chapterLabel }}</span>
      </div>
      <span class="mobile-reader-progress">{{ bookProgressLabel }}</span>
    </header>

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
          <p v-if="chapterLoading" class="empty-hint">正在加载章节...</p>
          <template v-else>
            <section
              v-for="block in displayedChapterBlocks"
              :key="block.index"
              class="chapter-content reading-chapter"
              :data-index="block.index"
            >
              <h1 data-pos="0">{{ block.title || '正文' }}</h1>
              <p v-for="(line, index) in block.paragraphs" :key="`${block.index}-${index}`" :data-pos="line.pos">{{ line.text }}</p>
              <p v-if="chapterLoaded && block.paragraphs.length === 0" class="empty-hint">当前章节暂无正文内容</p>
            </section>
          </template>
        </div>
      </article>
      <div class="reader-tap-zones" aria-hidden="true">
        <button class="tap-zone tap-left" type="button" tabindex="-1" @click="handleTapZone('left')" />
        <button class="tap-zone tap-center" type="button" tabindex="-1" @click="handleTapZone('center')" />
        <button class="tap-zone tap-right" type="button" tabindex="-1" @click="handleTapZone('right')" />
        <button class="tap-zone tap-upper" type="button" tabindex="-1" @click="handleTapZone('upper')" />
        <button class="tap-zone tap-lower" type="button" tabindex="-1" @click="handleTapZone('lower')" />
      </div>
      <div v-if="showClickZoneOverlay" class="click-zone-overlay" :class="{ flip: reader.mode === 'flip' }">
        <div class="click-zone-piece click-zone-prev"><span>{{ reader.mode === 'flip' ? '点击前一页' : '点击向上翻页' }}</span></div>
        <div class="click-zone-piece click-zone-menu"><span>点击显示菜单</span></div>
        <div class="click-zone-piece click-zone-next"><span>{{ reader.mode === 'flip' ? '点击后一页' : '点击向下翻页' }}</span></div>
        <button class="click-zone-close" type="button" @click="showClickZoneOverlay = false">关闭</button>
      </div>
    </section>

    <footer class="reader-page-control">
      <div class="progress-box">{{ bookProgressLabel }}</div>
      <button class="page-step chapter-step" type="button" title="上一章" :disabled="currentIndex <= 0" @click="goChapter(currentIndex - 1)">
        <el-icon :size="24"><ArrowLeft /></el-icon>
      </button>
      <button class="page-step chapter-step" type="button" title="下一章" :disabled="currentIndex >= chapters.length - 1" @click="goChapter(currentIndex + 1)">
        <el-icon :size="24"><ArrowRight /></el-icon>
      </button>
    </footer>

    <label class="desktop-progress-control" title="拖动定位当前章节进度">
      <input
        class="desktop-progress-slider"
        type="range"
        min="0"
        max="1000"
        step="1"
        :value="desktopChapterSliderValue"
        :aria-label="`当前章节进度 ${desktopChapterProgressLabel}`"
        @input="handleDesktopProgressInput"
        @change="handleDesktopProgressChange"
      />
      <span>{{ desktopChapterProgressLabel }}</span>
    </label>

    <footer class="reader-mobile-bottom">
      <div class="reader-mobile-progress-panel">
        <label class="mobile-progress-slider-row" title="拖动定位当前章节进度">
          <input
            class="mobile-progress-slider"
            type="range"
            min="0"
            max="1000"
            step="1"
            :value="desktopChapterSliderValue"
            :aria-label="`当前章节进度 ${desktopChapterProgressLabel}`"
            @input="handleDesktopProgressInput"
            @change="handleDesktopProgressChange"
          />
          <span>{{ desktopChapterProgressLabel }}</span>
        </label>
        <button class="mobile-chapter-step" type="button" :disabled="currentIndex <= 0" @click="goChapter(currentIndex - 1)">
          上一章
        </button>
        <button class="mobile-chapter-progress" type="button" @click="toggleReaderChrome">
          <strong>{{ bookProgressLabel }}</strong>
          <span>{{ chapterLabel }}</span>
        </button>
        <button class="mobile-chapter-step" type="button" :disabled="currentIndex >= chapters.length - 1" @click="goChapter(currentIndex + 1)">
          下一章
        </button>
      </div>
      <button class="mobile-tool-button" type="button" @click="openMobileTool(openTocDrawer)">
        <el-icon :size="20"><List /></el-icon>
        <span>目录</span>
      </button>
      <button class="mobile-tool-button" type="button" @click="openMobileTool(openBookmarkDrawer)">
        <el-icon :size="20"><CollectionTag /></el-icon>
        <span>书签</span>
      </button>
      <button class="mobile-tool-button" type="button" @click="openMobileTool(openContentSearch)">
        <el-icon :size="20"><Search /></el-icon>
        <span>搜索</span>
      </button>
      <button class="mobile-tool-button" type="button" @click="openMobileTool(openSettingsDrawer)">
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
      <span class="tts-progress">{{ ttsProgressLabel }}</span>
      <span class="tts-label">语速</span>
      <input :value="tts.state.rate" max="3" min="0.5" step="0.1" type="range" class="tts-slider" @input="setTTSRate($event.target.value)" />
      <span class="tts-label">音调</span>
      <input :value="tts.state.pitch" max="2" min="0.5" step="0.1" type="range" class="tts-slider" @input="setTTSPitch($event.target.value)" />
      <span class="tts-label">定时</span>
      <input :value="ttsSleepMinutes" max="180" min="0" step="1" type="range" class="tts-slider" @input="setTTSSleepMinutes($event.target.value)" />
      <span class="tts-label">{{ ttsSleepMinutes }}分钟</span>
    </div>

    <!-- Toast -->
    <div v-if="toastMsg" class="reader-toast">{{ toastMsg }}</div>

    <!-- ===== 书架抽屉 ===== -->
    <el-drawer v-model="showShelfDrawer" title="书架" :direction="drawerDirection" :size="drawerSize" @opened="locateReaderShelfCurrentBook">
      <div class="reader-drawer-title">
        <span>书架({{ filteredShelfBooks.length }})</span>
        <button type="button" :disabled="shelfLoading" @click="refreshReaderShelf">
          {{ shelfLoading ? '刷新中...' : '刷新' }}
        </button>
      </div>
      <div ref="shelfListRef" v-loading="shelfLoading" class="reader-shelf-list">
        <button
          v-for="item in filteredShelfBooks"
          :key="item.id"
          class="reader-shelf-card"
          :class="{ active: item.id === bookId }"
          :data-book-id="item.id"
          type="button"
          @click="changeBookFromShelf(item)"
        >
          <span class="reader-shelf-title-line">
            <strong>{{ item.title }}</strong>
            <em v-if="unreadCount(item)">{{ unreadCount(item) }}</em>
          </span>
          <span class="reader-shelf-chapter">{{ readChapterTitle(item) || '尚未阅读' }}</span>
        </button>
        <el-empty v-if="!shelfLoading && !filteredShelfBooks.length" description="书架暂无书籍" />
      </div>
    </el-drawer>

    <!-- ===== 目录抽屉 ===== -->
    <el-drawer v-model="showTocDrawer" title="目录" :direction="drawerDirection" :size="drawerSize" @opened="locateTocCurrentChapter">
      <div class="reader-drawer-title">
        <span>目录({{ chapters.length }})</span>
        <div class="reader-drawer-actions">
          <button v-if="chapters.length" type="button" @click="toggleTocReverse">{{ tocReverse ? '顺序' : '倒序' }}</button>
          <button v-if="chapters.length" type="button" @click="scrollTocTop">顶部</button>
          <button v-if="chapters.length" type="button" @click="scrollTocBottom">底部</button>
          <button v-if="isTextLocalBook" type="button" :disabled="tocRefreshing" @click="changeReaderLocalTocRule">修改规则</button>
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
    <el-drawer v-model="showSourceDrawer" title="书源" :direction="drawerDirection" :size="drawerSize" @open="loadSourceCandidates">
      <SourceSwitchPanel
        :book="book"
        :sources="sourceCandidates"
        :loading="loadingSources"
        :changing-source="changingSource"
        :current-source-name="currentSourceName"
        :group="sourceGroup"
        :groups="sourceGroups"
        @refresh="refreshSourceCandidates"
        @load-more="loadMoreSourceCandidates"
        @group-change="changeSourceGroup"
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
        <button v-if="isRemoteBook" type="button" class="mobile-more-item" @click="runMobileAction(goSourcePanel)">
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
        <button v-if="isRemoteBook" type="button" class="mobile-more-item" @click="runMobileAction(openCacheDrawer)">
          <el-icon :size="22"><Download /></el-icon>
          <span>缓存</span>
        </button>
        <button v-if="isRemoteBook" type="button" class="mobile-more-item" @click="runMobileAction(clearCurrentBookCache)">
          <el-icon :size="22"><Delete /></el-icon>
          <span>清缓存</span>
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

    <!-- ===== 缓存抽屉 ===== -->
    <el-drawer v-model="showCacheDrawer" title="缓存章节" :direction="drawerDirection" :size="drawerSize">
      <div class="reader-cache-panel">
        <div class="reader-cache-actions">
          <button type="button" :disabled="isCachingContent" @click="cacheFollowingChapters(50)">后面50章</button>
          <button type="button" :disabled="isCachingContent" @click="cacheFollowingChapters(100)">后面100章</button>
          <button type="button" :disabled="isCachingContent" @click="cacheFollowingChapters(true)">后面全部</button>
        </div>
        <div v-if="isCachingContent" class="reader-cache-status">
          <span>{{ cachingContentTip }}</span>
          <button type="button" @click="cancelCachingContent">取消</button>
        </div>
      </div>
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
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  ArrowDownBold,
  ArrowLeft,
  ArrowRight,
  ArrowUpBold,
  CollectionTag,
  Delete,
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
import { changeBookSource, listBookSourceCandidates, refreshBook, refreshLocalBook, searchBookContent as searchBookContentApi } from '../api/books'
import { createReplaceRule } from '../api/replaceRules'
import { listSources } from '../api/sources'
import { deleteAsset, uploadAsset } from '../api/uploads'
import ReaderBookmarkPanel from '../components/reader/ReaderBookmarkPanel.vue'
import ReaderSearchPanel from '../components/reader/ReaderSearchPanel.vue'
import ReaderSettingsPanel from '../components/reader/ReaderSettingsPanel.vue'
import SourceSwitchPanel from '../components/reader/SourceSwitchPanel.vue'
import ReaderTocPanel from '../components/reader/ReaderTocPanel.vue'
import { mergeShelfBook, useBookshelfStore } from '../stores/bookshelf'
import { useOverlayStore } from '../stores/overlay'
import { useReaderStore, themePresets } from '../stores/reader'
import { useKeyboard } from '../composables/useKeyboard'
import { useGesture } from '../composables/useGesture'
import { useTTS } from '../composables/useTTS'
import { newestBookProgress, sortByShelfOrder } from '../utils/bookOrder'
import { cacheBookChaptersToBrowser, clearBookBrowserChapterCache, isValidChapterContentResponse, listBookBrowserCachedChapters, loadBrowserChapterContent } from '../utils/bookChapterCache'
import { cacheFirstRequest, networkFirstRequest } from '../utils/browserCache'
import { simplized, traditionalized } from '../utils/chinese'
import { readerFontOptions, readerFontStack, syncReaderFontFaces } from '../utils/readerFonts'
import { readerRouteQueryFromBook, savedBookChapterPercent } from '../utils/readerRoute'
import { currentViewportWidth, shouldUseMiniInterface } from '../utils/responsive'
import { invalidateReaderDataCache as invalidateReaderCache, readerDataCacheKey as scopedReaderDataCacheKey, writeReaderDataCache as writeReaderCache } from '../utils/readerDataCache'
import {
  sourceCandidateAuthor,
  sourceCandidateBookUrl,
  sourceCandidateCover,
  sourceCandidateIntro,
  sourceCandidateKey,
  sourceCandidateSourceId,
  sourceCandidateSourceName,
  sourceCandidateTitle,
} from '../utils/sourceCandidate'

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
const chapterBlocks = ref([])
const chapterLoading = ref(true)
const chapterLoaded = ref(false)
const contentEl = ref(null)
const contentBody = ref(null)
const pageEl = ref(null)
const shellEl = ref(null)
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
const showCacheDrawer = ref(false)
const showNoteDialog = ref(false)
const showBookmarkEditor = ref(false)
const showClickZoneOverlay = ref(false)
const sourceCandidates = ref([])
const sourceGroupOptions = ref([])
const loadingSources = ref(false)
const changingSource = ref(null)
const sourceGroup = ref('')
const sourceOffset = ref(0)
const sourceCandidatesLoadedKey = ref('')
const shelfLoading = ref(false)
const shelfListRef = ref(null)
const tocPanelRef = ref(null)
const tocLocateKey = ref(0)
const tocReverse = ref(false)
const tocRefreshing = ref(false)
const browserCachedChapters = ref({})
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
let bookmarkReloadTimer
const toastMsg = ref('')
const isCachingContent = ref(false)
const cachingContentTip = ref('')
const progressVersion = ref(0)
const autoReading = ref(false)
const customBg = ref('')
const sliderLineHeight = ref(2.12)
const pageHeight = ref(600)
const pageWidth = ref(600)
const windowWidth = ref(currentViewportWidth())
const SAVE_PROGRESS_MIN_INTERVAL = 1200
const MOBILE_TAP_MOVE_TOLERANCE = 14

let saveTimer
let chapterLoadingTimer
let autoReadTimer
let autoReadAdvancing = false
let ttsContinueToken = 0
let savingProgress = false
let pendingProgressPayload = null
let lastProgressSaveKey = ''
let lastProgressRequestAt = 0
let restoringPosition = false
let chapterContentCache = null
let cachingContentCancelled = false
let readerTouchStart = null
let readerTouchMoved = false
let readerTouchMove = { x: 0, y: 0 }
let ignoreNextContentClick = false
let handledTouchTapAt = 0
let lastLocalProgressKey = ''
let lastWheelPageAt = 0
let extendingShowChapters = false
let selectionOperateTimer = null
let selectionOperating = false

const fontOptions = readerFontOptions
const SHOW_PREV_CHAPTER_SIZE = 1
const SHOW_NEXT_CHAPTER_SIZE = 2

const filteredShelfBooks = computed(() => {
  const books = Array.isArray(bookshelf.books) ? bookshelf.books : []
  return sortByShelfOrder(books, reader.progressByBook)
})
const sourceGroups = computed(() => {
  const sourceRows = sourceGroupOptions.value.length ? sourceGroupOptions.value : sourceCandidates.value
  return buildSourceGroupOptions(sourceRows)
})
const currentSourceName = computed(() => {
  if (!book.value?.sourceId) return '本地书籍'
  return sourceGroupOptions.value.find(source => Number(source.id) === Number(book.value.sourceId))?.name || '当前来源'
})
const isRemoteBook = computed(() => Number(book.value?.sourceId || 0) > 0)
const isTextLocalBook = computed(() => {
  if (isRemoteBook.value) return false
  const name = String(book.value?.originalFile || book.value?.libraryPath || book.value?.title || '').toLowerCase()
  return /\.(txt|text|md)$/.test(name)
})

const chapterParagraphs = computed(() => {
  return makeParagraphs(content.value, chapter.value?.title)
})
const lines = computed(() => chapterParagraphs.value.map(item => item.text))
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
const bookProgress = computed(() => {
  const total = Math.max(chapters.value.length, 1)
  return Math.min(1, Math.max(0, (currentIndex.value + currentChapterPercent()) / total))
})
const bookProgressLabel = computed(() => `${Math.round(bookProgress.value * 100)}%`)
const desktopChapterSliderValue = computed(() => {
  progressVersion.value
  return Math.round(Math.max(0, Math.min(1, currentChapterPercent())) * 1000)
})
const desktopChapterProgressLabel = computed(() => `${Math.round(desktopChapterSliderValue.value / 10)}%`)
const bookSearchStatus = computed(() => {
  if (!searchedBookContent.value) return ''
  const scanned = bookSearchLastIndex.value >= 0 ? bookSearchLastIndex.value + 1 : 0
  const total = bookSearchTotal.value || chapters.value.length || 0
  if (!total) return `${bookSearchResults.value.length} 条结果`
  return `已搜索 ${Math.min(scanned, total)} / ${total} 章，${bookSearchResults.value.length} 条结果`
})
const mobileChromeVisible = ref(false)
const CHAPTER_END_OFFSET = -1
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

function onModeChange(mode) {
  reader.setMode(mode)
}

onMounted(async () => {
  reader.normalizeSettings()
  syncReaderFontFaces(reader.customFontsMap)
  await loadReaderBook()
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
  clearTimeout(saveTimer)
  clearTimeout(chapterLoadingTimer)
  clearTimeout(selectionOperateTimer)
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
  clearBookmarkReloadTimer()
})

onBeforeRouteLeave(() => {
  saveCurrentProgress({ force: true, background: true })
})

watch(bookId, async () => {
  await loadReaderBook()
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
  clearTimeout(saveTimer)
  saveTimer = setTimeout(saveCurrentProgress, 300)
})

watch(() => reader.customFontsMap, (customFontsMap) => {
  syncReaderFontFaces(customFontsMap)
}, { deep: true })

watch(contentSearch, () => {
  resetContentSearchState()
})

function makeParagraphs(value, heading = '') {
  let wordCount = String(heading || '').length + 2
  return String(value || '').split(/\n+/).reduce((items, rawLine) => {
    const text = rawLine.trim()
    if (!text) return items
    const pos = wordCount
    wordCount += text.length + 2
    items.push({ text: formatChineseText(text), pos })
    return items
  }, [])
}

function formatChineseText(text) {
  if (!text) return ''
  return reader.chineseFont === '繁体' ? traditionalized(String(text)) : simplized(String(text))
}

function displayChapterTitle(title) {
  return formatChineseText(title || '')
}

function buildSourceGroupOptions(rows) {
  const counts = new Map()
  for (const item of rows || []) {
    if (item?.enabled === false) continue
    const group = String(item?.group || '').trim()
    if (!group) continue
    counts.set(group, (counts.get(group) || 0) + 1)
  }
  return [...counts.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([value, count]) => ({ value, label: value, count }))
}

function makeChapterBlock(index, chapterRow, text) {
  const fallback = chapters.value[index] || {}
  const title = chapterRow?.title || fallback.title || `第 ${index + 1} 章`
  return {
    index,
    id: chapterRow?.id || fallback.id,
    title: displayChapterTitle(title),
    content: String(text || ''),
    paragraphs: makeParagraphs(text, title),
  }
}

function chapterBlockTextLength(block) {
  const paragraphs = Array.isArray(block?.paragraphs) ? block.paragraphs : []
  if (!paragraphs.length) return 0
  const last = paragraphs[paragraphs.length - 1]
  return Number(last.pos || 0) + String(last.text || '').length
}

function resetContentSearchState() {
  bookSearchLastIndex.value = -1
  bookSearchHasMore.value = false
  bookSearchTotal.value = 0
  searchedBookContent.value = false
  bookSearchResults.value = []
}

async function loadReaderBook() {
  clearTimeout(saveTimer)
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
  sourceCandidates.value = []
  sourceCandidatesLoadedKey.value = ''
  sourceOffset.value = 0
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
  lastProgressSaveKey = progressSaveKey(currentProgressPayload())
}

function mergeLoadedBook(incoming) {
  if (!incoming?.id) return incoming
  const current = bookshelf.books.find(item => Number(item.id) === Number(incoming.id)) ||
    (Number(book.value?.id) === Number(incoming.id) ? book.value : null)
  return mergeShelfBook(current, incoming)
}

async function loadBookmarks(targetBookId = bookId.value) {
  const { data } = await api.get(`/books/${targetBookId}/bookmarks`)
  if (String(bookId.value) === String(targetBookId)) {
    bookmarks.value = data || []
  }
  return data || []
}

function handleBookmarksUpdated(event) {
  const bookIds = event?.detail?.bookIds || []
  if (bookIds.length && !bookIds.some(id => String(id) === String(bookId.value))) return
  scheduleBookmarkReload()
}

function scheduleBookmarkReload() {
  clearBookmarkReloadTimer()
  bookmarkReloadTimer = window.setTimeout(async () => {
    bookmarkReloadTimer = undefined
    try {
      await loadBookmarks()
    } catch {
      // Keep the current bookmark list; the next drawer open or sync event can recover.
    }
  }, 250)
}

function clearBookmarkReloadTimer() {
  if (!bookmarkReloadTimer) return
  window.clearTimeout(bookmarkReloadTimer)
  bookmarkReloadTimer = undefined
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
  chapterContentCache = null
  browserCachedChapters.value = {}
  if (!options.clearBrowser) return 0
  try {
    return await clearBookBrowserChapterCache(options.book || book.value, bookId.value)
  } catch {
    return 0
  }
}

async function loadChapter(index, offset = 0, options = {}) {
  currentIndex.value = Math.max(0, Math.min(index, Math.max(chapters.value.length - 1, 0)))
  mobileChromeVisible.value = false
  restoringPosition = true
  chapterLoaded.value = false
  clearTimeout(saveTimer)
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
    if (isContinuousScrollRead.value) {
      await computeShowChapterList({ reset: true })
    } else {
      chapterBlocks.value = [makeChapterBlock(currentIndex.value, chapter.value, content.value)]
    }
    chapterLoading.value = false
    await nextTick()
    updateFlipLayout()
    await restoreReadingPosition(offset, options)
    progressVersion.value += 1
    preloadNearbyChapters(currentIndex.value)
    if (options.saveAfterLoad) {
      await saveCurrentProgress({ force: true })
    } else {
      lastProgressSaveKey = progressSaveKey(currentProgressPayload())
    }
    chapterLoaded.value = true
  } finally {
    clearTimeout(chapterLoadingTimer)
    await nextFrame()
    restoringPosition = false
    chapterLoading.value = false
  }
}

async function computeShowChapterList() {
  if (!chapters.value.length) {
    chapterBlocks.value = []
    return
  }
  const startIndex = reader.mode === 'scroll2'
    ? Math.max(0, currentIndex.value - SHOW_PREV_CHAPTER_SIZE)
    : currentIndex.value
  const endIndex = isContinuousScrollRead.value
    ? Math.min(chapters.value.length - 1, currentIndex.value + SHOW_NEXT_CHAPTER_SIZE)
    : currentIndex.value
  const blocks = []
  for (let index = startIndex; index <= endIndex; index += 1) {
    const data = await loadChapterContent(index)
    blocks.push(makeChapterBlock(index, data.chapter || chapters.value[index], data.content || ''))
  }
  chapterBlocks.value = blocks
}

async function appendNextShowChapter() {
  if (!isContinuousScrollRead.value || !chapterBlocks.value.length) return
  const lastIndex = chapterBlocks.value[chapterBlocks.value.length - 1].index
  const nextIndex = lastIndex + 1
  if (nextIndex >= chapters.value.length) return
  if (chapterBlocks.value.some(block => block.index === nextIndex)) return
  const data = await loadChapterContent(nextIndex)
  chapterBlocks.value = [
    ...chapterBlocks.value,
    makeChapterBlock(nextIndex, data.chapter || chapters.value[nextIndex], data.content || ''),
  ]
}

async function prependPreviousShowChapter() {
  if (reader.mode !== 'scroll2' || !chapterBlocks.value.length || !contentEl.value) return
  const firstIndex = chapterBlocks.value[0].index
  const previousIndex = firstIndex - 1
  if (previousIndex < 0) return
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
  if (!options.refresh) {
    const cached = getChapterContentFromMemory(index)
    if (cached) return cached
  }
  const data = await loadBrowserChapterContent(book.value, bookId.value, index, { refresh: Boolean(options.refresh) })
  addChapterContentToMemory(index, data)
  if (isValidChapterContentResponse(data)) {
    browserCachedChapters.value = { ...browserCachedChapters.value, [index]: true }
  }
  return data
}

function preloadNearbyChapters(index) {
  if (!book.value || !chapters.value.length) return
  const targets = []
  for (let distance = 1; distance <= NEARBY_PRELOAD_RADIUS; distance += 1) {
    targets.push(index + distance, index - distance)
  }
  targets
    .filter(target => target >= 0 && target < chapters.value.length)
    .forEach(target => {
      if (getChapterContentFromMemory(target)) return
      loadChapterContent(target).catch(() => {})
    })
}

function getChapterContentFromMemory(index) {
  const cacheBookKey = currentChapterCacheBookKey()
  if (!chapterContentCache || chapterContentCache.bookKey !== cacheBookKey) return null
  const cached = chapterContentCache.chapters[index]
  return isValidChapterContentResponse(cached) ? cached : null
}

function addChapterContentToMemory(index, data) {
  if (!isValidChapterContentResponse(data)) return
  const cacheBookKey = currentChapterCacheBookKey()
  if (!chapterContentCache || chapterContentCache.bookKey !== cacheBookKey) {
    chapterContentCache = { bookKey: cacheBookKey, chapters: {} }
  }
  chapterContentCache.chapters[index] = data
}

function currentChapterCacheBookKey() {
  const currentBook = book.value || {}
  return currentBook.url || currentBook.bookUrl || currentBook.libraryPath || `book:${bookId.value}`
}

async function restoreReadingPosition(offset = 0, options = {}) {
  const restorePercent = Number(options.restorePercent)
  const hasRestorePercent = Number.isFinite(restorePercent)
  await nextTick()
  await nextFrame()
  updateFlipLayout()
  const chapterOffset = Number(offset || 0)
  if (reader.mode === 'flip') {
    page.value = chapterOffset === CHAPTER_END_OFFSET
      ? Math.max(0, pageCount.value - 1)
      : (hasRestorePercent
          ? Math.round(Math.max(0, Math.min(1, restorePercent)) * Math.max(0, pageCount.value - 1))
          : Math.min(Math.max(chapterOffset, 0), pageCount.value - 1))
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
    if (chapterOffset === CHAPTER_END_OFFSET) {
      contentEl.value.scrollTop = Math.max(0, contentEl.value.scrollHeight - contentEl.value.clientHeight)
    } else if (hasRestorePercent) {
      const bottom = Math.max(contentEl.value.scrollHeight - contentEl.value.clientHeight, 0)
      contentEl.value.scrollTop = Math.round(Math.max(0, Math.min(1, restorePercent)) * bottom)
    } else {
      contentEl.value.scrollTop = Math.max(chapterOffset, 0)
    }
  }
  applyScroll()
  await nextFrame()
  applyScroll()
}

function restoreScroll2ChapterPosition(chapterOffset, restorePercent = null) {
  const el = contentEl.value
  const activeChapter = contentBody.value?.querySelector(`.chapter-content[data-index="${currentIndex.value}"]`)
  if (!el || !activeChapter) return
  if (chapterOffset === CHAPTER_END_OFFSET) {
    el.scrollTop = Math.max(0, activeChapter.offsetTop + activeChapter.offsetHeight - el.clientHeight)
    return
  }
  if (Number.isFinite(restorePercent)) {
    const room = Math.max(activeChapter.offsetHeight - el.clientHeight, 0)
    el.scrollTop = Math.max(0, activeChapter.offsetTop + Math.round(Math.max(0, Math.min(1, restorePercent)) * room))
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
  const nodes = [...chapterEl.querySelectorAll('h1[data-pos], p[data-pos]')]
  if (!nodes.length) return null
  return [...nodes].reverse().find(node => Number(node.dataset.pos) <= position) || nodes[0]
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
    page.value = offset === CHAPTER_END_OFFSET ? Math.max(0, pageCount.value - 1) : 0
    progressVersion.value += 1
    saveCurrentProgress()
    return
  }
  if (jumpToLoadedChapter(currentIndex.value, offset)) return
  if (!contentEl.value) return
  contentEl.value.scrollTo({
    top: offset === CHAPTER_END_OFFSET
      ? Math.max(0, contentEl.value.scrollHeight - contentEl.value.clientHeight)
      : 0,
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
  if (offset === CHAPTER_END_OFFSET) {
    contentEl.value.scrollTo({
      top: Math.max(0, chapterEl.offsetTop + chapterEl.offsetHeight - contentEl.value.clientHeight),
      behavior: readerScrollBehavior(),
    })
  } else if (offset > 0) {
    const target = paragraphByChapterPosition(chapterEl, offset)
    if (target) {
      jumpToParagraph(target, { save: false, flash: false })
    } else {
      contentEl.value.scrollTo({
        top: Math.max(0, chapterEl.offsetTop),
        behavior: readerScrollBehavior(),
      })
    }
  } else {
    contentEl.value.scrollTo({
      top: Math.max(0, chapterEl.offsetTop),
      behavior: readerScrollBehavior(),
    })
  }
  progressVersion.value += 1
  clearTimeout(saveTimer)
  saveTimer = setTimeout(saveCurrentProgress, Math.max(300, reader.animateDuration + 80))
  return true
}

async function jumpFromToc(index) {
  showTocDrawer.value = false
  await goChapter(index)
}

function locateTocCurrentChapter() {
  updateCurrentChapterFromScroll()
  tocLocateKey.value += 1
  nextTick(() => tocPanelRef.value?.locateCurrentChapter?.())
}

function openTocDrawer() {
  mobileChromeVisible.value = false
  computeBrowserCachedChapters()
  showTocDrawer.value = true
  window.setTimeout(locateTocCurrentChapter, 0)
  window.setTimeout(locateTocCurrentChapter, 180)
}

function toggleTocReverse() {
  tocReverse.value = !tocReverse.value
  locateTocCurrentChapter()
}

function scrollTocTop() {
  tocPanelRef.value?.scrollToTop?.()
}

function scrollTocBottom() {
  tocPanelRef.value?.scrollToBottom?.()
}

async function refreshTocDrawer() {
  tocRefreshing.value = true
  try {
    if (isRemoteBook.value) {
      await refreshReaderBookCatalog()
    } else {
      await loadChapters()
    }
    await computeBrowserCachedChapters()
    locateTocCurrentChapter()
  } finally {
    tocRefreshing.value = false
  }
}

async function changeReaderLocalTocRule() {
  if (!book.value || !isTextLocalBook.value) return
  const result = await ElMessageBox.prompt('填写 TXT 目录行正则，留空则使用默认目录规则。', '修改目录规则', {
    confirmButtonText: '刷新目录',
    cancelButtonText: '取消',
    inputType: 'textarea',
    inputValue: book.value.tocRule || '',
    inputPlaceholder: '^第.+章.*$',
  }).catch(() => null)
  if (!result) return
  tocRefreshing.value = true
  try {
    const { data } = await refreshLocalBook(book.value.id, { tocRule: result.value || '' })
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
  } catch (err) {
    ElMessage.error(readError(err, '更新目录规则失败'))
  } finally {
    tocRefreshing.value = false
  }
}

async function computeBrowserCachedChapters() {
  try {
    browserCachedChapters.value = await listBookBrowserCachedChapters(book.value, bookId.value)
  } catch {
    browserCachedChapters.value = {}
  }
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
async function openShelfPanel() {
  mobileChromeVisible.value = false
  showShelfDrawer.value = true
  if (bookshelf.books.length) {
    window.setTimeout(locateReaderShelfCurrentBook, 0)
    return
  }
  shelfLoading.value = true
  try {
    await bookshelf.loadBooks({ all: true })
    locateReaderShelfCurrentBook()
  } catch (err) {
    ElMessage.error(readError(err, '加载书架失败'))
  } finally {
    shelfLoading.value = false
  }
}

function locateReaderShelfCurrentBook(attempt = 0) {
  nextTick(() => {
    const list = shelfListRef.value
    const active = list?.querySelector?.(`[data-book-id="${bookId.value}"]`)
    if (!list || !active) {
      if (attempt < 20 && showShelfDrawer.value && filteredShelfBooks.value.length) {
        window.setTimeout(() => locateReaderShelfCurrentBook(attempt + 1), 50)
      }
      return
    }
    const targetTop = active.offsetTop - Math.max(0, (list.clientHeight - active.clientHeight) / 2)
    const nextTop = Math.max(0, targetTop)
    list.scrollTo({ top: nextTop, behavior: 'auto' })
    requestAnimationFrame(() => {
      list.scrollTop = nextTop
      active.scrollIntoView({ block: 'center', inline: 'nearest' })
    })
  })
}

async function changeBookFromShelf(item) {
  showShelfDrawer.value = false
  if (item.id === bookId.value) return
  await saveCurrentProgress({ force: true })
  await router.push({ name: 'reader', params: { id: item.id }, query: readerRouteQueryForBook(item) })
}

function readChapterTitle(item) {
  const progress = shelfItemProgress(item)
  return progress?.chapterTitle || item.durChapterTitle || ''
}

function readerRouteQueryForBook(item) {
  return readerRouteQueryFromBook(item, shelfItemProgress(item), item?.chapterCount || chapters.value.length)
}

function unreadCount(item) {
  const progress = shelfItemProgress(item)
  const chapterIndex = Number.isInteger(progress?.chapterIndex) ? progress.chapterIndex : -1
  const total = Number(item.chapterCount || item.totalChapterNum || 0)
  return Math.max(0, total - 1 - chapterIndex)
}

function shelfItemProgress(item) {
  return newestBookProgress(item, reader.progressByBook)
}

async function refreshReaderShelf() {
  shelfLoading.value = true
  try {
    await bookshelf.loadBooks({ force: true, all: true })
  } catch (err) {
    ElMessage.error(readError(err, '刷新书架失败'))
  } finally {
    shelfLoading.value = false
  }
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
  if (!bookshelf.categories.length) {
    try {
      await bookshelf.loadCategories()
    } catch {
      // 分组弹层仍可打开，失败提示由保存时处理。
    }
  }
  overlay.openBookGroup('set', book.value, {
    categoryName: categoryName(book.value.categoryId),
    progress: bookProgress.value,
    statusLabel: `阅读中 · ${bookProgressLabel.value}`,
    statusType: 'success',
  })
}

function categoryName(id) {
  if (!id) return '未分组'
  return bookshelf.categories.find(category => Number(category.id) === Number(id))?.name || '未分组'
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
      paged: 1,
    })
    const rows = Array.isArray(data) ? data : (data?.list || [])
    sourceCandidates.value = append ? mergeSourceCandidates(sourceCandidates.value, rows) : rows
    sourceOffset.value = Number.isInteger(data?.nextOffset) ? data.nextOffset : sourceOffset.value + 10
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
  const seen = new Set(existing.map(item => sourceCandidateKey(item)))
  return existing.concat(incoming.filter(item => {
    const key = sourceCandidateKey(item)
    if (seen.has(key)) return false
    seen.add(key)
    return true
  }))
}

async function changeSource(source) {
  if (!book.value || source.current) return
  const nextSourceId = sourceCandidateSourceId(source)
  const previousBook = book.value
  changingSource.value = nextSourceId
  try {
    const { data } = await changeBookSource(bookId.value, {
      sourceId: nextSourceId,
      bookUrl: sourceCandidateBookUrl(source),
      title: sourceCandidateTitle(source, book.value?.title),
      author: sourceCandidateAuthor(source),
      coverUrl: sourceCandidateCover(source),
      intro: sourceCandidateIntro(source),
    })
    await invalidateReaderDataCache({ book: true, chapters: true })
    await resetReaderChapterCaches({ clearBrowser: true, book: previousBook })
    book.value = mergeLoadedBook(data)
    bookshelf.upsertBook(book.value)
    const chRes = await api.get(`/books/${bookId.value}/chapters`)
    chapters.value = Array.isArray(chRes.data) ? chRes.data : []
    await writeReaderDataCache({ bookData: book.value, chaptersData: chapters.value })
    currentIndex.value = Math.min(currentIndex.value, Math.max(chapters.value.length - 1, 0))
    await loadChapter(currentIndex.value, 0)
    sourceCandidatesLoadedKey.value = ''
    resetContentSearchState()
    await loadSourceCandidates({ force: true })
    showSourceDrawer.value = false
    ElMessage.success(`已切换到 ${sourceCandidateSourceName(source)}`)
  } catch (err) {
    ElMessage.error(readError(err, '换源失败'))
  } finally {
    changingSource.value = null
  }
}

function openContentSearch() {
  mobileChromeVisible.value = false
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

async function searchAllBookContent() {
  return runBookContentSearch({ append: true, scanAll: true })
}

async function runBookContentSearch({ append = false, scanAll = false } = {}) {
  const keyword = contentSearch.value.trim()
  if (!keyword) return
  if (bookSearching.value) return
  bookSearching.value = true
  searchedBookContent.value = true
  try {
    let lastIndex = append ? bookSearchLastIndex.value : -1
    let nextResults = append ? [...bookSearchResults.value] : []
    const maxRounds = scanAll ? 80 : (append ? 1 : (Number(book.value?.sourceId || 0) > 0 ? 4 : 1))
    let previousLastIndex = lastIndex
    for (let round = 0; round < maxRounds; round += 1) {
      const { data } = await searchBookContentApi(bookId.value, keyword, {
        paged: 1,
        lastIndex,
        scanUntilMatch: append ? 0 : 1,
        ...contentSearchPagingParams(book.value),
      })
      const rows = Array.isArray(data) ? data : (data?.list || [])
      nextResults = nextResults.concat(rows)
      bookSearchResults.value = nextResults
      bookSearchLastIndex.value = Number.isInteger(data?.lastIndex) ? data.lastIndex : -1
      bookSearchHasMore.value = Boolean(data?.hasMore)
      bookSearchTotal.value = Number(data?.total || 0)
      lastIndex = bookSearchLastIndex.value
      if (!scanAll && (rows.length || !bookSearchHasMore.value)) break
      if (scanAll && (!bookSearchHasMore.value || lastIndex <= previousLastIndex)) break
      previousLastIndex = lastIndex
    }
  } catch (err) {
    ElMessage.error(readError(err, '搜索正文失败'))
  } finally {
    bookSearching.value = false
  }
}

function contentSearchPagingParams(targetBook) {
  if (Number(targetBook?.sourceId || 0) > 0) {
    return { chapterLimit: 80, scanLimit: 240, matchLimit: 200, perChapterLimit: 20 }
  }
  return { chapterLimit: 500, scanLimit: 2000, matchLimit: 5000, perChapterLimit: 500, localFull: 1 }
}

function openNoteDialog() {
  noteText.value = ''
  showNoteDialog.value = true
}

async function reloadChapter() {
  await loadChapter(currentIndex.value, currentOffset(), { refresh: true })
  toastMsg.value = '章节已重新载入'
  setTimeout(() => { toastMsg.value = '' }, 1600)
}

async function cacheFollowingChapters(cacheCount) {
  if (!isRemoteBook.value || isCachingContent.value) return
  await computeBrowserCachedChapters()
  const targets = cacheChapterTargets(cacheCount)
  if (!targets.length) {
    ElMessage.error('不需要缓存')
    return
  }
  isCachingContent.value = true
  cachingContentCancelled = false
  cachingContentTip.value = `正在缓存章节 0/${targets.length}`
  try {
    const result = await cacheBookChaptersToBrowser(book.value, bookId.value, chapters.value, {
      startIndex: currentIndex.value + 1,
      count: cacheCount === true ? true : Number(cacheCount || 0),
      cancelled: () => cachingContentCancelled,
      onProgress: ({ finished, total }) => {
        cachingContentTip.value = `正在缓存章节 ${finished}/${total}`
      },
    })
    if (result.cancelled) {
      toastMsg.value = `已取消，缓存 ${result.cached} 章`
    } else {
      toastMsg.value = `缓存完成：${result.cached} 章`
    }
    setTimeout(() => { toastMsg.value = '' }, 1600)
  } finally {
    isCachingContent.value = false
    cachingContentTip.value = ''
    cachingContentCancelled = false
    computeBrowserCachedChapters()
    await loadChapters()
  }
}

function cacheChapterTargets(cacheCount) {
  const start = currentIndex.value + 1
  if (start >= chapters.value.length) return []
  const end = cacheCount === true
    ? chapters.value.length
    : Math.min(chapters.value.length, start + Number(cacheCount || 0))
  const targets = []
  for (let index = start; index < end; index += 1) {
    if (!browserCachedChapters.value[index]) targets.push(index)
  }
  return targets
}

function cancelCachingContent() {
  cachingContentCancelled = true
  cachingContentTip.value = '正在取消缓存...'
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

async function clearCurrentBookBrowserCache() {
  const removed = await clearBookBrowserChapterCache(book.value, bookId.value)
  chapterContentCache = null
  browserCachedChapters.value = {}
  return removed
}

function toggleAutoReading() {
  if (autoReading.value) {
    stopAutoReading()
    toastMsg.value = '自动阅读已停止'
    setTimeout(() => { toastMsg.value = '' }, 1200)
    return
  }
  autoReading.value = true
  runAutoReadLoop()
  toastMsg.value = '自动阅读已开始'
  setTimeout(() => { toastMsg.value = '' }, 1200)
}

function runAutoReadLoop(delay = 0) {
  clearTimeout(autoReadTimer)
  if (!autoReading.value) return
  autoReadTimer = setTimeout(async () => {
    if (!autoReading.value) return
    if (autoReadAdvancing || isOverlayOpen.value || mobileChromeVisible.value) {
      runAutoReadLoop(300)
      return
    }
    autoReadAdvancing = true
    try {
      if (reader.autoReadingMethod === '段落滚动') {
        await autoReadByParagraph()
      } else {
        await autoReadByPixel()
      }
    } finally {
      autoReadAdvancing = false
    }
  }, delay)
}

async function autoReadByPixel() {
  if (isVerticalRead.value && contentEl.value) {
    const el = contentEl.value
    const bottom = Math.max(0, el.scrollHeight - el.clientHeight)
    if (el.scrollTop < bottom - 4) {
      el.scrollTop = Math.min(bottom, el.scrollTop + reader.autoReadingPixel)
      runAutoReadLoop(reader.autoReadingLineTime)
      return
    }
  }
  const advanced = await advanceAutoReadPage()
  if (advanced) runAutoReadLoop(reader.autoReadingLineTime)
}

async function autoReadByParagraph() {
  if (!isVerticalRead.value || !contentEl.value || !contentBody.value) {
    const advanced = await advanceAutoReadPage()
    if (advanced) runAutoReadLoop(reader.autoReadingLineTime)
    return
  }
  const current = currentVisibleParagraph()
  const next = nextParagraphAfter(current)
  if (next) {
    const currentRect = current?.getBoundingClientRect?.()
    const lineHeight = Math.max(1, Number(reader.fontSize || 18) * Number(reader.lineHeight || 1.8))
    const lineCount = currentRect?.height ? Math.max(1, Math.ceil(currentRect.height / lineHeight)) : 1
    scrollParagraphIntoView(next)
    progressVersion.value += 1
    saveCurrentProgress()
    runAutoReadLoop(reader.autoReadingLineTime * lineCount)
    return
  }
  const advanced = await advanceAutoReadPage()
  if (advanced) runAutoReadLoop(reader.autoReadingLineTime)
}

function nextParagraphAfter(paragraph) {
  const paragraphs = [...(contentBody.value?.querySelectorAll('p') || [])]
  if (!paragraph) return paragraphs[0] || null
  const index = paragraphs.indexOf(paragraph)
  return index >= 0 ? paragraphs[index + 1] || null : paragraphs[0] || null
}

function scrollParagraphIntoView(paragraph) {
  if (!paragraph || !contentEl.value) return
  const viewport = contentEl.value.getBoundingClientRect()
  const rect = paragraph.getBoundingClientRect()
  const nextTop = contentEl.value.scrollTop + rect.top - viewport.top - 24
  contentEl.value.scrollTo({ top: Math.max(0, nextTop), behavior: readerScrollBehavior() })
}

async function advanceAutoReadPage() {
  const beforeChapter = currentIndex.value
  const beforePage = page.value
  try {
    await nextPage()
    if (beforeChapter === currentIndex.value && beforePage === page.value) {
      stopAutoReading()
      toastMsg.value = '已到本书末尾'
      setTimeout(() => { toastMsg.value = '' }, 1200)
      return false
    }
    return true
  } finally {
  }
}

function stopAutoReading() {
  autoReading.value = false
  autoReadAdvancing = false
  clearTimeout(autoReadTimer)
  autoReadTimer = null
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
      setTimeout(saveCurrentProgress, reader.animateDuration + 60)
      return
    }
  }
  if (currentIndex.value > 0) await goChapter(currentIndex.value - 1, CHAPTER_END_OFFSET)
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
      setTimeout(saveCurrentProgress, reader.animateDuration + 60)
      return
    }
  }
  if (currentIndex.value < chapters.value.length - 1) await goChapter(currentIndex.value + 1)
}

function scrollStep() {
  const viewportHeight = contentEl.value?.clientHeight || window.innerHeight || readableViewportSize().height
  return Math.max(1, Math.floor(viewportHeight - scrollOffset()))
}

function scrollOffset() {
  const fontSize = Number(reader.fontSize || 18)
  return (
    fontSize * Number(reader.lineHeight || 1.8) * 2 +
    fontSize * Number(reader.paragraphSpace || 0) * 2
  )
}

function readerScrollBehavior() {
  return reader.animateDuration > 0 ? 'smooth' : 'auto'
}

function handleDesktopProgressInput(event) {
  seekCurrentChapterPercent(Number(event.target.value || 0) / 1000, { save: false })
}

function handleDesktopProgressChange(event) {
  seekCurrentChapterPercent(Number(event.target.value || 0) / 1000, { save: true })
}

function seekCurrentChapterPercent(percent, options = {}) {
  const value = Math.max(0, Math.min(1, Number(percent) || 0))
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
  clearTimeout(saveTimer)
  if (options.save === false) {
    saveTimer = setTimeout(saveCurrentProgress, 500)
  } else {
    saveCurrentProgress()
  }
}

function handleTapZone(zone) {
  if (isOverlayOpen.value) return
  if (zone === 'center') {
    toggleMobileReaderChrome()
    return
  }

  if (autoReading.value) {
    toggleMobileReaderChrome()
    return
  }

  if (reader.clickMethod === 'next') {
    mobileChromeVisible.value = false
    nextPage()
    return
  }

  if (reader.clickMethod === 'none') {
    toggleMobileReaderChrome()
    return
  }

  if (reader.mode === 'flip') {
    if (zone === 'left') previousPage()
    if (zone === 'right') nextPage()
    return
  }

  if (zone === 'upper') {
    previousPage()
    return
  }
  if (zone === 'lower') nextPage()
}

function handleReaderContentClick(event) {
  if (isOverlayOpen.value || !pageEl.value) return
  if (Date.now() - handledTouchTapAt < 450) return
  if (ignoreNextContentClick) {
    ignoreNextContentClick = false
    return
  }
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
  if (Math.hypot(moveX, moveY) > MOBILE_TAP_MOVE_TOLERANCE) {
    readerTouchMoved = true
  }
  if (reader.mode === 'flip' && Math.abs(moveX) > 12 && Math.abs(moveX) > Math.abs(moveY) + 8) {
    event.preventDefault()
    event.stopPropagation()
  }
}

function handleReaderTouchEnd(event) {
  if (!isMobileReader.value) return
  const touch = event.changedTouches?.[0]
  if (scheduleSelectedTextOperation(200)) {
    ignoreNextContentClick = true
    readerTouchStart = null
    readerTouchMoved = false
    readerTouchMove = { x: 0, y: 0 }
    return
  }
  const elapsed = readerTouchStart ? Date.now() - readerTouchStart.at : 0
  const moveDistance = Math.hypot(Number(readerTouchMove.x || 0), Number(readerTouchMove.y || 0))
  const isTap = moveDistance <= MOBILE_TAP_MOVE_TOLERANCE && elapsed < 650 && Boolean(touch)
  ignoreNextContentClick = Boolean(touch)
  if (isTap) handledTouchTapAt = Date.now()
  setTimeout(() => {
    ignoreNextContentClick = false
  }, 360)
  if (readerTouchMoved && !isOverlayOpen.value && shouldHandleHorizontalSwipe()) {
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

function shouldHandleHorizontalSwipe() {
  if (reader.mode !== 'flip') return false
  const moveX = Number(readerTouchMove.x || 0)
  const moveY = Number(readerTouchMove.y || 0)
  return Math.abs(moveX) >= 42 && Math.abs(moveX) > Math.abs(moveY) * 1.2
}

function handleTapPoint(point) {
  if (isOverlayOpen.value || !point?.rect) return
  if (scheduleSelectedTextOperation(0)) {
    ignoreNextContentClick = true
    return
  }
  const viewportWidth = window.innerWidth || point.rect.width
  const viewportHeight = window.innerHeight || point.rect.height
  const pointX = Number.isFinite(point.clientX) ? point.clientX : point.relX
  const pointY = Number.isFinite(point.clientY) ? point.clientY : point.relY
  const midX = viewportWidth / 2
  const midY = viewportHeight / 2
  const centerWidthRatio = 0.2
  const centerHeightRatio = 0.2
  const inMenuZone = Math.abs(pointX - midX) <= viewportWidth * centerWidthRatio
    && Math.abs(pointY - midY) <= viewportHeight * centerHeightRatio

  if (inMenuZone) {
    toggleReaderChrome()
    return
  }

  if (autoReading.value) {
    toggleMobileReaderChrome()
    return
  }

  if (reader.clickMethod === 'next') {
    mobileChromeVisible.value = false
    nextPage()
    return
  }

  if (reader.clickMethod === 'none') {
    toggleReaderChrome()
    return
  }

  mobileChromeVisible.value = false
  if (reader.mode === 'flip') {
    if (pointX > midX) nextPage()
    else previousPage()
    return
  }

  if (pointY > midY) nextPage()
  else previousPage()
}

function handleDesktopTapPoint(point) {
  if (isOverlayOpen.value || !point?.rect) return
  if (scheduleSelectedTextOperation(0)) {
    ignoreNextContentClick = true
    return
  }
  const viewportWidth = window.innerWidth || point.rect.width
  const viewportHeight = window.innerHeight || point.rect.height
  const pointX = Number.isFinite(point.clientX) ? point.clientX : point.relX
  const pointY = Number.isFinite(point.clientY) ? point.clientY : point.relY
  const midX = viewportWidth / 2
  const midY = viewportHeight / 2
  const inCenter = Math.abs(pointX - midX) <= viewportWidth * 0.2
    && Math.abs(pointY - midY) <= viewportHeight * 0.2
  if (inCenter || reader.clickMethod === 'none') return
  if (reader.clickMethod === 'next') {
    nextPage()
    return
  }
  if (reader.mode === 'flip') {
    if (pointX > midX) nextPage()
    else previousPage()
    return
  }
  if (pointY > midY) nextPage()
  else previousPage()
}

function handleReaderWheel(event) {
  if (event._openReaderWheelHandled) return
  event._openReaderWheelHandled = true
  if (isOverlayOpen.value) return
  if (!shellEl.value?.contains(event.target)) return
  const target = event.target
  if (target?.closest?.('button, a, input, textarea, select, [role="button"], .el-drawer, .el-dialog') && !target?.closest?.('.tap-zone')) return
  const delta = normalizedWheelDelta(event)
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

function normalizedWheelDelta(event) {
  const rawDelta = Math.abs(event.deltaX) > Math.abs(event.deltaY) ? event.deltaX : event.deltaY
  if (event.deltaMode === 1) {
    const lineHeight = Number(reader.fontSize || 18) * Number(reader.lineHeight || 1.8)
    return rawDelta * Math.max(12, lineHeight)
  }
  if (event.deltaMode === 2) {
    return rawDelta * (contentEl.value?.clientHeight || window.innerHeight || 800)
  }
  return rawDelta
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
    pageWidth.value = viewport.width
    pageHeight.value = viewport.height
    pageCount.value = Math.max(1, Math.ceil(contentBody.value.scrollWidth / pageWidth.value))
    page.value = Math.min(page.value, pageCount.value - 1)
    return
  }
  if (reader.mode === 'page') {
    pageHeight.value = scrollStep()
    const scrollBottom = Math.max(contentEl.value.scrollHeight - contentEl.value.clientHeight, 1)
    pageCount.value = Math.max(1, Math.ceil(contentEl.value.scrollHeight / pageHeight.value))
    page.value = Math.max(0, Math.min(pageCount.value - 1, Math.round((contentEl.value.scrollTop / scrollBottom) * Math.max(pageCount.value - 1, 0))))
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
  if (!chapter.value || restoringPosition || savingProgress || pendingProgressPayload) return
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
  clearTimeout(saveTimer)
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
    lastProgressSaveKey = progressSaveKey(currentProgressPayload())
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
  chapterContentCache = null
  browserCachedChapters.value = {}
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
  clearTimeout(saveTimer)
  saveTimer = setTimeout(saveCurrentProgress, 500)
}

function currentChapterPercent() {
  progressVersion.value
  if (reader.mode === 'flip') {
    return pageCount.value <= 1 ? 0 : page.value / (pageCount.value - 1)
  }
  const snapshot = visibleChapterProgressSnapshot()
  if (snapshot) return snapshot.chapterPercent
  const el = contentEl.value
  if (!el) return 0
  const textLength = Math.max(chapterTextLength.value, 1)
  const position = currentChapterPosition()
  if (position > 0 || isContinuousScrollRead.value) return Math.max(0, Math.min(1, position / textLength))
  const bottom = Math.max(el.scrollHeight - el.clientHeight, 1)
  const scrollTop = Number(el.scrollTop || 0)
  if (scrollTop > 0) return scrollTop / bottom
  return position / textLength
}

function currentOffset() {
  if (reader.mode === 'flip') {
    return Math.max(0, Math.floor(page.value || 0))
  }
  const snapshot = visibleChapterProgressSnapshot()
  if (snapshot) return snapshot.offset
  return currentChapterPosition()
}

function currentChapterPosition() {
  const snapshot = visibleChapterProgressSnapshot()
  if (snapshot) return snapshot.offset
  const el = contentEl.value
  if (!el) return 0
  const activeChapter = activeChapterElement()
  const heading = activeChapter?.querySelector('h1') || contentBody.value?.querySelector('h1')
  const viewport = el.getBoundingClientRect()
  const headingRect = heading?.getBoundingClientRect()
  if (headingRect && headingRect.bottom >= viewport.top && headingRect.top <= viewport.bottom) return 0
  const paragraph = currentVisibleParagraph()
  const paragraphPos = Number(paragraph?.dataset?.pos)
  if (Number.isFinite(paragraphPos)) {
    const rect = paragraph.getBoundingClientRect()
    const anchorY = viewport.top + Math.min(viewport.height * 0.32, 180)
    const ratio = rect.height > 0 ? Math.max(0, Math.min(1, (anchorY - rect.top) / rect.height)) : 0
    const extra = Math.round((paragraph.textContent?.length || 0) * ratio)
    return Math.max(0, Math.round(paragraphPos + extra))
  }
  const bottom = Math.max(el.scrollHeight - el.clientHeight, 1)
  const textLength = Math.max(chapterTextLength.value, 1)
  const scrollPercent = Math.max(0, Math.min(1, Number(el.scrollTop || 0) / bottom))
  if (scrollPercent > 0) return Math.round(scrollPercent * textLength)
  return 0
}

function visibleChapterProgressSnapshot() {
  if (!contentEl.value || !contentBody.value) return null
  const paragraph = currentVisibleParagraph()
  if (!paragraph) return null
  const chapterEl = paragraph.closest?.('.chapter-content')
  const chapterIndex = Number(chapterEl?.dataset?.index)
  if (!Number.isInteger(chapterIndex)) return null
  const block = displayedChapterBlocks.value.find(item => item.index === chapterIndex)
    || chapterBlocks.value.find(item => item.index === chapterIndex)
    || (chapterIndex === currentIndex.value ? makeChapterBlock(currentIndex.value, chapter.value, content.value) : null)
  const paragraphPos = Number(paragraph.dataset?.pos)
  const offset = Number.isFinite(paragraphPos)
    ? visibleParagraphOffset(paragraph, paragraphPos)
    : 0
  const textLength = Math.max(chapterBlockTextLength(block), 1)
  return {
    chapterIndex,
    chapter: chapters.value[chapterIndex] || (block?.id ? { id: block.id, title: block.title, index: chapterIndex } : null),
    offset,
    chapterPercent: Math.max(0, Math.min(1, offset / textLength)),
  }
}

function visibleParagraphOffset(paragraph, paragraphPos) {
  const viewport = contentEl.value?.getBoundingClientRect()
  if (!viewport) return Math.max(0, Math.round(paragraphPos))
  const rect = paragraph.getBoundingClientRect()
  const anchorY = viewport.top + Math.min(viewport.height * 0.32, 180)
  const ratio = rect.height > 0 ? Math.max(0, Math.min(1, (anchorY - rect.top) / rect.height)) : 0
  const extra = Math.round((paragraph.textContent?.length || 0) * ratio)
  return Math.max(0, Math.round(paragraphPos + extra))
}

function currentVisibleParagraph() {
  const viewport = contentEl.value?.getBoundingClientRect()
  const paragraphs = [...(contentBody.value?.querySelectorAll('p') || [])]
  if (!viewport || !paragraphs.length) return null
  const visibleTop = viewport.top + 8
  const visibleBottom = viewport.bottom - 8
  const visibleLeft = viewport.left + 8
  const visibleRight = viewport.right - 8
  const anchorY = viewport.top + Math.min(viewport.height * 0.32, 180)
  const visible = paragraphs
    .map(node => ({ node, rect: node.getBoundingClientRect() }))
    .filter(({ rect }) => rect.bottom >= visibleTop && rect.top <= visibleBottom && rect.right >= visibleLeft && rect.left <= visibleRight)
  if (!visible.length) return null
  const anchored = visible.find(({ rect }) => rect.top <= anchorY && rect.bottom >= anchorY)
  if (anchored) return anchored.node
  return visible.sort((a, b) => Math.abs(a.rect.top - anchorY) - Math.abs(b.rect.top - anchorY))[0]?.node || null
}

function activeChapterElement() {
  const paragraph = currentVisibleParagraph()
  const chapterEl = paragraph?.closest?.('.chapter-content')
  if (chapterEl) return chapterEl
  return contentBody.value?.querySelector(`.chapter-content[data-index="${currentIndex.value}"]`) || null
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
  const nearBottom = el.scrollTop + el.clientHeight > el.scrollHeight - el.clientHeight * 2
  const nearTop = reader.mode === 'scroll2' && el.scrollTop < el.clientHeight
  if (!nearTop && !nearBottom) return
  extendingShowChapters = true
  Promise.all([
    nearTop ? prependPreviousShowChapter() : Promise.resolve(),
    nearBottom ? appendNextShowChapter() : Promise.resolve(),
  ])
    .catch(() => {})
    .finally(() => {
      extendingShowChapters = false
    })
}

function pruneScroll2ChapterWindow() {
  if (reader.mode !== 'scroll2' || !contentEl.value || !chapterBlocks.value.length) return
  const minIndex = Math.max(0, currentIndex.value - SHOW_PREV_CHAPTER_SIZE)
  const maxIndex = Math.min(chapters.value.length - 1, currentIndex.value + SHOW_NEXT_CHAPTER_SIZE)
  const currentBlocks = chapterBlocks.value
  if (currentBlocks.every(block => block.index >= minIndex && block.index <= maxIndex)) return
  const removedBeforeHeight = currentBlocks
    .filter(block => block.index < minIndex)
    .reduce((sum, block) => {
      const element = contentBody.value?.querySelector(`.chapter-content[data-index="${block.index}"]`)
      return sum + (element?.getBoundingClientRect?.().height || 0)
    }, 0)
  const beforeTop = contentEl.value.scrollTop
  chapterBlocks.value = currentBlocks.filter(block => block.index >= minIndex && block.index <= maxIndex)
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

function scheduleSelectedTextOperation(delay = 0) {
  if (reader.selectionAction === '忽略') return false
  clearTimeout(selectionOperateTimer)
  const selectedNow = selectedReaderText()
  selectionOperateTimer = window.setTimeout(() => {
    const text = selectedReaderText()
    if (!text) return
    ignoreNextContentClick = true
    handleSelectedTextOperation(text).catch(err => {
      if (err === 'cancel' || err === 'close') return
      ElMessage.error(readError(err, '处理选中文字失败'))
    })
  }, delay)
  return Boolean(selectedNow)
}

function selectedReaderText() {
  if (typeof window === 'undefined' || !contentBody.value) return ''
  const selection = window.getSelection?.()
  const text = selection?.toString?.().replace(/\s+/g, ' ').trim()
  if (!text || !selection.rangeCount) return ''
  const range = selection.getRangeAt(0)
  const container = range.commonAncestorContainer?.nodeType === window.Node?.ELEMENT_NODE
    ? range.commonAncestorContainer
    : range.commonAncestorContainer?.parentElement
  if (!container || !contentBody.value.contains(container)) return ''
  return text.slice(0, 1000)
}

async function handleSelectedTextOperation(text) {
  if (selectionOperating || reader.selectionAction === '忽略') return
  selectionOperating = true
  try {
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
  } finally {
    clearReaderSelection()
    selectionOperating = false
    window.setTimeout(() => {
      ignoreNextContentClick = false
    }, 320)
  }
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

async function createBookmarkFromSelectedText(text) {
  if (!chapter.value) return
  const cleanText = String(text || '').trim()
  const { data } = await api.post(`/books/${bookId.value}/bookmarks`, {
    chapterId: chapter.value.id,
    chapterIndex: currentIndex.value,
    offset: currentOffset(),
    percent: currentChapterPercent(),
    title: chapter.value.title,
    excerpt: cleanText.slice(0, 500),
  })
  bookmarks.value = [data, ...bookmarks.value]
  toastMsg.value = '书签已创建'
  setTimeout(() => { toastMsg.value = '' }, 1600)
}

function clearReaderSelection() {
  try {
    window.getSelection?.()?.removeAllRanges?.()
  } catch {
    // Selection APIs may be unavailable in embedded browsers.
  }
}

async function saveCurrentProgress(options = {}) {
  if (!chapter.value) return
  const force = Boolean(options.force)
  const background = Boolean(options.background)
  const baseUpdatedAt = progressServerBaseUpdatedAt()
  const payload = {
    ...currentProgressPayload(),
    baseUpdatedAt,
  }
  applyLocalProgressSnapshot(payload, { force })
  const key = progressSaveKey(payload)
  if (key === lastProgressSaveKey && !force) return
  pendingProgressPayload = payload
  if (background) {
    sendProgressKeepAlive(payload)
    flushProgressQueue(force).catch(() => {})
    return
  }
  await flushProgressQueue(force)
}

function sendProgressKeepAlive(payload) {
  if (typeof window === 'undefined' || typeof fetch !== 'function' || !payload?.bookId) return
  const token = window.localStorage?.getItem('openreader_token')
  if (!token) return
  try {
    fetch('/api/progress', {
      method: 'PUT',
      keepalive: true,
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({
        ...payload,
        mode: reader.mode,
        clientUpdatedAt: reader.progressByBook[payload.bookId]?.updatedAt || new Date().toISOString(),
        clientId: reader.ensureClientId(),
      }),
    }).catch(() => {})
  } catch {
    // The queued local progress remains pending and will sync on the next open.
  }
}

async function flushProgressQueue(force = false) {
  if (savingProgress) {
    if (!force) return
    await waitForProgressSaveIdle()
    if (savingProgress) return
  }
  savingProgress = true
  try {
    while (pendingProgressPayload) {
      const elapsed = Date.now() - lastProgressRequestAt
      if (!force && elapsed < SAVE_PROGRESS_MIN_INTERVAL) {
        clearTimeout(saveTimer)
        saveTimer = setTimeout(() => saveCurrentProgress(), SAVE_PROGRESS_MIN_INTERVAL - elapsed)
        break
      }
      const nextPayload = pendingProgressPayload
      pendingProgressPayload = null
      const nextKey = progressSaveKey(nextPayload)
      if (nextKey === lastProgressSaveKey && !force) continue
      lastProgressRequestAt = Date.now()
      const savedProgress = await reader.saveProgress(nextPayload)
      upsertReaderBookProgress(savedProgress, { replace: true })
      lastProgressSaveKey = nextKey
    }
  } finally {
    savingProgress = false
  }
}

function currentProgressPayload() {
  const snapshot = visibleChapterProgressSnapshot()
  const progressChapter = snapshot?.chapter || chapter.value
  const progressChapterIndex = Number.isInteger(snapshot?.chapterIndex) ? snapshot.chapterIndex : currentIndex.value
  const progressChapterPercent = snapshot ? snapshot.chapterPercent : currentChapterPercent()
  const progressTotal = Math.max(chapters.value.length, 1)
  return {
    bookId: bookId.value,
    chapterId: progressChapter?.id,
    chapterIndex: progressChapterIndex,
    offset: snapshot ? snapshot.offset : currentOffset(),
    percent: Math.min(1, Math.max(0, (progressChapterIndex + progressChapterPercent) / progressTotal)),
    chapterPercent: progressChapterPercent,
    chapterTitle: progressChapter?.title || '',
  }
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
  const progress = reader.progressByBook[targetBookId]
  if (!progress) return ''
  if (progress.pendingSync) return progress.baseUpdatedAt || ''
  return progress.updatedAt || ''
}

function waitForProgressSaveIdle(timeout = 1500) {
  const started = Date.now()
  return new Promise(resolve => {
    const tick = () => {
      if (!savingProgress || Date.now() - started >= timeout) {
        resolve()
        return
      }
      window.setTimeout(tick, 40)
    }
    tick()
  })
}

function progressSaveKey(payload) {
  return [
    payload.bookId,
    payload.chapterId,
    payload.chapterIndex,
    payload.offset,
    Math.round(Number(payload.percent || 0) * 10000),
    Math.round(Number(payload.chapterPercent || 0) * 10000),
    reader.mode,
  ].join(':')
}

function progressUpdatedAtMs(progress) {
  const time = Date.parse(progress?.updatedAt || '')
  return Number.isFinite(time) ? time : 0
}

async function createBookmark() {
  if (!chapter.value) return
  const excerpt = currentVisibleExcerpt()
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
  const excerpt = currentVisibleExcerpt()
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

async function removeBookmarks(rows) {
  if (!Array.isArray(rows) || !rows.length) return
  try {
    await ElMessageBox.confirm(`确认要删除所选择的 ${rows.length} 条书签吗？`, '批量删除书签', { type: 'warning' })
    await Promise.all(rows.map(item => api.delete(`/bookmarks/${item.id}`)))
    const deleted = new Set(rows.map(item => item.id))
    bookmarks.value = bookmarks.value.filter(item => !deleted.has(item.id))
    ElMessage.success('书签已删除')
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '批量删除书签失败'))
  }
}

async function importBookmarks(rows) {
  const payloads = normalizeImportedBookmarks(rows)
  if (!payloads.length) {
    ElMessage.error('书签文件没有可导入内容')
    return
  }
  try {
    await ElMessageBox.confirm(`确认要导入文件中的 ${payloads.length} 条书签到当前书籍吗？`, '导入书签', { type: 'info' })
    const created = []
    for (const payload of payloads) {
      const { data } = await api.post(`/books/${bookId.value}/bookmarks`, payload)
      if (data?.id) created.push(data)
    }
    bookmarks.value = [...created, ...bookmarks.value]
    ElMessage.success(`已导入 ${created.length} 条书签`)
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '导入书签失败'))
  }
}

function normalizeImportedBookmarks(rows) {
  return (Array.isArray(rows) ? rows : [])
    .map(row => {
      const chapterIndex = Math.max(0, Math.floor(Number(row.chapterIndex ?? row.durChapterIndex ?? 0)))
      return {
        chapterIndex,
        offset: Math.max(0, Math.floor(Number(row.offset ?? 0))),
        percent: clampPercent(row.percent),
        title: String(row.title || row.chapterName || row.chapterTitle || `第 ${chapterIndex + 1} 章`).trim(),
        excerpt: String(row.excerpt || row.bookText || '').trim(),
        note: String(row.note || row.content || '').trim(),
      }
    })
    .filter(row => row.title || row.excerpt || row.note)
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
  const query = bookmarkReaderQuery(bookmark)
  if (bookmark.chapterIndex === currentIndex.value) {
    await loadChapter(currentIndex.value, Number(query.offset || 0), { restorePercent: parseRoutePercent(query.percent), saveAfterLoad: true })
    return
  }
  await router.replace({ name: 'reader', params: { id: bookId.value }, query })
}

function bookmarkReaderQuery(bookmark) {
  return {
    chapter: bookmark.chapterIndex,
    offset: bookmark.offset || 0,
    percent: Number.isFinite(Number(bookmark.percent)) ? Number(bookmark.percent) : undefined,
  }
}

function parseRoutePercent(value) {
  if (value === undefined || value === null || value === '') return null
  const percent = Number(value)
  return Number.isFinite(percent) ? Math.max(0, Math.min(1, percent)) : null
}

function clampPercent(value) {
  const percent = Number(value)
  return Number.isFinite(percent) ? Math.max(0, Math.min(1, percent)) : 0
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
    else if (currentIndex.value > 0) goChapter(currentIndex.value - 1, CHAPTER_END_OFFSET)
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

// ---- TTS ----
const tts = useTTS()
const ttsVoices = computed(() => tts.voices.value)
const ttsSleepMinutes = ref(0)
const ttsSleepEndAt = ref(0)
const ttsProgressLabel = computed(() => {
  const total = tts.total.value || 0
  if (!tts.state.playing || total <= 0) return '段落 - / -'
  return `段落 ${Math.min(tts.currentIndex.value + 1, total)} / ${total}`
})
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

function setTTSSleepMinutes(value) {
  const minutes = Math.max(0, Math.min(180, Math.floor(Number(value) || 0)))
  ttsSleepMinutes.value = minutes
  ttsSleepEndAt.value = minutes > 0 ? Date.now() + minutes * 60 * 1000 : 0
}

function isTTSSleepExpired() {
  return ttsSleepEndAt.value > 0 && Date.now() > ttsSleepEndAt.value
}

function handleTTSParagraphStart() {
  if (!isTTSSleepExpired()) return
  ttsContinueToken += 1
  tts.stop()
  toastMsg.value = '定时关闭朗读'
  setTimeout(() => { toastMsg.value = '' }, 1400)
}

function toggleTTS() {
  if (!tts.state.supported) {
    toastMsg.value = '当前浏览器不支持朗读'
    return
  }
  if (tts.state.playing) {
    ttsContinueToken += 1
    tts.stop()
  } else {
    const token = ++ttsContinueToken
    if (ttsSleepMinutes.value > 0 && !ttsSleepEndAt.value) setTTSSleepMinutes(ttsSleepMinutes.value)
    tts.speak(content.value, () => {
      if (isTTSSleepExpired()) {
        handleTTSParagraphStart()
        return
      }
      if (currentIndex.value < chapters.value.length - 1) {
        speakNextChapter(currentIndex.value + 1, token)
      }
    }, handleTTSParagraphStart)
  }
}
function ttsStop() {
  ttsContinueToken += 1
  tts.stop()
}

async function speakNextChapter(index, token) {
  await goChapter(index)
  for (let attempt = 0; attempt < 30; attempt += 1) {
    if (token !== ttsContinueToken) return
    await new Promise(resolve => setTimeout(resolve, 120))
    if (currentIndex.value === index && content.value.trim()) {
      tts.speak(content.value, () => {
        if (isTTSSleepExpired()) {
          handleTTSParagraphStart()
          return
        }
        if (token === ttsContinueToken && currentIndex.value < chapters.value.length - 1) {
          speakNextChapter(currentIndex.value + 1, token)
        }
      }, handleTTSParagraphStart)
      return
    }
  }
}

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
    var(--reader-body-bg);
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
  background: color-mix(in srgb, var(--reader-popup-bg) 64%, transparent);
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
  background: color-mix(in srgb, var(--reader-popup-bg) 58%, transparent);
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
  background: color-mix(in srgb, var(--reader-popup-bg) 82%, transparent);
}

.rail-item:disabled {
  cursor: not-allowed;
  opacity: 0.42;
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
  grid-template-columns: 36px;
  grid-auto-rows: 36px;
  gap: 16px;
  max-height: calc(100vh - 190px);
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
  background: var(--reader-popup-bg);
  border: 1px solid rgba(255, 255, 255, 0.7);
  border-radius: 999px;
  box-shadow: 0 4px 10px rgba(80, 62, 28, 0.08);
  cursor: pointer;
}

.round-tool:hover,
.round-tool.active {
  color: #0f5451;
  background: var(--reader-popup-bg);
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

.reader-tap-zones {
  position: absolute;
  inset: 0;
  z-index: 2;
  display: none;
  pointer-events: none;
}

.tap-zone {
  position: absolute;
  padding: 0;
  background: transparent;
  border: 0;
  cursor: pointer;
  pointer-events: auto;
}

.tap-left {
  top: 0;
  bottom: 0;
  left: 0;
  width: 24%;
}

.tap-right {
  top: 0;
  right: 0;
  bottom: 0;
  width: 24%;
}

.tap-center {
  top: 35%;
  right: 24%;
  bottom: 35%;
  left: 24%;
}

.tap-upper {
  top: 0;
  right: 24%;
  left: 24%;
  height: 35%;
}

.tap-lower {
  right: 24%;
  bottom: 0;
  left: 24%;
  height: 35%;
}

.reader-shell.scroll .tap-left,
.reader-shell.scroll .tap-right,
.reader-shell.scroll2 .tap-left,
.reader-shell.scroll2 .tap-right,
.reader-shell.page .tap-left,
.reader-shell.page .tap-right {
  display: none;
}

.reader-shell.scroll .tap-upper,
.reader-shell.scroll .tap-lower,
.reader-shell.scroll2 .tap-upper,
.reader-shell.scroll2 .tap-lower,
.reader-shell.page .tap-upper,
.reader-shell.page .tap-lower {
  right: 0;
  left: 0;
}

.reader-shell.flip .tap-upper,
.reader-shell.flip .tap-lower {
  display: none;
}

.click-zone-overlay {
  position: absolute;
  inset: 0;
  z-index: 30;
  display: grid;
  grid-template-rows: 35% 30% 35%;
  background: rgba(20, 20, 20, 0.08);
}

.click-zone-overlay.flip {
  grid-template-columns: 24% 52% 24%;
  grid-template-rows: 1fr;
}

.click-zone-piece {
  display: grid;
  place-items: center;
  border: 1px dashed rgba(237, 66, 89, 0.55);
  background: rgba(237, 66, 89, 0.08);
  color: #ed4259;
  font-size: 16px;
  pointer-events: none;
}

.click-zone-piece span {
  border-radius: 999px;
  padding: 8px 14px;
  background: rgba(255, 255, 255, 0.82);
}

.click-zone-overlay.flip .click-zone-prev { grid-column: 1; }
.click-zone-overlay.flip .click-zone-menu { grid-column: 2; }
.click-zone-overlay.flip .click-zone-next { grid-column: 3; }

.click-zone-close {
  position: absolute;
  right: 18px;
  bottom: 18px;
  border: 0;
  border-radius: 999px;
  padding: 8px 16px;
  background: #ed4259;
  color: #fff;
  cursor: pointer;
}

@media (hover: hover) and (pointer: fine) {
  .reader-tap-zones {
    display: block;
  }
  .tap-zone {
    display: none;
  }
  .tap-center {
    display: block;
    pointer-events: none;
  }
  .reader-shell.scroll .tap-upper,
  .reader-shell.scroll .tap-lower,
  .reader-shell.scroll2 .tap-upper,
  .reader-shell.scroll2 .tap-lower,
  .reader-shell.page .tap-upper,
  .reader-shell.page .tap-lower {
    display: block;
  }
  .reader-shell.flip .tap-left,
  .reader-shell.flip .tap-right {
    display: block;
  }
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
.chapter-content {
  min-height: 1px;
}
.reader-shell.scroll .chapter-content + .chapter-content,
.reader-shell.scroll2 .chapter-content + .chapter-content {
  padding-top: 58px;
}
.reader-shell.scroll .reader-body::after,
.reader-shell.scroll2 .reader-body::after {
  content: "";
  display: block;
  height: min(40vh, 280px);
}
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
.reader-content p.reader-search-active {
  background: rgba(47, 111, 109, 0.16);
  box-shadow: -8px 0 0 rgba(47, 111, 109, 0.16), 8px 0 0 rgba(47, 111, 109, 0.16);
  transition: background 160ms ease, box-shadow 160ms ease;
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
.reader-shell.flip .reader-body h1,
.reader-shell.flip .reader-body p {
  break-inside: avoid;
}
.reader-shell.flip .reader-body {
  transition: transform var(--reader-animate-duration, 180ms) ease;
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
  background: color-mix(in srgb, var(--reader-popup-bg) 82%, transparent);
  border: 1px solid rgba(148, 132, 87, 0.38);
  border-bottom: 0;
}

.progress-box,
.page-step {
  display: grid;
  height: 43px;
  place-items: center;
  color: #121212;
  background: color-mix(in srgb, var(--reader-popup-bg) 62%, transparent);
  border: 0;
  border-bottom: 1px solid rgba(148, 132, 87, 0.32);
  font-size: 16px;
}

.desktop-progress-control {
  position: fixed;
  right: auto;
  left: calc(50vw + var(--reader-frame-width) / 2 + 100px);
  bottom: 0;
  z-index: 4;
  display: grid;
  width: 42px;
  min-height: 154px;
  place-items: center;
  gap: 7px;
  padding: 9px 0;
  color: #121212;
  background: color-mix(in srgb, var(--reader-popup-bg) 70%, transparent);
  border-bottom: 1px solid rgba(148, 132, 87, 0.32);
  font-size: 12px;
}

.desktop-progress-control span {
  line-height: 1;
}

.desktop-progress-slider {
  width: 18px;
  height: 124px;
  margin: 0;
  accent-color: #2f6f6d;
  cursor: pointer;
  writing-mode: vertical-lr;
}

.page-step {
  cursor: pointer;
}

.chapter-step {
  padding: 0;
  font-size: 16px;
}

.chapter-step:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.page-step:hover {
  background: var(--reader-popup-bg);
}

.reader-mobile-bottom {
  display: none;
}

.reader-mobile-progress-panel {
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
.tts-progress { color: #fff; font-size: 12px; white-space: nowrap; }
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
.reader-shelf-list {
  display: grid;
  max-height: calc(100vh - 154px);
  overflow-y: auto;
  overscroll-behavior: contain;
}
.reader-shelf-card {
  display: grid;
  gap: 6px;
  width: 100%;
  max-width: 100%;
  overflow: hidden;
  padding: 8px 0;
  color: #24282c;
  background: transparent;
  border: 0;
  border-bottom: 1px solid rgba(160, 139, 91, 0.22);
  cursor: pointer;
  text-align: left;
}
.reader-shelf-card:hover,
.reader-shelf-card.active {
  color: #ed4259;
  background: transparent;
}
.reader-shelf-title-line {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.reader-shelf-title-line strong,
.reader-shelf-chapter {
  min-width: 0;
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
.reader-shelf-chapter {
  color: #888;
  font-size: 14px;
}
.reader-cache-panel {
  display: grid;
  gap: 16px;
  color: #5f553f;
  font-size: 14px;
}
.reader-cache-panel p {
  margin: 0;
  color: #7b715e;
}
.reader-cache-actions {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
}
.reader-cache-actions button,
.reader-cache-status button {
  min-height: 42px;
  color: #2a2925;
  background: var(--reader-popup-bg);
  border: 1px solid #e7dabb;
  border-radius: 6px;
  cursor: pointer;
  font-size: 14px;
}
.reader-cache-actions button:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}
.reader-cache-status {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  background: color-mix(in srgb, var(--reader-popup-bg) 88%, transparent);
  border: 1px solid #eadfca;
  border-radius: 6px;
}
.reader-cache-status button {
  flex: 0 0 auto;
  min-height: 34px;
  padding: 0 14px;
}
/* ---- 编辑弹层 ---- */
.bookmark-editor {
  display: grid;
  gap: 10px;
}

.empty-hint { color: #999; text-align: center; padding-top: 40px; text-indent: 0; }

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
  .reader-shell.mobile-chrome-visible .reader-content {
    padding-bottom: calc(250px + env(safe-area-inset-bottom));
    scroll-padding-bottom: calc(250px + env(safe-area-inset-bottom));
  }
  .reader-content h1 { font-size: var(--reader-heading-size); margin-bottom: 28px; }
  .reader-left-rail,
  .reader-right-rail,
  .reader-page-control,
  .desktop-progress-control,
  .reader-tap-zones {
    display: none;
  }
  .reader-mobile-top {
    position: fixed;
    top: 0;
    right: 0;
    left: 0;
    z-index: 8;
    display: none;
    grid-template-columns: 44px minmax(0, 1fr) 52px;
    align-items: center;
    gap: 8px;
    min-height: 58px;
    padding: max(8px, env(safe-area-inset-top)) 12px 8px;
    background: color-mix(in srgb, var(--reader-popup-bg) 96%, transparent);
    border-bottom: 1px solid rgba(148, 132, 87, 0.28);
    box-shadow: 0 8px 24px rgba(73, 57, 27, 0.08);
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
    display: none;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    align-items: center;
    gap: 7px 4px;
    min-height: calc(76px + env(safe-area-inset-bottom));
    box-sizing: border-box;
    padding: 8px 10px max(10px, env(safe-area-inset-bottom));
    background: color-mix(in srgb, var(--reader-popup-bg) 94%, transparent);
    border-top: 1px solid rgba(148, 132, 87, 0.35);
    border-radius: 10px 10px 0 0;
    box-shadow: 0 -8px 24px rgba(73, 57, 27, 0.08);
  }
  .reader-mobile-progress-panel {
    display: grid;
    grid-column: 1 / -1;
    grid-template-columns: minmax(62px, 76px) minmax(0, 1fr) minmax(62px, 76px);
    align-items: center;
    gap: 8px;
    min-height: 84px;
    padding: 7px;
    background: color-mix(in srgb, var(--reader-popup-bg) 96%, transparent);
    border: 1px solid rgba(148, 132, 87, 0.28);
    border-radius: 8px;
    box-shadow: 0 -8px 24px rgba(73, 57, 27, 0.08);
  }
  .mobile-progress-slider-row {
    display: grid;
    grid-column: 1 / -1;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    gap: 10px;
    min-width: 0;
    padding: 0 3px;
    color: #8d8270;
    font-size: 12px;
  }
  .mobile-progress-slider {
    width: 100%;
    min-width: 0;
    accent-color: #409eff;
  }
  .reader-shell.mobile-chrome-visible .reader-mobile-top,
  .reader-shell.mobile-chrome-visible .reader-mobile-bottom {
    display: grid;
  }
  .mobile-chapter-step {
    min-width: 0;
    min-height: 38px;
    color: #24201b;
    background: var(--reader-popup-bg);
    border: 1px solid rgba(148, 132, 87, 0.3);
    border-radius: 6px;
    font-size: 13px;
  }
  .mobile-chapter-step:disabled {
    color: #a09282;
    opacity: 0.55;
  }
  .mobile-chapter-progress {
    display: grid;
    min-width: 0;
    justify-items: center;
    gap: 2px;
    padding: 0;
    background: transparent;
    border: 0;
    cursor: pointer;
  }
  .mobile-chapter-progress strong,
  .mobile-chapter-progress span {
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .mobile-chapter-progress strong {
    color: #121212;
    font-size: 14px;
  }
  .mobile-chapter-progress span {
    color: #756c5a;
    font-size: 12px;
  }
  .reader-mobile-bottom > .mobile-tool-button {
    display: none;
  }
  .reader-shell.mobile-chrome-visible .reader-mobile-bottom > .mobile-tool-button {
    display: grid;
  }
  .mobile-tool-button {
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
    background: var(--reader-popup-bg);
    border: 1px solid #eee4c9;
    border-radius: 8px;
    font-size: 13px;
  }
  .mobile-more-item.active {
    color: #0f5451;
    border-color: #0f5451;
    background: color-mix(in srgb, var(--reader-popup-bg) 90%, #fff1bc);
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
