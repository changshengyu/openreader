import assert from 'node:assert/strict'
import test from 'node:test'
import { chapterCacheKeyPrefix } from '../src/utils/bookChapterCache.js'

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
