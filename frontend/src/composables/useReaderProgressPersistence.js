import { getCurrentInstance, onBeforeUnmount } from 'vue'
import {
  readerProgressSaveKey,
  readerProgressThrottleDelay,
} from '../utils/readerProgressPersistence.js'
import { readAuthToken } from '../utils/authToken.js'
import { progressUpdatedAt } from '../utils/bookOrder.js'

export function useReaderProgressPersistence(options) {
  const minimumInterval = Math.max(0, Number(options.minimumInterval) || 1200)
  let saveTimer = null
  let saving = false
  let pendingPayload = null
  let lastSavedKey = ''
  let restoredPosition = null
  let restoringProgress = null
  let lastRequestAt = 0
  let suspended = false
  let generation = 0
  let backgroundRequest = null

  function key(payload) {
    return readerProgressSaveKey(payload, options.getMode?.())
  }

  function markSaved(payload, restoredProgress) {
    lastSavedKey = key(payload)
    restoredPosition = payload?.bookId
      && Number(restoredProgress?.bookId) === Number(payload.bookId)
      && restoredProgress.updatedAt
      && !restoredProgress.pendingSync
      ? { viewportKey: lastSavedKey, progress: restoredProgress }
      : null
    if (restoredPosition && Number(restoringProgress?.bookId) === Number(payload.bookId)
      && restoringProgress.updatedAt === restoredProgress.updatedAt) {
      restoringProgress = null
    }
  }

  function isRestoredPosition(payload) {
    if (!payload?.bookId) return false
    if (Number(restoringProgress?.bookId) === Number(payload.bookId)
      && hasConfirmedProgress(restoringProgress)) return true
    return Boolean(restoredPosition && key(payload) === restoredPosition.viewportKey
      && hasConfirmedProgress(restoredPosition.progress))
  }

  function hasConfirmedProgress(restoredProgress) {
    const stored = options.getStoredProgress?.(restoredProgress.bookId)
    if (!stored?.updatedAt || stored.pendingSync) return false
    const restoredTime = progressUpdatedAt(restoredProgress)
    return stored.updatedAt === restoredProgress.updatedAt
      || (restoredTime > 0 && progressUpdatedAt(stored) >= restoredTime)
  }

  async function trackRestore(progress, restore) {
    if (!progress?.bookId || !progress.updatedAt || progress.pendingSync) return restore()
    const task = { bookId: progress.bookId, updatedAt: progress.updatedAt }
    restoringProgress = task
    try {
      return await restore()
    } finally {
      if (restoringProgress === task) restoringProgress = null
    }
  }

  function isBusy() {
    return !suspended && (saving || Boolean(pendingPayload))
  }

  function cancelScheduled() {
    clearTimeout(saveTimer)
    saveTimer = null
  }

  function schedule(delay = 0) {
    if (suspended) return
    cancelScheduled()
    saveTimer = setTimeout(() => {
      saveTimer = null
      save().catch(() => {})
    }, Math.max(0, Number(delay) || 0))
  }

  async function save(saveOptions = {}) {
    if (suspended || options.isBlocked?.()) return
    const payload = options.getPayload?.()
    if (!payload?.bookId) return

    const force = Boolean(saveOptions.force)
    const background = Boolean(saveOptions.background)
    if ((!force || background) && isRestoredPosition(payload)) return
    restoredPosition = null
    const nextPayload = {
      ...payload,
      baseUpdatedAt: options.getBaseUpdatedAt?.(payload.bookId) || '',
    }
    options.applyLocal?.(nextPayload, { force })

    const nextKey = key(nextPayload)
    if (nextKey === lastSavedKey && !force) return
    pendingPayload = nextPayload

    if (background) {
      if (sendKeepAlive(nextPayload)) {
        pendingPayload = null
        return
      }
      await flush(force)
      return
    }
    await flush(force)
  }

  function sendKeepAlive(payload) {
    if (typeof window === 'undefined' || typeof fetch !== 'function') return false
    const token = readAuthToken()
    if (!token) return false
    const progress = options.getStoredProgress?.(payload.bookId)
    const payloadKey = key(payload)
    if (backgroundRequest?.key === payloadKey) return true
    const requestGeneration = generation
    try {
      const request = fetch('/api/progress', {
        method: 'PUT',
        keepalive: true,
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({
          ...payload,
          mode: options.getMode?.() || '',
          clientUpdatedAt: progress?.updatedAt || new Date().toISOString(),
          clientId: options.ensureClientId?.(),
        }),
      })
        .then(async (response) => {
          if (!response?.ok || requestGeneration !== generation) return
          if (typeof response.json !== 'function') return
          const savedProgress = await response.json().catch(() => null)
          if (requestGeneration !== generation || !savedProgress?.bookId) return
          options.onSaved?.(savedProgress)
          lastSavedKey = payloadKey
        })
        .catch(() => {})
      const tracked = { key: payloadKey, request }
      backgroundRequest = tracked
      request.finally(() => {
        if (backgroundRequest === tracked) backgroundRequest = null
      })
      return true
    } catch {
      // The optimistic local snapshot remains pending for the next sync attempt.
      return false
    }
  }

  async function flush(force = false) {
    if (suspended) return
    if (saving) {
      if (!force) return
      await waitForIdle()
      if (saving) return
    }

    saving = true
    try {
      while (pendingPayload && !suspended) {
        const delay = readerProgressThrottleDelay(lastRequestAt, Date.now(), minimumInterval)
        if (!force && delay > 0) {
          schedule(delay)
          break
        }

        const nextPayload = pendingPayload
        pendingPayload = null
        const nextKey = key(nextPayload)
        if (nextKey === lastSavedKey && !force) continue

        lastRequestAt = Date.now()
        const requestGeneration = generation
        let savedProgress
        try {
          savedProgress = await options.saveRemote(nextPayload)
        } catch (error) {
          if (suspended || requestGeneration !== generation) break
          throw error
        }
        if (suspended || requestGeneration !== generation) break
        options.onSaved?.(savedProgress)
        lastSavedKey = nextKey
      }
    } finally {
      saving = false
      if (pendingPayload && !suspended && !saveTimer) schedule(0)
    }
  }

  function waitForIdle(timeout = 1500) {
    const started = Date.now()
    return new Promise(resolve => {
      const tick = () => {
        if (!saving || Date.now() - started >= timeout) {
          resolve()
          return
        }
        setTimeout(tick, 40)
      }
      tick()
    })
  }

  function suspend() {
    suspended = true
    generation += 1
    backgroundRequest = null
    pendingPayload = null
    cancelScheduled()
  }

  function resume() {
    suspended = false
    generation += 1
    backgroundRequest = null
    pendingPayload = null
    lastSavedKey = ''
    restoredPosition = null
    restoringProgress = null
    lastRequestAt = 0
  }

  if (getCurrentInstance()) onBeforeUnmount(cancelScheduled)

  return {
    cancelScheduled,
    isBusy,
    isRestoredPosition,
    key,
    markSaved,
    save,
    schedule,
    suspend,
    resume,
    trackRestore,
  }
}
