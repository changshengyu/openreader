import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderBookLoad } from '../src/composables/useReaderBookLoad.js'

function createController(overrides = {}) {
  const calls = []
  const bookId = ref(7)
  const book = ref(null)
  const chapters = ref([])
  const currentIndex = ref(0)
  const bookmarks = ref([])
  const query = {}
  const saved = {
    bookId: 7,
    chapterIndex: 2,
    offset: 90,
    chapterPercent: 0.3,
    updatedAt: '2026-07-01T00:00:00Z',
  }
  const currentProgress = ref({ ...saved })
  const reader = {
    cachedProgress: () => saved,
    loadProgress: async () => ({ ...saved }),
    applyServerProgress: progress => calls.push(['server-progress', progress]),
  }
  const bookshelf = {
    applyBookProgress: progress => calls.push(['shelf-progress', progress]),
  }
  const controller = useReaderBookLoad({
    reader,
    bookshelf,
    bookId,
    book,
    chapters,
    currentIndex,
    bookmarks,
    getRouteQuery: () => query,
    cancelProgressSave: () => calls.push(['cancel']),
    loadBookmarks: async () => [{ id: 1 }],
    loadCachedBook: async () => ({
      data: { id: 7, title: '测试书' },
      fromCache: false,
    }),
    loadCachedChapters: async () => ({
      data: [{ id: 1 }, { id: 2 }, { id: 3 }, { id: 4 }],
      fromCache: false,
    }),
    refreshBook: async () => ({ data: { id: 7, title: '新书名' } }),
    refreshChapters: async () => ({ data: [{ id: 1 }] }),
    mergeLoadedBook: incoming => incoming,
    mergeBookProgress: (loadedBook, progress) => ({ ...loadedBook, progress }),
    resetSourceCandidates: () => calls.push(['reset-sources']),
    loadChapter: async (index, offset, options) => {
      calls.push(['load-chapter', index, offset, options])
      currentIndex.value = index
      currentProgress.value = { ...currentProgress.value, chapterIndex: index, offset }
    },
    progressKey: progress => JSON.stringify(progress),
    getCurrentProgress: () => currentProgress.value,
    navigate: async routeQuery => calls.push(['navigate', routeQuery]),
    markProgressSaved: progress => calls.push(['mark', progress]),
    jumpToRouteLine: async () => calls.push(['jump']),
    ...overrides,
  })
  return {
    book,
    bookId,
    bookmarks,
    calls,
    chapters,
    controller,
    currentIndex,
    currentProgress,
    query,
    saved,
  }
}

async function flushBackgroundTasks() {
  await Promise.resolve()
  await Promise.resolve()
}

test('opens a book from saved progress and loads bookmarks independently', async () => {
  const fixture = createController()
  await fixture.controller.load()
  await flushBackgroundTasks()
  assert.equal(fixture.book.value.title, '测试书')
  assert.equal(fixture.currentIndex.value, 2)
  assert.deepEqual(fixture.bookmarks.value, [{ id: 1 }])
  assert.deepEqual(fixture.calls, [
    ['cancel'],
    ['reset-sources'],
    ['shelf-progress', fixture.saved],
    ['load-chapter', 2, 90, { restorePercent: 0.3, saveAfterLoad: false }],
    ['jump'],
  ])
})

test('keeps explicit route chapter, offset, and percent ahead of saved progress', async () => {
  const fixture = createController()
  Object.assign(fixture.query, {
    chapter: '1',
    offset: '25',
    percent: '0.6',
  })
  await fixture.controller.load()
  assert.deepEqual(
    fixture.calls.find(call => call[0] === 'load-chapter'),
    ['load-chapter', 1, 25, { restorePercent: 0.6, saveAfterLoad: false }],
  )
})

test('drops cached book responses after the route switches to another book', async () => {
  let resolveBook
  let resolveChapters
  const fixture = createController({
    loadCachedBook: () => new Promise(resolve => {
      resolveBook = resolve
    }),
    loadCachedChapters: () => new Promise(resolve => {
      resolveChapters = resolve
    }),
  })
  const pending = fixture.controller.load()
  fixture.bookId.value = 8
  resolveBook({ data: { id: 7 }, fromCache: false })
  resolveChapters({ data: [{ id: 1 }], fromCache: false })
  await pending
  assert.equal(fixture.book.value, null)
  assert.deepEqual(fixture.calls, [['cancel']])
})

test('does not let a delayed book-detail refresh block the initial chapter', async () => {
  let resolveBook
  const fixture = createController({
    getShelfBook: () => ({ id: 7, title: '书架已有书名' }),
    loadCachedBook: () => new Promise(resolve => {
      resolveBook = resolve
    }),
  })
  const pending = fixture.controller.load()
  await flushBackgroundTasks()
  try {
    assert.equal(fixture.book.value.title, '书架已有书名')
    assert.equal(
      fixture.calls.some(call => call[0] === 'load-chapter'),
      true,
      'chapter content should enter its critical path before book detail resolves',
    )
    assert.equal(
      fixture.calls.some(call => call[0] === 'jump'),
      true,
      'reader route positioning should be interactive before book detail resolves',
    )
  } finally {
    resolveBook({ data: { id: 7, title: '服务端新书名' }, fromCache: false })
    await pending
  }
  assert.equal(fixture.book.value.title, '服务端新书名')
})

test('reconciles only a newer server position while the baseline is unchanged', async () => {
  const fixture = createController()
  fixture.chapters.value = [{}, {}, {}, {}]
  fixture.currentProgress.value = { bookId: 7, chapterIndex: 2, offset: 90 }
  const baselineKey = JSON.stringify(fixture.currentProgress.value)
  await fixture.controller.reconcileServerProgress({
    bookId: 7,
    chapterIndex: 3,
    offset: 120,
    chapterPercent: 0.75,
    updatedAt: '2026-07-02T00:00:00Z',
  }, {
    baseline: fixture.saved,
    baselineKey,
    resumeFromProgress: true,
    hasRouteOffset: false,
    routePercent: null,
  })
  assert.deepEqual(fixture.calls, [
    ['navigate', { resume: '1', chapter: 3, offset: 120, percent: 0.75 }],
    ['load-chapter', 3, 120, { restorePercent: 0.75, saveAfterLoad: false }],
    ['mark', { bookId: 7, chapterIndex: 3, offset: 120 }],
  ])
})
