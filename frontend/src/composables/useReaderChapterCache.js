import { getCurrentInstance, onBeforeUnmount, ref, unref, watch } from 'vue'
import {
  cacheBookChaptersToBrowser,
  clearBookBrowserChapterCache,
  listBookBrowserCachedChapters,
} from '../utils/bookChapterCache.js'
import {
  readerChapterCacheStatus,
  readerChapterCacheTargets,
} from '../utils/readerChapterCache.js'
import { currentUserScope } from '../utils/authScope.js'

export function useReaderChapterCache(options) {
  const cacheChapters = options.cacheChapters || cacheBookChaptersToBrowser
  const clearCachedChapters = options.clearCachedChapters || clearBookBrowserChapterCache
  const listCachedChapters = options.listCachedChapters || listBookBrowserCachedChapters
  const cachedChapters = ref({})
  const caching = ref(false)
  const statusText = ref('')
  let activeJob = null

  function contextKey() {
    if (typeof options.getContextKey === 'function') return String(options.getContextKey())
    const currentBook = unref(options.book) || {}
    return [
      unref(options.bookId),
      currentBook.url || currentBook.bookUrl || currentBook.libraryPath || '',
      currentBook.sourceId || '',
    ].join('|')
  }

  function cacheScope() {
    return options.getCacheScope?.() || currentUserScope()
  }

  function currentSnapshot() {
    return {
      book: { ...(unref(options.book) || {}) },
      bookId: unref(options.bookId),
      contextKey: contextKey(),
      scope: cacheScope(),
    }
  }

  function snapshotIsCurrent(snapshot) {
    return snapshot.contextKey === contextKey() && snapshot.scope === cacheScope()
  }

  async function refresh(snapshot = currentSnapshot()) {
    if (unref(options.isTemporaryReader)) {
      if (snapshotIsCurrent(snapshot)) cachedChapters.value = {}
      return cachedChapters.value
    }
    try {
      const next = await listCachedChapters(
        snapshot.book,
        snapshot.bookId,
        { scope: snapshot.scope },
      )
      if (snapshotIsCurrent(snapshot)) cachedChapters.value = next
    } catch {
      if (snapshotIsCurrent(snapshot)) cachedChapters.value = {}
    }
    return cachedChapters.value
  }

  function markCached(index) {
    const targetIndex = Number(index)
    if (!Number.isInteger(targetIndex) || targetIndex < 0) return
    cachedChapters.value = { ...cachedChapters.value, [targetIndex]: true }
  }

  function reset() {
    cachedChapters.value = {}
  }

  async function cacheFollowing(count) {
    if (unref(options.isTemporaryReader)) {
      options.onUnavailable?.()
      return
    }
    if (caching.value) return
    const snapshot = currentSnapshot()
    const chapters = [...(unref(options.chapters) || [])]
    const currentIndex = Number(unref(options.currentIndex) || 0)
    const targets = readerChapterCacheTargets({
      chapterCount: chapters.length,
      currentIndex,
      count,
    })
    if (!targets.length) {
      options.onNoTargets?.()
      return
    }

    const job = { ...snapshot, cancelled: false }
    activeJob = job
    caching.value = true
    statusText.value = readerChapterCacheStatus(0, targets.length)
    try {
      const result = await cacheChapters(
        job.book,
        job.bookId,
        chapters,
        {
          startIndex: currentIndex + 1,
          count: count === true ? true : Number(count || 0),
          scope: job.scope,
          cancelled: () => job.cancelled || !snapshotIsCurrent(job),
          onProgress: ({ finished, total }) => {
            if (activeJob === job && snapshotIsCurrent(job)) {
              statusText.value = readerChapterCacheStatus(finished, total)
            }
          },
        },
      )
      if (activeJob === job && snapshotIsCurrent(job) && !result.cancelled) {
        options.notify?.('缓存完成')
      }
    } catch (error) {
      if (activeJob === job && snapshotIsCurrent(job) && !job.cancelled) {
        options.onError?.(error)
      }
    } finally {
      if (activeJob === job && snapshotIsCurrent(job)) {
        activeJob = null
        caching.value = false
        statusText.value = ''
        await refresh(job)
      }
    }
  }

  function cancel() {
    if (activeJob) activeJob.cancelled = true
    activeJob = null
    caching.value = false
    statusText.value = ''
  }

  async function clearBrowserCache() {
    if (unref(options.isTemporaryReader)) return 0
    const snapshot = currentSnapshot()
    const removed = await clearCachedChapters(
      snapshot.book,
      snapshot.bookId,
      { scope: snapshot.scope },
    )
    if (snapshotIsCurrent(snapshot)) {
      options.onClearMemory?.()
      reset()
    }
    return removed
  }

  const stopContextWatch = watch(contextKey, cancel, { flush: 'sync' })

  if (getCurrentInstance()) {
    onBeforeUnmount(() => {
      stopContextWatch()
      cancel()
    })
  }

  return {
    cachedChapters,
    caching,
    statusText,
    refresh,
    markCached,
    reset,
    cacheFollowing,
    cancel,
    clearBrowserCache,
  }
}
