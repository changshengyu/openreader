<template>
  <el-dialog
    :model-value="overlay.bookmarkFormVisible"
    title="书签"
    width="640px"
    :fullscreen="isMobile"
    class="global-bookmark-form-dialog"
    @update:model-value="handleVisibleChange"
    @closed="overlay.clearBookmarkForm()"
  >
    <el-form v-if="draft" label-width="64px" class="bookmark-form">
      <el-form-item label="书名">
        <el-input :model-value="bookTitle" readonly />
      </el-form-item>
      <el-form-item label="作者">
        <el-input :model-value="bookAuthor" readonly />
      </el-form-item>
      <el-form-item label="章节">
        <el-input :model-value="draft.title || '—'" readonly />
      </el-form-item>
      <el-form-item label="内容">
        <el-input
          :model-value="draft.excerpt || '—'"
          type="textarea"
          :rows="5"
          readonly
        />
      </el-form-item>
      <el-form-item label="备注">
        <el-input
          v-model="draft.note"
          type="textarea"
          :rows="3"
          placeholder="写下笔记（可选）"
        />
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="overlay.finishBookmarkForm({ saved: false, reason: 'cancel' })">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">确定</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { createBookmark, updateBookmark } from '../../api/books'
import { useOverlayStore } from '../../stores/overlay'

defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const overlay = useOverlayStore()
const saving = ref(false)
const draft = computed(() => overlay.bookmarkFormDraft)
const book = computed(() => overlay.bookmarkFormBook)
const bookTitle = computed(() => book.value?.title || book.value?.name || '—')
const bookAuthor = computed(() => book.value?.author || '—')

function handleVisibleChange(visible) {
  if (!visible) overlay.finishBookmarkForm({ saved: false, reason: 'close' })
}

async function save() {
  const currentBook = book.value
  const currentDraft = draft.value
  if (!currentBook?.id) {
    ElMessage.error('书籍信息错误')
    return
  }
  if (!String(currentDraft?.excerpt || '').trim()) {
    ElMessage.error('书签内容不能为空')
    return
  }

  saving.value = true
  try {
    const payload = bookmarkPayload(currentDraft)
    let bookmark
    if (overlay.bookmarkFormMode === 'edit' && currentDraft.id) {
      const { data } = await updateBookmark(currentDraft.id, payload)
      bookmark = data
      ElMessage.success('编辑书签成功')
    } else {
      const { data } = await createBookmark(currentBook.id, payload)
      bookmark = data
      ElMessage.success('新增书签成功')
    }
    dispatchBookmarksUpdated(currentBook.id)
    overlay.finishBookmarkForm({
      saved: true,
      bookmarkId: bookmark?.id || currentDraft.id,
    })
  } catch (error) {
    ElMessage.error(readError(error, `${overlay.bookmarkFormMode === 'edit' ? '编辑' : '新增'}书签失败`))
  } finally {
    saving.value = false
  }
}

function bookmarkPayload(source = {}) {
  return {
    chapterId: source.chapterId,
    chapterIndex: Number(source.chapterIndex || 0),
    offset: Math.max(0, Number(source.offset || 0)),
    percent: Number.isFinite(Number(source.percent)) ? Number(source.percent) : 0,
    title: source.title || '',
    excerpt: source.excerpt || '',
    note: source.note || '',
  }
}

function dispatchBookmarksUpdated(bookId) {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent('openreader:bookmarks-updated', {
    detail: { bookIds: [bookId] },
  }))
}

function readError(error, fallback) {
  return error?.response?.data?.error?.message ||
    error?.response?.data?.error ||
    fallback
}
</script>

<style scoped>
.bookmark-form :deep(.el-textarea__inner) {
  resize: vertical;
}
</style>
