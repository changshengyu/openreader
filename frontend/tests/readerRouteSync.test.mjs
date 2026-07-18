import assert from 'node:assert/strict'
import test from 'node:test'
import { effectScope, nextTick, reactive, ref } from 'vue'
import { useReaderRouteSync } from '../src/composables/useReaderRouteSync.js'
import { parseReaderRoutePercent } from '../src/utils/readerRoute.js'

async function flushRouteSync() {
  await nextTick()
  await Promise.resolve()
  await nextTick()
}

function createController(overrides = {}) {
  const bookId = ref(1)
  const currentIndex = ref(0)
  const query = reactive({})
  const calls = []
  const scope = effectScope()
  let routeSync
  scope.run(() => {
    routeSync = useReaderRouteSync({
      bookId,
      currentIndex,
      positionQuery: () => [query.chapter, query.offset, query.percent],
      searchQuery: () => [query.line, query.match, query.q],
      loadBook: async () => calls.push(['book']),
      loadChapter: async (...args) => calls.push(['chapter', ...args]),
      jumpToRouteLine: async () => calls.push(['jump']),
      onBookLoadStart: () => calls.push(['start']),
      onBookLoadError: error => calls.push(['error', error.message]),
      ...overrides,
    })
  })
  return { bookId, calls, currentIndex, query, routeSync, scope }
}

test('reloads a changed book and reports load failures', async () => {
  const success = createController()
  success.bookId.value = 2
  await flushRouteSync()
  assert.deepEqual(success.calls, [['start'], ['book']])
  success.scope.stop()

  const failure = createController({
    loadBook: async () => {
      throw new Error('load failed')
    },
  })
  failure.bookId.value = 2
  await flushRouteSync()
  assert.deepEqual(failure.calls, [['start'], ['error', 'load failed']])
  failure.scope.stop()
})

test('loads explicit route positions before applying search location', async () => {
  const controller = createController()
  Object.assign(controller.query, {
    chapter: '3',
    offset: '120',
    percent: '1.4',
  })
  await flushRouteSync()
  assert.deepEqual(controller.calls, [
    ['chapter', 3, 120, { restorePercent: 1, saveAfterLoad: true }],
    ['jump'],
  ])
  controller.scope.stop()
})

test('suppresses the route reload for an externally applied position exactly once', async () => {
  const controller = createController()
  controller.routeSync.suppressNextPositionReload({
    chapter: 2,
    offset: 88,
    percent: 0.4,
  })
  Object.assign(controller.query, {
    chapter: '2',
    offset: '88',
    percent: '0.4',
  })
  await flushRouteSync()
  assert.deepEqual(controller.calls, [['jump']])

  controller.query.offset = '99'
  await flushRouteSync()
  assert.deepEqual(controller.calls, [
    ['jump'],
    ['chapter', 2, 99, { restorePercent: 0.4, saveAfterLoad: true }],
    ['jump'],
  ])
  controller.scope.stop()
})

test('search-only route changes jump without reloading the chapter', async () => {
  const controller = createController()
  controller.query.q = '关键字'
  await flushRouteSync()
  assert.deepEqual(controller.calls, [['jump']])
  controller.scope.stop()
})

test('parses optional route percentages within valid bounds', () => {
  assert.equal(parseReaderRoutePercent(undefined), null)
  assert.equal(parseReaderRoutePercent('bad'), null)
  assert.equal(parseReaderRoutePercent('-0.2'), 0)
  assert.equal(parseReaderRoutePercent('0.45'), 0.45)
  assert.equal(parseReaderRoutePercent('2'), 1)
})
