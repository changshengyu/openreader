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
  Notebook,
  Operation,
  Search,
  Setting,
  SwitchButton,
  Upload,
} from '@element-plus/icons-vue'
import { useUserStore } from '../stores/user'
import { useOverlayStore } from '../stores/overlay'
import { useSync } from '../composables/useSync'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const overlay = useOverlayStore()
const quickSearch = ref('')
const offline = ref(false)

const navSections = [
  {
    title: '书架',
    items: [
      { key: 'home', label: '书架', icon: Notebook, route: 'home' },
      { key: 'search', label: '搜索', icon: Search, route: 'search' },
      { key: 'discover', label: '书海', icon: Compass, route: 'discover' },
    ],
  },
  {
    title: '书源设置',
    items: [
      { key: 'sources', label: '书源管理', icon: Connection, route: 'sources' },
      { key: 'sourceDebug', label: '调试检测', icon: Operation, route: 'sources' },
    ],
  },
  {
    title: '书架设置',
    items: [
      { key: 'bookManage', label: '书籍管理', icon: Files, action: () => overlay.openBookManage() },
      { key: 'bookGroup', label: '分组管理', icon: Box, action: () => overlay.openBookGroup('manage') },
      { key: 'importBook', label: '导入书籍', icon: Upload, action: () => overlay.openImportBook(router), route: 'home' },
      { key: 'localStore', label: '本地书仓', icon: FolderOpened, action: () => overlay.openLocalStore(router), route: 'local-store' },
      { key: 'replaceRules', label: '替换规则', icon: Edit, action: () => overlay.openReplaceRules(router), route: 'settings', panel: 'replace' },
    ],
  },
  {
    title: '用户空间',
    items: [
      { key: 'webdav', label: 'WebDAV', icon: Upload, action: () => overlay.openWebDAV(router), route: 'settings', panel: 'webdav' },
      { key: 'rss', label: 'RSS', icon: Connection, action: () => overlay.openRSS(router), route: 'settings', panel: 'rss' },
      { key: 'userManage', label: '用户管理', icon: Operation, action: () => overlay.openUserManage(router), route: 'settings', panel: 'admin' },
      { key: 'settings', label: '设置', icon: Setting, route: 'settings', panel: 'account' },
    ],
  },
]

const userInitial = computed(() => (userStore.profile?.username || '?').slice(0, 1).toUpperCase())

const { connected: syncConnected, connect, disconnect } = useSync()

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
  if (item.route) goRoute(item.route)
}

function isNavActive(item) {
  if (!item.route || route.name !== item.route) return false
  if (item.key === 'importBook') return route.query.import === '1'
  if (!item.panel) return true
  return String(route.query.panel || 'account') === item.panel
}

function goSearch() {
  const keyword = quickSearch.value.trim()
  if (!keyword) return
  router.push({ name: 'search', query: { q: keyword } })
  quickSearch.value = ''
}

function handleLogout() {
  userStore.logout()
  router.push({ name: 'login' })
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
})

onBeforeUnmount(() => {
  window.removeEventListener('offline', setOffline)
  window.removeEventListener('online', setOnline)
})
</script>

<style scoped>
.app-shell {
  min-height: 100vh;
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
  color: #fff9ed;
  background: var(--app-nav-bg);
  border-right: 1px solid rgba(255, 249, 237, 0.08);
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
  color: #2b2118;
  background: var(--app-nav-active);
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
  color: var(--app-nav-muted);
  font-size: 12px;
}

.app-shell-search {
  margin: 0 4px 18px;
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
  color: rgba(244, 228, 197, 0.58);
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
  color: var(--app-nav-muted);
  background: transparent;
  border: 0;
  border-radius: var(--app-radius-sm);
  cursor: pointer;
  text-align: left;
}

.app-nav-item:hover {
  color: #fff9ed;
  background: rgba(255, 249, 237, 0.07);
}

.app-nav-item.active {
  color: var(--app-nav-active);
  background: rgba(244, 228, 197, 0.12);
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
  color: var(--app-nav-muted);
  background: rgba(255, 249, 237, 0.06);
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
  color: #fff9ed;
  background: rgba(255, 249, 237, 0.07);
  border: 1px solid rgba(255, 249, 237, 0.08);
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
  color: var(--app-nav-muted);
  font-size: 12px;
}

.app-workspace {
  min-height: 100vh;
  padding-left: var(--app-sidebar-width);
}

.app-content {
  min-height: 100vh;
}

@media (max-width: 860px) {
  .app-sidebar {
    width: 72px;
    overflow-y: auto;
    padding: 8px 6px;
    scrollbar-width: none;
  }

  .app-workspace {
    padding-left: 72px;
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
    color: rgba(244, 228, 197, 0.48);
    font-size: 10px;
    line-height: 1.2;
    text-align: center;
    white-space: nowrap;
  }

  .app-nav-item {
    display: grid;
    height: 66px;
    place-items: center;
    align-content: center;
    gap: 5px;
    padding: 0;
    border-radius: 0;
  }

  .app-nav-item span {
    font-size: 12px;
  }

  .app-nav-item + .app-nav-item {
    border-top: 1px solid rgba(255, 249, 237, 0.08);
  }
}

@media (max-width: 420px) {
  .app-sidebar {
    width: 64px;
  }

  .app-workspace {
    padding-left: 64px;
  }

  .app-nav-item {
    height: 62px;
  }

  .app-nav-item span {
    font-size: 11px;
  }
}
</style>
