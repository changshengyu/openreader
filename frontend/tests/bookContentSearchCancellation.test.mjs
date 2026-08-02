import assert from 'node:assert/strict'
import test from 'node:test'
import { nextTick, ref } from 'vue'
import { useBookContentSearch } from '../src/composables/useBookContentSearch.js'

function deferredSearchRequest(calls) {
  return (bookId, keyword, params, options = {}) => new Promise((resolve, reject) => {
    const call = { bookId, keyword, params, options, resolve, reject }
    calls.push(call)
    options.signal?.addEventListener('abort', () => {
      reject(new DOMException('The operation was aborted', 'AbortError'))
    }, { once: true })
  })
}

function searchController(searchRequest, onError = () => {}, options = {}) {
  return useBookContentSearch({
    bookId: ref(7),
    book: ref({ id: 7, sourceId: 9 }),
    chapters: ref([]),
    searchRequest,
    onError,
    ...options,
  })
}

test('closing a content search aborts transport without clearing completed same-book state', async () => {
  const calls = []
  const errors = []
  const controller = searchController(deferredSearchRequest(calls), error => errors.push(error))
  controller.keyword.value = '目标'
  await nextTick()

  const pending = controller.search()
  await nextTick()
  assert.equal(calls.length, 1)
  assert.ok(calls[0].options.signal instanceof AbortSignal)
  assert.equal(calls[0].options.signal.aborted, false)

  controller.cancel()
  await pending
  assert.equal(calls[0].options.signal.aborted, true)
  assert.equal(controller.loading.value, false)
  assert.equal(controller.keyword.value, '目标')
  assert.deepEqual(controller.results.value, [])
  assert.deepEqual(errors, [])
})

test('replacing the keyword aborts stale content search and a successful request stays active until completion', async () => {
  const calls = []
  const controller = searchController(deferredSearchRequest(calls))
  controller.keyword.value = '旧关键词'
  await nextTick()
  const stale = controller.search()
  await nextTick()

  controller.keyword.value = '新关键词'
  await nextTick()
  assert.equal(calls[0].options.signal.aborted, true)
  await stale

  const current = controller.search()
  await nextTick()
  assert.equal(calls.length, 2)
  assert.equal(calls[1].options.signal.aborted, false)
  calls[1].resolve({
    data: {
      list: [{ chapterIndex: 2, excerpt: '新关键词命中' }],
      lastIndex: 2,
      hasMore: false,
      total: 3,
    },
  })
  await current

  assert.equal(calls[1].options.signal.aborted, false)
  assert.equal(controller.loading.value, false)
  assert.equal(controller.results.value.length, 1)
})

test('does not commit content rows after the authenticated operation expires', async () => {
  const calls = []
  let current = true
  const controller = searchController(
    deferredSearchRequest(calls),
    () => {},
    {
      operationGuard: {
        begin: key => ({ key }),
        canCommit: () => current,
        reset: () => {},
      },
    },
  )
  controller.keyword.value = '账号 A'
  await nextTick()
  const pending = controller.search()
  await nextTick()

  current = false
  calls[0].resolve({
    data: {
      list: [{ chapterIndex: 1, excerpt: '账号 A 私有正文' }],
      lastIndex: 1,
      hasMore: false,
      total: 2,
    },
  })
  await pending

  assert.deepEqual(controller.results.value, [])
})

test('passes leading, trailing and all-space queries unchanged and rejects only the empty string', async () => {
  const calls = []
  const controller = searchController(deferredSearchRequest(calls))

  controller.keyword.value = ' 目标 '
  await nextTick()
  const padded = controller.search()
  await nextTick()
  assert.equal(calls[0].keyword, ' 目标 ')
  calls[0].resolve({
    data: { list: [], lastIndex: 0, hasMore: false, total: 1 },
  })
  await padded

  controller.keyword.value = '   '
  await nextTick()
  const spaces = controller.search()
  await nextTick()
  assert.equal(calls.length, 2)
  assert.equal(calls[1].keyword, '   ')
  calls[1].resolve({
    data: { list: [], lastIndex: 0, hasMore: false, total: 1 },
  })
  await spaces

  controller.keyword.value = ''
  await nextTick()
  await controller.search()
  assert.equal(calls.length, 2)
})
