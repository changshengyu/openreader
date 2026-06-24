<template>
  <template v-if="isReader">
    <router-view />
    <GlobalOverlayHost v-if="isLoggedIn" />
  </template>

  <template v-else-if="isLoggedIn">
    <AppLayout>
      <router-view />
    </AppLayout>
    <GlobalOverlayHost />
  </template>

  <router-view v-else />
  <AuthDialog />
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import AppLayout from './layouts/AppLayout.vue'
import AuthDialog from './components/AuthDialog.vue'
import GlobalOverlayHost from './components/GlobalOverlayHost.vue'
import { useUserStore } from './stores/user'
import { useReaderStore } from './stores/reader'
import { useBookshelfStore } from './stores/bookshelf'
import { usePreferencesStore } from './stores/preferences'
import { useSync } from './composables/useSync'

const route = useRoute()
const userStore = useUserStore()
const readerStore = useReaderStore()
const bookshelf = useBookshelfStore()
const preferences = usePreferencesStore()
const { connect, disconnect } = useSync()

const isLoggedIn = computed(() => !!userStore.token)
const isReader = computed(() => route.name === 'reader')
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
    readerStore.loadReaderSettings().then(applyAutoThemeFromSystem).catch(() => {})
    preferences.loadPreferences().catch(() => {})
  }
  applyAutoThemeFromSystem()
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
    readerStore.loadReaderSettings().then(applyAutoThemeFromSystem).catch(() => {})
    preferences.loadPreferences().catch(() => {})
  } else {
    disconnect()
  }
})

watch(
  () => readerStore.theme,
  (theme) => {
    if (typeof document === 'undefined') return
    document.documentElement.classList.toggle('dark-reader', theme === 'dark' || theme === 'black')
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
</script>
