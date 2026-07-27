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
  appendBookmarks,
  bookmarkUpdateTargetsBook,
  removeBookmarkIds,
  replaceBookmark,
} from '../utils/bookmark'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation.js'

export function useBookBookmarks(options) {
  const operations = options.operationGuard || createAuthenticatedOperationGuard({
    getIdentity: options.getAuthenticatedIdentity,
  })
  const items = ref([])
  const loading = ref(false)
  const mutating = ref(false)
  const trackItems = options.trackItems !== false
  let refreshTimer
  let loadToken = 0

  async function load(targetBookId = unref(options.bookId)) {
    const id = Number(targetBookId)
    if (!id) return []
    const token = ++loadToken
    const operation = operations.begin('load')
    loading.value = true
    try {
      const { data } = await listBookmarks(id)
      if (!operations.canCommit(operation)) return []
      const rows = Array.isArray(data) ? data : []
      if (trackItems && token === loadToken && String(unref(options.bookId)) === String(id)) {
        items.value = rows
      }
      return rows
    } finally {
      if (token === loadToken && operations.canCommit(operation)) loading.value = false
    }
  }

  function reset() {
    operations.reset()
    loadToken += 1
    loading.value = false
    if (trackItems) items.value = []
  }

  async function create(payload) {
    const id = Number(unref(options.bookId))
    if (!id) return null
    const operation = operations.begin('create')
    mutating.value = true
    try {
      const { data } = await createBookmark(id, payload)
      if (!operations.canCommit(operation)) return null
      if (trackItems && data && String(unref(options.bookId)) === String(id)) {
        items.value = appendBookmarks(items.value, [data])
      }
      return data || null
    } finally {
      if (operations.canCommit(operation)) mutating.value = false
    }
  }

  async function update(bookmarkId, payload) {
    if (!bookmarkId) return null
    const id = Number(unref(options.bookId))
    const operation = operations.begin(`update:${bookmarkId}`)
    mutating.value = true
    try {
      const { data } = await updateBookmark(bookmarkId, payload)
      if (!operations.canCommit(operation)) return null
      if (trackItems && data && String(unref(options.bookId)) === String(id)) {
        items.value = replaceBookmark(items.value, data)
      }
      return data || null
    } finally {
      if (operations.canCommit(operation)) mutating.value = false
    }
  }

  async function remove(bookmarkId) {
    if (!bookmarkId) return
    const id = Number(unref(options.bookId))
    const operation = operations.begin(`remove:${bookmarkId}`)
    mutating.value = true
    try {
      await deleteBookmark(bookmarkId)
      if (!operations.canCommit(operation)) return
      if (trackItems && String(unref(options.bookId)) === String(id)) {
        items.value = removeBookmarkIds(items.value, [bookmarkId])
      }
    } finally {
      if (operations.canCommit(operation)) mutating.value = false
    }
  }

  async function removeMany(rows) {
    const id = Number(unref(options.bookId))
    const bookmarkIds = (Array.isArray(rows) ? rows : []).map(item => item.id).filter(Boolean)
    if (!id || !bookmarkIds.length) return []
    const operation = operations.begin('remove-many')
    mutating.value = true
    try {
      const { data } = await deleteBookmarks(id, bookmarkIds)
      if (!operations.canCommit(operation)) return []
      const deletedIds = Array.isArray(data?.deletedIds) ? data.deletedIds : []
      if (trackItems && String(unref(options.bookId)) === String(id)) {
        items.value = removeBookmarkIds(items.value, deletedIds)
      }
      return deletedIds
    } finally {
      if (operations.canCommit(operation)) mutating.value = false
    }
  }

  async function importPayloads(payloads) {
    const id = Number(unref(options.bookId))
    if (!id || !Array.isArray(payloads) || !payloads.length) return []
    const operation = operations.begin('import')
    mutating.value = true
    try {
      const { data } = await createBookmarks(id, payloads)
      if (!operations.canCommit(operation)) return []
      const created = Array.isArray(data) ? data : []
      if (trackItems && String(unref(options.bookId)) === String(id)) {
        items.value = appendBookmarks(items.value, created)
      }
      return created
    } finally {
      if (operations.canCommit(operation)) mutating.value = false
    }
  }

  function handleUpdated(event) {
    if (!trackItems) return
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
    resetOperations: operations.reset,
  }
}
