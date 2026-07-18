import { getCurrentInstance, onBeforeUnmount } from 'vue'
import {
  readerProgressSaveKey,
  readerProgressThrottleDelay,
} from '../utils/readerProgressPersistence.js'

export function useReaderProgressPersistence(options) {
  const minimumInterval = Math.max(0, Number(options.minimumInterval) || 1200)
  let saveTimer = null
  let saving = false
  let pendingPayload = null
  let lastSavedKey = ''
  let lastRequestAt = 0

  function key(payload) {
    return readerProgressSaveKey(payload, options.getMode?.())
  }

  function markSaved(payload) {
    lastSavedKey = key(payload)
  }

  function isBusy() {
    return saving || Boolean(pendingPayload)
  }

  function cancelScheduled() {
    clearTimeout(saveTimer)
    saveTimer = null
  }

  function schedule(delay = 0) {
    cancelScheduled()
    saveTimer = setTimeout(() => {
      saveTimer = null
      save().catch(() => {})
    }, Math.max(0, Number(delay) || 0))
  }

  async function save(saveOptions = {}) {
    if (options.isBlocked?.()) return
    const payload = options.getPayload?.()
    if (!payload?.bookId) return

    const force = Boolean(saveOptions.force)
    const background = Boolean(saveOptions.background)
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
    const token = window.localStorage?.getItem('openreader_token')
    if (!token) return false
    const progress = options.getStoredProgress?.(payload.bookId)
    try {
      fetch('/api/progress', {
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
      }).catch(() => {})
      return true
    } catch {
      // The optimistic local snapshot remains pending for the next sync attempt.
      return false
    }
  }

  async function flush(force = false) {
    if (saving) {
      if (!force) return
      await waitForIdle()
      if (saving) return
    }

    saving = true
    try {
      while (pendingPayload) {
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
        const savedProgress = await options.saveRemote(nextPayload)
        options.onSaved?.(savedProgress)
        lastSavedKey = nextKey
      }
    } finally {
      saving = false
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

  if (getCurrentInstance()) onBeforeUnmount(cancelScheduled)

  return {
    cancelScheduled,
    isBusy,
    key,
    markSaved,
    save,
    schedule,
  }
}
