<template>
  <div
    class="app-shell"
    :class="{ 'mobile-shell': isMobileShell, 'mobile-nav-open': mobileNavigationVisible }"
    @touchstart="handleTouchStart"
    @touchmove="handleTouchMove"
    @touchend="handleTouchEnd"
    @touchcancel="handleTouchCancel"
  >
    <div v-if="offline" class="app-offline">网络已断开，部分同步能力会在恢复连接后继续。</div>

    <aside class="app-sidebar" :style="mobileNavigationStyle">
      <div class="app-sidebar-scroll">
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
          placeholder="搜索书籍"
          clearable
          @keyup.enter="goSearch"
          @clear="clearSearchQuery"
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
          <el-option v-for="count in concurrentOptions" :key="count" :label="concurrentLabel(count)" :value="count" />
        </el-select>
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
        <section
          v-for="section in navSections"
          :key="section.key"
          :ref="node => captureSidebarSection(section, node)"
          class="app-nav-section"
          :class="{ 'is-compat-focused': compatFocusedSection === section.key }"
          :data-sidebar-section="section.key"
        >
          <p class="app-nav-title">
            <span>{{ section.title }}</span>
            <button
              v-if="section.headingAction"
              class="app-nav-title-action"
              type="button"
              @click="section.headingAction.action"
            >{{ section.headingAction.label }}</button>
          </p>
          <button
            v-for="item in section.items"
            :key="item.key"
            :ref="node => captureExploreTrigger(item, node)"
            class="app-nav-item"
            :class="{ active: isNavActive(item) }"
            type="button"
            @click="runNavAction(item)"
          >
            <span>{{ item.label }}</span>
          </button>
        </section>
      </nav>
      </div>

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

    <div v-show="exploreChooserVisible" class="explore-popover-backdrop" aria-hidden="true" @click="closeExploreChooser" />
    <ExploreWorkspacePopover
      v-show="exploreChooserVisible"
      :is-mobile="isMobileShell"
      :style="exploreChooserStyle"
      @close="closeExploreChooser"
      @selected="handleExploreSelected"
    />

    <div class="app-workspace" @click="closeMobileNavigation">
      <main class="app-content">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Moon, Search, Sunny } from '@element-plus/icons-vue'
import { useUserStore } from '../stores/user'
import { useOverlayStore } from '../stores/overlay'
import { useBookshelfStore } from '../stores/bookshelf'
import { useReaderStore } from '../stores/reader'
import { usePreferencesStore } from '../stores/preferences'
import { useIndexWorkspaceStore } from '../stores/indexWorkspace'
import { useAppCacheManagement } from '../composables/useAppCacheManagement'
import { useAppMobileNavigation } from '../composables/useAppMobileNavigation'
import { useAppRecentReading } from '../composables/useAppRecentReading'
import { useAppSidebarSearch } from '../composables/useAppSidebarSearch'
import { useSync } from '../composables/useSync'
import ExploreWorkspacePopover from '../components/workspace/ExploreWorkspacePopover.vue'
import { clearCache, getCacheStats } from '../api/cache'
import { listSources } from '../api/sources'
import api from '../api/client'
import { cacheFirstRequest, networkFirstRequest, removeBrowserCache } from '../utils/browserCache'
import { clearBrowserLocalCacheGroup, currentBrowserLocalCacheStats } from '../utils/localCacheStats'
import { currentViewportWidth, shouldUseMiniInterface } from '../utils/responsive'
import { currentUserScope } from '../utils/authScope'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const overlay = useOverlayStore()
const bookshelf = useBookshelfStore()
const reader = useReaderStore()
const preferences = usePreferencesStore()
const workspace = useIndexWorkspaceStore()
const exploreChooserVisible = ref(false)
const exploreChooserStyle = ref({})
const exploreTrigger = ref(null)
const compatFocusedSection = ref('')
const sidebarSectionNodes = new Map()
const offline = ref(false)
const healthInfo = ref(null)
const routeBookInfoLoadingId = ref(null)
const routeBookInfoOpenedKey = ref('')
const FOREGROUND_REFRESH_INTERVAL = 30000
let lastForegroundRefreshAt = 0
let compatibilityFocusTimer
const { connected: syncConnected, connect, disconnect } = useSync()
const {
  visible: mobileNavigationVisible,
  isMobile: isMobileShell,
  navigationStyle: mobileNavigationStyle,
  updateViewport: updateViewportFlags,
  handleTouchStart,
  handleTouchMove,
  handleTouchEnd,
  handleTouchCancel,
  close: closeMobileNavigation,
  toggle: toggleMobileNavigation,
} = useAppMobileNavigation({
  currentViewportWidth,
  getViewportWidth: () => window.innerWidth,
  getViewportHeight: () => window.innerHeight,
  getPageMode: () => reader.pageMode,
  shouldUseMiniInterface,
  now: () => Date.now(),
})
const {
  clearingServer: cacheClearing,
  sectionTitle: cacheSectionTitle,
  clearServerLabel: clearServerChapterCacheLabel,
  browserNavItems: browserCacheNavItems,
  loadStats: loadCacheStats,
  clearServer: clearSystemCache,
} = useAppCacheManagement({
  getServerStats: getCacheStats,
  getBrowserStats: currentBrowserLocalCacheStats,
  clearServerCache: clearCache,
  clearBrowserGroup: clearBrowserLocalCacheGroup,
  confirm: (...args) => ElMessageBox.confirm(...args),
  onSuccess: message => ElMessage.success(message),
  onInfo: message => ElMessage.info(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})
const {
  recentBook,
  open: openRecentBook,
  clear: clearRecentBook,
  subtitle: recentSubTitle,
  refreshScope: refreshRecentReadingScope,
} = useAppRecentReading({
  getBooks: () => bookshelf.books,
  getProgressByBook: () => reader.progressByBook,
  getUserScope: currentUserScope,
  getStorage: () => window.localStorage,
  now: () => Date.now(),
  navigate: route => router.push(route),
})

const navSections = computed(() => [
  {
    key: 'backend',
    title: '后端设定',
    items: [
      { key: 'backendStatus', label: syncConnected.value ? '同步在线' : '同步未连接', action: refreshShelfData },
    ],
  },
  {
    key: 'sources',
    title: '书源设置',
    items: [
      { key: 'sources', label: '书源管理', action: () => overlay.openSourceManage('manage') },
      { key: 'discover', label: '探索书源', action: beginWorkspaceExplore, closeMobile: true },
      { key: 'importSources', label: '导入书源', action: () => overlay.openSourceManage('import') },
      { key: 'remoteSources', label: '远程书源', action: () => overlay.openSourceManage('remote') },
      { key: 'sourceHealth', label: '失效书源', action: () => overlay.openSourceManage('health') },
      { key: 'sourceDebug', label: '调试书源', action: () => overlay.openSourceManage('debug') },
    ],
  },
  {
    key: 'bookshelf',
    title: '书架设置',
    items: [
      { key: 'home', label: '书架', action: goHome, closeMobile: true },
      { key: 'bookManage', label: '书籍管理', action: () => overlay.openBookManage() },
      { key: 'bookGroup', label: '分组管理', action: () => overlay.openBookGroup('manage') },
      { key: 'importBook', label: '导入书籍', action: () => overlay.openImportBook() },
      { key: 'localStore', label: '浏览书仓', action: () => overlay.openLocalStore() },
      { key: 'refreshShelf', label: '刷新书架', action: refreshShelfData },
    ],
  },
  {
    key: 'account',
    title: userStore.profile?.username ? `用户空间 · ${userStore.profile.username}` : '用户空间',
    headingAction: userStore.profile ? { label: '注销', action: logoutUser } : null,
    items: [
      { key: 'backupConfig', label: '备份用户配置', action: backupUserConfig },
      { key: 'syncConfig', label: '同步用户配置', action: syncUserConfig },
      ...(userStore.profile?.role === 'admin'
        ? [{ key: 'userManage', label: '管理用户空间', action: () => overlay.openUserManage() }]
        : []),
    ],
  },
  {
    key: 'webdav',
    title: 'WebDAV',
    items: [
      { key: 'webdav', label: '文件管理', action: () => overlay.openWebDAV() },
      { key: 'backup', label: '保存备份', action: () => overlay.openBackup() },
    ],
  },
  {
    key: 'cache',
    title: cacheSectionTitle.value,
    items: [
      { key: 'cacheStats', label: '刷新缓存统计', action: loadCacheStats },
      { key: 'clearCache', label: cacheClearing.value ? '清理中' : clearServerChapterCacheLabel.value, action: clearSystemCache },
      ...browserCacheNavItems.value,
    ],
  },
  {
    key: 'other',
    title: '其它',
    items: [
      { key: 'rss', label: 'RSS', action: () => overlay.openRSS() },
      { key: 'replaceRules', label: '替换规则', action: () => overlay.openReplaceRules() },
    ],
  },
])

const {
  quickSearch,
  concurrentOptions,
  searchType: sidebarSearchType,
  searchGroup: sidebarSearchGroup,
  sourceId: sidebarSourceId,
  concurrent: sidebarConcurrent,
  enabledSources: sidebarEnabledSources,
  sourceGroups: sidebarSourceGroups,
  concurrentLabel,
  goSearch,
  clearSearchQuery,
  loadSources: loadSidebarSources,
  handleSourcesUpdated,
} = useAppSidebarSearch({
  preferences,
  route,
  router,
  listSources,
  cacheFirstRequest,
  networkFirstRequest,
  removeBrowserCache,
  getUserScope: currentUserScope,
  onWarning: message => ElMessage.warning(message),
  onWorkspaceSearch: beginWorkspaceSearch,
  afterNavigate: () => {
    if (isMobileShell.value) mobileNavigationVisible.value = false
  },
  afterSourcesUpdated: () => loadCacheStats(),
})
const isNightTheme = computed(() => reader.themeType === 'night')
const appVersionLabel = computed(() => {
  const version = String(healthInfo.value?.version || '').trim()
  const commit = shortCommit(healthInfo.value?.commit)
  if (version && !['dev', 'unknown'].includes(version)) return version
  return commit || 'dev'
})
function goHome() {
  workspace.backToShelf()
  if (route.name !== 'home') {
    router.push({ name: 'home' })
    return
  }
  if (route.query.workspace !== undefined) {
    router.replace({ name: 'home', query: withoutWorkspaceQuery(route.query) })
  }
}

function runNavAction(item) {
  if (item.action) {
    item.action()
    if (isMobileShell.value && item.closeMobile) mobileNavigationVisible.value = false
    return
  }
  if (item.route) {
    const query = navRouteQuery(item)
    router.push({ name: item.route, query })
    if (isMobileShell.value && item.closeMobile) mobileNavigationVisible.value = false
  }
}

function beginWorkspaceSearch(query = {}) {
  workspace.beginSearch({
    keyword: query.q || '',
    mode: query.mode,
    searchType: query.searchType,
    group: query.group,
    sourceId: query.sourceId,
    concurrent: query.concurrent,
  })
  if (route.name !== 'home') router.push({ name: 'home' })
}

function beginWorkspaceExplore() {
  workspace.requestExplore()
  if (route.name !== 'home') router.push({ name: 'home' })
}

function captureExploreTrigger(item, node) {
  if (item?.key === 'discover' && node) exploreTrigger.value = node
}

function captureSidebarSection(section, node) {
  if (!section?.key) return
  if (node) {
    sidebarSectionNodes.set(section.key, node)
    return
  }
  sidebarSectionNodes.delete(section.key)
}

function focusSidebarSection(sectionKey) {
  if (!['account', 'cache'].includes(sectionKey)) return
  if (isMobileShell.value) mobileNavigationVisible.value = true
  compatFocusedSection.value = sectionKey
  clearTimeout(compatibilityFocusTimer)
  nextTick(() => {
    sidebarSectionNodes.get(sectionKey)?.scrollIntoView?.({ block: 'center', behavior: 'smooth' })
  })
  compatibilityFocusTimer = setTimeout(() => {
    if (compatFocusedSection.value === sectionKey) compatFocusedSection.value = ''
  }, 2200)
}

function openExploreChooser() {
  nextTick(() => {
    positionExploreChooser()
    exploreChooserVisible.value = true
  })
}

function closeExploreChooser() {
  exploreChooserVisible.value = false
}

function handleExploreSelected() {
  closeExploreChooser()
  if (route.name !== 'home') router.push({ name: 'home' })
}

function positionExploreChooser() {
  if (isMobileShell.value) {
    exploreChooserStyle.value = {}
    return
  }
  const trigger = exploreTrigger.value
  const rect = trigger?.getBoundingClientRect?.()
  const top = rect
    ? Math.max(16, Math.min(rect.top, window.innerHeight - 440))
    : 48
  const left = rect
    ? Math.min(rect.right + 12, window.innerWidth - 544)
    : 272
  exploreChooserStyle.value = {
    top: `${top}px`,
    left: `${Math.max(12, left)}px`,
  }
}

function withoutWorkspaceQuery(query = {}) {
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
    ...rest
  } = query
  return rest
}

function navRouteQuery(item) {
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

async function backupUserConfig() {
  const confirmed = await ElMessageBox.confirm(
    '确认要备份当前账号的阅读配置、书架设置、搜索设置和自定义配置方案吗？',
    '备份用户配置',
    { type: 'warning' },
  ).catch(() => false)
  if (!confirmed) return
  const [readerSettings, shelfSettings, searchSettings] = await Promise.all([
    reader.saveReaderSettings(),
    preferences.savePreference('shelf'),
    preferences.savePreference('search'),
  ])
  if (!readerSettings || !shelfSettings || !searchSettings) {
    ElMessage.error('备份用户配置失败')
    return
  }
  ElMessage.success('用户配置已备份')
}

async function syncUserConfig() {
  const confirmed = await ElMessageBox.confirm(
    '确认要恢复当前账号的阅读配置、书架设置、搜索设置和自定义配置方案吗？',
    '同步用户配置',
    { type: 'warning' },
  ).catch(() => false)
  if (!confirmed) return
  const [readerSettings] = await Promise.all([
    reader.loadReaderSettings(),
    preferences.loadPreferences(),
  ])
  if (!readerSettings || Object.values(preferences.syncError).some(Boolean)) {
    ElMessage.error(reader.settingsSyncError || Object.values(preferences.syncError).find(Boolean) || '同步用户配置失败')
    return
  }
  const results = await Promise.allSettled([
    userStore.loadMe(),
    loadShelfForShell({ force: true, all: true }),
    loadCacheStats(),
  ])
  const failed = results.find(result => result.status === 'rejected')
  if (failed) {
    ElMessage.warning(readError(failed.reason, '用户配置已同步，部分工作台数据刷新失败'))
    return
  }
  const shelfResult = results[1]
  if (shelfResult.value?.categoryError) {
    ElMessage.warning(readError(shelfResult.value.categoryError, '用户配置已同步，分组刷新失败'))
    return
  }
  ElMessage.success('用户配置已同步')
}

function logoutUser() {
  userStore.logout()
  router.replace({ name: 'login' })
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

function toggleNightTheme() {
  reader.setTheme(isNightTheme.value ? 'parchment' : 'dark')
}

async function refreshShelfData() {
  try {
    const result = await loadShelfForShell({ force: true, all: true })
    if (result.categoryError) ElMessage.warning(readError(result.categoryError, '书架已刷新，分组刷新失败'))
  } catch (err) {
    ElMessage.error(readError(err, '刷新书架失败'))
  }
  router.push({ name: 'home' })
}

async function openRouteBookInfoOverlay() {
  const rawId = route.query.bookInfo
  const id = Number(Array.isArray(rawId) ? rawId[0] : rawId)
  if (!Number.isFinite(id) || id <= 0 || route.name === 'reader') return
  const key = `${route.name || ''}:${id}`
  if (routeBookInfoOpenedKey.value === key && overlay.bookInfoVisible) return
  routeBookInfoLoadingId.value = id
  try {
    const { data } = await api.get(`/books/${id}`)
    if (!data?.id) return
    const shelfBook = bookshelf.books.find(book => Number(book.id) === Number(data.id))
    const mergedBook = shelfBook ? mergeShelfBookForRoute(shelfBook, data) : data
    if (shelfBook) bookshelf.upsertBook(mergedBook)
    overlay.openBookInfo(mergedBook, {
      progress: routeBookProgress(mergedBook)?.percent || 0,
    })
    routeBookInfoOpenedKey.value = key
  } catch (error) {
    ElMessage.error(readError(error, '加载书籍信息失败'))
    if ([403, 404].includes(Number(error?.response?.status))) {
      clearRouteBookInfoOverlayIntent()
    }
  } finally {
    routeBookInfoLoadingId.value = null
  }
}

function clearRouteBookInfoOverlayIntent() {
  if (overlay.bookInfoVisible || !route.query.bookInfo || route.name === 'reader' || route.name === 'remote-reader') return
  routeBookInfoOpenedKey.value = ''
  const { bookInfo: _bookInfo, ...query } = route.query
  router.replace({ name: 'home', query })
}

function openRouteSourceManageOverlay() {
  if (route.name === 'reader' || route.query.overlay !== 'sources') return
  overlay.openSourceManage(route.query.sourceAction)
}

function clearRouteSourceManageOverlayIntent() {
  if (overlay.sourceManageVisible || route.query.overlay !== 'sources') return
  const { overlay: _overlay, sourceAction: _sourceAction, ...query } = route.query
  router.replace({ name: 'home', query })
}

function openRouteWorkspaceOperationOverlay() {
  if (route.name === 'reader') return
  switch (route.query.overlay) {
    case 'local-store':
      overlay.openLocalStore()
      break
    case 'webdav':
      overlay.openWebDAV()
      break
    case 'backup':
      overlay.openBackup()
      break
    case 'replace-rules':
      overlay.openReplaceRules()
      break
    case 'rss':
      overlay.openRSS()
      break
    case 'user-manage':
      overlay.openUserManage()
      break
  }
}

function clearRouteWorkspaceOperationOverlayIntent() {
  const overlayName = String(route.query.overlay || '')
  if (!overlayName || overlayName === 'sources') return
  const visibleByOverlay = {
    'local-store': overlay.localStoreVisible,
    webdav: overlay.webdavVisible,
    backup: overlay.backupVisible,
    'replace-rules': overlay.replaceRulesVisible,
    rss: overlay.rssVisible,
    'user-manage': overlay.userManageVisible,
  }
  if (visibleByOverlay[overlayName] === undefined || visibleByOverlay[overlayName]) return
  const { overlay: _overlay, ...query } = route.query
  router.replace({ name: 'home', query })
}

function consumeWorkspaceFocusIntent() {
  if (route.name === 'reader') return
  const sectionKey = String(route.query.workspaceFocus || '')
  if (!['account', 'cache'].includes(sectionKey)) return
  focusSidebarSection(sectionKey)
  const { workspaceFocus: _workspaceFocus, ...query } = route.query
  router.replace({ name: 'home', query })
}

function consumeWorkspaceNoticeIntent() {
  if (route.name === 'reader' || String(route.query.workspaceNotice || '') !== 'reader-settings') return
  ElMessage.info('阅读设置已迁移至书籍阅读页，请打开书籍后使用阅读器中的设置。')
  const { workspaceNotice: _workspaceNotice, ...query } = route.query
  router.replace({ name: 'home', query })
}

function routeBookProgress(book) {
  return reader.progressByBook?.[book?.id] || book?.progress || null
}

function mergeShelfBookForRoute(current, incoming) {
  return {
    ...current,
    ...incoming,
    progress: incoming?.progress || current?.progress,
    categories: incoming?.categories || current?.categories,
    categoryIds: incoming?.categoryIds || current?.categoryIds,
  }
}

function refreshShelfInForeground() {
  if (!userStore.token) return
  if (typeof document !== 'undefined' && document.visibilityState === 'hidden') return
  if (syncConnected.value && bookshelf.books.length) return
  const now = Date.now()
  if (now - lastForegroundRefreshAt < FOREGROUND_REFRESH_INTERVAL) return
  lastForegroundRefreshAt = now
  loadShelfForShell({ all: true }).catch(() => {})
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

watch(
  () => userStore.token,
  (token) => {
    refreshRecentReadingScope()
    if (token) {
      connect()
    } else {
      disconnect()
    }
  },
  { immediate: true },
)

watch(
  () => [route.name, route.query.overlay, userStore.token],
  () => {
    if (!userStore.token) return
    openRouteWorkspaceOperationOverlay()
  },
  { immediate: true },
)

watch(
  () => [route.name, route.query.workspaceFocus, route.query.workspaceNotice, userStore.token],
  () => {
    if (!userStore.token) return
    consumeWorkspaceFocusIntent()
    consumeWorkspaceNoticeIntent()
  },
  { immediate: true },
)

watch(
  () => workspace.exploreChooserRevision,
  (revision) => {
    if (!revision) return
    openExploreChooser()
  },
)

watch(
  () => [route.name, route.query.bookInfo, userStore.token, bookshelf.books.length],
  () => {
    if (!userStore.token) return
    openRouteBookInfoOverlay()
  },
  { immediate: true },
)

watch(
  () => overlay.bookInfoVisible,
  () => clearRouteBookInfoOverlayIntent(),
)

watch(
  () => [route.name, route.query.overlay, route.query.sourceAction, userStore.token],
  () => {
    if (!userStore.token) return
    openRouteSourceManageOverlay()
  },
  { immediate: true },
)

watch(
  () => overlay.sourceManageVisible,
  () => clearRouteSourceManageOverlayIntent(),
)

watch(
  () => [
    overlay.localStoreVisible,
    overlay.webdavVisible,
    overlay.backupVisible,
    overlay.replaceRulesVisible,
    overlay.rssVisible,
    overlay.userManageVisible,
  ],
  () => clearRouteWorkspaceOperationOverlayIntent(),
)

onMounted(() => {
  updateViewportFlags()
  window.addEventListener('offline', setOffline)
  window.addEventListener('online', setOnline)
  window.addEventListener('resize', updateViewportFlags)
  window.addEventListener('orientationchange', updateViewportFlags)
  window.addEventListener('resize', positionExploreChooser)
  window.addEventListener('orientationchange', positionExploreChooser)
  window.addEventListener('focus', refreshShelfInForeground)
  document.addEventListener('visibilitychange', handleVisibilityChange)
  window.addEventListener('openreader:toggle-mobile-nav', toggleMobileNavigation)
  window.addEventListener('openreader:sources-update', handleSourcesUpdated)
  offline.value = !navigator.onLine
  if (userStore.token && !userStore.profile) {
    userStore.loadMe().catch(() => {})
  }
  if (userStore.token && !bookshelf.books.length) {
    loadShelfForShell({ all: true }).catch(() => {})
  }
  if (userStore.token) loadSidebarSources()
  if (userStore.token) loadCacheStats()
  refreshHealthInfo(false)
})

onBeforeUnmount(() => {
  clearTimeout(compatibilityFocusTimer)
  window.removeEventListener('offline', setOffline)
  window.removeEventListener('online', setOnline)
  window.removeEventListener('resize', updateViewportFlags)
  window.removeEventListener('orientationchange', updateViewportFlags)
  window.removeEventListener('resize', positionExploreChooser)
  window.removeEventListener('orientationchange', positionExploreChooser)
  window.removeEventListener('focus', refreshShelfInForeground)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  window.removeEventListener('openreader:toggle-mobile-nav', toggleMobileNavigation)
  window.removeEventListener('openreader:sources-update', handleSourcesUpdated)
})

async function loadShelfForShell(options = {}) {
  const [categoryResult, booksResult] = await Promise.allSettled([
    bookshelf.loadCategories(options),
    bookshelf.loadBooks(options),
  ])
  if (booksResult.status === 'rejected') throw booksResult.reason
  return {
    books: booksResult.value,
    categories: categoryResult.status === 'fulfilled' ? categoryResult.value : bookshelf.categories,
    categoryError: categoryResult.status === 'rejected' ? categoryResult.reason : null,
  }
}

function readError(err, fallback) {
  return err?.response?.data?.error || err?.response?.data?.message || err?.message || fallback
}
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
  padding: 0;
  overflow: hidden;
  color: #24201b;
  background: #f7f7f7;
  border-right: 1px solid #eee;
  scrollbar-width: none;
}

.app-sidebar-scroll {
  width: 100%;
  height: 100%;
  box-sizing: border-box;
  padding: 48px 36px 88px;
  overflow-x: hidden;
  overflow-y: auto;
  scrollbar-width: none;
}

.app-sidebar-scroll::-webkit-scrollbar {
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

:global(html.dark-reader) .app-nav-title-action {
  color: #b5b5b5;
}

:global(html.dark-reader) .setting-select :deep(.el-select__wrapper),
:global(html.dark-reader) .app-shell-search :deep(.el-input__wrapper),
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
  display: block;
  padding: 0 0 20px;
}

.sidebar-bottom-icons {
  position: absolute;
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
  display: block;
  margin-bottom: 36px;
  scroll-margin-block: 24px;
}

.app-nav-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin: 0 0 4px;
  color: #b5b5b5;
  font-size: 14px;
  font-weight: 500;
  line-height: 1.35;
  letter-spacing: 0;
  white-space: normal;
  word-break: keep-all;
}

.app-nav-title-action {
  flex: 0 0 auto;
  padding: 0;
  color: #9d9d9d;
  background: transparent;
  border: 0;
  cursor: pointer;
  font: inherit;
  font-size: 13px;
  line-height: 1.35;
}

.app-nav-title-action:hover {
  color: var(--app-accent);
}

.app-nav-section.is-compat-focused {
  animation: sidebar-compat-focus 2.2s ease-out both;
}

@keyframes sidebar-compat-focus {
  0%, 55% {
    background: color-mix(in srgb, var(--app-accent) 12%, transparent);
    box-shadow: 0 0 0 4px color-mix(in srgb, var(--app-accent) 9%, transparent);
  }
  100% {
    background: transparent;
    box-shadow: none;
  }
}

.app-nav-item {
  display: inline-flex;
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
  margin-right: 15px;
  margin-bottom: 15px;
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

.explore-popover-backdrop {
  position: fixed;
  inset: 0;
  z-index: 34;
  background: transparent;
}

.explore-workspace-popover {
  position: fixed;
  z-index: 35;
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
  max-width: 86vw;
  min-width: 0;
  box-sizing: border-box;
  height: 100vh;
  height: 100dvh;
  overflow: hidden;
  padding: 0;
  scrollbar-width: none;
  box-shadow: 12px 0 28px rgba(36, 32, 27, 0.08);
  margin-left: calc(-1 * var(--mobile-nav-width, 72vw));
  transition: margin-left 0.3s;
  will-change: margin-left;
}

.app-shell.mobile-shell .app-sidebar-scroll {
  padding: max(48px, env(safe-area-inset-top)) 36px 88px;
}

.app-shell.mobile-shell .app-workspace {
  width: 100%;
  max-width: 100%;
  min-width: 0;
  padding-left: 0;
  overflow-x: hidden;
}

.app-shell.mobile-shell .explore-popover-backdrop {
  z-index: 34;
  background: rgba(36, 32, 27, 0.18);
}

.app-shell.mobile-shell .explore-workspace-popover {
  inset: 0;
  z-index: 35;
  width: 100vw;
  height: 100vh;
  height: 100dvh;
}

.app-shell.mobile-shell .app-content {
  min-height: 100vh;
}

.app-shell.mobile-shell .app-brand {
  display: flex;
  min-width: 0;
  max-width: 100%;
  justify-items: initial;
  gap: 12px;
  padding: 8px 0 18px;
}

.app-shell.mobile-shell .app-brand > div:last-child {
  display: block;
}

.app-shell.mobile-shell .app-shell-search {
  display: block;
  min-width: 0;
  max-width: 100%;
  margin: 0 0 18px;
}

.app-shell.mobile-shell .app-search-setting {
  min-width: 0;
  max-width: 100%;
  margin: 0 0 22px;
  gap: 10px;
}

.app-shell.mobile-shell .app-brand-mark {
  display: none;
}

.app-shell.mobile-shell .app-nav {
  display: block;
  padding: 0 0 20px;
}

.app-shell.mobile-shell .app-nav-section {
  display: block;
  margin-bottom: 36px;
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
  display: inline-flex;
  width: fit-content;
  min-width: 0;
  min-height: 32px;
  height: auto;
  align-items: center;
  justify-content: center;
  margin-right: 15px;
  margin-bottom: 15px;
  padding: 7px 12px;
  background: #fffdf8;
  border: 1px solid #e4d9c8;
  border-radius: 4px;
}

.app-shell.mobile-shell .app-nav-item span {
  overflow: hidden;
  font-size: 12px;
  line-height: 1.3;
  text-overflow: ellipsis;
  white-space: nowrap;
  word-break: keep-all;
}

.app-shell.mobile-shell .sidebar-recent {
  min-width: 0;
  max-width: 100%;
  margin: 0 0 8px;
}

.app-shell.mobile-shell .sidebar-recent .app-nav-title,
.app-shell.mobile-shell .sidebar-recent-book small {
  display: block;
}

.app-shell.mobile-shell .sidebar-recent-book {
  width: 100%;
  max-width: 100%;
  min-width: 0;
  min-height: auto;
  padding: 9px 10px;
  place-items: initial;
  text-align: left;
}

.app-shell.mobile-shell .sidebar-recent-book span {
  display: block;
  min-width: 0;
  font-size: 13px;
  line-height: 1.25;
  overflow-wrap: anywhere;
  white-space: normal;
  word-break: break-word;
}

.app-shell.mobile-shell .sidebar-bottom-icons {
  position: absolute;
  right: auto;
  bottom: 30px;
  left: 36px;
  display: flex;
  width: min(calc(var(--mobile-nav-width, 72vw) - 72px), calc(86vw - 72px));
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  pointer-events: none;
  transform: translateX(calc(-1 * var(--mobile-nav-drag-offset, 0px)));
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
  margin-left: 0;
}

.app-shell.mobile-shell.mobile-nav-open .app-workspace {
  transform: none;
}

</style>
