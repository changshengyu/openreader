<template>
  <el-dialog
    :model-value="modelValue"
    title="编辑RSS源"
    width="min(1000px, max(750px, 70vw))"
    :fullscreen="isMobile"
    class="rss-source-editor-dialog"
    append-to-body
    @update:model-value="handleVisibleChange"
  >
    <el-input
      :model-value="content"
      class="rss-json-editor"
      type="textarea"
      :autosize="{ minRows: 18, maxRows: 30 }"
      spellcheck="false"
      @update:model-value="$emit('update:content', $event)"
    />
    <template #footer>
      <el-button @click="$emit('close')">取 消</el-button>
      <el-button type="primary" :loading="saving" @click="$emit('save')">保 存</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
defineProps({
  modelValue: Boolean,
  content: {
    type: String,
    default: '',
  },
  isMobile: Boolean,
  saving: Boolean,
})

const emit = defineEmits(['update:modelValue', 'update:content', 'close', 'save'])

function handleVisibleChange(visible) {
  emit('update:modelValue', visible)
  if (!visible) emit('close')
}
</script>

<style scoped>
.rss-json-editor :deep(textarea) {
  min-height: min(58vh, 620px) !important;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  line-height: 1.55;
  tab-size: 4;
}
</style>
