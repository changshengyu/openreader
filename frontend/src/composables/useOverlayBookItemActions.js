import { computed, reactive, ref } from 'vue'
import { currentUserScope } from '../utils/authScope.js'

// Cache jobs outlive the BookManage dialog body. The dialog uses
// destroy-on-close, while upstream keeps per-book jobs visible after reopen.
const cacheJobs = reactive(new Map())
const serverControllers = new Map()

export function cancelAllBookManagementCacheJobs() {
  for (const token of cacheJobs.values()) {
    if (token.kind === 'browser') token.cancelled = true
  }
  for (const controller of serverControllers.values()) controller.abort()
  cacheJobs.clear()
  serverControllers.clear()
}

function jobKey(bookId) {
  return `${currentUserScope()}:${bookId}`
}

function isCancelled(error) {
  return error === 'cancel' || error === 'close'
}

function isCacheAbort(error) {
  return error?.name === 'AbortError' || error?.code === 'ERR_CANCELED'
}

export function useOverlayBookItemActions(options, sharedState = {}) {
  const batchBusy = sharedState.batchBusy || ref(false)
  const cachingBookId = computed(() => {
    const scopePrefix = `${currentUserScope()}:`
    const key = [...cacheJobs.keys()].find(value => value.startsWith(scopePrefix))
    return key ? Number(key.slice(scopePrefix.length)) : null
  })

  function currentJob(book) {
    return cacheJobs.get(jobKey(book?.id)) || null
  }

  function isCachingBook(book) {
    return Boolean(currentJob(book))
  }

  function setJobProgress(book, token, progress) {
    const key = jobKey(book.id)
    if (cacheJobs.get(key) !== token) return
    token.progress = { ...progress }
  }

  function cacheProgressLabel(book) {
    const progress = currentJob(book)?.progress
    if (!progress) return ''
    const total = Number(progress.total || 0)
    const processed = Number(progress.processed ?? progress.requested ?? progress.finished ?? 0)
    if (!total) return '准备中'
    return `${processed}/${total}`
  }

  async function cacheBook(book, command) {
    if (
      Number(book?.sourceId || 0) === 0 &&
      command !== 'cacheBookLocal' &&
      command !== 'deleteBookLocalCache'
    ) {
      options.onInfo('本地书无需服务器缓存')
      return
    }
    if (command === 'deleteBookCache') {
      await clearBookCache(book)
      return
    }
    if (command === 'deleteBookLocalCache') {
      await clearBookLocalCache(book)
      return
    }
    if (currentJob(book)) {
      cancelBookCache(book)
      return
    }
    if (command === 'cacheBookLocal') {
      await cacheBookLocal(book)
      return
    }
    await cacheBookServer(book)
  }

  async function cacheBookServer(book) {
    const key = jobKey(book.id)
    const controller = typeof options.cacheBookContentStream === 'function' && typeof AbortController !== 'undefined'
      ? new AbortController()
      : null
    const token = reactive({ kind: 'server', progress: { processed: 0, total: 0 } })
    cacheJobs.set(key, token)
    if (controller) serverControllers.set(key, controller)
    try {
      const payload = { all: true, chapterIndex: 0, refresh: false }
      const data = typeof options.cacheBookContentStream === 'function'
        ? await options.cacheBookContentStream(book.id, payload, {
          signal: controller?.signal,
          onEvent: ({ event, data: progress }) => {
            if (event === 'message') setJobProgress(book, token, progress)
          },
        })
        : (await options.cacheBookContent(book.id, payload)).data
      if (data?.book) options.bookshelf.upsertBook(data.book)
      const cached = Number(data?.cachedCount ?? data?.cached ?? 0)
      const total = Number(data?.total ?? data?.requested ?? 0)
      const failed = Number(data?.failedCount ?? data?.failed ?? 0)
      options.onSuccess(`已缓存 ${cached}/${total} 章${failed ? `，失败 ${failed} 章` : ''}`)
    } catch (error) {
      if (isCacheAbort(error)) options.onInfo('已取消服务器缓存')
      else options.onError(error, '缓存失败')
    } finally {
      if (cacheJobs.get(key) === token) cacheJobs.delete(key)
      if (serverControllers.get(key) === controller) serverControllers.delete(key)
    }
  }

  async function cacheBookLocal(book) {
    const key = jobKey(book.id)
    const token = reactive({ kind: 'browser', cancelled: false, progress: { processed: 0, total: 0 } })
    cacheJobs.set(key, token)
    try {
      const { data } = await options.listChapters(book.id)
      if (token.cancelled) {
        options.onInfo('已取消浏览器缓存')
        return
      }
      const result = await options.cacheBrowserChapters(
        book,
        book.id,
        Array.isArray(data) ? data : [],
        {
          startIndex: 0,
          count: true,
          concurrency: 2,
          cancelled: () => token.cancelled,
          onProgress: progress => setJobProgress(book, token, {
            ...progress,
            processed: progress.finished,
          }),
        },
      )
      if (result.cancelled || token.cancelled) {
        options.onInfo('已取消浏览器缓存')
      } else {
        options.onSuccess(`已缓存到浏览器 ${result.cached}/${result.requested} 章`)
      }
      await refreshBrowserCacheCounts()
    } catch (error) {
      options.onError(error, '缓存到浏览器失败')
    } finally {
      if (cacheJobs.get(key) === token) cacheJobs.delete(key)
    }
  }

  function cancelBookCache(book) {
    const key = jobKey(book?.id)
    const token = cacheJobs.get(key)
    if (!token) return false
    if (token.kind === 'server') {
      const controller = serverControllers.get(key)
      if (!controller) return false
      controller.abort()
      options.onInfo('正在停止服务器缓存')
      return true
    }
    token.cancelled = true
    options.onInfo('正在停止浏览器缓存')
    return true
  }

  // Compatibility name for callers outside the aligned BookManage surface.
  function cancelServerCache(book) {
    return cancelBookCache(book)
  }

  async function clearBookCache(book) {
    try {
      await options.confirm(
        `确认要删除服务器上《${book.title}》的缓存章节吗？`,
        '提示',
        { type: 'warning' },
      )
      const data = await options.bookshelf.batchClearCache([book.id])
      options.updateServerCacheCount(book, 0)
      options.onSuccess(`已清理 ${data.cleared || 0} 个章节缓存`)
    } catch (error) {
      if (isCancelled(error)) return
      options.onError(error, '清理缓存失败')
    }
  }

  async function clearBookLocalCache(book) {
    try {
      await options.confirm(
        `确认要删除浏览器中《${book.title}》的缓存章节吗？`,
        '提示',
        { type: 'warning' },
      )
      const removed = await options.clearBrowserChapterCache(book, book.id)
      await refreshBrowserCacheCounts()
      options.onSuccess(`已清理浏览器缓存 ${removed} 章`)
    } catch (error) {
      if (isCancelled(error)) return
      options.onError(error, '清理浏览器缓存失败')
    }
  }

  async function refreshBrowserCacheCounts() {
    await options.refreshManagedBrowserCacheCounts()
  }

  async function exportBook(book, format = 'txt') {
    batchBusy.value = true
    try {
      const normalizedFormat = format === 'epub' ? 'epub' : 'txt'
      const blob = await options.bookshelf.exportSelectedBooks([book.id], normalizedFormat)
      options.saveBlob(blob, exportBookFilename(book, normalizedFormat))
      options.onSuccess(`已导出《${book.title}》`)
    } catch (error) {
      options.onError(error, '导出失败')
    } finally {
      batchBusy.value = false
    }
  }

  function exportBookFilename(book, format) {
    const fallback = `book-${book?.id || options.now()}`
    const title = String(book?.title || fallback)
      .replace(/[\\/:*?"<>|]/g, '-')
      .trim() || fallback
    return `${title}.${format === 'epub' ? 'epub' : 'txt'}`
  }

  return {
    cachingBookId,
    cacheJobs,
    cacheProgressLabel,
    isCachingBook,
    cacheBook,
    cancelBookCache,
    cancelServerCache,
    cacheBookLocal,
    clearBookCache,
    clearBookLocalCache,
    exportBook,
    exportBookFilename,
  }
}
