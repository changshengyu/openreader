import assert from 'node:assert/strict'
import test from 'node:test'
import {
  buildBookSourceGroups,
  mergeBookSourceCandidates,
  nextBookSourcePage,
} from '../src/utils/bookSourceCandidates.js'

test('merges source candidate pages without duplicate sources', () => {
  const first = [{ sourceId: 1, bookUrl: 'https://one/book' }]
  const second = [
    { sourceId: 1, bookUrl: 'https://one/book' },
    { sourceId: 2, bookUrl: 'https://two/book' },
  ]
  assert.deepEqual(mergeBookSourceCandidates(first, second), [first[0], second[1]])
})

test('builds sorted enabled source groups with counts', () => {
  assert.deepEqual(buildBookSourceGroups([
    { group: '乙', enabled: true },
    { group: '甲', enabled: true },
    { group: '甲', enabled: true },
    { group: '隐藏', enabled: false },
  ]), [
    { value: '甲', label: '甲', count: 2 },
    { value: '乙', label: '乙', count: 1 },
  ])
})

test('uses server paging metadata and a stable fallback', () => {
  assert.deepEqual(nextBookSourcePage({ nextOffset: 24, hasMore: true }, 3, 10), {
    offset: 24,
    hasMore: true,
  })
  assert.deepEqual(nextBookSourcePage({}, 4, 0, 4), {
    offset: 4,
    hasMore: true,
  })
})
