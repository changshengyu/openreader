import assert from 'node:assert/strict'
import test from 'node:test'
import {
  appendBookmarks,
  bookmarkReaderQuery,
  bookmarkUpdateTargetsBook,
  parseBookmarkPercent,
  removeBookmarkIds,
  replaceBookmark,
} from '../src/utils/bookmark.js'

test('keeps bookmark creation order while updating collections without mutating unrelated rows', () => {
  const current = [{ id: 1, title: '一' }, { id: 2, title: '二' }]
  assert.deepEqual(appendBookmarks(current, [{ id: 3, title: '三' }]).map(item => item.id), [1, 2, 3])
  assert.deepEqual(appendBookmarks(current, [{ id: 3 }, { id: 4 }]).map(item => item.id), [1, 2, 3, 4])
  assert.deepEqual(replaceBookmark(current, { id: 2, title: '新二' }), [
    current[0],
    { id: 2, title: '新二' },
  ])
  assert.deepEqual(removeBookmarkIds(current, ['1']), [current[1]])
})

test('filters bookmark update events by book id', () => {
  assert.equal(bookmarkUpdateTargetsBook({ detail: { bookIds: [7, 8] } }, 8), true)
  assert.equal(bookmarkUpdateTargetsBook({ detail: { bookIds: [7] } }, 8), false)
  assert.equal(bookmarkUpdateTargetsBook({ detail: {} }, 8), true)
})

test('builds reader bookmark routes and parses optional percentages', () => {
  assert.deepEqual(bookmarkReaderQuery({
    chapterIndex: 4,
    offset: 120,
    percent: 1.4,
  }), {
    chapter: 4,
    offset: 120,
    percent: 1.4,
  })
  assert.equal(parseBookmarkPercent(''), null)
  assert.equal(parseBookmarkPercent('invalid'), null)
})

test('carries bookmark paragraph context so Reader can recover after stale offsets', () => {
  assert.deepEqual(bookmarkReaderQuery({
    chapterIndex: 2,
    offset: 18,
    percent: 0.2,
    excerpt: '第一段\n第二段',
  }), {
    chapter: 2,
    offset: 18,
    percent: 0.2,
    bookmark: '第一段\n第二段',
  })
})
