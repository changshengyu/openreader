import { computed, isRef, ref, unref } from 'vue'
import { listBookSourceCandidates } from '../api/books'
import {
  buildBookSourceGroups,
  mergeBookSourceCandidates,
  nextBookSourcePage,
} from '../utils/bookSourceCandidates'

export function useBookSourceCandidates(options) {
  const candidates = ref([])
  const loading = ref(false)
  const group = ref('')
  const offset = ref(0)
  const hasMore = ref(true)
  const loadedKey = ref('')
  const limit = Math.max(1, Number(options.limit) || 10)
  let requestToken = 0

  const groups = computed(() => {
    const sourceRows = unref(options.groupSources)
    return buildBookSourceGroups(sourceRows?.length ? sourceRows : candidates.value)
  })

  async function ensureGroupSources() {
    const sourceRows = unref(options.groupSources)
    if (sourceRows?.length || !options.loadGroupSources) return
    try {
      const rows = await options.loadGroupSources()
      if (isRef(options.groupSources)) options.groupSources.value = Array.isArray(rows) ? rows : []
    } catch {
      if (isRef(options.groupSources)) options.groupSources.value = []
    }
  }

  async function load({ append = false, silent = false } = {}) {
    const id = Number(unref(options.bookId))
    if (!id || loading.value) return

    const token = ++requestToken
    const key = `${id}:${group.value || 'all'}`
    loading.value = true
    try {
      await ensureGroupSources()
      if (!append) {
        offset.value = 0
        hasMore.value = true
      }
      const { data } = await listBookSourceCandidates(id, {
        group: group.value || undefined,
        offset: offset.value,
        limit,
        paged: 1,
      })
      if (token !== requestToken) return

      const rows = Array.isArray(data) ? data : (data?.list || [])
      candidates.value = append ? mergeBookSourceCandidates(candidates.value, rows) : rows
      const nextPage = nextBookSourcePage(data, rows.length, offset.value, limit)
      offset.value = nextPage.offset
      hasMore.value = nextPage.hasMore
      loadedKey.value = key
    } catch (error) {
      if (!silent && token === requestToken) options.onError?.(error)
    } finally {
      if (token === requestToken) loading.value = false
    }
  }

  function ensure() {
    const key = `${Number(unref(options.bookId))}:${group.value || 'all'}`
    if (loadedKey.value === key && candidates.value.length) return
    return load()
  }

  function refresh(options = {}) {
    requestToken += 1
    loading.value = false
    loadedKey.value = ''
    hasMore.value = true
    return load(options)
  }

  function loadMore() {
    if (!hasMore.value) {
      options.onInfo?.('没有更多啦')
      return
    }
    return load({ append: true })
  }

  function changeGroup(value) {
    group.value = value || ''
    return refresh()
  }

  function reset({ clearGroup = false } = {}) {
    requestToken += 1
    candidates.value = []
    loading.value = false
    offset.value = 0
    hasMore.value = true
    loadedKey.value = ''
    if (clearGroup) group.value = ''
  }

  return {
    candidates,
    loading,
    group,
    hasMore,
    groups,
    ensure,
    load,
    refresh,
    loadMore,
    changeGroup,
    reset,
  }
}
