<template>
  <el-dialog
    v-model="overlay.sourceManageVisible"
    title="书源管理"
    width="min(1120px, calc(100vw - 48px))"
    :fullscreen="isMobile"
    destroy-on-close
    class="global-source-manage-dialog"
    @closed="overlay.closeSourceManage"
  >
    <SourceManager embedded :intent="overlay.sourceManageIntent" />
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

const SourceManager = defineAsyncComponent(() => import('../workspace/SourceManager.vue'))
const overlay = useOverlayStore()
</script>
