import assert from 'node:assert/strict'
import test from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { useOverlayStore } from '../src/stores/overlay.js'

function freshOverlay() {
  setActivePinia(createPinia())
  return useOverlayStore()
}

test('a deleted book closes and clears only matching per-book overlay scenes', async () => {
  const overlay = freshOverlay()
  const deleted = { id: 7, title: 'Deleted' }
  overlay.openBookInfo(deleted, { progress: 0.4 })
  overlay.openBookEdit(deleted)
  overlay.openBookGroup('set', deleted)
  overlay.openBookmark(deleted, { createDraft: { excerpt: 'draft' } })
  const bookmarkCompletion = overlay.openBookmarkForm(deleted, { excerpt: 'draft' })
  overlay.openSearchBookContent(deleted)
  overlay.bookManageVisible = true

  assert.equal(overlay.reconcileDeletedBooks([7, '7', 0, 'bad']), true)

  assert.equal(overlay.bookInfoVisible, false)
  assert.equal(overlay.bookInfoBook, null)
  assert.deepEqual(overlay.bookInfoOptions, {})
  assert.equal(overlay.bookEditVisible, false)
  assert.equal(overlay.bookEditBook, null)
  assert.equal(overlay.bookGroupVisible, false)
  assert.equal(overlay.bookmarkVisible, false)
  assert.equal(overlay.bookmarkBook, null)
  assert.equal(overlay.bookmarkCreateDraft, null)
  assert.equal(overlay.bookmarkFormVisible, false)
  assert.equal(overlay.bookmarkFormBook, null)
  assert.equal(overlay.bookmarkFormDraft, null)
  assert.equal(overlay.searchBookContentVisible, false)
  assert.equal(overlay.searchBook, null)
  assert.equal(overlay.bookManageVisible, true)
  assert.deepEqual(await bookmarkCompletion, { saved: false, reason: 'book-deleted' })
})

test('deletion preserves another book and shelf-wide group management', () => {
  const overlay = freshOverlay()
  const kept = { id: 8, title: 'Kept' }
  overlay.openBookInfo(kept)
  overlay.openBookEdit(kept)
  overlay.openBookmark(kept)
  overlay.openSearchBookContent(kept)
  overlay.openBookGroup('manage')

  assert.equal(overlay.reconcileDeletedBooks([7]), false)
  assert.equal(overlay.bookInfoVisible, true)
  assert.deepEqual(overlay.bookInfoBook, kept)
  assert.equal(overlay.bookEditVisible, true)
  assert.equal(overlay.bookmarkVisible, true)
  assert.equal(overlay.searchBookContentVisible, true)
  assert.equal(overlay.bookGroupVisible, true)
  assert.equal(overlay.bookGroupMode, 'manage')
})
