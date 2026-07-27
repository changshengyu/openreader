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
  assert.deepEqual(writes, [[
    'user:11@Scoped_Reader@book-url-7@chapterContent-0',
    result,
  ]])
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
