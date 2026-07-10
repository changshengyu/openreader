import assert from 'node:assert/strict'
import test from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { useOverlayStore } from '../src/stores/overlay.js'

test('bookmark form opens with immutable reader context and resolves a close exactly once', async () => {
  setActivePinia(createPinia())
  const overlay = useOverlayStore()
  const book = { id: 7, title: '测试书', author: '测试作者' }
  const draft = {
    chapterId: 12,
    chapterIndex: 2,
    offset: 240,
    percent: 0.4,
    title: '第三章',
    excerpt: '当前摘录',
    note: '',
  }

  const completion = overlay.openBookmarkForm(book, draft, { mode: 'create' })
  assert.equal(overlay.bookmarkFormVisible, true)
  assert.deepEqual(overlay.bookmarkFormBook, book)
  assert.equal(overlay.bookmarkFormMode, 'create')
  assert.deepEqual(overlay.bookmarkFormDraft, draft)

  overlay.finishBookmarkForm({ saved: true, bookmarkId: 9 })
  overlay.finishBookmarkForm({ saved: false })
  assert.deepEqual(await completion, { saved: true, bookmarkId: 9 })
  assert.equal(overlay.bookmarkFormVisible, false)
})
