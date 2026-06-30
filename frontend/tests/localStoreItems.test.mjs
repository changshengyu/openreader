import assert from 'node:assert/strict'
import test from 'node:test'

import { filterLocalStoreItems, limitLocalStoreItems } from '../src/utils/localStoreItems.js'

test('limits a large local store result before rendering', () => {
  const items = Array.from({ length: 250 }, (_, index) => ({ name: `book-${index}.txt`, path: `books/book-${index}.txt` }))

  assert.equal(limitLocalStoreItems(items, 100).length, 100)
  assert.equal(items.length, 250)
})

test('filters the complete local store result before applying the render limit', () => {
  const items = [
    { name: 'alpha.txt', path: 'fiction/alpha.txt', extension: 'txt' },
    { name: 'beta.epub', path: 'fiction/beta.epub', extension: 'epub' },
    { name: 'fiction', path: 'fiction', isDir: true },
  ]
  const filtered = filterLocalStoreItems(items, { keyword: 'fiction', extension: 'txt' })

  assert.deepEqual(filtered.map(item => item.name), ['alpha.txt', 'fiction'])
  assert.deepEqual(limitLocalStoreItems(filtered, 1).map(item => item.name), ['alpha.txt'])
})
