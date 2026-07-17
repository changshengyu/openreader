<template>
  <el-dialog
    v-model="overlay.searchBookContentVisible"
    :width="dialogWidth"
    :fullscreen="isMobile"
    class="global-content-search-dialog"
    @opened="handleOpened"
  >
    <template #header>
      <el-input
        ref="searchInputRef"
        v-model="keyword"
        placeholder="搜索书籍内容"
        clearable
        @keyup.enter="search"
      />
    </template>

    <div v-loading="loading" class="reader-dialog-table">
      <el-table
        ref="resultTableRef"
        :data="results"
        max-height="520"
        @row-click="jumpToResult"
      >
        <el-table-column prop="chapterTitle" label="章节" min-width="150" />
        <el-table-column label="搜索结果" min-width="300">
          <template #default="scope">
            {{ scope.row.excerpt || scope.row.resultText || '—' }}
          </template>
        </el-table-column>
      </el-table>
      <el-alert
        v-if="searchNotice"
        class="reader-search-notice"
        type="warning"
        :closable="false"
        :title="searchNotice"
      />
      <el-empty
        v-if="keyword && searched && !loading && !results.length"
        :description="searchNotice || (hasMore ? '当前已搜索章节没有匹配，可继续搜索后续章节' : '没有匹配内容')"
      />
      <el-empty v-else-if="!keyword && !results.length" description="输入关键词搜索整本书正文" />
    </div>

    <template #footer>
      <div class="reader-dialog-footer">
        <el-button
          type="primary"
          :loading="loading"
          :disabled="!hasMore"
          @click="loadMore"
        >
          {{ loading ? '加载中' : (hasMore ? '加载更多' : '没有更多') }}
        </el-button>
        <el-button v-if="hasMore" plain :loading="loading" @click="loadAll">搜完全书</el-button>
        <el-button v-if="lastScrollTop > 0" @click="restoreScrollTop">跳转上次位置</el-button>
        <el-button @click="overlay.searchBookContentVisible = false">取消</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useBookContentSearch } from '../../composables/useBookContentSearch'
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
const searchInputRef = ref(null)
const resultTableRef = ref(null)
const lastScrollTop = ref(0)
const book = computed(() => overlay.searchBook)
const bookId = computed(() => overlay.searchBook?.id)
const activeBookKey = ref('')

const {
  keyword,
  results,
  loading,
  searched,
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
  bookId,
  book,
  chapters: [],
  onError: error => ElMessage.error(readError(error, '搜索正文失败')),
})

watch(
  () => overlay.searchBook?.id || overlay.searchBook?.bookUrl || '',
  (key) => {
    const nextKey = String(key || '')
    if (nextKey === activeBookKey.value) return
    activeBookKey.value = nextKey
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
    const key = String(
      overlay.searchBook?.id ||
      overlay.searchBook?.bookUrl ||
      '',
    )
    if (!key || key === activeBookKey.value) return
    activeBookKey.value = key
    resetSearch()
  },
)

onBeforeUnmount(cancel)

function resetSearch() {
  keyword.value = ''
  lastScrollTop.value = 0
  reset()
}

function jumpToResult(result) {
  captureScrollTop()
  const currentBook = overlay.searchBook
  if (!currentBook?.id) return
  overlay.searchBookContentVisible = false
  router.push({
    name: 'reader',
    params: { id: currentBook.id },
    query: {
      chapter: Number(result.chapterIndex || 0),
      line: Number.isInteger(result.lineIndex)
        ? result.lineIndex
        : undefined,
      match: Number.isInteger(result.resultCountWithinChapter)
        ? result.resultCountWithinChapter
        : undefined,
      percent: Number.isFinite(Number(result.percent))
        ? Number(result.percent)
        : undefined,
      q: keyword.value.trim() || undefined,
    },
  })
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
  nextTick(() => {
    searchInputRef.value?.focus?.()
    restoreScrollTop()
  })
}

function readError(error, fallback) {
  return error?.response?.data?.error?.message ||
    error?.response?.data?.error ||
    fallback
}
</script>

<style scoped>
.reader-dialog-table {
  min-height: 220px;
}

.reader-dialog-footer {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 10px;
}

.reader-search-notice {
  margin-top: 12px;
}

@media (max-width: 750px) {
  .reader-dialog-table {
    min-height: 0;
  }

  .reader-dialog-footer > * {
    flex: 1 1 auto;
  }
}
</style>
