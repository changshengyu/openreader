import assert from 'node:assert/strict'
import test from 'node:test'
import { computed, nextTick, ref } from 'vue'
import { useReaderChapterCache } from '../src/composables/useReaderChapterCache.js'
import { cacheBookChaptersToBrowser } from '../src/utils/bookChapterCache.js'

function deferred() {
  let resolve
  const promise = new Promise(done => {
    resolve = done
  })
  return { promise, resolve }
}

function validChapter(index, content = `chapter-${index}`) {
  return { chapter: { index }, content }
}

test('runs the complete cache-first range with two workers and no duplicate network fetch', async () => {
  const browserCached = new Set([2, 4])
  const loaded = []
  const network = []
  const progress = []
  let active = 0
  let peak = 0

  const result = await cacheBookChaptersToBrowser(
    { id: 7, title: 'Range', author: 'Reader', url: 'range-url' },
    7,
    Array.from({ length: 6 }, (_, index) => ({ index })),
    {
      startIndex: 1,
      count: 4,
      scope: 'user:11',
      loadChapterContent: async (_book, _bookId, index, options) => {
        loaded.push([index, options.scope])
        active += 1
        peak = Math.max(peak, active)
        await new Promise(resolve => setTimeout(resolve, 2))
        if (!browserCached.has(index)) network.push(index)
        active -= 1
        return validChapter(index)
      },
      onProgress: value => progress.push(value),
    },
  )

  assert.deepEqual(loaded.map(([index]) => index).sort((a, b) => a - b), [1, 2, 3, 4])
  assert.deepEqual(loaded.map(([, scope]) => scope), Array(4).fill('user:11'))
  assert.deepEqual(network.sort((a, b) => a - b), [1, 3])
  assert.equal(peak, 2)
  assert.deepEqual(result, { cached: 4, requested: 4, cancelled: false })
  assert.equal(progress.at(-1).finished, 4)
  assert.equal(progress.at(-1).total, 4)
})

test('cancels queued chapters while allowing only the two in-flight workers to finish', async () => {
  const inFlight = deferred()
  const started = []
  let cancelled = false
  const pending = cacheBookChaptersToBrowser(
    { id: 8, title: 'Cancel', author: 'Reader', url: 'cancel-url' },
    8,
    Array.from({ length: 6 }, (_, index) => ({ index })),
    {
      startIndex: 1,
      count: 5,
      scope: 'user:11',
      cancelled: () => cancelled,
      loadChapterContent: async (_book, _bookId, index) => {
        started.push(index)
        await inFlight.promise
        return validChapter(index)
      },
    },
  )

  while (started.length < 2) await new Promise(resolve => setTimeout(resolve, 0))
  cancelled = true
  inFlight.resolve()
  const result = await pending

  assert.deepEqual(started.sort((a, b) => a - b), [1, 2])
  assert.deepEqual(result, { cached: 2, requested: 5, cancelled: true })
})

function createController(overrides = {}) {
  const calls = []
  const book = ref({ id: 9, title: 'Local', author: 'Reader', sourceId: 0, libraryPath: 'local.txt' })
  const bookId = ref(9)
  const chapters = ref(Array.from({ length: 5 }, (_, index) => ({ index })))
  const currentIndex = ref(1)
  const contextKey = ref('book:9|generation:1')
  const controller = useReaderChapterCache({
    book,
    bookId,
    chapters,
    currentIndex,
    isRemoteBook: computed(() => Number(book.value?.sourceId || 0) > 0),
    isTemporaryReader: ref(false),
    getContextKey: () => contextKey.value,
    getCacheScope: () => 'user:11',
    listCachedChapters: async (_book, _bookId, options) => {
      calls.push(['list', options.scope])
      return { 2: true, 3: true }
    },
    cacheChapters: async (_book, _bookId, _chapters, options) => {
      calls.push(['cache', options.startIndex, options.count, options.scope])
      options.onProgress({ finished: 2, total: 3, cached: 2 })
      return { cached: 3, requested: 3, cancelled: options.cancelled() }
    },
    notify: message => calls.push(['notify', message]),
    onNoTargets: () => calls.push(['no-targets']),
    onUnavailable: () => calls.push(['unavailable']),
    onError: error => calls.push(['error', error.message]),
    ...overrides,
  })
  return { book, bookId, calls, chapters, contextKey, controller, currentIndex }
}

test('caches an already shelved local book and restores the exact upstream completion state', async () => {
  const fixture = createController()
  await fixture.controller.cacheFollowing(50)

  assert.equal(fixture.controller.caching.value, false)
  assert.equal(fixture.controller.statusText.value, '')
  assert.deepEqual(fixture.calls, [
    ['cache', 2, 50, 'user:11'],
    ['notify', '缓存完成'],
    ['list', 'user:11'],
  ])
})

test('cancels synchronously without a toast and isolates a later job from the old queue', async () => {
  const first = deferred()
  const cacheOptions = []
  let invocation = 0
  const fixture = createController({
    cacheChapters: async (_book, _bookId, _chapters, options) => {
      cacheOptions.push(options)
      invocation += 1
      if (invocation === 1) await first.promise
      return { cached: 1, requested: 3, cancelled: options.cancelled() }
    },
  })

  const oldJob = fixture.controller.cacheFollowing(50)
  await nextTick()
  assert.equal(fixture.controller.caching.value, true)
  fixture.controller.cancel()
  assert.equal(fixture.controller.caching.value, false)
  assert.equal(fixture.controller.statusText.value, '')
  assert.equal(cacheOptions[0].cancelled(), true)

  await fixture.controller.cacheFollowing(100)
  first.resolve()
  await oldJob

  assert.equal(cacheOptions[1].cancelled(), false)
  assert.deepEqual(fixture.calls.filter(call => call[0] === 'notify'), [
    ['notify', '缓存完成'],
  ])
})

test('cancels the old job when the Reader context changes and suppresses its late completion', async () => {
  const work = deferred()
  let options
  const fixture = createController({
    cacheChapters: async (_book, _bookId, _chapters, nextOptions) => {
      options = nextOptions
      await work.promise
      return { cached: 1, requested: 3, cancelled: nextOptions.cancelled() }
    },
  })

  const pending = fixture.controller.cacheFollowing(true)
  await nextTick()
  fixture.contextKey.value = 'book:10|generation:1'
  await nextTick()

  assert.equal(options.cancelled(), true)
  assert.equal(fixture.controller.caching.value, false)
  assert.equal(fixture.controller.statusText.value, '')
  work.resolve()
  await pending
  assert.deepEqual(fixture.calls.filter(call => ['notify', 'list'].includes(call[0])), [])
})

test('keeps temporary Reader chapter responses no-store', async () => {
  const fixture = createController({ isTemporaryReader: ref(true) })
  await fixture.controller.cacheFollowing(50)
  assert.deepEqual(fixture.calls, [['unavailable']])
})
