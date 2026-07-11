<template>
  <el-drawer
    v-model="overlay.workspaceSettingsVisible"
    title="设置"
    :direction="direction"
    :size="size"
    class="global-workspace-settings-drawer"
    destroy-on-close
    @closed="overlay.closeWorkspaceSettings()"
  >
    <Settings :panel="overlay.workspaceSettingsPanel" />
  </el-drawer>
</template>

<script setup>
import { useOverlayStore } from '../../stores/overlay'
import Settings from '../../views/Settings.vue'

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

const overlay = useOverlayStore()
</script>
