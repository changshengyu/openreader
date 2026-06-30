import assert from 'node:assert/strict'
import test from 'node:test'
import {
  readerProgressBaseUpdatedAt,
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
