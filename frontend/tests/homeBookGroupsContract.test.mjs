import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const home = readFileSync(new URL('../src/views/Home.vue', import.meta.url), 'utf8')

test('drives shelf tabs from the persisted unified book-group projection', () => {
  assert.match(home, /visibleBookGroups\(bookshelf\.bookGroups, bookshelf\.books\)/)
  assert.match(home, /filterBooksByBookGroup\(sortedBooks\.value, selectedGroup\.value\)/)
  assert.match(home, /preferences\.shelf\.groupKey/)
  assert.match(home, /preferences\.setShelfGroup/)
  assert.equal((home.match(/preferences\.setShelfGroup/g) || []).length, 1, 'only direct tab selection may persist the group token')
  assert.doesNotMatch(home, /\{ id: '', name: '全部'/)
  assert.doesNotMatch(home, /selectedGroup\.value === 'local'/)
  assert.doesNotMatch(home, /:title="`\$\{item\.name} \(\$\{item\.count}\)`"/)
})

test('warms the projection with the shelf instead of constructing built-ins locally', () => {
  assert.match(home, /bookshelf\.ensureBookGroupsLoaded\(\)/)
  assert.match(home, /bookshelf\.loadBookGroups\(\{ force: true \}\)/)
})
