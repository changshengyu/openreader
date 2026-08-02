<template>
  <el-dialog
    v-if="isNormalPage"
    v-model="overlay.searchBookContentVisible"
    width="min(1000px, max(750px, 70vw))"
    top="max(15dvh, calc((100dvh - 584px) / 2))"
    :fullscreen="isMobile"
    class="global-content-search-dialog"
    @opened="handleOpened"
  >
    <template #header>
      <el-input
        v-model="keyword"
        class="content-search-title-input"
        size="small"
        :prefix-icon="Search"
        placeholder="搜索书籍内容"
        @keyup.enter="search"
      />
    </template>

    <div class="reader-dialog-table">
      <el-table
        ref="resultTableRef"
        :data="results"
        :height="isMobile ? 'calc(100dvh - 184px)' : 'min(400px, calc(70dvh - 184px))'"
        @row-click="jumpToResult"
      >
        <el-table-column property="chapterTitle" label="章节" min-width="100" />
        <el-table-column property="resultText" label="搜索结果" min-width="250" />
      </el-table>
      <el-alert
        v-if="searchNotice"
        class="reader-search-notice"
        type="warning"
        :closable="false"
        :title="searchNotice"
      />
    </div>

    <template #footer>
      <div class="reader-dialog-footer">
        <div class="reader-dialog-footer-left">
          <el-button
            type="primary"
            :disabled="loading"
            @click="loadMore"
          >
            {{ loading ? '加载中' : '加载更多' }}
          </el-button>
          <el-button v-if="!isMobile && hasMore" plain :disabled="loading" @click="loadAll">搜完全书</el-button>
          <el-button v-if="lastScrollTop > 0" type="primary" @click="restoreScrollTop">跳转上次位置</el-button>
        </div>
        <el-button @click="overlay.searchBookContentVisible = false">取消</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { Search } from '@element-plus/icons-vue'
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useAuthenticatedOperationGuard } from '../../composables/useAuthenticatedOperationGuard'
import { useBookContentSearch } from '../../composables/useBookContentSearch'
import { useOverlayStore } from '../../stores/overlay'
import { useReaderStore } from '../../stores/reader'
import { bookContentSearchBookIdentity } from '../../utils/readerBookSearch'

defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const overlay = useOverlayStore()
const reader = useReaderStore()
const operations = useAuthenticatedOperationGuard()
const resultTableRef = ref(null)
const lastScrollTop = ref(0)
const book = computed(() => overlay.searchBook)
const bookId = computed(() => overlay.searchBook?.id)
const isNormalPage = computed(() => reader.pageType === 'normal')
const activeBookKey = ref('')

const {
  keyword,
  results,
  loading,
  hasMore,
  incomplete,
  unavailableChapters,
  truncated,
  notice: searchNotice,
  cancel,
  reset,
  search,
  loadMore,
  loadAll,
} = useBookContentSearch({
  operationGuard: operations,
  bookId,
  book,
  chapters: [],
  onError: error => ElMessage.error(readError(error, '搜索正文失败')),
})

watch(
  () => bookContentSearchBookIdentity(overlay.searchBook),
  (key) => {
    if (key === activeBookKey.value) return
    activeBookKey.value = key
    resetSearch()
  },
)

watch(
  () => overlay.searchBookContentVisible,
  (visible) => {
    if (!visible) {
      cancel()
      return
    }
    const key = bookContentSearchBookIdentity(overlay.searchBook)
    if (!key || key === activeBookKey.value) return
    activeBookKey.value = key
    resetSearch()
  },
)

watch(isNormalPage, (normal) => {
  if (normal || !overlay.searchBookContentVisible) return
  overlay.searchBookContentVisible = false
  resetSearch()
})

onBeforeUnmount(cancel)

function resetSearch() {
  keyword.value = ''
  lastScrollTop.value = 0
  reset()
}

function jumpToResult(result) {
  captureScrollTop()
  overlay.requestSearchBookContentJump(result, keyword.value)
}

function getResultScrollElement() {
  return resultTableRef.value?.$el?.querySelector('.el-scrollbar__wrap') || null
}

function captureScrollTop() {
  lastScrollTop.value = Math.max(0, getResultScrollElement()?.scrollTop || 0)
}

function restoreScrollTop() {
  nextTick(() => {
    const scrollEl = getResultScrollElement()
    if (scrollEl) scrollEl.scrollTop = lastScrollTop.value
  })
}

function handleOpened() {
  restoreScrollTop()
}

function readError(error, fallback) {
  return error?.response?.data?.error?.message ||
    error?.response?.data?.error ||
    fallback
}
</script>

<style scoped>
.reader-dialog-table {
  position: relative;
}

.reader-dialog-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.reader-dialog-footer-left {
  display: flex;
  align-items: center;
  gap: 10px;
}

.reader-search-notice {
  position: absolute;
  right: 8px;
  bottom: 8px;
  left: 8px;
  z-index: 2;
}

.content-search-title-input {
  display: inline-block;
  width: 75%;
  margin: 0 auto;
  transform: translateX(20%);
}

@media (max-width: 750px) {
  .reader-dialog-footer-left {
    min-width: 0;
  }
}
</style>
