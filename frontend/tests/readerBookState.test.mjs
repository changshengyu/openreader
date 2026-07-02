import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderBookState } from '../src/composables/useReaderBookState.js'

function createController(overrides = {}) {
  const calls = []
  const book = ref({ id: 7, title: '当前阅读', progress: { chapterIndex: 3 } })
  const bookshelf = {
    books: [
      { id: 8, title: '书架版本', progress: { chapterIndex: 5 } },
    ],
  }
  const controller = useReaderBookState({
    book,
    bookId: ref(7),
    bookshelf,
    mergeBook: (current, incoming) => ({ ...current, ...incoming }),
    makeCacheKey: (...args) => {
      calls.push(['key', ...args])
      return args.join('@')
    },
    invalidateCache: async (...args) => calls.push(['invalidate', ...args]),
    writeCache: async (...args) => calls.push(['write', ...args]),
    ...overrides,
  })
  return { book, bookshelf, calls, controller }
}

test('merges incoming data with the matching shelf book', () => {
  const fixture = createController()
  assert.deepEqual(fixture.controller.mergeLoadedBook({
    id: 8,
    title: '接口版本',
  }), {
    id: 8,
    title: '接口版本',
    progress: { chapterIndex: 5 },
  })
})

test('falls back to the current reader book and preserves invalid payloads', () => {
  const fixture = createController()
  assert.deepEqual(fixture.controller.mergeLoadedBook({
    id: 7,
    title: '刷新后',
  }), {
    id: 7,
    title: '刷新后',
    progress: { chapterIndex: 3 },
  })
  assert.equal(fixture.controller.mergeLoadedBook(null), null)
})

test('scopes cache keys to explicit or current book ids', () => {
  const fixture = createController()
  assert.equal(fixture.controller.cacheKey('chapters:9'), '9@chapters')
  assert.equal(fixture.controller.cacheKey('book'), '7@book')
  assert.deepEqual(fixture.calls, [
    ['key', '9', 'chapters'],
    ['key', 7, 'book'],
  ])
})

test('invalidates and writes cache using the same book-id fallback', async () => {
  const fixture = createController()
  await fixture.controller.invalidate({ chapters: true })
  await fixture.controller.write({ bookId: 9, chaptersData: [] })
  assert.deepEqual(fixture.calls, [
    ['invalidate', 7, { chapters: true }],
    ['write', 9, { bookId: 9, chaptersData: [] }],
  ])
})
