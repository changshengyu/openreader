import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive } from 'vue'
import { useBookInfoAddToShelf } from '../src/composables/useBookInfoAddToShelf.js'

function createController(overrides = {}) {
  const calls = []
  const controller = useBookInfoAddToShelf({
    selectCategories: async initialCategoryIds => {
      calls.push(['select-categories', initialCategoryIds])
      return [3, 3, 0, '2']
    },
    buildPayload: (book, categoryIds, context) => ({
      title: book.title,
      sourceId: context.sourceId,
      sourceName: context.sourceName,
      categoryIds,
    }),
    createRemoteBook: async payload => {
      calls.push(['create-remote', payload])
      return { data: { id: 61, ...payload } }
    },
    upsertBook: book => calls.push(['upsert', book]),
    onSuccess: message => calls.push(['success', message]),
    onError: (error, fallback) => calls.push(['error', error?.message, fallback]),
    ...overrides,
  })
  return { calls, controller }
}

test('requires a shared category selection before mutating a remote shelf book', async () => {
  const fixture = createController({
    selectCategories: async initialCategoryIds => {
      fixture.calls.push(['select-categories', initialCategoryIds])
      return null
    },
  })

  const result = await fixture.controller.addRemoteBook(
    { title: '待入架', sourceId: 8 },
    { key: '8-book', categoryIds: [4, '2'], sourceId: 8, sourceName: '测试书源' },
  )

  assert.equal(result, null)
  assert.deepEqual(fixture.calls, [['select-categories', [4, 2]]])
  assert.equal(fixture.controller.addingBookKey.value, '')
})

test('creates only confirmed categories, deduplicates them, and resets the shared loading key', async () => {
  const fixture = createController()

  const added = await fixture.controller.addRemoteBook(
    { title: '已确认入架', sourceId: 8 },
    { key: '8-book', categoryIds: [4, '2'], sourceId: 8, sourceName: '测试书源' },
  )

  assert.deepEqual(added, {
    id: 61,
    title: '已确认入架',
    sourceId: 8,
    sourceName: '测试书源',
    categoryIds: [3, 2],
  })
  assert.deepEqual(fixture.calls, [
    ['select-categories', [4, 2]],
    ['create-remote', {
      title: '已确认入架',
      sourceId: 8,
      sourceName: '测试书源',
      categoryIds: [3, 2],
    }],
    ['upsert', added],
    ['success', '已加入书架：《已确认入架》'],
  ])
  assert.equal(fixture.controller.addingBookKey.value, '')
})

test('surfaces a failed confirmed create without leaving an in-flight BookInfo action', async () => {
  const fixture = createController({
    createRemoteBook: async () => {
      throw new Error('network down')
    },
  })

  const result = await fixture.controller.addRemoteBook(
    { title: '失败书', sourceId: 8 },
    { key: '8-book', categoryIds: [], sourceId: 8, sourceName: '测试书源' },
  )

  assert.equal(result, null)
  assert.deepEqual(fixture.calls.at(-1), ['error', 'network down', '加入书架失败'])
  assert.equal(fixture.controller.addingBookKey.value, '')
})

test('Overlay owns one cancellable BookInfo category-selection transaction', async () => {
  const { useOverlayStore } = await import('../src/stores/overlay.js')
  const { createPinia, setActivePinia } = await import('pinia')
  setActivePinia(createPinia())
  const overlay = useOverlayStore()

  const first = overlay.selectBookAddCategories([3, '2', 0, 3])
  assert.equal(overlay.bookAddCategoryVisible, true)
  assert.deepEqual(overlay.bookAddCategoryIds, [3, 2])

  const second = overlay.selectBookAddCategories([7])
  assert.deepEqual(await first, null, 'a replaced selector resolves as cancelled')
  assert.deepEqual(overlay.bookAddCategoryIds, [7])

  overlay.finishBookAddCategories([9, '7', 0, 9])
  assert.deepEqual(await second, [9, 7])
  assert.equal(overlay.bookAddCategoryVisible, false)
  assert.deepEqual(overlay.bookAddCategoryIds, [])
})
