<template>
  <template v-if="isReader && isLoggedIn && !authenticatedSessionBlocked">
    <router-view :key="readerSessionKey" />
    <GlobalOverlayHost />
  </template>

  <div v-else-if="isReader" class="reader-auth-blocked" role="status">
    正在等待重新登录…
  </div>

  <template v-else-if="isLoggedIn && !authenticatedSessionBlocked">
    <AppLayout>
      <router-view />
    </AppLayout>
    <GlobalOverlayHost />
  </template>

  <div v-else-if="isLoggedIn" class="workspace-auth-blocked" role="status">
    正在恢复当前账号…
  </div>

  <router-view v-else-if="isLoginRoute" />

  <div v-else class="workspace-auth-blocked" role="status">
    正在进入登录页面…
  </div>
  <AuthDialog />
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from './layouts/AppLayout.vue'
import AuthDialog from './components/AuthDialog.vue'
import GlobalOverlayHost from './components/GlobalOverlayHost.vue'
import { useUserStore } from './stores/user'
import { useReaderStore } from './stores/reader'
import { useBookshelfStore } from './stores/bookshelf'
import { usePreferencesStore } from './stores/preferences'
import { useSync } from './composables/useSync'
import { initializeReaderTheme } from './utils/readerSettingsBootstrap'
import { safeReturnTo } from './utils/authNavigation'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const readerStore = useReaderStore()
const bookshelf = useBookshelfStore()
const preferences = usePreferencesStore()
const { connect, disconnect } = useSync()

const isLoggedIn = computed(() => !!userStore.token)
const isReader = computed(() => ['reader', 'remote-reader'].includes(route.name))
const isLoginRoute = computed(() => route.name === 'login')
const authenticatedSessionBlocked = computed(() => userStore.readerSessionBlocked)
const readerSessionKey = computed(() => `reader-session:${userStore.sessionGeneration}`)
let systemThemeMedia

function handleAuthRequired(event) {
  userStore.requireLogin(event?.detail?.reason, event?.detail?.rejectedToken)
}

if (typeof window !== 'undefined') {
  window.addEventListener('openreader:auth-required', handleAuthRequired)
  if (window.__openreaderAuthRequired) {
    handleAuthRequired({ detail: window.__openreaderAuthRequired })
  }
}

onMounted(() => {
  readerStore.normalizeSettings()
  setupAutoThemeListener()
  if (userStore.token && !userStore.profile) {
    userStore.loadMe().catch(() => {})
  }
  if (userStore.token) {
    bookshelf.ensureShelfScope()
    preferences.ensurePreferenceScope()
    readerStore.ensureReaderSettingsScope()
    connect()
    loadReaderSettingsAndApplyTheme()
    preferences.loadPreferences().catch(() => {})
  } else {
    applyAutoThemeFromSystem()
  }
})

onBeforeUnmount(() => {
  if (typeof window !== 'undefined') {
    window.removeEventListener('openreader:auth-required', handleAuthRequired)
  }
  if (!systemThemeMedia) return
  if (typeof systemThemeMedia.removeEventListener === 'function') {
    systemThemeMedia.removeEventListener('change', applyAutoThemeFromSystem)
  } else if (typeof systemThemeMedia.removeListener === 'function') {
    systemThemeMedia.removeListener(applyAutoThemeFromSystem)
  }
})

watch(isLoggedIn, (loggedIn) => {
  if (loggedIn) {
    bookshelf.ensureShelfScope()
    readerStore.ensureProgressScope()
    readerStore.ensureReaderSettingsScope()
    preferences.ensurePreferenceScope()
    connect()
    loadReaderSettingsAndApplyTheme()
    preferences.loadPreferences().catch(() => {})
  } else {
    disconnect()
    if (!userStore.authDialogVisible && route.name !== 'login') {
      router.replace({
        name: 'login',
        query: { returnTo: safeReturnTo(route.fullPath) },
      })
    }
  }
})

watch(
  () => readerStore.themeType,
  (themeType) => {
    if (typeof document === 'undefined') return
    document.documentElement.classList.toggle('dark-reader', themeType === 'night')
  },
  { immediate: true },
)

watch(
  () => readerStore.autoTheme,
  () => applyAutoThemeFromSystem(),
)

function setupAutoThemeListener() {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return
  systemThemeMedia = window.matchMedia('(prefers-color-scheme: dark)')
  if (typeof systemThemeMedia.addEventListener === 'function') {
    systemThemeMedia.addEventListener('change', applyAutoThemeFromSystem)
  } else if (typeof systemThemeMedia.addListener === 'function') {
    systemThemeMedia.addListener(applyAutoThemeFromSystem)
  }
}

function applyAutoThemeFromSystem() {
  if (!readerStore.autoTheme || typeof window === 'undefined' || typeof window.matchMedia !== 'function') return
  readerStore.applyAutoTheme(window.matchMedia('(prefers-color-scheme: dark)').matches)
}

function loadReaderSettingsAndApplyTheme() {
  return initializeReaderTheme({
    authenticated: Boolean(userStore.token),
    loadSettings: () => readerStore.loadReaderSettings(),
    applyTheme: applyAutoThemeFromSystem,
  })
}
</script>

<style scoped>
.reader-auth-blocked,
.workspace-auth-blocked {
  min-height: 100vh;
  display: grid;
  place-items: center;
  background: #faf8f2;
  color: #8b8173;
  font-size: 14px;
}
</style>
