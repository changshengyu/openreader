<template>
  <el-drawer
    v-model="overlay.webdavVisible"
    title="WebDAV"
    :direction="direction"
    :size="size"
    class="global-file-drawer"
  >
    <WebDAVBrowser :is-mobile="isMobile" />
  </el-drawer>
</template>

<script setup>
import { useOverlayStore } from '../../stores/overlay'
import WebDAVBrowser from '../WebDAVBrowser.vue'

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
