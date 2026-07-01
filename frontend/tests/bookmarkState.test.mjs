import assert from 'node:assert/strict'
import test from 'node:test'
import {
  bookmarkReaderQuery,
  bookmarkUpdateTargetsBook,
  parseBookmarkPercent,
  prependBookmarks,
  removeBookmarkIds,
  replaceBookmark,
} from '../src/utils/bookmark.js'

test('updates bookmark collections without mutating unrelated rows', () => {
  const current = [{ id: 1, title: '一' }, { id: 2, title: '二' }]
  assert.deepEqual(prependBookmarks(current, [{ id: 3, title: '三' }]).map(item => item.id), [3, 1, 2])
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
