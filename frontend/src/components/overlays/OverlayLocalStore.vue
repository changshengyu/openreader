<template>
  <el-dialog
    v-model="overlay.localStoreVisible"
    title="本地书仓"
    width="min(1120px, calc(100vw - 48px))"
    :fullscreen="isMobile"
    class="global-local-store-dialog"
    destroy-on-close
  >
    <LocalStore embedded />
  </el-dialog>
</template>

<script setup>
import { defineAsyncComponent } from 'vue'
import { useOverlayStore } from '../../stores/overlay'

defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const LocalStore = defineAsyncComponent(() => import('../../views/LocalStore.vue'))
const overlay = useOverlayStore()
</script>
