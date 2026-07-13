import { onBeforeUnmount, ref, unref } from 'vue'
import {
  cacheBookChaptersToBrowser,
  clearBookBrowserChapterCache,
  listBookBrowserCachedChapters,
} from '../utils/bookChapterCache'
import {
  readerChapterCacheStatus,
  readerChapterCacheTargets,
} from '../utils/readerChapterCache'

export function useReaderChapterCache(options) {
  const cachedChapters = ref({})
  const caching = ref(false)
  const statusText = ref('')
  let cancelled = false

  async function refresh() {
    if (unref(options.isTemporaryReader)) {
      cachedChapters.value = {}
      return cachedChapters.value
    }
    try {
      cachedChapters.value = await listBookBrowserCachedChapters(
        unref(options.book),
        unref(options.bookId),
      )
    } catch {
      cachedChapters.value = {}
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
    if (!unref(options.isRemoteBook) || caching.value) return
    await refresh()
    const chapters = unref(options.chapters) || []
    const currentIndex = Number(unref(options.currentIndex) || 0)
    const targets = readerChapterCacheTargets({
      chapterCount: chapters.length,
      currentIndex,
      count,
      cachedMap: cachedChapters.value,
    })
    if (!targets.length) {
      options.onNoTargets?.()
      return
    }

    caching.value = true
    cancelled = false
    statusText.value = readerChapterCacheStatus(0, targets.length)
    try {
      const result = await cacheBookChaptersToBrowser(
        unref(options.book),
        unref(options.bookId),
        chapters,
        {
          startIndex: currentIndex + 1,
          count: count === true ? true : Number(count || 0),
          cancelled: () => cancelled,
          onProgress: ({ finished, total }) => {
            statusText.value = readerChapterCacheStatus(finished, total)
          },
        },
      )
      options.notify?.(
        result.cancelled
          ? `已取消，缓存 ${result.cached} 章`
          : `缓存完成：${result.cached} 章`,
      )
    } catch (error) {
      options.onError?.(error)
    } finally {
      caching.value = false
      statusText.value = ''
      cancelled = false
      await refresh()
      await options.afterCache?.()
    }
  }

  function cancel() {
    cancelled = true
    statusText.value = '正在取消缓存...'
  }

  async function clearBrowserCache() {
    if (unref(options.isTemporaryReader)) return 0
    const removed = await clearBookBrowserChapterCache(
      unref(options.book),
      unref(options.bookId),
    )
    options.onClearMemory?.()
    reset()
    return removed
  }

  onBeforeUnmount(() => {
    cancelled = true
  })

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
