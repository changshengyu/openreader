import assert from 'node:assert/strict'
import test from 'node:test'
import {
  chapterCacheKeyPrefix,
  clearBookBrowserChapterCache,
  loadBrowserChapterContent,
} from '../src/utils/bookChapterCache.js'

test('can freeze a deletion cache prefix to the authenticated scope that received the event', () => {
  const book = { id: 7, title: 'Scoped', author: 'Reader', url: 'book-url-7' }
  assert.equal(
    chapterCacheKeyPrefix(book, 7, 'user:11'),
    'user:11@Scoped_Reader@book-url-7',
  )
  assert.equal(
    chapterCacheKeyPrefix(book, 7, 'user:22'),
    'user:22@Scoped_Reader@book-url-7',
  )
})

test('authenticated chapter loading never falls back to an unowned legacy cache key', async () => {
  const book = { id: 7, title: 'Scoped', author: 'Reader', url: 'book-url-7' }
  const reads = []
  const writes = []
  let fetched = 0
  const result = await loadBrowserChapterContent(book, 7, 0, {
    scope: 'user:11',
    getCache: async key => {
      reads.push(key)
      if (key === 'Scoped_Reader@book-url-7@chapterContent-0') {
        return { chapter: { index: 0 }, content: 'legacy content' }
      }
      return null
    },
    setCache: async (...args) => writes.push(args),
    getChapterContent: async () => {
      fetched += 1
      return {
        data: { chapter: { index: 0 }, content: 'network content' },
      }
    },
  })

  assert.deepEqual(reads, ['user:11@Scoped_Reader@book-url-7@chapterContent-0'])
  assert.equal(fetched, 1)
  assert.equal(result.content, 'network content')
  assert.equal(writes.length, 1)
  const [writtenKey, value, canWrite] = writes[0]
  assert.equal(writtenKey, 'user:11@Scoped_Reader@book-url-7@chapterContent-0')
  assert.deepEqual(value, result)
  assert.equal(canWrite(), true)
})

test('forwards cancellation and never caches a response cancelled during fetch', async () => {
  const book = { id: 7, title: 'Scoped', author: 'Reader', url: 'book-url-7' }
  const controller = new AbortController()
  const calls = []
  const writes = []
  const result = await loadBrowserChapterContent(book, 7, 0, {
    scope: 'user:11',
    refresh: true,
    signal: controller.signal,
    getChapterContent: async (...args) => {
      calls.push(args)
      controller.abort()
      return { data: { chapter: { index: 0 }, content: 'cancelled content' } }
    },
    setCache: async (...args) => writes.push(args),
  })

  assert.deepEqual(calls, [[7, 0, { signal: controller.signal }]])
  assert.equal(result.content, 'cancelled content')
  assert.deepEqual(writes, [])
})

test('book cleanup deletes only the captured scoped chapter prefix', async () => {
  const book = { id: 7, title: 'Scoped', author: 'Reader', url: 'book-url-7' }
  const prefixes = []
  const removed = await clearBookBrowserChapterCache(book, 7, {
    scope: 'user:11',
    removeKeys: async prefix => {
      prefixes.push(prefix)
      return 2
    },
  })

  assert.equal(removed, 2)
  assert.deepEqual(prefixes, [
    'user:11@Scoped_Reader@book-url-7@chapterContent-',
  ])
})

test('publishes explicit refresh content even when an older browser cache load finishes later', async () => {
  const book = { id: 7, title: 'Scoped', author: 'Reader', url: 'book-url-7' }
  let completeOld
  let signalOldStarted
  const oldStarted = new Promise(resolve => { signalOldStarted = resolve })
  const existing = { chapter: { index: 0 }, content: 'previous body' }
  const fresh = { chapter: { index: 0 }, content: 'updated body' }
  const writes = []
  const queries = []
  let reads = 0
  const options = {
    scope: 'user:11',
    getCache: async () => {
      reads += 1
      return null
    },
    setCache: async (_key, data) => writes.push(data),
    getChapterContent: (_id, _index, request) => {
      queries.push(request.refreshRemote)
      if (queries.length === 1) {
        return new Promise(resolve => {
          completeOld = resolve
          signalOldStarted()
        })
      }
      return Promise.resolve({ data: fresh })
    },
  }

  const old = loadBrowserChapterContent(book, 7, 0, options)
  await oldStarted
  const refreshed = await loadBrowserChapterContent(book, 7, 0, { ...options, refreshRemote: true })
  completeOld({ data: existing })

  assert.deepEqual(await old, existing)
  assert.deepEqual(refreshed, fresh)
  assert.equal(reads, 1)
  assert.deepEqual(queries, [undefined, true])
  assert.deepEqual(writes, [fresh])
})
