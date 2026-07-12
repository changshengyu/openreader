import assert from 'node:assert/strict'
import test from 'node:test'

import {
  LOCAL_STORE_INITIAL_ITEM_LIMIT,
  filterLocalStoreItems,
  limitLocalStoreItems,
  shouldFilterLocalStoreItems,
  visibleLocalStoreItems,
} from '../src/utils/localStoreItems.js'

test('uses the upstream 101-row first page and one-action reveal-all state', () => {
  const items = Array.from({ length: 250 }, (_, index) => ({ name: `book-${index}.txt`, path: `books/book-${index}.txt` }))

  assert.equal(LOCAL_STORE_INITIAL_ITEM_LIMIT, 101)
  assert.equal(limitLocalStoreItems(items, LOCAL_STORE_INITIAL_ITEM_LIMIT).length, 101)
  assert.equal(visibleLocalStoreItems(items, false).length, 101)
  assert.equal(visibleLocalStoreItems(items, true).length, 250)
  assert.equal(items.length, 250)
})

test('keeps the upstream search threshold: only more than two trimmed characters filter', () => {
  const items = [
    { name: 'alpha.txt', path: 'fiction/alpha.txt', extension: 'txt' },
    { name: 'beta.epub', path: 'fiction/beta.epub', extension: 'epub' },
    { name: 'fiction', path: 'fiction', isDir: true },
  ]

  assert.equal(shouldFilterLocalStoreItems(' fi '), false)
  assert.equal(shouldFilterLocalStoreItems(' fic '), true)
  assert.deepEqual(filterLocalStoreItems(items, { keyword: 'fi' }).map(item => item.name), items.map(item => item.name))
  assert.deepEqual(filterLocalStoreItems(items, { keyword: 'fic' }).map(item => item.name), ['alpha.txt', 'beta.epub', 'fiction'])
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
