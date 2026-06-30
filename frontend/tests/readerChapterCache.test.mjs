import assert from 'node:assert/strict'
import test from 'node:test'
import {
  readerChapterCacheStatus,
  readerChapterCacheTargets,
} from '../src/utils/readerChapterCache.js'

test('selects uncached chapters after the current chapter', () => {
  assert.deepEqual(readerChapterCacheTargets({
    chapterCount: 8,
    currentIndex: 2,
    count: 4,
    cachedMap: { 4: true, 6: true },
  }), [3, 5])
})

test('supports caching every remaining chapter and end-of-book bounds', () => {
  assert.deepEqual(readerChapterCacheTargets({
    chapterCount: 5,
    currentIndex: 3,
    count: true,
    cachedMap: {},
  }), [4])
  assert.deepEqual(readerChapterCacheTargets({
    chapterCount: 5,
    currentIndex: 4,
    count: true,
    cachedMap: {},
  }), [])
})

test('formats reader chapter caching progress', () => {
  assert.equal(readerChapterCacheStatus(3, 10), '正在缓存章节 3/10')
})
