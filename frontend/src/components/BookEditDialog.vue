<template>
  <el-dialog
    :model-value="modelValue"
    title="编辑书籍"
    width="540px"
    :fullscreen="isMobile"
    @update:model-value="emit('update:modelValue', $event)"
    @open="resetDraft"
  >
    <el-form label-position="top" class="book-editor">
      <el-form-item label="书名">
        <el-input v-model="draft.title" />
      </el-form-item>
      <el-form-item label="作者">
        <el-input v-model="draft.author" />
      </el-form-item>
      <el-form-item label="自定义封面">
        <div class="cover-upload-row">
          <el-input v-model="draft.customCoverUrl" placeholder="封面地址或上传本地图片" />
          <el-upload
            accept="image/jpg,image/png,image/jpeg"
            :show-file-list="false"
            :auto-upload="false"
            @change="uploadCover"
          >
            <el-button :loading="uploadingCover">上传</el-button>
          </el-upload>
        </div>
      </el-form-item>
      <el-form-item label="简介">
        <el-input v-model="draft.intro" type="textarea" :rows="5" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" :loading="saving" @click="submit">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { uploadAsset } from '../api/uploads'
import { useReaderStore } from '../stores/reader'
import { currentViewportWidth, shouldUseMiniInterface } from '../utils/responsive'

const props = defineProps({
  modelValue: {
    type: Boolean,
    default: false,
  },
  book: {
    type: Object,
    default: null,
  },
  saving: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['update:modelValue', 'save'])
const reader = useReaderStore()
const windowWidth = ref(currentViewportWidth())
const uploadingCover = ref(false)
const draft = reactive({
  title: '',
  author: '',
  customCoverUrl: '',
  intro: '',
})

const isMobile = computed(() => shouldUseMiniInterface(reader.pageMode, windowWidth.value))

watch(
  () => [props.modelValue, props.book],
  ([visible]) => {
    if (visible) resetDraft()
  },
)

onMounted(() => window.addEventListener('resize', updateWindowWidth, { passive: true }))
onBeforeUnmount(() => window.removeEventListener('resize', updateWindowWidth))

function updateWindowWidth() {
  windowWidth.value = currentViewportWidth()
}

function resetDraft() {
  Object.assign(draft, {
    title: props.book?.title || props.book?.name || '',
    author: props.book?.author || '',
    customCoverUrl: props.book?.customCoverUrl || '',
    intro: props.book?.intro || '',
  })
}

async function uploadCover(data) {
  const file = data.raw || data.file
  if (!file) return
  uploadingCover.value = true
  try {
    const { data: result } = await uploadAsset({ file, type: 'cover' })
    draft.customCoverUrl = result.url
    ElMessage.success('封面已上传')
  } catch (err) {
    ElMessage.error(readError(err, '上传封面失败'))
  } finally {
    uploadingCover.value = false
  }
}

function submit() {
  const title = draft.title.trim()
  if (!title) {
    ElMessage.warning('书名不能为空')
    return
  }
  emit('save', {
    title,
    author: draft.author.trim(),
    customCoverUrl: draft.customCoverUrl.trim(),
    intro: draft.intro,
  })
}

function readError(err, fallback) {
  return err?.response?.data?.error?.message || err?.response?.data?.error || fallback
}
</script>

<style scoped>
.book-editor {
  display: grid;
}

.cover-upload-row {
  display: flex;
  width: 100%;
  gap: 8px;
}

.cover-upload-row .el-input {
  min-width: 0;
  flex: 1;
}

@media (max-width: 750px) {
  .cover-upload-row {
    align-items: stretch;
  }
}
</style>
