import { getCurrentInstance, onBeforeUnmount, unref, watch } from 'vue'

function abortError() {
  const error = new Error('reader chapter wait aborted')
  error.name = 'AbortError'
  return error
}

function chapterError(value) {
  return value instanceof Error ? value : new Error(String(value || '章节加载失败'))
}

export function useReaderChapterReady(options) {
  const waiters = new Set()
  let disposed = false

  function scopeKey() {
    return String(options.getScopeKey?.() ?? '')
  }

  function cleanup(waiter) {
    waiters.delete(waiter)
    waiter.signal?.removeEventListener?.('abort', waiter.abort)
  }

  function resolveWaiter(waiter) {
    cleanup(waiter)
    waiter.resolve()
  }

  function rejectWaiter(waiter, error) {
    cleanup(waiter)
    waiter.reject(error)
  }

  function settle() {
    const currentScope = scopeKey()
    const currentIndex = Number(unref(options.currentIndex))
    const loading = Boolean(unref(options.chapterLoading))
    const loaded = Boolean(unref(options.chapterLoaded))
    const error = unref(options.chapterLoadError)

    for (const waiter of [...waiters]) {
      if (waiter.scope !== currentScope) {
        rejectWaiter(waiter, new Error('reader scope changed'))
        continue
      }
      if (currentIndex !== waiter.index) continue
      if (error) {
        rejectWaiter(waiter, chapterError(error))
        continue
      }
      if (loaded && !loading) resolveWaiter(waiter)
    }
  }

  function wait(index, waitOptions = {}) {
    const target = Number(index)
    const signal = waitOptions.signal
    if (disposed) return Promise.reject(abortError())
    if (!Number.isInteger(target) || target < 0) {
      return Promise.reject(new Error('invalid reader chapter index'))
    }
    if (signal?.aborted) return Promise.reject(abortError())

    const currentIndex = Number(unref(options.currentIndex))
    const loading = Boolean(unref(options.chapterLoading))
    const loaded = Boolean(unref(options.chapterLoaded))
    const error = unref(options.chapterLoadError)
    if (currentIndex === target && error) return Promise.reject(chapterError(error))
    if (currentIndex === target && loaded && !loading) return Promise.resolve()

    return new Promise((resolve, reject) => {
      const waiter = {
        abort: null,
        index: target,
        reject,
        resolve,
        scope: scopeKey(),
        signal,
      }
      waiter.abort = () => rejectWaiter(waiter, abortError())
      waiters.add(waiter)
      signal?.addEventListener?.('abort', waiter.abort, { once: true })
      settle()
    })
  }

  const stopWatching = watch(
    [
      () => unref(options.currentIndex),
      () => unref(options.chapterLoaded),
      () => unref(options.chapterLoading),
      () => unref(options.chapterLoadError),
      () => scopeKey(),
    ],
    settle,
    { flush: 'sync' },
  )

  function dispose() {
    if (disposed) return
    disposed = true
    stopWatching()
    for (const waiter of [...waiters]) rejectWaiter(waiter, abortError())
  }

  if (getCurrentInstance()) onBeforeUnmount(dispose)

  return {
    dispose,
    pendingCount: () => waiters.size,
    wait,
  }
}
