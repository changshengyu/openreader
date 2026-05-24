<template>
  <div class="app-shell">
    <div v-if="offline" class="app-offline">网络已断开，部分同步能力会在恢复连接后继续。</div>

    <aside class="app-sidebar">
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
          placeholder="搜索远程书籍"
          clearable
          @keyup.enter="goSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
      </div>

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

    <div class="app-workspace">
      <main class="app-content">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ArrowUp,
  Box,
  Compass,
  Connection,
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
import { useSync } from '../composables/useSync'
import { compareRecentBook } from '../utils/bookOrder'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const overlay = useOverlayStore()
const bookshelf = useBookshelfStore()
const reader = useReaderStore()
const quickSearch = ref('')
const offline = ref(false)
const { connected: syncConnected, connect, disconnect } = useSync()

const navSections = computed(() => [
  {
    title: '搜索设置',
    items: [
      { key: 'search', label: '搜索', icon: Search, route: 'search' },
      { key: 'discover', label: '书海', icon: Compass, route: 'discover' },
    ],
  },
  {
    title: '后端设定',
    items: [
      { key: 'backendStatus', label: syncConnected.value ? '同步在线' : '同步未连', icon: Connection, action: refreshShelfData },
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
      { key: 'localStore', label: '浏览书仓', icon: FolderOpened, action: () => overlay.openLocalStore(router), route: 'local-store' },
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
    title: 'RSS',
    items: [
      { key: 'rss', label: 'RSS', icon: Connection, action: () => overlay.openRSS() },
    ],
  },
])

const userInitial = computed(() => (userStore.profile?.username || '?').slice(0, 1).toUpperCase())
const recentBook = computed(() => {
  const rows = [...(Array.isArray(bookshelf.books) ? bookshelf.books : [])]
  rows.sort((a, b) => compareRecentBook(a, b, reader.progressByBook))
  return rows[0] || null
})

function goHome() {
  router.push({ name: 'home' })
}

function goRoute(name) {
  router.push({ name })
}

function runNavAction(item) {
  if (item.action) {
    item.action()
    return
  }
  if (item.route) router.push({ name: item.route, query: item.query || (item.panel ? { panel: item.panel } : {}) })
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
  if (!keyword) return
  router.push({ name: 'search', query: { q: keyword } })
  quickSearch.value = ''
}

function openRecentBook() {
  if (!recentBook.value) return
  router.push({ name: 'reader', params: { id: recentBook.value.id } })
}

function recentSubTitle(book) {
  const progress = reader.progressByBook[book.id] || book.progress
  if (progress?.chapterTitle) return progress.chapterTitle
  if (Number.isInteger(progress?.chapterIndex)) return `第 ${progress.chapterIndex + 1} 章`
  return book.lastChapter || book.author || '继续阅读'
}

function handleLogout() {
  userStore.logout()
  router.push({ name: 'login' })
}

async function refreshShelfData() {
  await Promise.all([bookshelf.loadCategories(), bookshelf.loadBooks()]).catch(() => {})
  router.push({ name: 'home' })
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
    if (token) {
      connect()
    } else {
      disconnect()
    }
  },
  { immediate: true },
)

onMounted(() => {
  window.addEventListener('offline', setOffline)
  window.addEventListener('online', setOnline)
  offline.value = !navigator.onLine
  if (userStore.token && !userStore.profile) {
    userStore.loadMe().catch(() => {})
  }
  if (userStore.token && !bookshelf.books.length) {
    Promise.all([bookshelf.loadCategories(), bookshelf.loadBooks()]).catch(() => {})
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('offline', setOffline)
  window.removeEventListener('online', setOnline)
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
  padding: 18px 14px;
  color: #24201b;
  background: #f7f4ea;
  border-right: 1px solid #e4d9c8;
}

.app-brand {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 8px 18px;
  cursor: pointer;
}

.app-brand-mark {
  display: inline-grid;
  width: 38px;
  height: 38px;
  place-items: center;
  flex: 0 0 38px;
  color: #f7f4ea;
  background: #24201b;
  border-radius: var(--app-radius-md);
  font-weight: 800;
}

.app-brand-title {
  font-size: 17px;
  font-weight: 800;
  line-height: 1.2;
}

.app-brand-subtitle {
  margin-top: 3px;
  color: #918575;
  font-size: 12px;
}

.app-shell-search {
  margin: 0 4px 18px;
}

.sidebar-recent {
  display: grid;
  gap: 8px;
  margin: 0 4px 18px;
}

.sidebar-recent-book {
  display: grid;
  gap: 4px;
  min-width: 0;
  padding: 9px 10px;
  color: #24201b;
  background: #fffdf8;
  border: 1px solid #e4d9c8;
  border-radius: 6px;
  cursor: pointer;
  text-align: left;
}

.sidebar-recent-book:disabled {
  cursor: default;
  opacity: 0.7;
}

.sidebar-recent-book span,
.sidebar-recent-book small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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
  gap: 14px;
  overflow-y: auto;
  padding: 0 4px 14px;
  scrollbar-width: thin;
}

.app-nav-section {
  display: grid;
  gap: 4px;
}

.app-nav-title {
  margin: 0 8px 4px;
  color: #a09282;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0;
}

.app-nav-item {
  display: flex;
  width: 100%;
  height: 42px;
  align-items: center;
  gap: 10px;
  padding: 0 12px;
  color: #766a5c;
  background: transparent;
  border: 0;
  border-radius: var(--app-radius-sm);
  cursor: pointer;
  text-align: left;
}

.app-nav-item:hover {
  color: #1f5654;
  background: #fffdf8;
}

.app-nav-item.active {
  color: #1f5654;
  background: #d9ece7;
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
  background: #fffdf8;
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
  background: #fffdf8;
  border: 1px solid #e4d9c8;
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

@media (max-width: 860px), (hover: none) and (pointer: coarse) {
  .app-sidebar {
    width: 78px;
    overflow-y: auto;
    padding: 8px 6px;
    scrollbar-width: none;
  }

  .app-workspace {
    width: calc(100vw - 78px);
    max-width: calc(100vw - 78px);
    margin-left: 78px;
    padding-left: 0;
  }

  .app-content {
    min-height: 100vh;
  }

  .app-brand {
    display: grid;
    justify-items: center;
    gap: 5px;
    padding: 8px 0 14px;
  }

  .app-brand > div:last-child,
  .app-shell-search,
  .app-sidebar-footer {
    display: none;
  }

  .app-brand-mark {
    width: 38px;
    height: 38px;
    flex-basis: 38px;
    border-radius: 4px;
  }

  .app-nav {
    gap: 0;
    overflow-y: visible;
    padding: 0 0 12px;
  }

  .app-nav-section {
    gap: 0;
  }

  .app-nav-title {
    margin: 9px 0 3px;
    overflow: hidden;
    color: #a09282;
    font-size: 10px;
    line-height: 1.2;
    text-align: center;
    white-space: nowrap;
  }

  .app-nav-item {
    display: grid;
    height: 60px;
    place-items: center;
    align-content: center;
    gap: 5px;
    padding: 0;
    border-radius: 0;
  }

  .app-nav-item span {
    font-size: 11px;
    line-height: 1.15;
  }

  .app-nav-item + .app-nav-item {
    border-top: 1px solid #e4d9c8;
  }

  .sidebar-recent {
    margin: 0 0 8px;
  }

  .sidebar-recent .app-nav-title,
  .sidebar-recent-book small {
    display: none;
  }

  .sidebar-recent-book {
    min-height: 58px;
    padding: 5px;
    place-items: center;
    text-align: center;
  }

  .sidebar-recent-book span {
    display: -webkit-box;
    font-size: 11px;
    line-height: 1.25;
    white-space: normal;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
  }
}

@media (max-width: 420px), (hover: none) and (pointer: coarse) and (max-width: 520px) {
  .app-sidebar {
    width: 72px;
  }

  .app-workspace {
    width: calc(100vw - 72px);
    max-width: calc(100vw - 72px);
    margin-left: 72px;
  }

  .app-nav-item {
    height: 58px;
  }

  .app-nav-item span {
    font-size: 11px;
  }
}
</style>
