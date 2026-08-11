import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useBookSourceCandidates } from '../src/composables/useBookSourceCandidates.js'
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

test('separates available refresh and per-group search cursors without clearing visible candidates', async () => {
  const calls = []
  const controller = useBookSourceCandidates({
    bookId: ref(7),
    groupSources: ref([
      { group: '甲', enabled: true },
      { group: '乙', enabled: true },
    ]),
    listCandidates: async (bookId, params) => {
      calls.push([bookId, { ...params }])
      if (params.mode === 'available') {
        return { data: [{ sourceId: 1, bookUrl: 'https://one/current', current: true }] }
      }
      if (params.mode === 'refresh') {
        return { data: [
          { sourceId: 1, bookUrl: 'https://one/current', current: true },
          { sourceId: 2, bookUrl: 'https://two/cached', current: false },
        ] }
      }
      const nextOffset = params.group === '甲' ? 3 : 4
      return {
        data: {
          list: [{ sourceId: nextOffset + 10, bookUrl: `https://search/${params.group || 'all'}/${params.offset}` }],
          nextOffset,
          hasMore: true,
        },
      }
    },
  })

  await controller.open()
  await controller.refresh()
  await controller.loadMore()
  await controller.changeGroup('甲')
  assert.equal(controller.candidates.value.length, 3)
  await controller.loadMore()
  await controller.changeGroup('')
  await controller.loadMore()

  assert.deepEqual(calls, [
    [7, { mode: 'available' }],
    [7, { mode: 'refresh' }],
    [7, { mode: 'search', group: undefined, offset: 0, limit: 10, paged: 1 }],
    [7, { mode: 'search', group: '甲', offset: 0, limit: 10, paged: 1 }],
    [7, { mode: 'search', group: undefined, offset: 4, limit: 10, paged: 1 }],
  ])
  assert.equal(controller.opening.value, false)
  assert.equal(controller.refreshing.value, false)
  assert.equal(controller.loadingMore.value, false)
})

test('projects a changed source locally without a candidate request', async () => {
  let requestCount = 0
  const controller = useBookSourceCandidates({
    bookId: ref(9),
    listCandidates: async () => {
      requestCount += 1
      return { data: [] }
    },
  })
  controller.candidates.value = [
    { sourceId: 1, bookUrl: 'https://one/book', current: true },
    { sourceId: 2, bookUrl: 'https://two/book', current: false, sourceName: '新来源' },
  ]

  controller.applyChangedSource({ sourceId: 2, bookUrl: 'https://two/book', sourceName: '新来源' })

  assert.equal(requestCount, 0)
  assert.deepEqual(controller.candidates.value.map(item => item.current), [false, true])
})
