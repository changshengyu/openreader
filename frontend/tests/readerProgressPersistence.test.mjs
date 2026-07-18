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

test('background progress save queues one keepalive without a duplicate ordinary request', async () => {
  const previousWindow = globalThis.window
  const previousFetch = globalThis.fetch
  const keepalive = []
  const remote = []
  globalThis.window = {
    localStorage: {
      getItem: key => key === 'openreader_token' ? 'progress-token' : null,
    },
  }
  globalThis.fetch = async (url, options) => {
    keepalive.push({ url, options })
    return { ok: true }
  }
  try {
    const controller = useReaderProgressPersistence({
      minimumInterval: 0,
      getPayload: () => ({ bookId: 7, chapterId: 13, chapterIndex: 3, offset: 240 }),
      getBaseUpdatedAt: () => 'server-v1',
      getStoredProgress: () => ({ updatedAt: 'local-v2' }),
      getMode: () => 'scroll',
      ensureClientId: () => 'client-a',
      applyLocal: () => {},
      saveRemote: async payload => {
        remote.push(payload)
        return payload
      },
      onSaved: () => {},
    })

    await controller.save({ force: true, background: true })
    assert.equal(keepalive.length, 1)
    assert.equal(remote.length, 0)
    assert.equal(keepalive[0].url, '/api/progress')
    assert.equal(keepalive[0].options.keepalive, true)
    assert.equal(JSON.parse(keepalive[0].options.body).baseUpdatedAt, 'server-v1')
    controller.cancelScheduled()
  } finally {
    globalThis.window = previousWindow
    globalThis.fetch = previousFetch
  }
})

test('background progress save falls back once when keepalive is unavailable', async () => {
  const previousWindow = globalThis.window
  const previousFetch = globalThis.fetch
  const remote = []
  globalThis.window = {
    localStorage: {
      getItem: () => 'progress-token',
    },
  }
  globalThis.fetch = undefined
  try {
    const controller = useReaderProgressPersistence({
      minimumInterval: 0,
      getPayload: () => ({ bookId: 8, chapterId: 14, chapterIndex: 4, offset: 80 }),
      getBaseUpdatedAt: () => 'server-v2',
      getMode: () => 'page',
      applyLocal: () => {},
      saveRemote: async payload => {
        remote.push(payload)
        return payload
      },
      onSaved: () => {},
    })

    await controller.save({ force: true, background: true })
    assert.equal(remote.length, 1)
    assert.equal(remote[0].baseUpdatedAt, 'server-v2')
    controller.cancelScheduled()
  } finally {
    globalThis.window = previousWindow
    globalThis.fetch = previousFetch
  }
})
