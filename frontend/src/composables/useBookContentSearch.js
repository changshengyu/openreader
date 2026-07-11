import { computed, ref, unref, watch } from 'vue'
import { searchBookContent } from '../api/books'
import {
  bookContentSearchMaxRounds,
  bookContentSearchNotice,
  bookContentSearchPagingParams,
  bookContentSearchStatus,
} from '../utils/readerBookSearch'

export function useBookContentSearch(options) {
  const keyword = ref('')
  const results = ref([])
  const loading = ref(false)
  const searched = ref(false)
  const lastIndex = ref(-1)
  const hasMore = ref(false)
  const total = ref(0)
  const incomplete = ref(false)
  const unavailableChapters = ref(0)
  const truncated = ref(false)
  let requestToken = 0

  const status = computed(() => bookContentSearchStatus({
    searched: searched.value,
    lastIndex: lastIndex.value,
    total: total.value,
    chapterCount: unref(options.chapters)?.length,
    resultCount: results.value.length,
  }))

  function reset() {
    requestToken += 1
    loading.value = false
    lastIndex.value = -1
    hasMore.value = false
    total.value = 0
    incomplete.value = false
    unavailableChapters.value = 0
    truncated.value = false
    searched.value = false
    results.value = []
  }

  async function search() {
    return run({ append: false })
  }

  async function loadMore() {
    return run({ append: true })
  }

  async function loadAll() {
    return run({ append: true, scanAll: true })
  }

  async function run({ append = false, scanAll = false } = {}) {
    const query = keyword.value.trim()
    if (!query || loading.value) return

    const token = ++requestToken
    const currentBook = unref(options.book)
    loading.value = true
    searched.value = true
    try {
      let cursor = append ? lastIndex.value : -1
      let nextResults = append ? [...results.value] : []
      const maxRounds = bookContentSearchMaxRounds({
        append,
        scanAll,
        remote: Number(currentBook?.sourceId || 0) > 0,
      })
      let previousCursor = cursor
      for (let round = 0; round < maxRounds; round += 1) {
        const { data } = await searchBookContent(unref(options.bookId), query, {
          paged: 1,
          lastIndex: cursor,
          scanUntilMatch: append ? 0 : 1,
          ...bookContentSearchPagingParams(currentBook),
        })
        if (token !== requestToken) return

        const rows = Array.isArray(data) ? data : (data?.list || [])
        nextResults = nextResults.concat(rows)
        results.value = nextResults
        lastIndex.value = Number.isInteger(data?.lastIndex) ? data.lastIndex : -1
        hasMore.value = Boolean(data?.hasMore)
        total.value = Number(data?.total || 0)
        const pageUnavailable = Math.max(0, Number(data?.unavailableChapters) || 0)
        unavailableChapters.value = append
          ? unavailableChapters.value + pageUnavailable
          : pageUnavailable
        truncated.value = append
          ? truncated.value || Boolean(data?.truncated)
          : Boolean(data?.truncated)
        incomplete.value = Boolean(data?.incomplete) ||
          unavailableChapters.value > 0 ||
          truncated.value
        cursor = lastIndex.value

        if (!scanAll && (rows.length || !hasMore.value)) break
        if (scanAll && (!hasMore.value || cursor <= previousCursor)) break
        previousCursor = cursor
      }
    } catch (error) {
      if (token === requestToken) options.onError?.(error)
    } finally {
      if (token === requestToken) loading.value = false
    }
  }

  watch(keyword, reset)

  return {
    keyword,
    results,
    loading,
    searched,
    hasMore,
    incomplete,
    unavailableChapters,
    truncated,
    notice: computed(() => bookContentSearchNotice({
      incomplete: incomplete.value,
      unavailableChapters: unavailableChapters.value,
      truncated: truncated.value,
    })),
    status,
    reset,
    search,
    loadMore,
    loadAll,
  }
}
