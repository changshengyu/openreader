<template>
  <div
    class="app-shell"
    :class="{ 'mobile-shell': isMobileShell, 'mobile-nav-open': mobileNavigationVisible }"
    @touchstart="handleTouchStart"
    @touchmove="handleTouchMove"
    @touchend="handleTouchEnd"
  >
    <div v-if="offline" class="app-offline">网络已断开，部分同步能力会在恢复连接后继续。</div>

    <aside class="app-sidebar" :style="mobileNavigationStyle">
      <div class="app-brand" @click="goHome">
        <div class="app-brand-mark">阅</div>
        <div>
          <div class="app-brand-title-row">
            <div class="app-brand-title">阅读</div>
            <button class="app-version-text" type="button" @click.stop="refreshHealthInfo(true)">{{ appVersionLabel }}</button>
          </div>
          <div class="app-brand-subtitle">清风不识字，何故乱翻书</div>
        </div>
      </div>

      <div class="app-shell-search">
        <el-input
          v-model="quickSearch"
          :placeholder="quickSearchPlaceholder"
          clearable
          @keyup.enter="goSearch"
          @clear="clearShelfSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
      </div>

      <section class="app-search-setting">
        <p class="app-nav-title">搜索设置</p>
        <el-select v-model="sidebarSearchType" size="small" class="setting-select">
          <el-option label="多源搜索" value="all" />
          <el-option label="分组搜索" value="group" />
          <el-option label="单源搜索" value="single" />
        </el-select>
        <el-select
          v-if="sidebarSearchType === 'group'"
          v-model="sidebarSearchGroup"
          size="small"
          class="setting-select"
          placeholder="全部分组"
        >
          <el-option v-for="group in sidebarSourceGroups" :key="group.value" :label="`${group.label} (${group.count})`" :value="group.value" />
        </el-select>
        <el-select
          v-if="sidebarSearchType === 'single'"
          v-model="sidebarSourceId"
          size="small"
          class="setting-select"
          filterable
          placeholder="选择书源"
        >
          <el-option v-for="source in sidebarEnabledSources" :key="source.id" :label="source.name" :value="source.id" />
        </el-select>
        <el-select v-model="sidebarConcurrent" size="small" class="setting-select">
          <el-option v-for="count in concurrentOptions" :key="count" :label="`${count}并发线程`" :value="count" />
        </el-select>
        <div class="sidebar-search-actions">
          <button type="button" @click="goSearchRoute('remote')">
            <el-icon><Search /></el-icon>
            <span>书源搜索</span>
          </button>
          <button type="button" @click="goSearchRoute('local')">
            <el-icon><FolderOpened /></el-icon>
            <span>本地书籍</span>
          </button>
        </div>
      </section>

      <section class="sidebar-recent">
        <div class="sidebar-recent-title">
          <p class="app-nav-title">最近阅读</p>
          <button v-if="recentBook" type="button" @click="clearRecentBook">清除</button>
        </div>
        <button
          class="sidebar-recent-book"
          type="button"
          :disabled="!recentBook"
          @click="openRecentBook"
        >
          <span>{{ recentBook?.title || '暂无阅读记录' }}</span>
          <small>{{ recentBook ? recentSubTitle(recentBook) : '打开一本书后会显示在这里' }}</small>
        </button>
      </section>

      <nav class="app-nav">
        <section v-for="section in navSections" :key="section.title" class="app-nav-section">
          <p class="app-nav-title">{{ section.title }}</p>
          <button
            v-for="item in section.items"
            :key="item.key"
            class="app-nav-item"
            :class="{ active: isNavActive(item) }"
            type="button"
            @click="runNavAction(item)"
          >
            <span>{{ item.label }}</span>
          </button>
        </section>
      </nav>

      <div class="sidebar-bottom-icons" aria-label="侧栏快捷入口">
        <a class="sidebar-bottom-icon" href="https://github.com/changshengyu/openreader" target="_blank" rel="noopener noreferrer" aria-label="GitHub">
          <svg viewBox="0 0 24 24" aria-hidden="true">
            <path
              fill="currentColor"
              d="M12 .5a12 12 0 0 0-3.8 23.4c.6.1.8-.3.8-.6v-2.1c-3.3.7-4-1.4-4-1.4-.5-1.4-1.3-1.8-1.3-1.8-1.1-.8.1-.8.1-.8 1.2.1 1.9 1.3 1.9 1.3 1.1 1.9 2.9 1.3 3.6 1 .1-.8.4-1.3.8-1.6-2.7-.3-5.5-1.3-5.5-5.9 0-1.3.5-2.4 1.2-3.2-.1-.3-.5-1.6.1-3.2 0 0 1-.3 3.3 1.2a11.5 11.5 0 0 1 6 0C17.5 4.6 18.5 5 18.5 5c.6 1.6.2 2.9.1 3.2.8.8 1.2 1.9 1.2 3.2 0 4.6-2.8 5.6-5.5 5.9.5.4.9 1.1.9 2.2v3.2c0 .3.2.7.8.6A12 12 0 0 0 12 .5Z"
            />
          </svg>
        </a>
        <button class="sidebar-bottom-icon theme-toggle" type="button" :class="{ night: isNightTheme }" :aria-label="isNightTheme ? '切换日间主题' : '切换夜间主题'" @click="toggleNightTheme">
          <el-icon v-if="isNightTheme"><Sunny /></el-icon>
          <el-icon v-else><Moon /></el-icon>
        </button>
      </div>
    </aside>

    <div class="app-workspace" @click="closeMobileNavigation">
      <main class="app-content">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  FolderOpened,
  Moon,
  Search,
  Sunny,
} from '@element-plus/icons-vue'
import { useUserStore } from '../stores/user'
import { useOverlayStore } from '../stores/overlay'
import { useBookshelfStore } from '../stores/bookshelf'
import { useReaderStore } from '../stores/reader'
import { usePreferencesStore } from '../stores/preferences'
import { useSync } from '../composables/useSync'
import { clearCache, getCacheStats } from '../api/cache'
import { listSources } from '../api/sources'
import api from '../api/client'
import { newestBookProgress, progressUpdatedAt } from '../utils/bookOrder'
import { clearCurrentUserBrowserChapterCache, currentUserBrowserChapterCacheStats } from '../utils/bookChapterCache'
import { readerRouteQueryFromBook } from '../utils/readerRoute'
import { currentViewportWidth, shouldUseMiniInterface } from '../utils/responsive'
import { currentUserScope } from '../utils/authScope'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const overlay = useOverlayStore()
const bookshelf = useBookshelfStore()
const reader = useReaderStore()
const preferences = usePreferencesStore()
const quickSearch = ref('')
const offline = ref(false)
const windowWidth = ref(currentViewportWidth())
const mobileNavigationVisible = ref(false)
const touchStart = ref(null)
const touchMoveX = ref(0)
const cacheStats = ref({})
const browserCacheStats = ref({})
const healthInfo = ref(null)
const recentSuppressedAt = ref(readRecentSuppressedAt())
const cacheLoading = ref(false)
const cacheClearing = ref(false)
const browserCacheClearing = ref(false)
const MOBILE_NAV_TRIGGER = 72
const FOREGROUND_REFRESH_INTERVAL = 30000
let lastForegroundRefreshAt = 0
const { connected: syncConnected, connect, disconnect } = useSync()

const navSections = computed(() => [
  {
    title: '后端设定',
    items: [
      { key: 'backendStatus', label: syncConnected.value ? '同步在线' : '同步未连接', action: refreshShelfData },
    ],
  },
  {
    title: '书源设置',
    items: [
      { key: 'sources', label: '书源管理', route: 'sources' },
      { key: 'discover', label: '探索书源', route: 'discover' },
      { key: 'importSources', label: '导入书源', route: 'sources', query: { action: 'import' } },
      { key: 'remoteSources', label: '远程书源', route: 'sources', query: { panel: 'remote' } },
      { key: 'sourceHealth', label: '失效书源', route: 'sources', query: { action: 'health' } },
      { key: 'sourceDebug', label: '调试书源', route: 'sources', query: { action: 'debug' } },
    ],
  },
  {
    title: '书架设置',
    items: [
      { key: 'home', label: '书架', route: 'home' },
      { key: 'bookManage', label: '书籍管理', action: () => overlay.openBookManage() },
      { key: 'bookGroup', label: '分组管理', action: () => overlay.openBookGroup('manage') },
      { key: 'importBook', label: '导入书籍', action: () => overlay.openImportBook() },
      { key: 'localStore', label: '浏览书仓', action: () => overlay.openLocalStore() },
      { key: 'refreshShelf', label: '刷新书架', action: refreshShelfData },
    ],
  },
  {
    title: '用户空间',
    items: [
      { key: 'account', label: userStore.profile?.username || '默认', route: 'settings', panel: 'account' },
      { key: 'backupConfig', label: '备份用户配置', action: () => overlay.openBackup() },
      { key: 'syncConfig', label: '同步用户配置', action: syncUserConfig },
      { key: 'userManage', label: '加载用户空间', action: () => overlay.openUserManage() },
    ],
  },
  {
    title: 'WebDAV',
    items: [
      { key: 'webdav', label: '文件管理', action: () => overlay.openWebDAV() },
      { key: 'backup', label: '保存备份', action: () => overlay.openBackup() },
    ],
  },
  {
    title: cacheSectionTitle.value,
    items: [
      { key: 'cacheStats', label: '刷新缓存统计', action: loadCacheStats },
      { key: 'clearCache', label: cacheClearing.value ? '清理中' : clearServerChapterCacheLabel.value, action: clearSystemCache },
      { key: 'clearBrowserCache', label: browserCacheClearing.value ? '清理中' : clearBrowserChapterCacheLabel.value, action: clearBrowserChapterCache },
    ],
  },
  {
    title: '其它',
    items: [
      { key: 'rss', label: 'RSS', action: () => overlay.openRSS() },
      { key: 'replaceRules', label: '替换规则', action: () => overlay.openReplaceRules() },
    ],
  },
])

const concurrentOptions = [8, 16, 32, 60]
const sidebarSources = ref([])
const sidebarSearchType = computed({
  get: () => preferences.search.searchType,
  set: value => preferences.setSearchConfig({ searchType: value }),
})
const sidebarSearchGroup = computed({
  get: () => preferences.search.group,
  set: value => preferences.setSearchConfig({ group: value }),
})
const sidebarSourceId = computed({
  get: () => preferences.search.sourceId,
  set: value => preferences.setSearchConfig({ sourceId: value }),
})
const sidebarConcurrent = computed({
  get: () => preferences.search.concurrent,
  set: value => preferences.setSearchConfig({ concurrent: value }),
})
const sidebarEnabledSources = computed(() => sidebarSources.value.filter(source => source.enabled))
const sidebarSourceGroups = computed(() => {
  const groups = new Map()
  for (const source of sidebarEnabledSources.value) {
    const name = source.group || '默认分组'
    groups.set(name, (groups.get(name) || 0) + 1)
  }
  return [...groups.entries()].map(([label, count]) => ({ label, value: label, count }))
})
const cacheSectionTitle = computed(() => {
  const size = Number(cacheStats.value?.size || 0) + Number(browserCacheStats.value?.size || 0)
  return size ? `本地缓存 ${formatSize(size)}` : '本地缓存'
})
const clearServerChapterCacheLabel = computed(() => {
  const size = Number(cacheStats.value?.size || 0)
  return size ? `清空服务器缓存 ${formatSize(size)}` : '清空服务器缓存'
})
const clearBrowserChapterCacheLabel = computed(() => {
  const size = Number(browserCacheStats.value?.size || 0)
  return size ? `清空浏览器缓存 ${formatSize(size)}` : '清空浏览器缓存'
})
const isNightTheme = computed(() => reader.theme === 'dark' || reader.theme === 'black')
const appVersionLabel = computed(() => {
  const version = String(healthInfo.value?.version || '').trim()
  const commit = shortCommit(healthInfo.value?.commit)
  if (version && !['dev', 'unknown'].includes(version)) return version
  return commit || 'dev'
})
const isMobileShell = computed(() => shouldUseMiniInterface(reader.pageMode, windowWidth.value))
const mobileNavigationWidth = computed(() => {
  return 260
})
const mobileNavigationStyle = computed(() => {
  const width = mobileNavigationWidth.value
  const base = { '--mobile-nav-width': `${width}px` }
  if (!isMobileShell.value || !touchMoveX.value) return base
  if (!mobileNavigationVisible.value && touchMoveX.value > 0 && touchMoveX.value <= width) {
    return { ...base, transform: `translateX(${touchMoveX.value - width}px)` }
  }
  if (mobileNavigationVisible.value && touchMoveX.value < 0 && touchMoveX.value >= -width) {
    return { ...base, transform: `translateX(${touchMoveX.value}px)` }
  }
  return base
})
const recentBook = computed(() => {
  const rows = (Array.isArray(bookshelf.books) ? bookshelf.books : [])
    .filter(book => {
      const progress = progressForBook(book)
      return hasReadingProgress(progress) && progressUpdatedAt(progress) > recentSuppressedAt.value
    })
    .sort((a, b) => {
      const aProgress = progressForBook(a)
      const bProgress = progressForBook(b)
      const aTime = progressUpdatedAt(aProgress)
      const bTime = progressUpdatedAt(bProgress)
      if (aTime !== bTime) return bTime - aTime
      return Number(b?.id || 0) - Number(a?.id || 0)
    })
  return rows[0] || null
})
const quickSearchPlaceholder = computed(() => route.name === 'home' ? '搜索书架' : '搜索书籍')

function goHome() {
  router.push({ name: 'home' })
}

function runNavAction(item) {
  if (item.action) {
    item.action()
    if (isMobileShell.value) mobileNavigationVisible.value = false
    return
  }
  if (item.route) {
    const query = navRouteQuery(item)
    router.push({ name: item.route, query })
    if (isMobileShell.value) mobileNavigationVisible.value = false
  }
}

function navRouteQuery(item) {
  if (item.key === 'search') return searchRouteQuery()
  if (item.key === 'localSearch') return localSearchRouteQuery()
  return item.query || (item.panel ? { panel: item.panel } : {})
}

function isNavActive(item) {
  if (!item.route || route.name !== item.route) return false
  if (item.key === 'sources') return !route.query.panel && !route.query.action
  if (item.query) {
    return Object.entries(item.query).every(([key, value]) => String(route.query[key] || '') === String(value))
  }
  if (!item.panel) return true
  return String(route.query.panel || 'account') === item.panel
}

function goSearch() {
  const keyword = quickSearch.value.trim()
  if (route.name === 'home') {
    setShelfSearch(keyword)
    return
  }
  const query = searchRouteQuery(keyword)
  if (!keyword) {
    router.push({ name: 'search', query })
    return
  }
  router.push({ name: 'search', query })
}

function goSearchRoute(mode = 'remote') {
  const keyword = quickSearch.value.trim()
  const query = mode === 'local' ? localSearchRouteQuery(keyword) : searchRouteQuery(keyword)
  router.push({ name: 'search', query })
  if (isMobileShell.value) mobileNavigationVisible.value = false
}

function searchRouteQuery(keyword = '') {
  const query = {}
  if (keyword) query.q = keyword
  query.searchType = sidebarSearchType.value
  query.concurrent = sidebarConcurrent.value
  if (sidebarSearchType.value === 'group' && sidebarSearchGroup.value) query.group = sidebarSearchGroup.value
  if (sidebarSearchType.value === 'single' && sidebarSourceId.value) query.sourceId = sidebarSourceId.value
  return query
}

function localSearchRouteQuery(keyword = quickSearch.value.trim()) {
  const query = { mode: 'local' }
  if (keyword) query.q = keyword
  return query
}

function setShelfSearch(keyword) {
  const nextQuery = { ...route.query }
  if (keyword) {
    nextQuery.shelfQ = keyword
  } else {
    delete nextQuery.shelfQ
  }
  router.replace({ name: 'home', query: nextQuery })
}

function clearShelfSearch() {
  if (route.name === 'home' && route.query.shelfQ !== undefined) {
    const { shelfQ, ...query } = route.query
    router.replace({ name: 'home', query })
    return
  }
  if (route.name === 'search' && route.query.q !== undefined) {
    const { q, ...query } = route.query
    router.replace({ name: 'search', query })
  }
}

async function loadSidebarSources() {
  try {
    const { data } = await listSources()
    sidebarSources.value = Array.isArray(data) ? data : []
    if (!sidebarSearchGroup.value && sidebarSourceGroups.value.length) sidebarSearchGroup.value = sidebarSourceGroups.value[0].value
    if (!sidebarSourceId.value && sidebarEnabledSources.value.length) sidebarSourceId.value = sidebarEnabledSources.value[0].id
  } catch {
    sidebarSources.value = []
  }
}

async function loadCacheStats() {
  cacheLoading.value = true
  const [serverResult, browserResult] = await Promise.allSettled([
    getCacheStats(),
    currentUserBrowserChapterCacheStats(),
  ])
  if (serverResult.status === 'fulfilled') {
    cacheStats.value = serverResult.value?.data || {}
  } else {
    cacheStats.value = {}
  }
  if (browserResult.status === 'fulfilled') {
    browserCacheStats.value = browserResult.value || {}
  } else {
    browserCacheStats.value = {}
  }
  cacheLoading.value = false
}

async function syncUserConfig() {
  try {
    await Promise.all([
      userStore.loadMe(),
      preferences.loadPreferences(),
      reader.loadReaderSettings(),
      bookshelf.loadCategories({ force: true }),
      bookshelf.loadBooks({ force: true, all: true }),
      loadCacheStats(),
    ])
    ElMessage.success('用户配置已同步')
  } catch (err) {
    ElMessage.error(readError(err, '同步用户配置失败'))
  }
}

async function refreshHealthInfo(showMessage = false) {
  try {
    const { data } = await api.get('/health')
    healthInfo.value = data || {}
    if (showMessage) {
      const commit = shortCommit(data?.commit) || '-'
      const buildText = data?.buildDate && data.buildDate !== 'unknown' ? `构建 ${data.buildDate}` : '开发构建'
      ElMessage.success(`${buildText} · ${commit}`)
    }
  } catch (err) {
    if (showMessage) ElMessage.error(readError(err, '读取版本信息失败'))
  }
}

function shortCommit(value) {
  if (!value || value === 'unknown') return ''
  return String(value).slice(0, 12)
}

async function clearSystemCache() {
  try {
    await ElMessageBox.confirm('确定清理服务器章节缓存吗？清理后阅读时会重新加载远程章节内容。', '清理缓存', { type: 'warning' })
    cacheClearing.value = true
    const { data } = await clearCache()
    ElMessage.success(`已清理 ${data.clearedFiles || 0} 个文件，释放 ${formatSize(data.clearedSize || 0)}`)
    await loadCacheStats()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '清理缓存失败'))
  } finally {
    cacheClearing.value = false
  }
}

async function clearBrowserChapterCache() {
  try {
    await ElMessageBox.confirm('确定清理当前用户的浏览器章节缓存吗？清理后本机阅读时会重新加载章节内容。', '清理浏览器缓存', { type: 'warning' })
    browserCacheClearing.value = true
    const removed = await clearCurrentUserBrowserChapterCache()
    ElMessage.success(`已清理浏览器章节缓存 ${removed} 章`)
    await loadCacheStats()
  } catch (err) {
    if (err === 'cancel' || err === 'close') return
    ElMessage.error(readError(err, '清理浏览器缓存失败'))
  } finally {
    browserCacheClearing.value = false
  }
}

function formatSize(bytes) {
  const value = Number(bytes || 0)
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  if (value < 1024 * 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB`
  return `${(value / 1024 / 1024 / 1024).toFixed(2)} GB`
}

function openRecentBook() {
  if (!recentBook.value) return
  router.push({ name: 'reader', params: { id: recentBook.value.id }, query: readerRouteQuery(recentBook.value) })
}

function clearRecentBook() {
  const progress = recentBook.value ? progressForBook(recentBook.value) : null
  const nextValue = Math.max(Date.now(), progressUpdatedAt(progress))
  recentSuppressedAt.value = nextValue
  writeRecentSuppressedAt(nextValue)
}

function toggleNightTheme() {
  reader.setTheme(isNightTheme.value ? 'parchment' : 'dark')
}

function recentSubTitle(book) {
  const progress = progressForBook(book)
  if (progress?.chapterTitle) return progress.chapterTitle
  if (Number.isInteger(progress?.chapterIndex)) return `第 ${progress.chapterIndex + 1} 章`
  return book.lastChapter || book.author || '继续阅读'
}

function readerRouteQuery(book) {
  return readerRouteQueryFromBook(book, progressForBook(book))
}

function progressForBook(book) {
  return newestBookProgress(book, reader.progressByBook)
}

function hasReadingProgress(progress) {
  if (!progress?.bookId) return false
  if (progressUpdatedAt(progress) > 0) return true
  if (progress.chapterTitle) return true
  if (Number.isInteger(progress.chapterIndex) && progress.chapterIndex >= 0) return true
  return Number(progress.offset || 0) > 0 ||
    Number(progress.percent || 0) > 0 ||
    Number(progress.chapterPercent || 0) > 0
}

function recentSuppressedCacheKey() {
  return `openreader:readingRecentClearedAt:${currentUserScope()}`
}

function readRecentSuppressedAt() {
  try {
    return Number(window.localStorage?.getItem(recentSuppressedCacheKey()) || 0)
  } catch {
    return 0
  }
}

function writeRecentSuppressedAt(value) {
  try {
    window.localStorage?.setItem(recentSuppressedCacheKey(), String(Number(value || 0)))
  } catch {
    // Ignore private-mode storage errors; the in-memory value still hides it for this session.
  }
}

async function refreshShelfData() {
  await Promise.all([bookshelf.loadCategories({ force: true }), bookshelf.loadBooks({ force: true, all: true })]).catch(() => {})
  router.push({ name: 'home' })
}

function refreshShelfInForeground() {
  if (!userStore.token) return
  if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return
  const now = Date.now()
  if (now - lastForegroundRefreshAt < FOREGROUND_REFRESH_INTERVAL) return
  lastForegroundRefreshAt = now
  Promise.all([
    bookshelf.loadCategories(),
    bookshelf.loadBooks({ all: true }),
  ]).catch(() => {})
}

function handleVisibilityChange() {
  if (document.visibilityState === 'visible') {
    connect()
    refreshShelfInForeground()
  }
}

function setOffline() {
  offline.value = true
}

function setOnline() {
  offline.value = false
}

function updateViewportFlags() {
  windowWidth.value = currentViewportWidth()
}

function handleTouchStart(event) {
  if (!isMobileShell.value || event.touches?.length !== 1) return
  const touch = event.touches[0]
  if (touch.clientY <= 20 || touch.clientY >= window.innerHeight - 20) {
    touchStart.value = null
    return
  }
  if (touch.clientX <= 20 || touch.clientX >= window.innerWidth - 20) {
    touchStart.value = null
    return
  }
  touchStart.value = { x: touch.clientX, y: touch.clientY }
  touchMoveX.value = 0
}

function handleTouchMove(event) {
  if (!isMobileShell.value || !touchStart.value || event.touches?.length !== 1) return
  const touch = event.touches[0]
  const moveX = touch.clientX - touchStart.value.x
  const moveY = touch.clientY - touchStart.value.y
  if (Math.abs(moveY) > Math.abs(moveX)) {
    touchMoveX.value = 0
    return
  }
  const width = mobileNavigationWidth.value
  if ((!mobileNavigationVisible.value && moveX > 0 && moveX <= width) || (mobileNavigationVisible.value && moveX < 0 && moveX >= -width)) {
    event.preventDefault()
    event.stopPropagation()
    touchMoveX.value = moveX
  }
}

function handleTouchEnd() {
  if (!isMobileShell.value) return
  if (touchMoveX.value > MOBILE_NAV_TRIGGER) mobileNavigationVisible.value = true
  if (touchMoveX.value < -MOBILE_NAV_TRIGGER) mobileNavigationVisible.value = false
  touchStart.value = null
  touchMoveX.value = 0
}

function closeMobileNavigation() {
  if (isMobileShell.value && mobileNavigationVisible.value) {
    mobileNavigationVisible.value = false
  }
}

function toggleMobileNavigation() {
  if (isMobileShell.value) {
    mobileNavigationVisible.value = !mobileNavigationVisible.value
  }
}

watch(
  () => userStore.token,
  (token) => {
    recentSuppressedAt.value = readRecentSuppressedAt()
    if (token) {
      connect()
    } else {
      disconnect()
    }
  },
  { immediate: true },
)

watch(
  () => [route.name, route.query.q, route.query.shelfQ],
  ([name, value, shelfQ]) => {
    if (name === 'search') {
      quickSearch.value = typeof value === 'string' ? value : ''
    } else if (name === 'home') {
      quickSearch.value = typeof shelfQ === 'string' ? shelfQ : ''
    } else if (name !== 'home') {
      quickSearch.value = ''
    }
  },
  { immediate: true },
)

onMounted(() => {
  updateViewportFlags()
  window.addEventListener('offline', setOffline)
  window.addEventListener('online', setOnline)
  window.addEventListener('resize', updateViewportFlags)
  window.addEventListener('orientationchange', updateViewportFlags)
  window.addEventListener('focus', refreshShelfInForeground)
  document.addEventListener('visibilitychange', handleVisibilityChange)
  window.addEventListener('openreader:toggle-mobile-nav', toggleMobileNavigation)
  offline.value = !navigator.onLine
  if (userStore.token && !userStore.profile) {
    userStore.loadMe().catch(() => {})
  }
  if (userStore.token && !bookshelf.books.length) {
    Promise.all([bookshelf.loadCategories(), bookshelf.loadBooks({ all: true })]).catch(() => {})
  }
  if (userStore.token) loadSidebarSources()
  if (userStore.token) loadCacheStats()
  refreshHealthInfo(false)
})

onBeforeUnmount(() => {
  window.removeEventListener('offline', setOffline)
  window.removeEventListener('online', setOnline)
  window.removeEventListener('resize', updateViewportFlags)
  window.removeEventListener('orientationchange', updateViewportFlags)
  window.removeEventListener('focus', refreshShelfInForeground)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  window.removeEventListener('openreader:toggle-mobile-nav', toggleMobileNavigation)
})
</script>

<style scoped>
.app-shell {
  min-height: 100vh;
  width: 100%;
  max-width: 100%;
  box-sizing: border-box;
  overflow-x: hidden;
  background:
    linear-gradient(180deg, rgba(255, 253, 248, 0.86), rgba(245, 241, 232, 0.92)),
    var(--app-bg);
}

.app-offline {
  position: fixed;
  z-index: 50;
  top: 12px;
  left: 50%;
  transform: translateX(-50%);
  padding: 8px 14px;
  color: #fff9ed;
  background: var(--app-warning);
  border-radius: 999px;
  box-shadow: var(--app-shadow-md);
  font-size: 13px;
}

.app-sidebar {
  position: fixed;
  inset: 0 auto 0 0;
  z-index: 30;
  display: block;
  width: var(--app-sidebar-width);
  box-sizing: border-box;
  height: 100vh;
  height: 100dvh;
  padding: 48px 36px 88px;
  overflow-y: auto;
  color: #24201b;
  background: #f7f7f7;
  border-right: 1px solid #eee;
  scrollbar-width: none;
}

.app-sidebar::-webkit-scrollbar {
  display: none;
}

:global(html.dark-reader) .app-shell {
  background: #181715;
}

:global(html.dark-reader) .app-sidebar {
  color: var(--app-text);
  background: #222;
  border-right-color: #303030;
}

:global(html.dark-reader) .app-brand-title {
  color: #bbb;
}

:global(html.dark-reader) .app-version-text {
  color: #7f766c;
}

:global(html.dark-reader) .app-brand-subtitle,
:global(html.dark-reader) .app-nav-title {
  color: #7f766c;
}

:global(html.dark-reader) .setting-select :deep(.el-select__wrapper),
:global(html.dark-reader) .app-shell-search :deep(.el-input__wrapper),
:global(html.dark-reader) .sidebar-search-actions button,
:global(html.dark-reader) .app-nav-item,
:global(html.dark-reader) .sidebar-recent-book,
:global(html.dark-reader) .sidebar-bottom-icon {
  color: #aaa;
  background: #2a2927;
  border-color: #39352f;
  box-shadow: none;
}

:global(html.dark-reader) .app-nav-item:hover,
:global(html.dark-reader) .app-nav-item.active,
:global(html.dark-reader) .sidebar-search-actions button:hover,
:global(html.dark-reader) .sidebar-bottom-icon:hover {
  color: var(--app-primary-strong);
  background: #243b37;
  border-color: #365b55;
}

:global(html.dark-reader) .sidebar-recent-book {
  color: #d39d3d;
}

.app-brand {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0;
  cursor: pointer;
}

.app-brand-title-row {
  display: flex;
  min-width: 0;
  align-items: baseline;
  gap: 12px;
}

.app-brand-mark {
  display: inline-grid;
  width: 0;
  height: 0;
  place-items: center;
  flex: 0 0 0;
  overflow: hidden;
  color: transparent;
  background: transparent;
  border-radius: 0;
  font-weight: 800;
}

.app-brand-title {
  color: #26394a;
  font-size: 24px;
  font-weight: 800;
  line-height: 1.2;
  word-break: keep-all;
}

.app-version-text {
  min-width: 0;
  padding: 0;
  color: #b5b5b5;
  background: transparent;
  border: 0;
  cursor: pointer;
  font-size: 14px;
  font-weight: 500;
  line-height: 1.2;
  overflow-wrap: anywhere;
  text-align: left;
}

.app-version-text:hover {
  color: #8f8f8f;
}

.app-brand-subtitle {
  margin-top: 18px;
  color: #b5b5b5;
  font-size: 16px;
  line-height: 1.35;
  white-space: normal;
  word-break: keep-all;
}

.app-shell-search {
  margin: 24px 0 28px;
}

.app-shell-search :deep(.el-input__wrapper) {
  min-height: 28px;
  border-radius: 14px;
  box-shadow: 0 0 0 1px #e6e6e6 inset;
}

.app-search-setting {
  display: grid;
  gap: 12px;
  margin: 0 0 36px;
}

.setting-select {
  width: 100%;
}

.setting-select :deep(.el-select__wrapper) {
  min-height: 28px;
  background: #fffdf8;
  border-radius: 4px;
  box-shadow: 0 0 0 1px #e6e6e6 inset;
}

.sidebar-search-actions {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 12px;
}

.sidebar-search-actions button {
  display: flex;
  min-width: 0;
  min-height: 32px;
  align-items: center;
  justify-content: center;
  gap: 5px;
  padding: 7px 8px;
  color: #9aa1aa;
  background: #fafafa;
  border: 1px solid #e6e9ef;
  border-radius: 4px;
  cursor: pointer;
}

.sidebar-search-actions button:hover {
  color: #1f6feb;
  background: #fff;
}

.sidebar-search-actions span {
  min-width: 0;
  overflow: visible;
  overflow-wrap: anywhere;
  font-size: 12px;
  line-height: 1.25;
  text-overflow: clip;
  white-space: normal;
  word-break: break-word;
}

.sidebar-recent {
  display: grid;
  gap: 18px;
  margin: 0 0 36px;
}

.sidebar-recent-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.sidebar-recent-title .app-nav-title {
  margin: 0;
}

.sidebar-recent-title button {
  flex: 0 0 auto;
  padding: 0;
  color: #b5b5b5;
  background: transparent;
  border: 0;
  font: inherit;
  font-size: 13px;
  line-height: 1.35;
  cursor: pointer;
}

.sidebar-recent-title button:hover {
  color: var(--app-accent);
}

.sidebar-recent-book {
  display: grid;
  gap: 4px;
  min-width: 0;
  width: fit-content;
  max-width: 100%;
  padding: 9px 12px;
  color: #d39d3d;
  background: #fffaf0;
  border: 1px solid #fde6bd;
  border-radius: 4px;
  cursor: pointer;
  text-align: left;
}

.sidebar-recent-book:disabled {
  cursor: default;
  opacity: 0.7;
}

.sidebar-recent-book span,
.sidebar-recent-book small {
  overflow: visible;
  text-overflow: clip;
  white-space: normal;
  word-break: break-word;
}

.sidebar-recent-book span {
  font-size: 13px;
  font-weight: 700;
}

.sidebar-recent-book small {
  color: #766a5c;
  font-size: 12px;
}

.app-nav {
  display: grid;
  gap: 36px;
  padding: 0 0 20px;
}

.sidebar-bottom-icons {
  position: fixed;
  bottom: 30px;
  left: 36px;
  z-index: 31;
  display: flex;
  width: calc(var(--app-sidebar-width) - 72px);
  align-items: center;
  justify-content: space-between;
  pointer-events: none;
}

.app-nav-section {
  display: grid;
  grid-template-columns: repeat(2, minmax(78px, 1fr));
  gap: 12px 14px;
}

.app-nav-title {
  grid-column: 1 / -1;
  margin: 0 0 4px;
  color: #b5b5b5;
  font-size: 14px;
  font-weight: 500;
  line-height: 1.35;
  letter-spacing: 0;
  white-space: normal;
  word-break: keep-all;
}

.app-nav-item {
  display: flex;
  width: fit-content;
  max-width: 100%;
  min-height: 32px;
  align-items: center;
  justify-content: center;
  padding: 7px 12px;
  color: #9aa1aa;
  background: #fafafa;
  border: 1px solid #e6e9ef;
  border-radius: 4px;
  cursor: pointer;
  text-align: center;
}

.app-nav-item:hover {
  color: #1f6feb;
  background: #fff;
}

.app-nav-item.active {
  color: #1f6feb;
  background: #eef6ff;
  border-color: #bfdbfe;
}

.app-nav-item span {
  min-width: 0;
  overflow: visible;
  font-size: 13px;
  line-height: 1.3;
  overflow-wrap: anywhere;
  text-overflow: clip;
  white-space: normal;
  word-break: break-word;
}

.sidebar-bottom-icon {
  display: inline-grid;
  width: 36px;
  height: 36px;
  place-items: center;
  color: #24201b;
  background: transparent;
  border: 0;
  border-radius: 50%;
  cursor: pointer;
  pointer-events: auto;
}

.sidebar-bottom-icon svg {
  width: 32px;
  height: 32px;
}

.theme-toggle {
  color: #f7f7f7;
  background: #1f1f1f;
}

.theme-toggle.night {
  color: #121212;
  background: #f4e4c5;
}

.app-workspace {
  min-height: 100vh;
  width: 100%;
  max-width: 100%;
  min-width: 0;
  box-sizing: border-box;
  padding-left: var(--app-sidebar-width);
  overflow-x: hidden;
}

.app-content {
  min-height: 100vh;
  width: 100%;
  max-width: 100%;
  min-width: 0;
  box-sizing: border-box;
  overflow-x: hidden;
}

.app-shell.mobile-shell {
  min-height: 100vh;
  min-height: 100dvh;
  overflow-x: hidden;
}

.app-shell.mobile-shell .app-sidebar {
  position: fixed;
  inset: 0 auto 0 0;
  width: var(--mobile-nav-width, 72vw);
  height: 100vh;
  height: 100dvh;
  overflow-y: auto;
  padding: max(48px, env(safe-area-inset-top)) 36px 88px;
  scrollbar-width: none;
  box-shadow: 12px 0 28px rgba(36, 32, 27, 0.08);
  transform: translateX(calc(-1 * var(--mobile-nav-width, 72vw)));
  transition: transform 0.3s;
  will-change: transform;
}

.app-shell.mobile-shell .app-workspace {
  width: 100%;
  max-width: 100%;
  min-width: 0;
  padding-left: 0;
  overflow-x: hidden;
}

.app-shell.mobile-shell .app-content {
  min-height: 100vh;
}

.app-shell.mobile-shell .app-brand {
  display: flex;
  justify-items: initial;
  gap: 12px;
  padding: 8px 0 18px;
}

.app-shell.mobile-shell .app-brand > div:last-child {
  display: block;
}

.app-shell.mobile-shell .app-shell-search {
  display: block;
  margin: 0 0 18px;
}

.app-shell.mobile-shell .app-search-setting {
  margin: 0 0 22px;
  gap: 10px;
}

.app-shell.mobile-shell .sidebar-search-actions {
  gap: 10px 12px;
}

.app-shell.mobile-shell .sidebar-search-actions button {
  min-height: 38px;
  padding: 8px;
  background: #fffdf8;
  border-color: #e4d9c8;
}

.app-shell.mobile-shell .sidebar-search-actions span {
  font-size: 13px;
}

.app-shell.mobile-shell .app-brand-mark {
  display: none;
}

.app-shell.mobile-shell .app-nav {
  gap: 36px;
  padding: 0 0 20px;
}

.app-shell.mobile-shell .app-nav-section {
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 12px;
}

.app-shell.mobile-shell .app-nav-title {
  margin: 0 0 8px;
  overflow: visible;
  color: #a09282;
  font-size: 14px;
  line-height: 1.3;
  text-align: left;
  white-space: normal;
  word-break: keep-all;
}

.app-shell.mobile-shell .app-nav-item {
  display: flex;
  width: 100%;
  min-width: 0;
  min-height: 34px;
  height: auto;
  align-items: center;
  justify-content: center;
  gap: 5px;
  margin: 0;
  padding: 6px 8px;
  background: #fffdf8;
  border: 1px solid #e4d9c8;
  border-radius: 4px;
}

.app-shell.mobile-shell .app-nav-item span {
  overflow: visible;
  overflow-wrap: anywhere;
  font-size: 12px;
  line-height: 1.25;
  text-overflow: clip;
  white-space: normal;
  word-break: break-word;
}

.app-shell.mobile-shell .sidebar-recent {
  margin: 0 0 8px;
}

.app-shell.mobile-shell .sidebar-recent .app-nav-title,
.app-shell.mobile-shell .sidebar-recent-book small {
  display: block;
}

.app-shell.mobile-shell .sidebar-recent-book {
  min-height: auto;
  padding: 9px 10px;
  place-items: initial;
  text-align: left;
}

.app-shell.mobile-shell .sidebar-recent-book span {
  display: block;
  font-size: 13px;
  line-height: 1.25;
  white-space: normal;
}

.app-shell.mobile-shell .sidebar-bottom-icons {
  position: fixed;
  right: auto;
  bottom: 30px;
  left: 36px;
  display: flex;
  width: calc(var(--mobile-nav-width, 72vw) - 72px);
  align-items: center;
  justify-content: space-between;
  pointer-events: none;
}

.app-shell.mobile-shell .sidebar-bottom-icon {
  background: transparent;
  border: 0;
  box-shadow: none;
}

.app-shell.mobile-shell .sidebar-bottom-icon svg {
  width: 22px;
  height: 22px;
}

.app-shell.mobile-shell .theme-toggle {
  color: #f7f7f7;
  background: #1f1f1f;
  border-color: #1f1f1f;
}

.app-shell.mobile-shell .theme-toggle.night {
  color: #121212;
  background: #f4e4c5;
  border-color: #f4e4c5;
}

.app-shell.mobile-shell.mobile-nav-open .app-sidebar {
  transform: translateX(0);
}

.app-shell.mobile-shell.mobile-nav-open .app-workspace {
  transform: none;
}

</style>
