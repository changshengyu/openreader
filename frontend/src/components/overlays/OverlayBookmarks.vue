<template>
  <el-dialog
    v-model="overlay.bookmarkVisible"
    :width="dialogWidth"
    :fullscreen="isMobile"
    class="global-bookmark-dialog"
    @closed="handleClosed"
  >
    <template #header>
      <div class="reader-dialog-title">
        <span>{{ bookTitle }} 书签管理</span>
        <div class="reader-dialog-title-actions">
          <el-button v-if="canAddCurrentPage" link type="primary" @click="addCurrentPage">添加当前页</el-button>
          <el-button link type="primary" @click="pickImportFile">导入</el-button>
        </div>
      </div>
    </template>

    <input
      ref="fileRef"
      class="bookmark-file-input"
      type="file"
      accept=".json,application/json"
      @change="onImportFileChange"
    />

    <div v-loading="loading" class="reader-dialog-table">
      <el-table
        :data="items"
        max-height="520"
        @selection-change="selectedRows = $event"
      >
        <el-table-column type="selection" width="42" />
        <el-table-column label="书籍" min-width="150">
          <template #default>
            {{ bookTitle }}
          </template>
        </el-table-column>
        <el-table-column prop="title" label="章节" min-width="160" />
        <el-table-column label="内容" min-width="200">
          <template #default="scope">
            {{ scope.row.excerpt || '—' }}
          </template>
        </el-table-column>
        <el-table-column label="备注" min-width="160">
          <template #default="scope">
            {{ scope.row.note || '—' }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="112" fixed="right">
          <template #default="scope">
            <el-button link type="primary" @click="jump(scope.row)">跳转</el-button>
            <el-button link type="primary" @click="openEditor(scope.row)">编辑</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <template #footer>
      <div class="reader-dialog-footer">
        <el-button
          type="primary"
          :disabled="!selectedRows.length"
          @click="removeMany(selectedRows)"
        >
          批量删除
        </el-button>
        <span>已选择 {{ selectedRows.length }} 个</span>
        <el-button @click="overlay.bookmarkVisible = false">取消</el-button>
      </div>
    </template>
  </el-dialog>

</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useBookBookmarks } from '../../composables/useBookBookmarks'
import { useOverlayBookmarkActions } from '../../composables/useOverlayBookmarkActions'
import { useOverlayStore } from '../../stores/overlay'

defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const dialogWidth = '880px'
const router = useRouter()
const overlay = useOverlayStore()
const fileRef = ref(null)
const selectedRows = ref([])
const bookId = computed(() => overlay.bookmarkBook?.id)
const bookTitle = computed(() => (
  overlay.bookmarkBook?.title || overlay.bookmarkBook?.name || '书签'
))
const canAddCurrentPage = computed(() => Boolean(
  overlay.bookmarkBook?.id && String(overlay.bookmarkCreateDraft?.excerpt || '').trim(),
))

const {
  items,
  loading,
  load,
  reset,
  removeMany: removeManyData,
  importPayloads,
  handleUpdated,
} = useBookBookmarks({
  bookId,
  isActive: () => overlay.bookmarkVisible,
  onLoadError: error => ElMessage.error(readError(error, '加载书签失败')),
})

const {
  jump,
  removeMany,
  importRows,
} = useOverlayBookmarkActions({
  getBook: () => overlay.bookmarkBook,
  closePanel: () => {
    overlay.bookmarkVisible = false
  },
  navigate: routeLocation => router.push(routeLocation),
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
      selectedRows.value = []
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

function pickImportFile() {
  fileRef.value?.click()
}

function addCurrentPage() {
  if (!canAddCurrentPage.value) return
  overlay.openBookmarkForm(
    overlay.bookmarkBook,
    overlay.bookmarkCreateDraft,
    { mode: 'create' },
  )
}

function handleClosed() {
  selectedRows.value = []
  overlay.clearBookmark()
}

function openEditor(bookmark) {
  overlay.openBookmarkForm(overlay.bookmarkBook, bookmark, { mode: 'edit' })
}

function onImportFileChange(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    try {
      const rows = JSON.parse(String(reader.result || '[]'))
      if (!Array.isArray(rows) || !rows.length) {
        ElMessage.error('书签文件错误')
        return
      }
      importRows(rows)
    } catch {
      ElMessage.error('书签文件错误')
    }
  }
  reader.onerror = () => ElMessage.error('读取书签文件失败')
  reader.readAsText(file)
}

function readError(error, fallback) {
  return error?.response?.data?.error?.message ||
    error?.response?.data?.error ||
    fallback
}
</script>

<style scoped>
.reader-dialog-title,
.reader-dialog-footer {
  display: flex;
  align-items: center;
  gap: 12px;
}

.reader-dialog-title {
  justify-content: space-between;
  min-width: 0;
}

.reader-dialog-title span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.reader-dialog-title-actions {
  display: flex;
  flex: none;
  align-items: center;
  gap: 4px;
}

.reader-dialog-table {
  min-height: 180px;
}

.reader-dialog-footer span {
  flex: 1;
  color: var(--el-text-color-secondary);
  text-align: left;
}

.bookmark-file-input {
  display: none;
}

@media (max-width: 750px) {
  .reader-dialog-footer {
    flex-wrap: wrap;
  }

  .reader-dialog-footer span {
    order: 3;
    flex-basis: 100%;
  }
}
</style>
