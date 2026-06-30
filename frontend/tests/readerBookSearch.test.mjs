import assert from 'node:assert/strict'
import test from 'node:test'
import {
  bookContentSearchMaxRounds,
  bookContentSearchPagingParams,
  bookContentSearchStatus,
} from '../src/utils/readerBookSearch.js'

test('uses bounded remote and expanded local book search windows', () => {
  assert.deepEqual(bookContentSearchPagingParams({ sourceId: 7 }), {
    chapterLimit: 10,
    scanLimit: 10,
    matchLimit: 120,
    perChapterLimit: 20,
  })
  assert.deepEqual(bookContentSearchPagingParams({ sourceId: 0 }), {
    chapterLimit: 160,
    scanLimit: 480,
    matchLimit: 1000,
    perChapterLimit: 100,
    localFull: 1,
  })
})

test('keeps initial, continuation and full-scan round limits distinct', () => {
  assert.equal(bookContentSearchMaxRounds({ remote: true }), 4)
  assert.equal(bookContentSearchMaxRounds({ remote: false }), 1)
  assert.equal(bookContentSearchMaxRounds({ append: true, remote: true }), 1)
  assert.equal(bookContentSearchMaxRounds({ scanAll: true }), 80)
})

test('formats book content search progress from scanned chapters', () => {
  assert.equal(bookContentSearchStatus({
    searched: true,
    lastIndex: 11,
    total: 30,
    chapterCount: 40,
    resultCount: 5,
  }), '已搜索 12 / 30 章，5 条结果')
  assert.equal(bookContentSearchStatus({
    searched: false,
    resultCount: 5,
  }), '')
})
