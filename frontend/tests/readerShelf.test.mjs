import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { computed, ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { useReaderShelf } from '../src/composables/useReaderShelf.js'
import { sortByShelfOrder } from '../src/utils/bookOrder.js'
import { readerRouteQueryFromBook } from '../src/utils/readerRoute.js'

const readerShelfSource = readFileSync(new URL('../src/composables/useReaderShelf.js', import.meta.url), 'utf8')

// Contract: docs/compat/reader-shelf-switch-stale-catalog-fixed-baseline-second-audit-p1-contract.md R2 (switch route
// percent), 8.2(1) book 6. useReaderShelf.js cannot be imported under node --test (extensionless imports).
test('a reader shelf switch never derives the route percent from the previous book catalog', () => {
  // Book 6: the shelf row has no chapterCount and old/WebDAV progress has only a book-level percent.
  const item = { id: 6, progress: { chapterIndex: 4, offset: 0, percent: 0.75 } }
  assert.deepEqual(readerRouteQueryFromBook(item, item.progress), { resume: '1', chapter: 4 })
  // Borrowing a 6-chapter previous catalog fabricates 0.75 * 6 - 4 = 0.5 for a book whose own length is unknown.
  assert.deepEqual(readerRouteQueryFromBook(item, item.progress, 6), { resume: '1', chapter: 4, percent: 0.5 })

  const select = readerShelfSource.match(/async function select\(item\) \{[\s\S]*?\n {2}\}/)?.[0] || ''
  assert.ok(select, 'useReaderShelf must define select(item)')
  assert.doesNotMatch(
    select,
    /currentChapterCount|fallbackCount/,
    'select must not fall back to the current (previous) book chapter count',
  )
  assert.match(
    select,
    /readerRouteQueryFromBook\(item, progress(?:, item\?\.chapterCount)?\)/,
    'select builds the switch route only from the selected book row and its newest progress',
  )
})

test('offset-only saved progress never adds a fabricated zero percentage', () => {
  for (const chapterPercent of [0, null, undefined, '']) {
    assert.deepEqual(readerRouteQueryFromBook({ chapterCount: 2, progress: {
      chapterIndex: 1, offset: 240, percent: 0.5, chapterPercent,
    } }), { resume: '1', chapter: 1, offset: 240 })
  }
  assert.deepEqual(readerRouteQueryFromBook({ progress: { chapterIndex: 0, offset: 0, chapterPercent: 0 } }),
    { resume: '1', chapter: 0, percent: 0 })
})

test('orders the reader shelf by the newest merged reading activity', () => {
  const books = [
    { id: 1, updatedAt: '2026-01-01T00:00:00Z' },
    { id: 2, updatedAt: '2026-01-02T00:00:00Z' },
  ]
  const progress = {
    1: { bookId: 1, updatedAt: '2026-01-03T00:00:00Z' },
  }
  assert.deepEqual(sortByShelfOrder(books, progress).map(book => book.id), [1, 2])
})

test('builds a reader resume route from shelf progress', () => {
  assert.deepEqual(readerRouteQueryFromBook({
    id: 8,
    chapterCount: 20,
    progress: {
      chapterIndex: 4,
      offset: 120,
      chapterPercent: 0.35,
    },
  }), {
    resume: '1',
    chapter: 4,
    offset: 120,
    percent: 0.35,
  })
})

// docs/compat/reader-failure-progress-payload-fixed-baseline-second-audit-p0-contract.md P5: reader-dev's changeBook does
// not wait for any progress save; the save before an OpenReader shelf switch is best effort.
test('a rejected progress save does not stop a reader shelf book switch (P5)', async () => {
  for (const failure of [
    Object.assign(new Error('Request failed with status code 400'), { response: { status: 400 } }),
    new Error('Network Error'),
  ]) {
    const pushed = []
    const shelf = useReaderShelf({
      bookshelf: { books: [{ id: 1, title: '甲' }, { id: 2, title: '乙', chapterCount: 3 }] },
      reader: { progressByBook: {} },
      currentBookId: ref(1),
      currentChapterCount: () => 8,
      router: { push: async location => pushed.push(location) },
      saveProgress: () => Promise.reject(failure),
    })
    await assert.doesNotReject(shelf.select({ id: 2, title: '乙', chapterCount: 3 }), `${failure.message} must not abort the switch`)
    assert.equal(pushed.length, 1, `${failure.message}: the switch must still navigate`)
    assert.deepEqual(pushed[0]?.params, { id: 2 })
  }
})


// Shelf switch feedback contract §10.1: these are real Router results, including resolved navigation failures.
async function feedbackShelf({ saveFails = false, guard } = {}) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/books/:id/read', name: 'reader', component: {} },
      { path: '/other', name: 'other', component: {} },
    ],
  })
  await router.push({ name: 'reader', params: { id: 1 } })
  if (guard) router.beforeEach(guard)
  router.onError(() => {})
  const messages = []
  const shelf = useReaderShelf({
    bookshelf: { books: [] },
    reader: { progressByBook: {} },
    currentBookId: computed(() => router.currentRoute.value.params.id),
    router,
    saveProgress: async () => { if (saveFails) throw new Error('save failed') },
    onSuccess: message => messages.push(message),
  })
  return { shelf, router, messages }
}

for (const saveFails of [false, true]) {
  test(`successful shelf navigation reports once after navigation (saveFails=${saveFails})`, async () => {
    let release
    const gate = new Promise(resolve => { release = resolve })
    const { shelf, router, messages } = await feedbackShelf({ saveFails, guard: () => gate })
    const switching = shelf.select({ id: 2 })
    await new Promise(resolve => setImmediate(resolve))
    assert.deepEqual(messages, [], 'no success while navigation is pending')
    release(true)
    await switching
    assert.equal(router.currentRoute.value.params.id, '2')
    assert.deepEqual(messages, ['换书成功'])
    await shelf.select({ id: 2 })
    assert.deepEqual(messages, ['换书成功'], 'current book selection stays silent')
  })
}

for (const [label, guard] of [
  ['aborted', () => false],
  ['rejected', () => { throw new Error('guard failed') }],
  ['redirected to another book', to => to.params.id === '2' ? { name: 'reader', params: { id: 3 } } : true],
  ['redirected away', to => to.name === 'reader' ? { name: 'other' } : true],
]) {
  test(`shelf navigation ${label} never reports success`, async () => {
    const { shelf, messages } = await feedbackShelf({ guard })
    await shelf.select({ id: 2 }).catch(() => {})
    assert.deepEqual(messages, [])
  })
}

test('concurrent duplicate shelf navigation only reports the successful navigation', async () => {
  const { shelf, messages } = await feedbackShelf()
  await Promise.all([shelf.select({ id: 2 }), shelf.select({ id: 2 })])
  assert.deepEqual(messages, ['换书成功'])
})

test('a duplicate navigation result stays silent even if currentBookId is stale', async () => {
  const { shelf, router, messages } = await feedbackShelf()
  await router.push({ name: 'reader', params: { id: 2 }, query: { resume: '1' } })
  // Simulate a reader identity snapshot that has not caught up with the route yet.
  const duplicate = useReaderShelf({
    bookshelf: { books: [] }, reader: { progressByBook: {} }, currentBookId: ref(1), router,
    onSuccess: message => messages.push(message),
  })
  await duplicate.select({ id: 2 })
  assert.deepEqual(messages, [])
})

test('a cancelled navigation cannot report success after a newer switch', async () => {
  let release
  const gate = new Promise(resolve => { release = resolve })
  const { shelf, router, messages } = await feedbackShelf({ guard: to => to.params.id === '2' ? gate : true })
  const first = shelf.select({ id: 2 })
  await new Promise(resolve => setImmediate(resolve))
  await shelf.select({ id: 3 })
  release(true)
  await first
  assert.equal(router.currentRoute.value.params.id, '3')
  assert.deepEqual(messages, ['换书成功'])
})
