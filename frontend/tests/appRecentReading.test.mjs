import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive } from 'vue'
import {
  hasReadingProgress,
  useAppRecentReading,
} from '../src/composables/useAppRecentReading.js'

function createController(overrides = {}) {
  const state = reactive({
    books: [
      {
        id: 1,
        title: '较早',
        chapterCount: 10,
        progress: {
          bookId: 1,
          chapterIndex: 1,
          updatedAt: '2026-07-01T00:00:00Z',
        },
      },
      {
        id: 2,
        title: '较新',
        author: '作者',
        chapterCount: 20,
        progress: {
          bookId: 2,
          chapterIndex: 2,
          updatedAt: '2026-07-02T00:00:00Z',
        },
      },
    ],
    progressByBook: {
      1: {
        bookId: 1,
        chapterIndex: 4,
        chapterTitle: '第五章',
        offset: 18,
        chapterPercent: 0.25,
        updatedAt: '2026-07-03T00:00:00Z',
      },
    },
    scope: 'user:7',
  })
  const stored = new Map()
  const routes = []
  const storage = {
    getItem: key => stored.get(key) || null,
    setItem: (key, value) => stored.set(key, value),
  }
  const controller = useAppRecentReading({
    getBooks: () => state.books,
    getProgressByBook: () => state.progressByBook,
    getUserScope: () => state.scope,
    getStorage: () => storage,
    now: () => Date.parse('2026-07-04T00:00:00Z'),
    navigate: route => routes.push(route),
    ...overrides,
  })
  return { controller, routes, state, stored }
}

test('selects the newest merged reading progress', () => {
  const fixture = createController()

  assert.equal(fixture.controller.recentBook.value.id, 1)
  assert.equal(fixture.controller.subtitle(fixture.state.books[0]), '第五章')
})

test('opens the reader with the existing resume route contract', () => {
  const fixture = createController()
  fixture.controller.open()

  assert.deepEqual(fixture.routes, [{
    name: 'reader',
    params: { id: 1 },
    query: {
      resume: '1',
      chapter: 4,
      offset: 18,
      percent: 0.25,
    },
  }])
})

test('clears recent reading for the current user scope', () => {
  const fixture = createController()
  fixture.controller.clear()

  assert.equal(fixture.controller.recentBook.value, null)
  assert.equal(
    fixture.stored.get('openreader:readingRecentClearedAt:user:7'),
    String(Date.parse('2026-07-04T00:00:00Z')),
  )
})

test('reloads scoped suppression and recognizes legacy progress shapes', () => {
  const fixture = createController()
  fixture.stored.set(
    'openreader:readingRecentClearedAt:user:9',
    String(Date.parse('2026-07-05T00:00:00Z')),
  )
  fixture.state.scope = 'user:9'
  fixture.controller.refreshScope()

  assert.equal(fixture.controller.recentBook.value, null)
  assert.equal(hasReadingProgress({ bookId: 3, chapterIndex: 0 }), true)
  assert.equal(hasReadingProgress({ bookId: 3, percent: 0.1 }), true)
  assert.equal(hasReadingProgress({ bookId: 3 }), false)
})
