<template>
  <el-dialog
    :model-value="modelValue"
    title="书籍信息"
    width="480px"
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
      :show-category-action="showCategoryAction"
      :category-action-label="categoryActionLabel"
      variant="dialog"
      @cover-upload="$emit('coverUpload', $event)"
      @can-update-change="$emit('canUpdateChange', $event)"
      @category-action="$emit('categoryAction')"
    >
      <slot />
    </BookInfoPanel>
  </el-dialog>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import BookInfoPanel from './BookInfoPanel.vue'
import { useReaderStore } from '../stores/reader'

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
  showCategoryAction: {
    type: Boolean,
    default: false,
  },
  categoryActionLabel: {
    type: String,
    default: '设置分组',
  },
})

defineEmits(['update:modelValue', 'coverUpload', 'canUpdateChange', 'categoryAction'])

const MINI_INTERFACE_MAX_WIDTH = 750
const reader = useReaderStore()
const windowWidth = ref(typeof window === 'undefined' ? 1024 : window.innerWidth)
const isMobile = computed(() => reader.pageMode === 'mobile' || windowWidth.value <= MINI_INTERFACE_MAX_WIDTH)

function handleResize() {
  windowWidth.value = window.innerWidth
}

onMounted(() => window.addEventListener('resize', handleResize))
onBeforeUnmount(() => window.removeEventListener('resize', handleResize))
</script>
