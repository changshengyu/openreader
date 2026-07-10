<template>
  <OverlayBookInfo />

  <OverlayBookAddToShelf :is-mobile="isMobileOverlay" />

  <OverlayBookImport :is-mobile="isMobileOverlay" />

  <OverlaySources :is-mobile="isMobileOverlay" />

  <OverlayBookManagement
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
  />

  <OverlayBookGroups
    :direction="narrowDrawerDirection"
    :size="narrowDrawerSize"
  />

  <OverlayBookContentSearch :is-mobile="isMobileOverlay" />

  <OverlayBookmarks :is-mobile="isMobileOverlay" />

  <OverlayBookmarkForm :is-mobile="isMobileOverlay" />

  <OverlayLocalStore
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
  />

  <OverlayWebDAV
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
    :is-mobile="isMobileOverlay"
  />

  <OverlayBackups
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
  />

  <OverlayUserManagement
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
    :is-mobile="isMobileOverlay"
  />

  <OverlayReplaceRules
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
    :is-mobile="isMobileOverlay"
  />

  <OverlayRSS
    :direction="wideDrawerDirection"
    :size="wideDrawerSize"
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
const narrowDrawerDirection = computed(() => (
  isMobileOverlay.value ? 'btt' : 'rtl'
))
const narrowDrawerSize = computed(() => (
  isMobileOverlay.value ? '86%' : '420px'
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
