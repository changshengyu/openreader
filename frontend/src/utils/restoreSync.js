import { useBookshelfStore } from '../stores/bookshelf'
import { usePreferencesStore } from '../stores/preferences'
import { useReaderStore } from '../stores/reader'

function count(result, key) {
  return Number(result?.[key] || 0)
}

function dispatch(name, detail) {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent(name, { detail }))
}

export async function applyRestoreResult(result = {}, options = {}) {
  const canCommit = options.canCommit || (() => true)
  if (!canCommit()) return false
  const bookshelf = useBookshelfStore()
  const reader = useReaderStore()
  const preferences = usePreferencesStore()
  const jobs = []

  if (count(result, 'categories') > 0) {
    jobs.push(bookshelf.loadCategories({ force: true }))
  }
  if (count(result, 'books') + count(result, 'progress') + count(result, 'categories') > 0) {
    jobs.push(bookshelf.loadBooks({ force: true, all: true }))
  }
  if (count(result, 'settings') > 0) {
    jobs.push(reader.loadReaderSettings())
    jobs.push(preferences.loadPreferences())
  }

  await Promise.allSettled(jobs)
  if (!canCommit()) return false

  if (count(result, 'sources') > 0) {
    dispatch('openreader:sources-update', { kind: 'restore-backup' })
  }
  if (count(result, 'bookmarks') > 0) {
    dispatch('openreader:bookmarks-updated', {
      bookIds: [],
      payload: { kind: 'restore-backup' },
    })
  }
  if (count(result, 'replaceRules') > 0) {
    dispatch('openreader:replace-rules-updated', { kind: 'restore-backup' })
  }
  return true
}
