import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import api from '../src/api/client.js'
import { getChapterContent } from '../src/api/books.js'
import { getRemoteReaderChapterContent } from '../src/api/remoteReader.js'
import { useReaderChapterContent } from '../src/composables/useReaderChapterContent.js'

function validContent(index) {
  return {
    chapter: { id: index + 1, title: `第 ${index + 1} 章` },
    content: `正文 ${index}`,
  }
}

function staleChapterError(message = 'chapter content changed; retry') {
  const error = new Error(message)
  error.response = {
    status: 409,
    data: { error: message },
  }
  return error
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
  assert.equal(fixture.calls[0][0], 'load')
  assert.deepEqual(fixture.calls[0][1], { id: 7, url: 'https://example.com/book/7' })
  assert.equal(fixture.calls[0][2], 7)
  assert.equal(fixture.calls[0][3], 3)
  assert.equal(fixture.calls[0][4].refresh, true)
  assert.ok(fixture.calls[0][4].signal instanceof AbortSignal)
  assert.equal(fixture.calls[0][4].signal.aborted, false)
  assert.deepEqual(fixture.calls[1], ['cached', 3])
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

test('uses the upstream 30 second budget for shelf and temporary chapter content', async () => {
  const originalAdapter = api.defaults.adapter
  const originalLocalStorage = Object.getOwnPropertyDescriptor(globalThis, 'localStorage')
  const requests = []
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: { getItem: () => null },
  })
  api.defaults.adapter = async config => {
    requests.push(config)
    return {
      data: validContent(0),
      status: 200,
      statusText: 'OK',
      headers: {},
      config,
      request: {},
    }
  }
  try {
    await getChapterContent(7, 0)
    await getRemoteReaderChapterContent('session-7', 0)
  } finally {
    api.defaults.adapter = originalAdapter
    if (originalLocalStorage) {
      Object.defineProperty(globalThis, 'localStorage', originalLocalStorage)
    } else {
      delete globalThis.localStorage
    }
  }

  assert.equal(requests.length, 2)
  assert.equal(requests[0].timeout, 30_000)
  assert.equal(requests[1].timeout, 30_000)
})

test('keeps a shared same-chapter request alive when a new caller replaces an aborted caller', async () => {
  let requestCount = 0
  let resolveLoad
  const fixture = createController({
    loadBrowserContent: (_book, _bookId, index, options) => {
      requestCount += 1
      return new Promise((resolve, reject) => {
        resolveLoad = () => resolve(validContent(index))
        options.signal.addEventListener('abort', () => {
          const error = new Error('chapter request cancelled')
          error.name = 'AbortError'
          reject(error)
        }, { once: true })
      })
    },
  })
  const firstController = new AbortController()
  const secondController = new AbortController()
  const first = fixture.controller.load(2, { signal: firstController.signal })
    .catch(error => error)

  firstController.abort()
  const second = fixture.controller.load(2, { signal: secondController.signal })
  resolveLoad()

  const firstResult = await first
  assert.equal(firstResult.name, 'AbortError')
  assert.deepEqual(await second, validContent(2))
  assert.equal(requestCount, 1)
  assert.deepEqual(fixture.controller.get(2), validContent(2))
})

test('aborts the underlying request after its final caller leaves without replacement', async () => {
  let internalSignal
  const fixture = createController({
    loadBrowserContent: (_book, _bookId, _index, options) => {
      internalSignal = options.signal
      return new Promise((resolve, reject) => {
        options.signal.addEventListener('abort', () => {
          const error = new Error('chapter request cancelled')
          error.name = 'AbortError'
          reject(error)
        }, { once: true })
      })
    },
  })
  const caller = new AbortController()
  const pending = fixture.controller.load(2, { signal: caller.signal }).catch(error => error)

  caller.abort()
  const result = await pending
  await new Promise(resolve => setImmediate(resolve))

  assert.equal(result.name, 'AbortError')
  assert.equal(internalSignal.aborted, true)
  assert.equal(fixture.controller.get(2), null)
})

test('clearing a book aborts its in-flight chapter request', async () => {
  let capturedOptions
  let resolveLoad
  const fixture = createController({
    loadBrowserContent: (_book, _bookId, _index, options) => {
      capturedOptions = options
      return new Promise((resolve, reject) => {
        resolveLoad = resolve
        options.signal?.addEventListener('abort', () => {
          const error = new Error('chapter request cancelled')
          error.name = 'AbortError'
          reject(error)
        }, { once: true })
      })
    },
  })

  const pending = fixture.controller.load(1).then(
    data => ({ status: 'fulfilled', data }),
    error => ({ status: 'rejected', error }),
  )
  fixture.controller.clear(fixture.book.value, fixture.bookId.value)
  if (!capturedOptions?.signal?.aborted) resolveLoad(validContent(1))
  const result = await pending

  assert.ok(capturedOptions.signal instanceof AbortSignal)
  assert.equal(capturedOptions.signal.aborted, true)
  assert.equal(result.status, 'rejected')
  assert.equal(result.error.name, 'AbortError')
  assert.equal(fixture.controller.get(1), null)
  assert.deepEqual(fixture.calls, [])
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

test('retries one transient stale chapter conflict without exposing it', async () => {
  let requestCount = 0
  const refreshes = []
  const fixture = createController({
    loadBrowserContent: async (_book, _bookId, index, options) => {
      requestCount += 1
      refreshes.push(options.refresh)
      if (requestCount === 1) throw staleChapterError()
      return validContent(index)
    },
  })

  const data = await fixture.controller.load(2)

  assert.deepEqual(data, validContent(2))
  assert.equal(requestCount, 2)
  assert.deepEqual(refreshes, [false, false], 'stale recovery must allow a concurrently published cache hit')
  assert.deepEqual(fixture.controller.get(2), validContent(2))
  assert.deepEqual(fixture.calls, [['cached', 2]])
})

test('serializes stale conflict retries within the same book scope', async () => {
  const attempts = new Map()
  let activeRetries = 0
  let maxActiveRetries = 0
  const fixture = createController({
    loadBrowserContent: async (_book, _bookId, index) => {
      const attempt = (attempts.get(index) || 0) + 1
      attempts.set(index, attempt)
      if (attempt === 1) throw staleChapterError()
      activeRetries += 1
      maxActiveRetries = Math.max(maxActiveRetries, activeRetries)
      await new Promise(resolve => setImmediate(resolve))
      activeRetries -= 1
      return validContent(index)
    },
  })

  const [first, second] = await Promise.all([
    fixture.controller.load(1),
    fixture.controller.load(2),
  ])

  assert.deepEqual(first, validContent(1))
  assert.deepEqual(second, validContent(2))
  assert.equal(maxActiveRetries, 1)
  assert.deepEqual([...attempts.entries()], [[1, 2], [2, 2]])
})

test('does not loop repeated stale conflicts or retry unrelated conflicts', async () => {
  let staleCount = 0
  const staleFixture = createController({
    loadBrowserContent: async () => {
      staleCount += 1
      throw staleChapterError()
    },
  })

  await assert.rejects(
    staleFixture.controller.load(1),
    error => error.message === '章节状态已更新，请重试',
  )
  assert.equal(staleCount, 2)

  let unrelatedCount = 0
  const unrelated = staleChapterError('another conflict')
  const unrelatedFixture = createController({
    loadBrowserContent: async () => {
      unrelatedCount += 1
      throw unrelated
    },
  })

  await assert.rejects(unrelatedFixture.controller.load(1), error => error === unrelated)
  assert.equal(unrelatedCount, 1)
})

test('does not start a queued stale retry after clearing the book scope', async () => {
  const attempts = new Map()
  let signalRetryStarted
  const retryStarted = new Promise(resolve => {
    signalRetryStarted = resolve
  })
  const fixture = createController({
    loadBrowserContent: async (_book, _bookId, index, options) => {
      const attempt = (attempts.get(index) || 0) + 1
      attempts.set(index, attempt)
      if (attempt === 1) throw staleChapterError()
      if (index !== 1) throw new Error('queued retry must not start')
      signalRetryStarted()
      return new Promise((resolve, reject) => {
        options.signal.addEventListener('abort', () => {
          const error = new Error('chapter request cancelled')
          error.name = 'AbortError'
          reject(error)
        }, { once: true })
      })
    },
  })

  const first = fixture.controller.load(1).catch(error => error)
  const second = fixture.controller.load(2).catch(error => error)
  await Promise.race([retryStarted, Promise.all([first, second])])
  fixture.controller.clear(fixture.book.value, fixture.bookId.value)
  const [firstError, secondError] = await Promise.all([first, second])

  assert.equal(firstError.name, 'AbortError')
  assert.equal(secondError.name, 'AbortError')
  assert.equal(attempts.get(1), 2)
  assert.equal(attempts.get(2), 1)
  assert.equal(fixture.controller.get(1), null)
  assert.equal(fixture.controller.get(2), null)
})
