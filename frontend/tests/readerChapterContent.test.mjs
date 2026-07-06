import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderChapterContent } from '../src/composables/useReaderChapterContent.js'

function validContent(index) {
  return {
    chapter: { id: index + 1, title: `第 ${index + 1} 章` },
    content: `正文 ${index}`,
  }
}

function createMemoryCache() {
  const rows = new Map()
  return {
    clearBook(key) {
      rows.delete(key)
    },
    get(key, index) {
      return rows.get(key)?.get(index)
    },
    set(key, index, value) {
      if (!rows.has(key)) rows.set(key, new Map())
      rows.get(key).set(index, value)
    },
  }
}

function createController(overrides = {}) {
  const book = ref({ id: 7, url: 'https://example.com/book/7' })
  const bookId = ref(7)
  const chapters = ref(Array.from({ length: 6 }, (_, index) => ({ id: index + 1 })))
  const calls = []
  const memoryCache = createMemoryCache()
  const controller = useReaderChapterContent({
    book,
    bookId,
    chapters,
    memoryCache,
    preloadRadius: 2,
    markCached: index => calls.push(['cached', index]),
    loadBrowserContent: async (...args) => {
      calls.push(['load', ...args])
      return validContent(args[2])
    },
    ...overrides,
  })
  return { book, bookId, calls, chapters, controller, memoryCache }
}

test('returns valid memory content before consulting browser storage', async () => {
  const fixture = createController()
  fixture.controller.set(2, validContent(2))
  const data = await fixture.controller.load(2)
  assert.deepEqual(data, validContent(2))
  assert.deepEqual(fixture.calls, [])
})

test('loads, stores, and marks fresh content for the active book', async () => {
  const fixture = createController()
  const data = await fixture.controller.load(3, { refresh: true })
  assert.deepEqual(data, validContent(3))
  assert.deepEqual(fixture.calls, [
    [
      'load',
      { id: 7, url: 'https://example.com/book/7' },
      7,
      3,
      { refresh: true },
    ],
    ['cached', 3],
  ])
  assert.deepEqual(fixture.controller.get(3), validContent(3))
})

test('does not mark a completed request after switching books', async () => {
  let resolveLoad
  const fixture = createController({
    loadBrowserContent: () => new Promise(resolve => {
      resolveLoad = resolve
    }),
  })
  const pending = fixture.controller.load(1)
  fixture.book.value = { id: 8, url: 'https://example.com/book/8' }
  fixture.bookId.value = 8
  resolveLoad(validContent(1))
  await pending
  assert.deepEqual(fixture.calls, [])
  assert.equal(fixture.controller.get(1), null)
})

test('deduplicates concurrent loads for the same book and chapter', async () => {
  let resolveLoad
  let requestCount = 0
  const fixture = createController({
    loadBrowserContent: () => {
      requestCount += 1
      return new Promise(resolve => {
        resolveLoad = resolve
      })
    },
  })
  const first = fixture.controller.load(2)
  const second = fixture.controller.load(2)
  assert.equal(requestCount, 1)
  resolveLoad(validContent(2))
  assert.deepEqual(await first, validContent(2))
  assert.deepEqual(await second, validContent(2))
})

test('preloads uncached neighboring chapters within the configured radius', async () => {
  const fixture = createController()
  fixture.controller.set(1, validContent(1))
  fixture.controller.preload(2)
  await new Promise(resolve => setImmediate(resolve))
  const loadedIndexes = fixture.calls
    .filter(call => call[0] === 'load')
    .map(call => call[3])
    .sort((left, right) => left - right)
  assert.deepEqual(loadedIndexes, [0, 3, 4])
})
