<template>
  <el-drawer
    v-model="overlay.rssVisible"
    title="RSS"
    :direction="direction"
    :size="size"
    class="global-rss-drawer"
  >
    <RSSManager :is-mobile="isMobile" />
  </el-drawer>
</template>

<script setup>
import { useOverlayStore } from '../../stores/overlay'
import RSSManager from '../RSSManager.vue'

defineProps({
  direction: {
    type: String,
    required: true,
  },
  size: {
    type: [String, Number],
    required: true,
  },
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const overlay = useOverlayStore()
</script>
