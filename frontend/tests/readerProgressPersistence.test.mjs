import assert from 'node:assert/strict'
import test from 'node:test'
import { useReaderProgressPersistence } from '../src/composables/useReaderProgressPersistence.js'
import {
  readerProgressBaseUpdatedAt,
  readerProgressPayload,
  readerProgressSaveKey,
  readerProgressThrottleDelay,
} from '../src/utils/readerProgressPersistence.js'

test('builds a stable reader progress fingerprint at display precision', () => {
  const payload = {
    bookId: 7,
    chapterId: 12,
    chapterIndex: 3,
    offset: 88,
    percent: 0.123456,
    chapterPercent: 0.654321,
  }
  assert.equal(readerProgressSaveKey(payload, 'scroll'), '7:12:3:88:1235:6543:scroll')
  assert.equal(readerProgressSaveKey({ ...payload, baseUpdatedAt: 'ignored' }, 'scroll'), '7:12:3:88:1235:6543:scroll')
})

test('preserves the server base while local progress is pending', () => {
  assert.equal(readerProgressBaseUpdatedAt({
    pendingSync: true,
    baseUpdatedAt: 'server-v1',
    updatedAt: 'local-v2',
  }), 'server-v1')
  assert.equal(readerProgressBaseUpdatedAt({
    pendingSync: false,
    updatedAt: 'server-v2',
  }), 'server-v2')
})

test('calculates the remaining progress request throttle window', () => {
  assert.equal(readerProgressThrottleDelay(1000, 1500, 1200), 700)
  assert.equal(readerProgressThrottleDelay(1000, 2500, 1200), 0)
})

test('builds progress from the visible chapter snapshot when available', () => {
  assert.deepEqual(readerProgressPayload({
    bookId: 7,
    visibleSnapshot: {
      chapterIndex: 3,
      chapter: { id: 13, title: '第四章' },
      offset: 240,
      chapterPercent: 0.25,
    },
    currentChapter: { id: 12, title: '第三章' },
    currentChapterIndex: 2,
    currentOffset: 100,
    currentChapterPercent: 0.5,
    totalChapters: 10,
  }), {
    bookId: 7,
    chapterId: 13,
    chapterIndex: 3,
    offset: 240,
    percent: 0.325,
    chapterPercent: 0.25,
    chapterTitle: '第四章',
  })
})

test('falls back to the current chapter and clamps whole-book progress', () => {
  assert.deepEqual(readerProgressPayload({
    bookId: 7,
    visibleSnapshot: null,
    currentChapter: { id: 19, title: '末章' },
    currentChapterIndex: 9,
    currentOffset: 800,
    currentChapterPercent: 1,
    totalChapters: 10,
  }), {
    bookId: 7,
    chapterId: 19,
    chapterIndex: 9,
    offset: 800,
    percent: 1,
    chapterPercent: 1,
    chapterTitle: '末章',
  })
})

test('does not apply or upload a transient progress snapshot while the reader window is busy', async () => {
  let blocked = true
  const local = []
  const remote = []
  const controller = useReaderProgressPersistence({
    minimumInterval: 0,
    isBlocked: () => blocked,
    getPayload: () => ({
      bookId: 7,
      chapterId: 13,
      chapterIndex: 3,
      offset: 240,
      percent: 0.325,
      chapterPercent: 0.25,
    }),
    getBaseUpdatedAt: () => 'server-v1',
    applyLocal: payload => local.push(payload),
    saveRemote: async payload => {
      remote.push(payload)
      return payload
    },
    onSaved: () => {},
  })

  await controller.save({ force: true })
  assert.deepEqual(local, [])
  assert.deepEqual(remote, [])

  blocked = false
  await controller.save({ force: true })
  assert.equal(local.length, 1)
  assert.equal(remote.length, 1)
  controller.cancelScheduled()
})
