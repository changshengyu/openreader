<template>
  <el-dialog
    v-model="overlay.webdavVisible"
    title="WebDAV"
    width="min(1120px, calc(100vw - 48px))"
    :fullscreen="isMobile"
    class="global-webdav-dialog"
    destroy-on-close
  >
    <WebDAVBrowser :is-mobile="isMobile" />
  </el-dialog>
</template>

<script setup>
import { useOverlayStore } from '../../stores/overlay'
import WebDAVBrowser from '../WebDAVBrowser.vue'

defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const overlay = useOverlayStore()
</script>
