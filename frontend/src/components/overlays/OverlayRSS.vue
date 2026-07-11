<template>
  <el-dialog
    v-model="overlay.rssVisible"
    title="RSS 订阅"
    width="min(760px, calc(100vw - 48px))"
    :fullscreen="isMobile"
    class="global-rss-dialog"
    destroy-on-close
  >
    <RSSManager :is-mobile="isMobile" :visible="overlay.rssVisible" />
  </el-dialog>
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
