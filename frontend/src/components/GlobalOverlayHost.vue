<template>
  <OverlayBookInfo />

  <OverlayBookAddToShelf :is-mobile="isMobileOverlay" />

  <OverlayBookImport :is-mobile="isMobileOverlay" />

  <OverlaySources :is-mobile="isMobileOverlay" />

  <OverlayBookManagement :is-mobile="isMobileOverlay" />

  <OverlayBookGroups :is-mobile="isMobileOverlay" />

  <OverlayBookContentSearch :is-mobile="isMobileOverlay" />

  <OverlayBookmarks :is-mobile="isMobileOverlay" />

  <OverlayBookmarkForm :is-mobile="isMobileOverlay" />

  <OverlayLocalStore
    :is-mobile="isMobileOverlay"
  />

  <OverlayWebDAV
    :is-mobile="isMobileOverlay"
  />

  <OverlayBackups
    :is-mobile="isMobileOverlay"
  />

  <OverlayWorkspaceSettings
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
  />

  <OverlayUserManagement
    :is-mobile="isMobileOverlay"
  />

  <OverlayReplaceRules
    :is-mobile="isMobileOverlay"
  />

  <OverlayRSS
    :is-mobile="isMobileOverlay"
  />
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useReaderStore } from '../stores/reader'
import {
  currentViewportWidth,
  shouldUseMiniInterface,
} from '../utils/responsive'
import OverlayBackups from './overlays/OverlayBackups.vue'
import OverlayBookAddToShelf from './overlays/OverlayBookAddToShelf.vue'
import OverlayBookContentSearch from './overlays/OverlayBookContentSearch.vue'
import OverlayBookGroups from './overlays/OverlayBookGroups.vue'
import OverlayBookImport from './overlays/OverlayBookImport.vue'
import OverlayBookInfo from './overlays/OverlayBookInfo.vue'
import OverlayBookManagement from './overlays/OverlayBookManagement.vue'
import OverlayBookmarks from './overlays/OverlayBookmarks.vue'
import OverlayBookmarkForm from './overlays/OverlayBookmarkForm.vue'
import OverlayLocalStore from './overlays/OverlayLocalStore.vue'
import OverlayReplaceRules from './overlays/OverlayReplaceRules.vue'
import OverlayRSS from './overlays/OverlayRSS.vue'
import OverlaySources from './overlays/OverlaySources.vue'
import OverlayUserManagement from './overlays/OverlayUserManagement.vue'
import OverlayWebDAV from './overlays/OverlayWebDAV.vue'
import OverlayWorkspaceSettings from './overlays/OverlayWorkspaceSettings.vue'

const reader = useReaderStore()
const windowWidth = ref(currentViewportWidth())
const isMobileOverlay = computed(() => (
  shouldUseMiniInterface(reader.pageMode, windowWidth.value)
))
const wideDrawerDirection = computed(() => (
  isMobileOverlay.value ? 'btt' : 'rtl'
))
const wideDrawerSize = computed(() => (
  isMobileOverlay.value ? '88%' : '82%'
))
onMounted(() => {
  window.addEventListener('resize', updateWindowWidth, { passive: true })
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', updateWindowWidth)
})

function updateWindowWidth() {
  windowWidth.value = currentViewportWidth()
}
</script>
