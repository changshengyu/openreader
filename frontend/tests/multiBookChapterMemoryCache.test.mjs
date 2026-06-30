import assert from 'node:assert/strict'
import test from 'node:test'

import { createMultiBookChapterMemoryCache } from '../src/utils/multiBookChapterMemoryCache.js'

test('preserves chapter content across book switches', () => {
  const cache = createMultiBookChapterMemoryCache(3)
  cache.set('book-a', 0, { content: 'A0' })
  cache.set('book-b', 0, { content: 'B0' })

  assert.deepEqual(cache.get('book-a', 0), { content: 'A0' })
  assert.deepEqual(cache.get('book-b', 0), { content: 'B0' })
})

test('evicts the least recently used book after reaching the limit', () => {
  const cache = createMultiBookChapterMemoryCache(3)
  cache.set('book-a', 0, { content: 'A0' })
  cache.set('book-b', 0, { content: 'B0' })
  cache.set('book-c', 0, { content: 'C0' })
  cache.get('book-a', 0)
  cache.set('book-d', 0, { content: 'D0' })

  assert.deepEqual(cache.bookKeys(), ['book-c', 'book-a', 'book-d'])
  assert.equal(cache.get('book-b', 0), null)
})

test('clears only the selected book', () => {
  const cache = createMultiBookChapterMemoryCache(3)
  cache.set('book-a', 0, { content: 'A0' })
  cache.set('book-b', 0, { content: 'B0' })
  cache.clearBook('book-a')

  assert.equal(cache.get('book-a', 0), null)
  assert.deepEqual(cache.get('book-b', 0), { content: 'B0' })
})
