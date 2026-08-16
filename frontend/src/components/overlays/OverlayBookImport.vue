<template>
  <el-dialog
    v-model="overlay.importBookVisible"
    title="导入本地书籍"
    width="520px"
    class="import-book-dialog direct-import-picker-dialog"
    :fullscreen="isMobile"
    @closed="resetPicker"
  >
    <el-upload
      ref="upload"
      drag
      multiple
      :limit="maxFiles"
      :show-file-list="false"
      :auto-upload="false"
      accept=".txt,.epub,.umd,.cbz"
      @change="pickFiles"
      @exceed="rejectOverflow"
    >
      <el-icon class="upload-icon"><UploadFilled /></el-icon>
      <div class="upload-text">拖入或选择 TXT / EPUB / UMD / CBZ 文件</div>
    </el-upload>

    <template #footer>
      <el-button @click="overlay.importBookVisible = false">取消</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { onBeforeUnmount, ref } from 'vue'
import { UploadFilled } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { useOverlayStore } from '../../stores/overlay'
import { isDirectImportableLocalPath } from '../../utils/localBookToc'

defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const maxFiles = 64
const overlay = useOverlayStore()
const upload = ref(null)
let selectionTimer = null

function pickFiles(_file, uploadFiles) {
  clearSelectionTimer()
  selectionTimer = setTimeout(() => {
    selectionTimer = null
    const files = uploadFiles.map(item => item.raw).filter(Boolean)
    if (!files.length) return
    if (files.length > maxFiles) {
      rejectOverflow()
      return
    }
    if (files.some(file => !isDirectImportableLocalPath(file.name))) {
      ElMessage.error('仅支持 TXT / EPUB / UMD / CBZ 格式')
      upload.value?.clearFiles()
      return
    }
    overlay.importBookVisible = false
    overlay.openStorageImport('direct', files)
    upload.value?.clearFiles()
  }, 0)
}

function rejectOverflow() {
  clearSelectionTimer()
  upload.value?.clearFiles()
  ElMessage.error(`一次最多导入 ${maxFiles} 本书籍`)
}

function resetPicker() {
  clearSelectionTimer()
  upload.value?.clearFiles()
}

function clearSelectionTimer() {
  if (selectionTimer !== null) clearTimeout(selectionTimer)
  selectionTimer = null
}

onBeforeUnmount(resetPicker)
</script>

<style scoped>
.upload-icon {
  color: var(--app-primary);
  font-size: 32px;
}

.upload-text {
  color: var(--app-text-muted);
  overflow-wrap: anywhere;
}
</style>
