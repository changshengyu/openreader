import { onBeforeUnmount, ref, unref } from 'vue'
import {
  createBookmark,
  createBookmarks,
  deleteBookmark,
  deleteBookmarks,
  listBookmarks,
  updateBookmark,
} from '../api/books'
import {
  bookmarkUpdateTargetsBook,
  prependBookmarks,
  removeBookmarkIds,
  replaceBookmark,
} from '../utils/bookmark'

export function useBookBookmarks(options) {
  const items = ref([])
  const loading = ref(false)
  const mutating = ref(false)
  let refreshTimer
  let loadToken = 0

  async function load(targetBookId = unref(options.bookId)) {
    const id = Number(targetBookId)
    if (!id) return []
    const token = ++loadToken
    loading.value = true
    try {
      const { data } = await listBookmarks(id)
      const rows = Array.isArray(data) ? data : []
      if (token === loadToken && String(unref(options.bookId)) === String(id)) {
        items.value = rows
      }
      return rows
    } finally {
      if (token === loadToken) loading.value = false
    }
  }

  function reset() {
    loadToken += 1
    loading.value = false
    items.value = []
  }

  async function create(payload) {
    const id = Number(unref(options.bookId))
    if (!id) return null
    mutating.value = true
    try {
      const { data } = await createBookmark(id, payload)
      if (data && String(unref(options.bookId)) === String(id)) {
        items.value = prependBookmarks(items.value, [data])
      }
      return data || null
    } finally {
      mutating.value = false
    }
  }

  async function update(bookmarkId, payload) {
    if (!bookmarkId) return null
    const id = Number(unref(options.bookId))
    mutating.value = true
    try {
      const { data } = await updateBookmark(bookmarkId, payload)
      if (data && String(unref(options.bookId)) === String(id)) {
        items.value = replaceBookmark(items.value, data)
      }
      return data || null
    } finally {
      mutating.value = false
    }
  }

  async function remove(bookmarkId) {
    if (!bookmarkId) return
    const id = Number(unref(options.bookId))
    mutating.value = true
    try {
      await deleteBookmark(bookmarkId)
      if (String(unref(options.bookId)) === String(id)) {
        items.value = removeBookmarkIds(items.value, [bookmarkId])
      }
    } finally {
      mutating.value = false
    }
  }

  async function removeMany(rows) {
    const id = Number(unref(options.bookId))
    const bookmarkIds = (Array.isArray(rows) ? rows : []).map(item => item.id).filter(Boolean)
    if (!id || !bookmarkIds.length) return []
    mutating.value = true
    try {
      const { data } = await deleteBookmarks(id, bookmarkIds)
      const deletedIds = Array.isArray(data?.deletedIds) ? data.deletedIds : []
      if (String(unref(options.bookId)) === String(id)) {
        items.value = removeBookmarkIds(items.value, deletedIds)
      }
      return deletedIds
    } finally {
      mutating.value = false
    }
  }

  async function importPayloads(payloads) {
    const id = Number(unref(options.bookId))
    if (!id || !Array.isArray(payloads) || !payloads.length) return []
    mutating.value = true
    try {
      const { data } = await createBookmarks(id, payloads)
      const created = Array.isArray(data) ? data : []
      if (String(unref(options.bookId)) === String(id)) {
        items.value = prependBookmarks(items.value, created)
      }
      return created
    } finally {
      mutating.value = false
    }
  }

  function handleUpdated(event) {
    const id = unref(options.bookId)
    if (options.isActive && !options.isActive()) return
    if (!bookmarkUpdateTargetsBook(event, id)) return
    scheduleRefresh()
  }

  function scheduleRefresh() {
    clearScheduledRefresh()
    refreshTimer = setTimeout(() => {
      refreshTimer = undefined
      load().catch(error => options.onLoadError?.(error))
    }, 250)
  }

  function clearScheduledRefresh() {
    if (!refreshTimer) return
    clearTimeout(refreshTimer)
    refreshTimer = undefined
  }

  onBeforeUnmount(clearScheduledRefresh)

  return {
    items,
    loading,
    mutating,
    load,
    reset,
    create,
    update,
    remove,
    removeMany,
    importPayloads,
    handleUpdated,
  }
}
