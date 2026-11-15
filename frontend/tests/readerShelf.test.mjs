import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderShelf } from '../src/composables/useReaderShelf.js'
import { sortByShelfOrder } from '../src/utils/bookOrder.js'
import { readerRouteQueryFromBook } from '../src/utils/readerRoute.js'

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
