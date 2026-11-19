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

test('suspends pending and in-flight progress after deletion until a new book resumes it', async () => {
  let finishRemote
  const saved = []
  const controller = useReaderProgressPersistence({
    minimumInterval: 0,
    getPayload: () => ({
      bookId: 7,
      chapterId: 13,
      chapterIndex: 3,
      offset: 240,
      percent: 0.325,
      chapterPercent: 0.25,
    }),
    applyLocal: progress => saved.push(['local', progress.bookId]),
    saveRemote: progress => new Promise(resolve => {
      saved.push(['remote', progress.bookId])
      finishRemote = () => resolve({ ...progress, updatedAt: 'server' })
    }),
    onSaved: progress => saved.push(['saved', progress.bookId]),
  })

  const inFlight = controller.save({ force: true })
  await Promise.resolve()
  controller.suspend()
  finishRemote()
  await inFlight
  await controller.save({ force: true })

  assert.deepEqual(saved, [
    ['local', 7],
    ['remote', 7],
  ])

  controller.resume()
  const resumed = controller.save({ force: true })
  await Promise.resolve()
  finishRemote()
  await resumed
  assert.deepEqual(saved.slice(2), [
    ['local', 7],
    ['remote', 7],
    ['saved', 7],
  ])
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

test('deduplicates the same background snapshot and confirms its keepalive response', async () => {
  const previousWindow = globalThis.window
  const previousFetch = globalThis.fetch
  const keepalive = []
  const saved = []
  let resolveFetch
  globalThis.window = {
    localStorage: {
      getItem: key => key === 'openreader_token' ? 'progress-token' : null,
    },
  }
  globalThis.fetch = (url, options) => {
    keepalive.push({ url, options })
    return new Promise(resolve => { resolveFetch = resolve })
  }
  try {
    const progress = {
      bookId: 7,
      chapterId: 13,
      chapterIndex: 3,
      offset: 240,
      updatedAt: 'server-v2',
    }
    const controller = useReaderProgressPersistence({
      minimumInterval: 0,
      getPayload: () => ({ bookId: 7, chapterId: 13, chapterIndex: 3, offset: 240 }),
      getBaseUpdatedAt: () => 'server-v1',
      getStoredProgress: () => ({ updatedAt: 'local-v2' }),
      getMode: () => 'scroll',
      ensureClientId: () => 'client-a',
      applyLocal: () => {},
      saveRemote: async payload => payload,
      onSaved: value => saved.push(value),
    })

    await controller.save({ force: true, background: true })
    await controller.save({ force: true, background: true })
    assert.equal(keepalive.length, 1, 'route navigation and unmount must share one keepalive')

    resolveFetch({
      ok: true,
      headers: { get: () => null },
      json: async () => progress,
    })
    await new Promise(resolve => setTimeout(resolve, 0))
    assert.deepEqual(saved, [progress], 'an accepted keepalive must clear the optimistic pending state')
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

test('keeping or leaving a restored viewport preserves the exact confirmed reading position', async () => {
  const restored = { bookId: 7, chapterId: 13, chapterIndex: 3, offset: 4400, updatedAt: 'server-v1' }
  let viewport = { ...restored, offset: 2495 }
  let stored = restored
  const local = []
  const remote = []
  const controller = useReaderProgressPersistence({
    getPayload: () => viewport,
    getBaseUpdatedAt: () => readerProgressBaseUpdatedAt(stored),
    getStoredProgress: () => stored,
    applyLocal: payload => local.push(payload),
    saveRemote: async payload => {
      remote.push(payload)
      return payload
    },
  })

  controller.markSaved(viewport, restored)
  await controller.save()
  await controller.save({ force: true, background: true })
  assert.deepEqual(local, [])
  assert.deepEqual(remote, [])

  viewport = { ...viewport, offset: 4800 }
  await controller.save({ force: true, background: true })
  assert.equal(local.at(-1)?.offset, 4800)
  assert.equal(remote.at(-1)?.offset, 4800)

  viewport = { ...viewport, offset: 2495 }
  stored = { ...restored, pendingSync: true, baseUpdatedAt: restored.updatedAt }
  controller.markSaved(viewport, stored)
  await controller.save({ force: true, background: true })
  assert.equal(remote.at(-1)?.offset, 2495)
  assert.equal(remote.at(-1)?.baseUpdatedAt, 'server-v1')

  stored = restored
  controller.markSaved(viewport)
  await controller.save({ force: true, background: true })
  assert.equal(remote.length, 3, 'an explicitly opened position still saves on leave')
})

test('leaving during an asynchronous restoration preserves confirmed progress', async () => {
  const restored = { bookId: 7, chapterId: 13, chapterIndex: 3, offset: 4400, updatedAt: 'server-v1' }
  let viewport = { ...restored, offset: 1296 }
  const saved = []
  const controller = useReaderProgressPersistence({
    getPayload: () => viewport,
    getStoredProgress: () => restored,
    applyLocal: payload => saved.push(['local', payload.offset]),
    saveRemote: async payload => saved.push(['remote', payload.offset]),
  })
  let finishRestore
  const restoring = controller.trackRestore(restored, () => new Promise(resolve => { finishRestore = resolve }))

  await controller.save({ force: true, background: true })
  assert.deepEqual(saved, [])

  controller.markSaved(viewport, restored)
  viewport = { ...viewport, offset: 2700 }
  await controller.save({ force: true, background: true })
  finishRestore()
  await restoring
  assert.deepEqual(saved, [['local', 2700], ['remote', 2700]])
})

test('a newer server version survives an untouched viewport during and after restoring an older version', async () => {
  const restored = { bookId: 7, chapterId: 13, chapterIndex: 3, offset: 4400, updatedAt: '2026-07-01T00:00:00.000001Z' }
  const viewport = { ...restored, offset: 2495 }
  const saved = []
  let stored = restored
  const controller = useReaderProgressPersistence({
    getPayload: () => viewport,
    getBaseUpdatedAt: () => readerProgressBaseUpdatedAt(stored),
    getStoredProgress: () => stored,
    applyLocal: payload => saved.push(['local', payload.offset, payload.baseUpdatedAt]),
    saveRemote: async payload => saved.push(['remote', payload.offset, payload.baseUpdatedAt]),
  })

  await controller.trackRestore(restored, async () => {
    stored = { ...restored, offset: 8000, updatedAt: '2026-07-01T00:00:00.000002Z' }
    await controller.save({ force: true, background: true })
    controller.markSaved(viewport, restored)
  })
  await controller.save({ force: true, background: true })
  assert.deepEqual(saved, [])

  stored = { ...stored, pendingSync: true, baseUpdatedAt: restored.updatedAt }
  await controller.save({ force: true, background: true })
  assert.deepEqual(saved, [
    ['local', 2495, restored.updatedAt],
    ['remote', 2495, restored.updatedAt],
  ])
})

test('background save during a pending restoration uses the viewport moved by native input', async () => {
  const confirmed = { bookId: 7, chapterId: 13, chapterIndex: 3, offset: 4400, updatedAt: 'server-v1' }
  const scrollViewport = { scrollTop: 900 }
  let scrollContainer = scrollViewport
  let payload = { ...confirmed, offset: 1296 }
  const saved = []
  const controller = useReaderProgressPersistence({
    getPayload: () => payload,
    getRestoreScrollViewport: () => scrollContainer,
    getStoredProgress: () => confirmed,
    getBaseUpdatedAt: () => confirmed.updatedAt,
    applyLocal: progress => saved.push(['local', progress.bookId, progress.offset, progress.baseUpdatedAt]),
    saveRemote: async progress => saved.push(['remote', progress.bookId, progress.offset, progress.baseUpdatedAt]),
  })

  await controller.trackRestore(confirmed, async beginLoading => {
    const background = { force: true, background: true }
    scrollViewport.scrollTop = 920
    await controller.save(background)
    controller.recordRestoreScroll?.(7, -1)
    await controller.save(background)
    scrollViewport.scrollTop = 720
    payload = { ...payload, offset: 950 }
    await controller.save()
    scrollContainer = { scrollTop: 700 }
    await controller.save(background)
    assert.deepEqual(saved, [], 'layout, touch at an edge, idle saves and other viewports keep the confirmed progress')

    scrollContainer = scrollViewport
    controller.recordRestoreScroll?.(7, 1)
    scrollViewport.scrollTop = 960
    payload = { ...payload, offset: 1350 }
    await controller.save(background)
    assert.deepEqual(saved, [
      ['local', 7, 1350, 'server-v1'],
      ['remote', 7, 1350, 'server-v1'],
    ])

    beginLoading?.()
    scrollViewport.scrollTop = 1200
    payload = { ...payload, offset: 2500 }
    await controller.save(background)
    assert.equal(saved.length, 2, 'programmatic chapter positioning retains the recovery protection')
  }, { waitForNavigation: true })
})

test('a newer pending navigation preserves native movement in the same reader viewport', async () => {
  const initial = { bookId: 7, chapterId: 13, chapterIndex: 3, offset: 4400, updatedAt: 'server-v1' }
  const latest = { ...initial, offset: 8000, updatedAt: 'server-v2' }
  const viewport = { scrollTop: 1000 }
  let stored = initial
  let payload = { ...initial, offset: 1296 }
  const writes = []
  const controller = useReaderProgressPersistence({
    getPayload: () => payload,
    getRestoreScrollViewport: () => viewport,
    getStoredProgress: () => stored,
    getBaseUpdatedAt: () => stored.updatedAt,
    applyLocal: progress => writes.push(['local', progress.offset, progress.baseUpdatedAt]),
    saveRemote: async progress => writes.push(['remote', progress.offset, progress.baseUpdatedAt]),
  })

  const firstNavigation = Promise.withResolvers()
  const restoringFirst = controller.trackRestore(initial, () => firstNavigation.promise, { waitForNavigation: true })
  controller.recordRestoreScroll(7, -1)
  viewport.scrollTop = 850
  payload = { ...payload, offset: 950 }

  stored = latest
  await controller.trackRestore(latest, async () => {
    firstNavigation.resolve()
    await restoringFirst
    await controller.save({ force: true, background: true })
  }, { waitForNavigation: true })

  assert.deepEqual(writes, [
    ['local', 950, latest.updatedAt],
    ['remote', 950, latest.updatedAt],
  ])
})

test('a pending navigation does not inherit movement outside the same book, viewport, and waiting stage', async t => {
  const scenarios = [
    { name: 'another book', bookId: 8 },
    { name: 'another viewport', changeViewport: true },
    { name: 'the previous restoration already began loading', previousLoading: true },
    { name: 'the next restoration begins loading immediately', waitForNavigation: false },
  ]
  for (const scenario of scenarios) {
    await t.test(scenario.name, async () => {
      const initial = { bookId: 7, chapterId: 13, chapterIndex: 3, offset: 4400, updatedAt: 'server-v1' }
      const latest = { ...initial, bookId: scenario.bookId ?? 7, offset: 8000, updatedAt: 'server-v2' }
      const viewport = { scrollTop: 1000 }
      let activeViewport = viewport
      let payload = { ...initial, offset: 1296 }
      let stored = initial
      const writes = []
      const controller = useReaderProgressPersistence({
        getPayload: () => payload,
        getRestoreScrollViewport: () => activeViewport,
        getStoredProgress: () => stored,
        applyLocal: progress => writes.push(progress),
        saveRemote: async progress => writes.push(progress),
      })

      await controller.trackRestore(initial, async beginLoading => {
        controller.recordRestoreScroll(initial.bookId, -1)
        viewport.scrollTop = 850
        if (scenario.previousLoading) beginLoading()
        if (scenario.changeViewport) activeViewport = { scrollTop: 1000 }
        stored = latest
        payload = { ...latest, offset: 950 }
        await controller.trackRestore(latest, async () => {
          activeViewport = viewport
          await controller.save({ force: true, background: true })
        }, { waitForNavigation: scenario.waitForNavigation ?? true })
      }, { waitForNavigation: true })

      assert.deepEqual(writes, [])
    })
  }
})
