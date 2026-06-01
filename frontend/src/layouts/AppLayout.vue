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
          <div class="app-brand-title">阅读</div>
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
        <el-select v-model="sidebarSearchType" size="small" class="setting-select" @change="saveSearchSetting">
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
          @change="saveSearchSetting"
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
          @change="saveSearchSetting"
        >
          <el-option v-for="source in sidebarEnabledSources" :key="source.id" :label="source.name" :value="source.id" />
        </el-select>
        <el-select v-model="sidebarConcurrent" size="small" class="setting-select" @change="saveSearchSetting">
          <el-option v-for="count in concurrentOptions" :key="count" :label="`${count}并发线程`" :value="count" />
        </el-select>
      </section>

      <section class="sidebar-recent">
        <p class="app-nav-title">最近阅读</p>
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
            <el-icon><component :is="item.icon" /></el-icon>
            <span>{{ item.label }}</span>
          </button>
        </section>
      </nav>

      <div class="app-sidebar-footer">
        <div class="sync-pill" :class="{ connected: syncConnected }">
          <span class="sync-dot" />
          {{ syncConnected ? '实时同步在线' : '同步未连接' }}
        </div>

        <el-dropdown trigger="click" placement="top-start">
          <button class="user-card" type="button">
            <el-avatar :size="34">{{ userInitial }}</el-avatar>
            <span class="user-card-main">
              <strong>{{ userStore.profile?.username || '用户' }}</strong>
              <small>账户与设置</small>
            </span>
            <el-icon><ArrowUp /></el-icon>
          </button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item @click="goRoute('settings')">
                <el-icon><Setting /></el-icon>
                设置
              </el-dropdown-item>
              <el-dropdown-item divided @click="handleLogout">
                <el-icon><SwitchButton /></el-icon>
                退出登录
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
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
  ArrowUp,
  Box,
  Compass,
  Connection,
  Delete,
  Edit,
  Files,
  FolderOpened,
  Link as LinkIcon,
  Notebook,
  Operation,
  Refresh,
  Search,
  Setting,
  SwitchButton,
  Upload,
} from '@element-plus/icons-vue'
import { useUserStore } from '../stores/user'
import { useOverlayStore } from '../stores/overlay'
import { useBookshelfStore } from '../stores/bookshelf'
import { useReaderStore } from '../stores/reader'
import { usePreferencesStore } from '../stores/preferences'
import { useSync } from '../composables/useSync'
import { clearCache, getCacheStats } from '../api/cache'
import { listSources } from '../api/sources'
import { compareRecentBook, newestBookProgress } from '../utils/bookOrder'
import { readerRouteQueryFromBook } from '../utils/readerRoute'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const overlay = useOverlayStore()
const bookshelf = useBookshelfStore()
const reader = useReaderStore()
const preferences = usePreferencesStore()
const quickSearch = ref('')
const offline = ref(false)
const windowWidth = ref(typeof window === 'undefined' ? 1280 : window.innerWidth)
const mobileNavigationVisible = ref(false)
const touchStart = ref(null)
const touchMoveX = ref(0)
const cacheStats = ref({})
const cacheLoading = ref(false)
const cacheClearing = ref(false)
const MINI_INTERFACE_MAX_WIDTH = 750
const MOBILE_NAV_EDGE = 48
const MOBILE_NAV_TRIGGER = 72
const FOREGROUND_REFRESH_INTERVAL = 5000
let lastForegroundRefreshAt = 0
const { connected: syncConnected, connect, disconnect } = useSync()

const navSections = computed(() => [
  {
    title: '搜索入口',
    items: [
      { key: 'search', label: '书源搜索', icon: Search, route: 'search' },
      { key: 'localSearch', label: '本地书籍', icon: FolderOpened, route: 'search', query: { mode: 'local' } },
      { key: 'discover', label: '书海', icon: Compass, route: 'discover' },
    ],
  },
  {
    title: '后端设定',
    items: [
      { key: 'backendStatus', label: syncConnected.value ? '同步在线' : '同步未连接', icon: Connection, action: refreshShelfData },
    ],
  },
  {
    title: '书源设置',
    items: [
      { key: 'sources', label: '书源管理', icon: Connection, route: 'sources' },
      { key: 'remoteSources', label: '远程书源', icon: LinkIcon, route: 'sources', query: { panel: 'remote' } },
      { key: 'sourceHealth', label: '失效书源', icon: Operation, route: 'sources', query: { action: 'health' } },
    ],
  },
  {
    title: '书架设置',
    items: [
      { key: 'home', label: '书架', icon: Notebook, route: 'home' },
      { key: 'bookManage', label: '书籍管理', icon: Files, action: () => overlay.openBookManage() },
      { key: 'bookGroup', label: '分组管理', icon: Box, action: () => overlay.openBookGroup('manage') },
      { key: 'importBook', label: '导入书籍', icon: Upload, action: () => overlay.openImportBook() },
      { key: 'localStore', label: '浏览书仓', icon: FolderOpened, action: () => overlay.openLocalStore() },
      { key: 'refreshShelf', label: '刷新书架', icon: Refresh, action: refreshShelfData },
      { key: 'replaceRules', label: '替换规则', icon: Edit, action: () => overlay.openReplaceRules() },
    ],
  },
  {
    title: '用户空间',
    items: [
      { key: 'userManage', label: '用户管理', icon: Operation, action: () => overlay.openUserManage() },
      { key: 'settings', label: '设置', icon: Setting, route: 'settings', panel: 'account' },
    ],
  },
  {
    title: 'WebDAV',
    items: [
      { key: 'webdav', label: '文件管理', icon: Upload, action: () => overlay.openWebDAV() },
      { key: 'backup', label: '保存备份', icon: Refresh, action: () => overlay.openBackup() },
    ],
  },
  {
    title: '本地缓存',
    items: [
      { key: 'cacheStats', label: cacheStatsLabel.value, icon: Files, action: loadCacheStats },
      { key: 'clearCache', label: cacheClearing.value ? '清理中' : '清空章节缓存', icon: Delete, action: clearSystemCache },
    ],
  },
  {
    title: 'RSS',
    items: [
      { key: 'rss', label: 'RSS', icon: Connection, action: () => overlay.openRSS() },
    ],
  },
])

const userInitial = computed(() => (userStore.profile?.username || '?').slice(0, 1).toUpperCase())
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
const cacheStatsLabel = computed(() => {
  if (cacheLoading.value) return '缓存读取中'
  const size = formatSize(cacheStats.value?.size || 0)
  const chapters = Number(cacheStats.value?.cachedChapters || 0)
  return `章节缓存 ${size}${chapters ? ` / ${chapters}章` : ''}`
})
const isMobileShell = computed(() => reader.pageMode === 'mobile' || windowWidth.value <= MINI_INTERFACE_MAX_WIDTH)
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
  const rows = [...(Array.isArray(bookshelf.books) ? bookshelf.books : [])]
  rows.sort((a, b) => compareRecentBook(a, b, reader.progressByBook))
  return rows[0] || null
})
const quickSearchPlaceholder = computed(() => route.name === 'home' ? '搜索书架' : '搜索书籍')

function goHome() {
  router.push({ name: 'home' })
}

function goRoute(name) {
  router.push({ name })
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

function saveSearchSetting() {
  preferences.savePreference('search').catch(() => {})
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
  try {
    const { data } = await getCacheStats()
    cacheStats.value = data || {}
  } catch {
    cacheStats.value = {}
  } finally {
    cacheLoading.value = false
  }
}

async function clearSystemCache() {
  try {
    await ElMessageBox.confirm('确定清理全部章节缓存吗？清理后阅读时会重新加载章节内容。', '清理缓存', { type: 'warning' })
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

function handleLogout() {
  userStore.logout()
  router.push({ name: 'login' })
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
    bookshelf.loadCategories({ force: true }),
    bookshelf.loadBooks({ force: true, all: true }),
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
  windowWidth.value = window.innerWidth
}

function handleTouchStart(event) {
  if (!isMobileShell.value || event.touches?.length !== 1) return
  const touch = event.touches[0]
  if (touch.clientY <= 20 || touch.clientY >= window.innerHeight - 20) {
    touchStart.value = null
    return
  }
  if (!mobileNavigationVisible.value && touch.clientX > MOBILE_NAV_EDGE) {
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
  max-width: 100vw;
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
  display: flex;
  width: var(--app-sidebar-width);
  flex-direction: column;
  padding: 20px 28px 24px;
  color: #24201b;
  background: #f7f7f7;
  border-right: 1px solid #eee;
}

.app-brand {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 0 0 18px;
  cursor: pointer;
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
}

.app-brand-subtitle {
  margin-top: 18px;
  color: #b5b5b5;
  font-size: 16px;
}

.app-shell-search {
  margin: 0 0 28px;
}

.app-shell-search :deep(.el-input__wrapper) {
  min-height: 28px;
  border-radius: 14px;
  box-shadow: 0 0 0 1px #e6e6e6 inset;
}

.app-search-setting {
  display: grid;
  gap: 12px;
  margin: 0 0 28px;
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
  gap: 8px;
  margin: 0 0 28px;
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
  gap: 28px;
  overflow-y: auto;
  padding: 0 0 18px;
  scrollbar-width: thin;
}

.app-nav-section {
  display: grid;
  grid-template-columns: repeat(2, minmax(78px, 1fr));
  gap: 10px 12px;
}

.app-nav-title {
  grid-column: 1 / -1;
  margin: 0 0 6px;
  color: #b5b5b5;
  font-size: 16px;
  font-weight: 600;
  letter-spacing: 0;
}

.app-nav-item {
  display: flex;
  width: 100%;
  max-width: 100%;
  min-height: 36px;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 0 8px;
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
  font-size: 12px;
  text-overflow: clip;
  white-space: nowrap;
}

.app-sidebar-footer {
  display: grid;
  gap: 10px;
  margin-top: auto;
}

.sync-pill {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 7px 9px;
  color: #766a5c;
  background: #fafafa;
  border-radius: 999px;
  font-size: 12px;
}

.sync-pill.connected {
  color: #bfe6c9;
}

.sync-dot {
  width: 7px;
  height: 7px;
  background: #8d8174;
  border-radius: 50%;
}

.sync-pill.connected .sync-dot {
  background: #77c78b;
}

.user-card {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 10px;
  padding: 9px;
  color: #24201b;
  background: #fafafa;
  border: 1px solid #e6e9ef;
  border-radius: var(--app-radius-md);
  cursor: pointer;
}

.user-card-main {
  display: grid;
  min-width: 0;
  flex: 1;
  text-align: left;
}

.user-card-main strong,
.user-card-main small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-card-main small {
  margin-top: 2px;
  color: #766a5c;
  font-size: 12px;
}

.app-workspace {
  min-height: 100vh;
  width: 100%;
  max-width: 100vw;
  min-width: 0;
  padding-left: var(--app-sidebar-width);
  overflow-x: hidden;
}

.app-content {
  min-height: 100vh;
  width: 100%;
  max-width: 100%;
  min-width: 0;
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
  padding: max(20px, env(safe-area-inset-top)) 20px 66px;
  scrollbar-width: none;
  box-shadow: 12px 0 28px rgba(36, 32, 27, 0.08);
  transform: translateX(calc(-1 * var(--mobile-nav-width, 72vw)));
  transition: transform 0.3s;
  will-change: transform;
}

.app-shell.mobile-shell .app-workspace {
  width: 100vw;
  width: 100dvw;
  max-width: 100vw;
  max-width: 100dvw;
  padding-left: 0;
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

.app-shell.mobile-shell .app-sidebar-footer {
  display: none;
}

.app-shell.mobile-shell .app-brand-mark {
  display: none;
}

.app-shell.mobile-shell .app-nav {
  gap: 14px;
  overflow-y: visible;
  padding: 0 0 14px;
}

.app-shell.mobile-shell .app-nav-section {
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px 12px;
}

.app-shell.mobile-shell .app-nav-title {
  margin: 0 0 8px;
  overflow: hidden;
  color: #a09282;
  font-size: 14px;
  line-height: 1.3;
  text-align: left;
  white-space: nowrap;
}

.app-shell.mobile-shell .app-nav-item {
  display: flex;
  width: 100%;
  min-width: 0;
  min-height: 38px;
  height: auto;
  align-items: center;
  justify-content: center;
  gap: 5px;
  margin: 0;
  padding: 8px 8px;
  background: #fffdf8;
  border: 1px solid #e4d9c8;
  border-radius: 4px;
}

.app-shell.mobile-shell .app-nav-item span {
  overflow: visible;
  font-size: 13px;
  line-height: 1.25;
  text-overflow: clip;
  white-space: normal;
  word-break: keep-all;
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

.app-shell.mobile-shell.mobile-nav-open .app-sidebar {
  transform: translateX(0);
}

</style>
