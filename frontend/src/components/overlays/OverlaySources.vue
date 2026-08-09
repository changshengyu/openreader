<template>
  <SourceManager
    v-if="isNormalPage"
    :visible="overlay.sourceManageVisible && isManagerIntent"
    :failure-mode="overlay.sourceManageIntent === 'health'"
    :is-mobile="isMobile"
    @close="overlay.closeSourceManage"
  />

  <SourceTransferOverlay
    v-if="isNormalPage && overlay.sourceManageVisible && !isManagerIntent"
    :visible="overlay.sourceManageVisible"
    :intent="overlay.sourceManageIntent"
    :is-mobile="isMobile"
    @close="overlay.closeSourceManage"
  />
</template>

<script setup>
import { computed, defineAsyncComponent, watch } from 'vue'
import { useOverlayStore } from '../../stores/overlay'
import { useReaderStore } from '../../stores/reader'

defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const SourceManager = defineAsyncComponent(() => import('../workspace/SourceManager.vue'))
const SourceTransferOverlay = defineAsyncComponent(() => import('../workspace/SourceTransferOverlay.vue'))
const overlay = useOverlayStore()
const reader = useReaderStore()
const isNormalPage = computed(() => reader.pageType === 'normal')
const isManagerIntent = computed(() => ['manage', 'health'].includes(overlay.sourceManageIntent))

watch(isNormalPage, normal => {
  if (!normal && overlay.sourceManageVisible) overlay.closeSourceManage()
})
</script>
