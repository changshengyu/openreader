import assert from 'node:assert/strict'
import test from 'node:test'
import {
  adjacentReaderChapterIndex,
  nearbyReaderChapterIndexes,
  readerChapterWindowExtension,
  readerChapterWindowIndexes,
  readerChapterWindowPrunePlan,
} from '../src/utils/readerChapterWindow.js'

test('builds mode-specific chapter windows at book boundaries', () => {
  assert.deepEqual(readerChapterWindowIndexes({
    mode: 'scroll2',
    anchorIndex: 0,
    totalChapters: 10,
  }), [0, 1])
  assert.deepEqual(readerChapterWindowIndexes({
    mode: 'scroll2',
    anchorIndex: 5,
    totalChapters: 10,
    startIndex: 2,
  }), [5, 6])
  assert.deepEqual(readerChapterWindowIndexes({
    mode: 'scroll',
    anchorIndex: 5,
    startIndex: 2,
    totalChapters: 10,
  }), [2, 3, 4, 5, 6])
  assert.deepEqual(readerChapterWindowIndexes({
    mode: 'scroll',
    anchorIndex: 9,
    totalChapters: 10,
  }), [9])
  assert.deepEqual(readerChapterWindowIndexes({
    mode: 'scroll2',
    anchorIndex: 0,
    totalChapters: 0,
  }), [])
})

test('selects adjacent and nearby chapters without crossing book bounds', () => {
  const blocks = [{ index: 2 }, { index: 3 }, { index: 4 }]
  assert.equal(adjacentReaderChapterIndex({
    blocks,
    direction: 'previous',
    totalChapters: 6,
  }), 1)
  assert.equal(adjacentReaderChapterIndex({
    blocks,
    direction: 'next',
    totalChapters: 5,
  }), null)
  assert.deepEqual(nearbyReaderChapterIndexes({
    chapterIndex: 1,
    totalChapters: 4,
    radius: 2,
  }), [2, 0, 3])
})

test('detects chapter extension zones for continuous reading modes', () => {
  assert.deepEqual(readerChapterWindowExtension({
    mode: 'scroll2',
    scrollTop: 800,
    clientHeight: 800,
    scrollHeight: 4000,
  }), {
    next: false,
  })
  assert.deepEqual(readerChapterWindowExtension({
    mode: 'scroll2',
    scrollTop: 801,
    clientHeight: 800,
    scrollHeight: 4000,
  }), {
    next: true,
  })
})

test('scroll2 drops every read chapter while scroll retains its explicit start', () => {
  const blocks = [1, 2, 3, 4, 5, 6].map(index => ({ index }))
  const plan = readerChapterWindowPrunePlan({
    blocks,
    mode: 'scroll2',
    currentIndex: 4,
    totalChapters: 10,
  })
  assert.deepEqual(plan.blocks.map(block => block.index), [4, 5, 6])
  assert.deepEqual(plan.removedBeforeIndexes, [1, 2, 3])
  assert.equal(plan.changed, true)

  const retained = readerChapterWindowPrunePlan({
    blocks,
    mode: 'scroll',
    currentIndex: 4,
    totalChapters: 10,
  })
  assert.equal(retained.changed, false)
  assert.equal(retained.blocks[0], blocks[0])
})
