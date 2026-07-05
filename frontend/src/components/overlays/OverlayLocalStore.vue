<template>
  <el-drawer
    v-model="overlay.localStoreVisible"
    title="本地书仓"
    :direction="direction"
    :size="size"
    class="global-local-store-drawer"
    destroy-on-close
  >
    <LocalStore embedded />
  </el-drawer>
</template>

<script setup>
import { defineAsyncComponent } from 'vue'
import { useOverlayStore } from '../../stores/overlay'

defineProps({
  direction: {
    type: String,
    required: true,
  },
  size: {
    type: [String, Number],
    required: true,
  },
})

const LocalStore = defineAsyncComponent(() => import('../../views/LocalStore.vue'))
const overlay = useOverlayStore()
</script>
