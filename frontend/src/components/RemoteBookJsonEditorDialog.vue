<template>
  <el-dialog
    :model-value="visible"
    title="保存书籍"
    width="500px"
    :fullscreen="isMobile"
    :close-on-click-modal="!saving"
    :close-on-press-escape="!saving"
    append-to-body
    class="remote-book-json-editor"
    @update:model-value="handleVisibilityChange"
  >
    <el-input
      :model-value="content"
      type="textarea"
      :rows="18"
      resize="vertical"
      aria-label="书籍 JSON"
      @update:model-value="$emit('update:content', $event)"
    />
    <template #footer>
      <el-button :disabled="saving" @click="$emit('close')">取 消</el-button>
      <el-button type="primary" :loading="saving" @click="$emit('save')">保 存</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
defineProps({
  visible: { type: Boolean, default: false },
  content: { type: String, default: '' },
  saving: { type: Boolean, default: false },
  isMobile: { type: Boolean, default: false },
})

const emit = defineEmits(['update:content', 'close', 'save'])

function handleVisibilityChange(value) {
  if (!value) emit('close')
}
</script>

<style>
.remote-book-json-editor .el-textarea__inner {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  line-height: 1.55;
}
</style>
