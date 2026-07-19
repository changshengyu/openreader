import assert from 'node:assert/strict'
import test from 'node:test'
import {
  bookGroupBookCount,
  filterBooksByBookGroup,
  resolveBookGroupSelection,
  visibleBookGroups,
} from '../src/utils/bookGroups.js'

const groups = [
  { key: 'builtin:audio', kind: 'builtin', semantic: 'audio', name: '有声', show: true, sortOrder: 10 },
  { key: 'category:2', kind: 'category', semantic: 'category', categoryId: 2, name: '历史', show: true, sortOrder: 20 },
  { key: 'builtin:all', kind: 'builtin', semantic: 'all', name: '全部', show: true, sortOrder: 30 },
  { key: 'builtin:local', kind: 'builtin', semantic: 'local', name: '本地', show: false, sortOrder: 40 },
  { key: 'builtin:ungrouped', kind: 'builtin', semantic: 'ungrouped', name: '未分组', show: true, sortOrder: 50 },
  { key: 'category:1', kind: 'category', semantic: 'category', categoryId: 1, name: '玄幻', show: true, sortOrder: 60 },
  { key: 'category:3', kind: 'category', semantic: 'category', categoryId: 3, name: '空分组', show: true, sortOrder: 70 },
]

const books = [
  { id: 1, sourceId: 0, type: 0, categoryIds: [1] },
  { id: 2, sourceId: 8, type: 1, categoryIds: [1, 2] },
  { id: 3, sourceId: 8, type: 0, categoryIds: [] },
]

test('implements all four upstream built-in filters and many-to-many custom filters', () => {
  assert.deepEqual(
    groups.map(group => bookGroupBookCount(group, books)),
    [1, 1, 3, 1, 1, 2, 0],
  )
  assert.deepEqual(filterBooksByBookGroup(books, 'builtin:all').map(book => book.id), [1, 2, 3])
  assert.deepEqual(filterBooksByBookGroup(books, 'builtin:local').map(book => book.id), [1])
  assert.deepEqual(filterBooksByBookGroup(books, 'builtin:audio').map(book => book.id), [2])
  assert.deepEqual(filterBooksByBookGroup(books, 'builtin:ungrouped').map(book => book.id), [3])
  assert.deepEqual(filterBooksByBookGroup(books, 'category:1').map(book => book.id), [1, 2])
  assert.deepEqual(filterBooksByBookGroup(books, 'category:2').map(book => book.id), [2])
  assert.deepEqual(filterBooksByBookGroup(books, 'category:999').map(book => book.id), [])
})

test('shows only visible non-empty groups in unified order and falls back to the first one', () => {
  const visible = visibleBookGroups(groups, books)
  assert.deepEqual(visible.map(group => group.key), [
    'builtin:audio',
    'category:2',
    'builtin:all',
    'builtin:ungrouped',
    'category:1',
  ])
  assert.deepEqual(visible.map(group => group.count), [1, 1, 3, 1, 2])
  assert.equal(resolveBookGroupSelection(groups, books, 'category:2'), 'category:2')
  assert.equal(resolveBookGroupSelection(groups, books, 'builtin:local'), 'builtin:audio')
  assert.equal(resolveBookGroupSelection(groups, books, 'category:3'), 'builtin:audio')
  assert.equal(resolveBookGroupSelection(groups, [], 'builtin:all'), 'builtin:all')
})
