<template>
  <el-dialog
    :model-value="modelValue"
    title="书籍信息"
    width="620px"
    class="book-info-dialog"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <BookInfoPanel
      v-if="book"
      :book="book"
      :source-name="sourceName"
      :category-name="categoryName"
      :progress="progress"
      :chapters="chapters"
      :status-label="statusLabel"
      :status-type="statusType"
    >
      <slot />
    </BookInfoPanel>
  </el-dialog>
</template>

<script setup>
import BookInfoPanel from './BookInfoPanel.vue'

defineProps({
  modelValue: {
    type: Boolean,
    default: false,
  },
  book: {
    type: Object,
    default: null,
  },
  sourceName: {
    type: String,
    default: '',
  },
  categoryName: {
    type: String,
    default: '',
  },
  progress: {
    type: Number,
    default: 0,
  },
  chapters: {
    type: [Array, Number],
    default: 0,
  },
  statusLabel: {
    type: String,
    default: '',
  },
  statusType: {
    type: String,
    default: 'info',
  },
})

defineEmits(['update:modelValue'])
</script>
