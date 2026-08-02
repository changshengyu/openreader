import assert from 'node:assert/strict'
import test from 'node:test'
import { useRemoteBookAddToShelf } from '../src/composables/useRemoteBookAddToShelf.js'

function deferred() {
  let resolve
  let reject
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function createController(overrides = {}) {
  const calls = []
  const controller = useRemoteBookAddToShelf({
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

test('BookInfo direct add never opens the category selector', async () => {
  const fixture = createController()
  const added = await fixture.controller.addRemoteBook(
    { title: '直接入架', sourceId: 8 },
    {
      key: '8-book',
      categoryIds: [4, '2', 4, 0],
      sourceId: 8,
      sourceName: '测试书源',
    },
  )

  assert.equal(fixture.calls.some(([kind]) => kind === 'select-categories'), false)
  assert.deepEqual(added, {
    id: 61,
    title: '直接入架',
    sourceId: 8,
    sourceName: '测试书源',
    categoryIds: [4, 2],
  })
  assert.deepEqual(fixture.calls.at(-1), ['success', '加入书架成功'])
  assert.equal(fixture.controller.addingBookKey.value, '')
})

test('search and explore category-confirmed add resets selection and honors cancel', async () => {
  const fixture = createController({
    selectCategories: async initialCategoryIds => {
      fixture.calls.push(['select-categories', initialCategoryIds])
      return null
    },
  })

  const result = await fixture.controller.addRemoteBookWithCategories(
    { title: '暂不加入', sourceId: 8 },
    { key: '8-book', categoryIds: [9], sourceId: 8, sourceName: '测试书源' },
  )

  assert.equal(result, null)
  assert.deepEqual(fixture.calls, [['select-categories', []]])
  assert.equal(fixture.controller.addingBookKey.value, '')
})

test('search and explore persist only the confirmed deduplicated categories', async () => {
  const fixture = createController()
  const added = await fixture.controller.addRemoteBookWithCategories(
    { title: '确认入架', sourceId: 8 },
    { key: '8-book', categoryIds: [9], sourceId: 8, sourceName: '测试书源' },
  )

  assert.deepEqual(fixture.calls, [
    ['select-categories', []],
    ['create-remote', {
      title: '确认入架',
      sourceId: 8,
      sourceName: '测试书源',
      categoryIds: [3, 2],
    }],
    ['upsert', added],
    ['success', '加入书架成功'],
  ])
})

test('surfaces a failed create without leaving an in-flight add action', async () => {
  const fixture = createController({
    createRemoteBook: async () => {
      throw new Error('network down')
    },
  })

  const result = await fixture.controller.addRemoteBook(
    { title: '失败书', sourceId: 8 },
    { key: '8-book', sourceId: 8, sourceName: '测试书源' },
  )

  assert.equal(result, null)
  assert.deepEqual(fixture.calls.at(-1), ['error', 'network down', '加入书架失败'])
  assert.equal(fixture.controller.addingBookKey.value, '')
})

test('does not upsert or announce a remote book after the authenticated operation expires', async () => {
  const response = deferred()
  let current = true
  const fixture = createController({
    operationGuard: {
      begin: key => ({ key }),
      canCommit: () => current,
      reset: () => {
        current = false
      },
    },
    createRemoteBook: async payload => {
      fixture.calls.push(['create-remote', payload])
      return response.promise
    },
  })

  const pending = fixture.controller.addRemoteBook(
    { title: '账号 A 的书', sourceId: 8 },
    { key: '8-book', sourceId: 8, sourceName: '测试书源' },
  )
  await Promise.resolve()
  current = false
  response.resolve({ data: { id: 61, title: '账号 A 的书' } })

  assert.equal(await pending, null)
  assert.equal(fixture.calls.some(([kind]) => kind === 'upsert'), false)
  assert.equal(fixture.calls.some(([kind]) => kind === 'success'), false)
  assert.equal(fixture.controller.addingBookKey.value, '', 'expired operations must not leave a stale loading tag')
})

test('an older same-book response cannot clear the newer add loading state', async () => {
  const firstResponse = deferred()
  const secondResponse = deferred()
  let requestCount = 0
  const fixture = createController({
    createRemoteBook: async payload => {
      fixture.calls.push(['create-remote', payload])
      requestCount += 1
      return requestCount === 1 ? firstResponse.promise : secondResponse.promise
    },
  })

  const first = fixture.controller.addRemoteBook(
    { title: '同一本书', sourceId: 8 },
    { key: '8-same-book', sourceId: 8, sourceName: '测试书源' },
  )
  const second = fixture.controller.addRemoteBook(
    { title: '同一本书', sourceId: 8 },
    { key: '8-same-book', sourceId: 8, sourceName: '测试书源' },
  )

  firstResponse.resolve({ data: { id: 61, title: '旧响应' } })
  assert.equal(await first, null)
  assert.equal(fixture.controller.addingBookKey.value, '8-same-book')

  secondResponse.resolve({ data: { id: 61, title: '新响应' } })
  assert.equal((await second)?.title, '新响应')
  assert.equal(fixture.controller.addingBookKey.value, '')
})

test('Overlay owns one cancellable result-card category-selection transaction', async () => {
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
