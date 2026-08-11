import { computed, isRef, reactive, ref, unref } from 'vue'
import { listBookSourceCandidates } from '../api/books.js'
import {
  buildBookSourceGroups,
  mergeBookSourceCandidates,
  nextBookSourcePage,
} from '../utils/bookSourceCandidates.js'
import { sourceCandidateKey } from '../utils/sourceCandidate.js'

export function useBookSourceCandidates(options) {
  const candidates = ref([])
  const opening = ref(false)
  const refreshing = ref(false)
  const loadingMore = ref(false)
  const group = ref('')
  const loadedBookId = ref(0)
  const limit = Math.max(1, Number(options.limit) || 10)
  const pageByGroup = reactive(new Map())
  const requestCandidates = options.listCandidates || listBookSourceCandidates
  let generation = 0

  const groups = computed(() => {
    const sourceRows = unref(options.groupSources)
    return buildBookSourceGroups(sourceRows?.length ? sourceRows : candidates.value)
  })
  const loading = computed(() => opening.value || refreshing.value || loadingMore.value)
  const hasMore = computed(() => pageState(group.value).hasMore)

  function groupKey(value = group.value) {
    return value || '__all__'
  }

  function pageState(value = group.value) {
    const key = groupKey(value)
    if (!pageByGroup.has(key)) {
      pageByGroup.set(key, { offset: 0, hasMore: true })
    }
    return pageByGroup.get(key)
  }

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

  async function open({ silent = false } = {}) {
    const id = Number(unref(options.bookId))
    if (!id || opening.value) return
    if (loadedBookId.value === id) return

    const token = generation
    opening.value = true
    try {
      await ensureGroupSources()
      const { data } = await requestCandidates(id, { mode: 'available' })
      if (token !== generation || Number(unref(options.bookId)) !== id) return
      candidates.value = Array.isArray(data) ? data : (data?.list || [])
      loadedBookId.value = id
    } catch (error) {
      if (!silent && token === generation) options.onError?.(error)
    } finally {
      if (token === generation) opening.value = false
    }
  }

  function ensure(options = {}) {
    return open(options)
  }

  async function refresh({ silent = false } = {}) {
    const id = Number(unref(options.bookId))
    if (!id || opening.value || refreshing.value || loadingMore.value) return

    const token = generation
    refreshing.value = true
    try {
      await ensureGroupSources()
      const { data } = await requestCandidates(id, { mode: 'refresh' })
      if (token !== generation || Number(unref(options.bookId)) !== id) return
      candidates.value = Array.isArray(data) ? data : (data?.list || [])
      loadedBookId.value = id
    } catch (error) {
      if (!silent && token === generation) options.onError?.(error)
    } finally {
      if (token === generation) refreshing.value = false
    }
  }

  async function loadMore({ silent = false } = {}) {
    const id = Number(unref(options.bookId))
    const state = pageState()
    if (!id || opening.value || refreshing.value || loadingMore.value) return
    if (!state.hasMore) {
      options.onInfo?.('没有更多啦')
      return
    }

    const token = generation
    const requestedGroup = group.value
    loadingMore.value = true
    try {
      await ensureGroupSources()
      const { data } = await requestCandidates(id, {
        mode: 'search',
        group: requestedGroup || undefined,
        offset: state.offset,
        limit,
        paged: 1,
      })
      if (token !== generation || Number(unref(options.bookId)) !== id) return
      const rows = Array.isArray(data) ? data : (data?.list || [])
      candidates.value = mergeBookSourceCandidates(candidates.value, rows)
      const nextPage = nextBookSourcePage(data, rows.length, state.offset, limit)
      state.offset = nextPage.offset
      state.hasMore = nextPage.hasMore
      loadedBookId.value = id
      if (!rows.length && !state.hasMore) options.onInfo?.('没有更多啦')
    } catch (error) {
      if (!silent && token === generation) options.onError?.(error)
    } finally {
      if (token === generation) loadingMore.value = false
    }
  }

  function changeGroup(value) {
    group.value = value || ''
    pageState(group.value)
  }

  function applyChangedSource(candidate) {
    const selectedKey = sourceCandidateKey(candidate)
    let found = false
    candidates.value = candidates.value.map((item) => {
      const selected = sourceCandidateKey(item) === selectedKey
      if (selected) found = true
      return selected ? { ...item, ...candidate, current: true } : { ...item, current: false }
    })
    if (!found && selectedKey) {
      candidates.value = [...candidates.value, { ...candidate, current: true }]
    }
  }

  function reset({ clearGroup = false } = {}) {
    generation += 1
    candidates.value = []
    opening.value = false
    refreshing.value = false
    loadingMore.value = false
    loadedBookId.value = 0
    pageByGroup.clear()
    if (clearGroup) group.value = ''
  }

  return {
    candidates,
    loading,
    opening,
    refreshing,
    loadingMore,
    group,
    hasMore,
    groups,
    open,
    ensure,
    refresh,
    loadMore,
    changeGroup,
    applyChangedSource,
    reset,
  }
}
