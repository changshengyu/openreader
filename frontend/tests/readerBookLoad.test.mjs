import assert from 'node:assert/strict'
import test from 'node:test'
import { computed, effectScope, nextTick, ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { useReaderBookLoad } from '../src/composables/useReaderBookLoad.js'
import { useReaderRouteSync } from '../src/composables/useReaderRouteSync.js'

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

test('cold load restores an offset-only WebDAV position without percentage zero', async () => {
  for (const chapterPercent of [0, null, undefined, '']) {
    const fixture = createController()
    fixture.saved.chapterPercent = chapterPercent
    fixture.saved.percent = 0.5
    await fixture.controller.load()
    assert.deepEqual(fixture.calls.find(call => call[0] === 'load-chapter'),
      ['load-chapter', 2, 90, { restorePercent: null, saveAfterLoad: false }])
  }
})

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

test('restores a newer server position after the initial local chapter', async t => {
  const fixture = await createResumeFixture({ serverProgress: {
    chapterPercent: 0.75,
  } })
  t.after(() => fixture.scope.stop())
  await fixture.controller.load()
  fixture.publish()
  await finishBackgroundRefreshes()

  assert.deepEqual(fixture.router.currentRoute.value.query, { resume: '1', chapter: '3', offset: '120', percent: '0.75' })
  assert.deepEqual(fixture.loads, [[7, 0, 0], [7, 3, 120]])
  assert.deepEqual(fixture.routeLoads, [])
  assert.deepEqual(fixture.marks, [])
})

test('opening from a pending shelf entry preserves its unconfirmed progress', async () => {
  const fixture = createController({
    getShelfBook: () => ({ id: 7, progress: { ...fixture.saved, pendingSync: true } }),
  })
  await fixture.controller.load()
  assert.equal(fixture.calls.some(([event]) => event === 'server-progress'), false)
  assert.ok(fixture.calls.some(([event, progress]) => event === 'shelf-progress' && progress.pendingSync))
})

test('a late catalog error from an earlier visit cannot fail the reopened book', async () => {
  let rejectCatalog
  const fixture = createController({
    loadCachedChapters: () => rejectCatalog
      ? Promise.resolve({ data: [{ id: 1 }] })
      : new Promise((_, reject) => { rejectCatalog = reject }),
  })
  const initial = fixture.controller.load()
  fixture.bookId.value = 8
  await fixture.controller.load()
  fixture.bookId.value = 7
  await fixture.controller.load()
  rejectCatalog(new Error('first catalog failed'))
  await initial
})

function deferred() {
  let resolve
  const promise = new Promise(done => { resolve = done })
  return { promise, resolve }
}

function finishBackgroundRefreshes() {
  return new Promise(resolve => setImmediate(resolve))
}

async function createResumeFixture({ duplicate = false, loadChapter, serverProgress } = {}) {
  const scope = effectScope()
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/books/:id/read', name: 'reader', component: {} },
      { path: '/', name: 'home', component: {} },
    ],
  })
  const target = { name: 'reader', params: { id: 7 }, query: { resume: '1', chapter: 3, offset: 120 } }
  await router.push(duplicate ? target : '/books/7/read?resume=1')

  const bookId = computed(() => Number(router.currentRoute.value.params.id))
  const requested = deferred()
  const loads = []
  const routeLoads = []
  const marks = []
  let firstProgress = true
  let navigation
  const fixture = scope.run(() => {
    const fixture = createController({
      bookId,
      getRouteQuery: () => router.currentRoute.value.query,
      reader: {
        cachedProgress: id => ({ bookId: id, chapterIndex: 0, updatedAt: '2026-07-01T00:00:00Z' }),
        loadProgress: async () => {
          if (!firstProgress) return null
          firstProgress = false
          return requested.promise
        },
      },
      loadCachedBook: async id => ({ data: { id } }),
      loadChapter: async (index, offset, options) => {
        loads.push([bookId.value, index, offset])
        return loadChapter ? loadChapter(index, offset, options) : true
      },
      markProgressSaved: () => marks.push(bookId.value),
      navigate: query => {
        navigation = routeSync.replacePosition({ name: 'reader', params: { id: bookId.value }, query })
        return navigation
      },
    })
    const routeSync = useReaderRouteSync({
      router,
      bookId,
      currentIndex: fixture.currentIndex,
      positionQuery: () => {
        const { chapter, offset, percent } = router.currentRoute.value.query
        return [chapter, offset, percent]
      },
      searchQuery: () => [],
      loadBook: async () => {
        if (router.currentRoute.value.name === 'reader') await fixture.controller.load()
      },
      loadChapter: async (...args) => routeLoads.push(args),
      jumpToRouteLine: async () => {},
    })
    return fixture
  })
  const publish = () => requested.resolve({
    bookId: 7, chapterIndex: 3, offset: 120, updatedAt: '2026-07-02T00:00:00Z', ...serverProgress,
  })
  return { ...fixture, scope, router, target, marks, loads, routeLoads, publish, navigation: () => navigation }
}

for (const backToA of [false, true]) {
  test(`cancelled progress restoration cannot load a chapter after switching books (backToA=${backToA})`, async t => {
    const fixture = await createResumeFixture()
    t.after(() => fixture.scope.stop())
    const started = deferred()
    const release = deferred()
    fixture.router.beforeEach(async to => {
      if (to.params.id !== '7' || to.query.chapter !== '3') return
      started.resolve()
      await release.promise
    })

    await fixture.controller.load()
    fixture.publish()
    await started.promise
    await fixture.router.push('/books/8/read?resume=1')
    if (backToA) await fixture.router.push('/books/7/read?resume=1')
    await finishBackgroundRefreshes()
    const activeLoads = fixture.loads.slice()
    release.resolve()
    await fixture.navigation()
    await finishBackgroundRefreshes()

    assert.deepEqual(fixture.loads, activeLoads)
    assert.deepEqual(fixture.marks, [])
  })
}

test('a progress restoration navigation aborted by a guard leaves the next explicit navigation usable', async t => {
  const fixture = await createResumeFixture()
  t.after(() => fixture.scope.stop())
  const removeGuard = fixture.router.beforeEach(to => to.query.chapter === '3' ? false : undefined)

  await fixture.controller.load()
  fixture.publish()
  await finishBackgroundRefreshes()
  assert.deepEqual(fixture.loads, [[7, 0, 0]])
  assert.deepEqual(fixture.marks, [])
  removeGuard()

  await fixture.router.replace(fixture.target)
  await nextTick()
  assert.deepEqual(fixture.routeLoads, [[3, 120, { restorePercent: null, saveAfterLoad: true }]])
})

test('a progress restoration redirected to a different position leaves that position to route sync', async t => {
  const fixture = await createResumeFixture()
  t.after(() => fixture.scope.stop())
  fixture.router.beforeEach(to => to.query.chapter === '3' ? '/books/7/read?chapter=1' : undefined)

  await fixture.controller.load()
  fixture.publish()
  await finishBackgroundRefreshes()

  assert.equal(fixture.router.currentRoute.value.query.chapter, '1')
  assert.deepEqual(fixture.loads, [[7, 0, 0]])
  assert.deepEqual(fixture.routeLoads, [[1, 0, { restorePercent: null, saveAfterLoad: true }]])
  assert.deepEqual(fixture.marks, [])
})

test('restores content when the resume URL already points to the newer server position', async t => {
  const fixture = await createResumeFixture({ duplicate: true })
  t.after(() => fixture.scope.stop())
  await fixture.controller.load()
  fixture.publish()
  await finishBackgroundRefreshes()

  assert.deepEqual(fixture.loads, [[7, 0, 0], [7, 3, 120]])
  assert.deepEqual(fixture.routeLoads, [])
})

test('a chapter that fails to apply during progress restoration is not marked saved', async t => {
  const fixture = await createResumeFixture({ loadChapter: async index => index === 0 })
  t.after(() => fixture.scope.stop())
  await fixture.controller.load()
  fixture.publish()
  await finishBackgroundRefreshes()

  assert.deepEqual(fixture.loads, [[7, 0, 0], [7, 3, 120]])
  assert.deepEqual(fixture.marks, [])
})

test('an obsolete chapter restoration cannot mark the newly opened book saved', async t => {
  const started = deferred()
  const release = deferred()
  const fixture = await createResumeFixture({
    async loadChapter(index) {
      if (index === 3) {
        started.resolve()
        await release.promise
        return false
      }
      return true
    },
  })
  t.after(() => fixture.scope.stop())
  await fixture.controller.load()
  fixture.publish()
  await started.promise
  await fixture.router.push('/books/8/read?resume=1')
  release.resolve()
  await finishBackgroundRefreshes()

  assert.deepEqual(fixture.loads, [[7, 0, 0], [7, 3, 120], [8, 0, 0]])
  assert.deepEqual(fixture.marks, [])
})

test('a disposed reader cannot resume restoring its chapter after navigation settles', async t => {
  const fixture = await createResumeFixture()
  t.after(() => fixture.scope.stop())
  const started = deferred()
  const release = deferred()
  fixture.router.beforeEach(async to => {
    if (to.query.chapter !== '3') return
    started.resolve()
    await release.promise
  })

  await fixture.controller.load()
  fixture.publish()
  await started.promise
  fixture.scope.stop()
  release.resolve()
  await fixture.navigation()
  await finishBackgroundRefreshes()

  assert.deepEqual(fixture.loads, [[7, 0, 0]])
  assert.deepEqual(fixture.marks, [])
})

for (const resource of ['book', 'chapters']) {
  test(`an older ${resource} response cannot replace the result of reopening the same book`, async () => {
    const firstVisit = deferred()
    let visitsToA = 0
    const bookData = (id, title) => ({ id, title })
    const rows = (id, title) => [{ id: id * 10, title }]
    const payload = resource === 'book' ? bookData : rows
    async function refresh(id) {
      if (id === 7 && ++visitsToA === 1) return firstVisit.promise
      return { data: payload(id, 'latest') }
    }
    const fixture = createController({
      loadCachedBook: async id => ({ data: bookData(id, 'cached'), fromCache: resource === 'book' }),
      loadCachedChapters: async id => ({ data: rows(id, 'cached'), fromCache: resource === 'chapters' }),
      refreshBook: refresh,
      refreshChapters: refresh,
    })
    const currentTitle = () => resource === 'book' ? fixture.book.value.title : fixture.chapters.value[0].title

    await fixture.controller.load()
    fixture.bookId.value = 8
    await fixture.controller.load()
    fixture.bookId.value = 7
    await fixture.controller.load()
    await finishBackgroundRefreshes()
    assert.equal(currentTitle(), 'latest')

    firstVisit.resolve({ data: payload(7, 'obsolete') })
    await finishBackgroundRefreshes()
    assert.equal(currentTitle(), 'latest')
  })
}

test('an older chapter load cannot start a refresh after the reader switches books', async () => {
  const firstChapter = deferred()
  const chapterStarted = deferred()
  const refreshes = []
  let loads = 0
  const fixture = createController({
    loadCachedChapters: async id => ({ data: [{ id }], fromCache: id === 7 }),
    loadChapter: async () => {
      if (++loads !== 1) return
      chapterStarted.resolve()
      await firstChapter.promise
    },
    refreshChapters: async id => {
      refreshes.push(id)
      return { data: [{ id }] }
    },
  })
  const first = fixture.controller.load()
  await chapterStarted.promise
  fixture.bookId.value = 8
  await fixture.controller.load()
  firstChapter.resolve()
  await first
  assert.deepEqual(refreshes, [])
})

test('a refresh from a disposed reader cannot update its book or catalog', async () => {
  const scope = effectScope()
  const bookResponse = deferred()
  const catalogResponse = deferred()
  const fixture = scope.run(() => createController({
    loadCachedBook: async id => ({ data: { id, title: 'cached' }, fromCache: true }),
    loadCachedChapters: async () => ({ data: [{ id: 1 }], fromCache: true }),
    refreshBook: () => bookResponse.promise,
    refreshChapters: () => catalogResponse.promise,
  }))
  await fixture.controller.load()
  scope.stop()
  bookResponse.resolve({ data: { id: 7, title: 'obsolete' } })
  catalogResponse.resolve({ data: [{ id: 2 }] })
  await finishBackgroundRefreshes()
  assert.equal(fixture.book.value.title, 'cached')
  assert.deepEqual(fixture.chapters.value, [{ id: 1 }])
})
