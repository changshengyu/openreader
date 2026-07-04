<template>
  <el-drawer
    v-model="overlay.searchBookContentVisible"
    :title="`搜索正文${overlay.searchBook?.title ? ` · ${overlay.searchBook.title}` : ''}`"
    :direction="direction"
    :size="size"
    class="global-search-drawer"
  >
    <ReaderSearchPanel
      v-model="keyword"
      :results="results"
      :loading="loading"
      :searched="searched"
      :has-more="hasMore"
      :status-text="status"
      @search="search"
      @load-more="loadMore"
      @load-all="loadAll"
      @jump="jumpToResult"
    />
  </el-drawer>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { useBookContentSearch } from '../../composables/useBookContentSearch'
import { useOverlayStore } from '../../stores/overlay'
import ReaderSearchPanel from '../reader/ReaderSearchPanel.vue'

defineProps({
  direction: {
    type: String,
    required: true,
  },
  size: {
    type: [String, Number],
    required: true,
  },
})

const router = useRouter()
const overlay = useOverlayStore()
const book = computed(() => overlay.searchBook)
const bookId = computed(() => overlay.searchBook?.id)
const activeBookKey = ref('')

const {
  keyword,
  results,
  loading,
  searched,
  hasMore,
  status,
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
    if (!visible) return
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

function resetSearch() {
  keyword.value = ''
  reset()
}

function jumpToResult(result) {
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

function readError(error, fallback) {
  return error?.response?.data?.error?.message ||
    error?.response?.data?.error ||
    fallback
}
</script>
