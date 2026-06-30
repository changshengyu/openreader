<template>
  <el-dialog
    :model-value="modelValue"
    :title="dialogTitle"
    :width="width"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <div class="bookmark-form">
      <el-input
        v-if="showDetails"
        :model-value="title"
        placeholder="标题"
        @update:model-value="$emit('update:title', $event)"
      />
      <el-input
        v-if="showDetails"
        :model-value="excerpt"
        type="textarea"
        :rows="3"
        placeholder="摘录"
        @update:model-value="$emit('update:excerpt', $event)"
      />
      <el-input
        :model-value="note"
        type="textarea"
        :rows="4"
        :placeholder="notePlaceholder"
        @update:model-value="$emit('update:note', $event)"
      />
    </div>
    <template #footer>
      <el-button @click="$emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="saving" @click="$emit('save')">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
defineProps({
  modelValue: {
    type: Boolean,
    default: false,
  },
  dialogTitle: {
    type: String,
    default: '编辑书签',
  },
  width: {
    type: String,
    default: '380px',
  },
  showDetails: {
    type: Boolean,
    default: false,
  },
  title: {
    type: String,
    default: '',
  },
  excerpt: {
    type: String,
    default: '',
  },
  note: {
    type: String,
    default: '',
  },
  notePlaceholder: {
    type: String,
    default: '笔记',
  },
  saving: {
    type: Boolean,
    default: false,
  },
})

defineEmits([
  'update:modelValue',
  'update:title',
  'update:excerpt',
  'update:note',
  'save',
])
</script>

<style scoped>
.bookmark-form {
  display: grid;
  gap: 10px;
}
</style>
