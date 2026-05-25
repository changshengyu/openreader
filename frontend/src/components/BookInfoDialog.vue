<template>
  <el-dialog
    :model-value="modelValue"
    title="书籍信息"
    width="620px"
    class="book-info-dialog"
    :fullscreen="isMobile"
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
      :cover-editable="coverEditable"
      :cover-uploading="coverUploading"
      :show-update-switch="showUpdateSwitch"
      :can-update="canUpdate"
      :update-switch-loading="updateSwitchLoading"
      :browser-cache-count="browserCacheCount"
      @cover-upload="$emit('coverUpload', $event)"
      @can-update-change="$emit('canUpdateChange', $event)"
    >
      <slot />
    </BookInfoPanel>
  </el-dialog>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
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
  coverEditable: {
    type: Boolean,
    default: false,
  },
  coverUploading: {
    type: Boolean,
    default: false,
  },
  showUpdateSwitch: {
    type: Boolean,
    default: false,
  },
  canUpdate: {
    type: Boolean,
    default: true,
  },
  updateSwitchLoading: {
    type: Boolean,
    default: false,
  },
  browserCacheCount: {
    type: Number,
    default: -1,
  },
})

defineEmits(['update:modelValue', 'coverUpload', 'canUpdateChange'])

const windowWidth = ref(typeof window === 'undefined' ? 1024 : window.innerWidth)
const coarsePointer = ref(typeof window === 'undefined' ? false : window.matchMedia?.('(hover: none) and (pointer: coarse)').matches || false)
const isMobile = computed(() => windowWidth.value <= 860 || coarsePointer.value)

function handleResize() {
  windowWidth.value = window.innerWidth
  coarsePointer.value = window.matchMedia?.('(hover: none) and (pointer: coarse)').matches || false
}

onMounted(() => window.addEventListener('resize', handleResize))
onBeforeUnmount(() => window.removeEventListener('resize', handleResize))
</script>
