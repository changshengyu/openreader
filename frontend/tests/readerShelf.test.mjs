import assert from 'node:assert/strict'
import test from 'node:test'
import { sortByShelfOrder } from '../src/utils/bookOrder.js'
import { readerRouteQueryFromBook } from '../src/utils/readerRoute.js'

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
