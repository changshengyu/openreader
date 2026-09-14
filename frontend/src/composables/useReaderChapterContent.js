import { unref } from 'vue'
import {
  chapterCacheBookKey,
  isValidChapterContentResponse,
  loadBrowserChapterContent,
} from '../utils/bookChapterCache.js'
import { nearbyReaderChapterIndexes } from '../utils/readerChapterWindow.js'

export function useReaderChapterContent(options) {
  const loadBrowserContent = options.loadBrowserContent ?? loadBrowserChapterContent
  const preloadRadius = Math.max(0, Number(options.preloadRadius) || 0)
  const inFlight = new Map()
  const staleRetryTails = new Map()

  function cacheKey(targetBook = unref(options.book), fallbackBookId = unref(options.bookId)) {
    return chapterCacheBookKey(targetBook, fallbackBookId)
  }

  function get(index, targetCacheKey = cacheKey()) {
    if (options.shouldCache?.() === false) return null
    const cached = options.memoryCache.get(targetCacheKey, index)
    return isValidChapterContentResponse(cached) ? cached : null
  }

  function set(index, data, targetCacheKey = cacheKey()) {
    if (options.shouldCache?.() === false) return
    if (!isValidChapterContentResponse(data)) return
    options.memoryCache.set(targetCacheKey, index, data)
  }

  function clear(targetBook = unref(options.book), fallbackBookId = unref(options.bookId)) {
    const targetCacheKey = cacheKey(targetBook, fallbackBookId)
    options.memoryCache.clearBook(targetCacheKey)
    inFlight.forEach((entry, requestKey) => {
      if (entry.cacheKey !== targetCacheKey) return
      entry.controller.abort()
      inFlight.delete(requestKey)
    })
  }

  async function load(index, loadOptions = {}) {
    const targetBook = { ...(unref(options.book) || {}) }
    const targetBookId = unref(options.bookId)
    const targetCacheKey = cacheKey(targetBook, targetBookId)
    if (!loadOptions.refresh) {
      const cached = get(index, targetCacheKey)
      if (cached) return cached
    }
    const requestKey = [
      targetCacheKey,
      Number(index),
      loadOptions.refresh ? 'refresh' : 'normal',
    ].join(':')
    if (inFlight.has(requestKey)) return inFlight.get(requestKey).promise

    const controller = new AbortController()
    const externalSignal = loadOptions.signal
    const abortFromExternal = () => controller.abort(externalSignal.reason)
    if (externalSignal?.aborted) abortFromExternal()
    else externalSignal?.addEventListener('abort', abortFromExternal, { once: true })
    const request = (async () => {
      let data
      try {
        data = await loadBrowserContent(
          targetBook,
          targetBookId,
          index,
          {
            refresh: Boolean(loadOptions.refresh),
            signal: controller.signal,
          },
        )
      } catch (error) {
        if (controller.signal.aborted) throw chapterAbortError(controller.signal)
        if (!isStaleChapterConflict(error)) throw error
        try {
          data = await enqueueStaleRetry(
            staleRetryTails,
            targetCacheKey,
            controller.signal,
            () => loadBrowserContent(
              targetBook,
              targetBookId,
              index,
              {
                refresh: false,
                signal: controller.signal,
              },
            ),
          )
        } catch (retryError) {
          if (controller.signal.aborted) throw chapterAbortError(controller.signal)
          if (isStaleChapterConflict(retryError)) {
            throw chapterStaleRetryError(retryError)
          }
          throw retryError
        }
      }
      if (controller.signal.aborted) throw chapterAbortError(controller.signal)
      const isCurrentBook = Number(unref(options.bookId)) === Number(targetBookId)
        && cacheKey() === targetCacheKey
      if (
        options.shouldCache?.() !== false
        &&
        isValidChapterContentResponse(data)
        && isCurrentBook
      ) {
        set(index, data, targetCacheKey)
        options.markCached(index)
      }
      return data
    })()
    const entry = { cacheKey: targetCacheKey, controller, promise: request }
    inFlight.set(requestKey, entry)
    try {
      return await request
    } finally {
      externalSignal?.removeEventListener('abort', abortFromExternal)
      if (inFlight.get(requestKey) === entry) inFlight.delete(requestKey)
    }
  }

  function preload(index) {
    const chapterRows = unref(options.chapters) || []
    if (!unref(options.book) || !chapterRows.length) return
    nearbyReaderChapterIndexes({
      chapterIndex: index,
      totalChapters: chapterRows.length,
      radius: preloadRadius,
    }).forEach(target => {
      if (get(target)) return
      load(target).catch(() => {})
    })
  }

  return {
    cacheKey,
    clear,
    get,
    load,
    preload,
    set,
  }
}

function chapterAbortError(signal) {
  if (signal.reason instanceof Error && signal.reason.name === 'AbortError') {
    return signal.reason
  }
  const error = new Error('chapter request cancelled')
  error.name = 'AbortError'
  return error
}

async function enqueueStaleRetry(retryTails, scopeKey, signal, retry) {
  const previous = retryTails.get(scopeKey) ?? Promise.resolve()
  let release
  const turn = new Promise(resolve => {
    release = resolve
  })
  const tail = previous.then(() => turn, () => turn)
  retryTails.set(scopeKey, tail)

  try {
    await previous.catch(() => {})
    if (signal.aborted) throw chapterAbortError(signal)
    return await retry()
  } finally {
    release()
    if (retryTails.get(scopeKey) === tail) retryTails.delete(scopeKey)
  }
}

function isStaleChapterConflict(error) {
  return Number(error?.response?.status) === 409
    && error?.response?.data?.error === 'chapter content changed; retry'
}

function chapterStaleRetryError(cause) {
  const error = new Error('章节状态已更新，请重试')
  error.name = 'ChapterStaleConflictError'
  error.cause = cause
  return error
}
