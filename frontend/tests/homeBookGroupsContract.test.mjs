import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const home = readFileSync(new URL('../src/views/Home.vue', import.meta.url), 'utf8')

test('drives shelf tabs from the persisted unified book-group projection', () => {
  assert.match(home, /visibleBookGroups\(bookshelf\.bookGroups, bookshelf\.books\)/)
  assert.match(home, /filterBooksByBookGroup\(sortedBooks\.value, selectedGroup\.value\)/)
  assert.match(home, /preferences\.shelf\.groupKey/)
  assert.match(home, /preferences\.setShelfGroup/)
  assert.match(home, /if \(!bookshelf\.bookGroupsLoadedAt \|\| !bookshelf\.booksLoadedAt\) return/)
  assert.doesNotMatch(home, /\{ id: '', name: '全部'/)
  assert.doesNotMatch(home, /selectedGroup\.value === 'local'/)
})

test('warms the projection with the shelf instead of constructing built-ins locally', () => {
  assert.match(home, /bookshelf\.ensureBookGroupsLoaded\(\)/)
  assert.match(home, /bookshelf\.loadBookGroups\(\{ force: true \}\)/)
})
