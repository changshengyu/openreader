import assert from 'node:assert/strict'
import test from 'node:test'
import { createPinia, setActivePinia } from 'pinia'
import { useOverlayStore } from '../src/stores/overlay.js'

function freshOverlay() {
  setActivePinia(createPinia())
  return useOverlayStore()
}

test('turns every search row click into a monotonic repeatable Reader intent', () => {
  const overlay = freshOverlay()
  const book = { id: 7, bookUrl: 'https://book.example/7' }
  const result = {
    chapterIndex: 3,
    resultCountWithinChapter: 1,
    lineIndex: 8,
    percent: 0.4,
  }
  overlay.openSearchBookContent(book)

  assert.equal(overlay.requestSearchBookContentJump(result, '目标'), true)
  const first = JSON.parse(JSON.stringify(overlay.searchBookContentJump))
  assert.equal(overlay.searchBookContentVisible, false)
  assert.deepEqual(first, {
    requestId: 1,
    bookId: 7,
    bookUrl: 'https://book.example/7',
    query: '目标',
    result,
  })

  overlay.searchBookContentVisible = true
  assert.equal(overlay.requestSearchBookContentJump(result, '目标'), true)
  assert.equal(overlay.searchBookContentJump.requestId, 2)
  assert.deepEqual(overlay.searchBookContentJump.result, result)
  assert.notEqual(overlay.searchBookContentJump, first)
})

test('does not create a search jump without an active book or result', () => {
  const overlay = freshOverlay()
  assert.equal(overlay.requestSearchBookContentJump({ chapterIndex: 1 }, '目标'), false)
  overlay.openSearchBookContent({ id: 7 })
  assert.equal(overlay.requestSearchBookContentJump(null, '目标'), false)
  assert.equal(overlay.searchBookContentJump, null)
})

test('keeps the exact query in the repeatable Reader intent', () => {
  const overlay = freshOverlay()
  overlay.openSearchBookContent({ id: 7, bookUrl: 'https://book.example/7' })

  assert.equal(overlay.requestSearchBookContentJump({ chapterIndex: 1 }, ' 目标 '), true)
  assert.equal(overlay.searchBookContentJump.query, ' 目标 ')
})
