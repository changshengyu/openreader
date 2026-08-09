<template>
  <RSSManager
    :is-mobile="isMobile"
    :visible="overlay.rssVisible"
    @close="overlay.rssVisible = false"
  />
</template>

<script setup>
import { useOverlayStore } from '../../stores/overlay'
import RSSManager from '../RSSManager.vue'

defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const overlay = useOverlayStore()
</script>
