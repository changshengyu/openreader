<template>
  <el-drawer
    v-model="overlay.bookmarkVisible"
    :title="`书签${overlay.bookmarkBook?.title ? ` · ${overlay.bookmarkBook.title}` : ''}`"
    :direction="direction"
    :size="size"
    class="global-bookmark-drawer"
  >
    <div v-loading="loading">
      <ReaderBookmarkPanel
        :bookmarks="items"
        :show-add="false"
        @jump="jump"
        @edit="openEditor"
        @remove="removeOne"
        @remove-many="removeMany"
        @import="importRows"
      />
    </div>
  </el-drawer>

  <el-dialog
    v-model="editorVisible"
    title="编辑书签"
    width="380px"
    :fullscreen="isMobile"
  >
    <div class="bookmark-editor">
      <el-input v-model="draft.title" placeholder="标题" />
      <el-input
        v-model="draft.excerpt"
        type="textarea"
        :rows="3"
        placeholder="摘录"
      />
      <el-input
        v-model="draft.note"
        type="textarea"
        :rows="4"
        placeholder="笔记"
      />
    </div>
    <template #footer>
      <el-button @click="editorVisible = false">取消</el-button>
      <el-button type="primary" :loading="saving" @click="saveEdit">
        保存
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useBookBookmarks } from '../../composables/useBookBookmarks'
import { useOverlayBookmarkActions } from '../../composables/useOverlayBookmarkActions'
import { useOverlayStore } from '../../stores/overlay'
import ReaderBookmarkPanel from '../reader/ReaderBookmarkPanel.vue'

defineProps({
  direction: {
    type: String,
    required: true,
  },
  size: {
    type: [String, Number],
    required: true,
  },
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const router = useRouter()
const overlay = useOverlayStore()
const bookId = computed(() => overlay.bookmarkBook?.id)

const {
  items,
  loading,
  mutating: saving,
  load,
  reset,
  update,
  remove,
  removeMany: removeManyData,
  importPayloads,
  handleUpdated,
} = useBookBookmarks({
  bookId,
  isActive: () => overlay.bookmarkVisible,
  onLoadError: error => ElMessage.error(readError(error, '加载书签失败')),
})

const {
  editorVisible,
  draft,
  jump,
  openEditor,
  saveEdit,
  removeOne,
  removeMany,
  importRows,
} = useOverlayBookmarkActions({
  getBook: () => overlay.bookmarkBook,
  closePanel: () => {
    overlay.bookmarkVisible = false
  },
  navigate: routeLocation => router.push(routeLocation),
  update,
  remove,
  removeMany: removeManyData,
  importPayloads,
  confirm: (...args) => ElMessageBox.confirm(...args),
  onSuccess: message => ElMessage.success(message),
  onInvalidImport: message => ElMessage.error(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})

watch(
  () => overlay.bookmarkVisible,
  async (visible) => {
    if (!visible) {
      reset()
      return
    }
    await load()
  },
)

onMounted(() => {
  window.addEventListener('openreader:bookmarks-updated', handleUpdated)
})

onBeforeUnmount(() => {
  window.removeEventListener('openreader:bookmarks-updated', handleUpdated)
})

function readError(error, fallback) {
  return error?.response?.data?.error?.message ||
    error?.response?.data?.error ||
    fallback
}
</script>

<style scoped>
.bookmark-editor {
  display: grid;
  gap: 10px;
}
</style>
