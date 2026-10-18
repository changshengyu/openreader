import assert from 'node:assert/strict'
import test from 'node:test'
import { computed, effectScope, ref, watch } from 'vue'
import { createMemoryHistory, createRouter, routeLocationKey } from 'vue-router'
import { useReaderBookLoad } from '../src/composables/useReaderBookLoad.js'
import { useReaderBookState } from '../src/composables/useReaderBookState.js'
import { useReaderChapterCache } from '../src/composables/useReaderChapterCache.js'
import { useReaderChapterContent } from '../src/composables/useReaderChapterContent.js'
import { useReaderChapterLoader } from '../src/composables/useReaderChapterLoader.js'
import { useReaderLocalProgress } from '../src/composables/useReaderLocalProgress.js'
import { useReaderProgressControls } from '../src/composables/useReaderProgressControls.js'
import { useReaderProgressPersistence } from '../src/composables/useReaderProgressPersistence.js'
import { useReaderRouteSync } from '../src/composables/useReaderRouteSync.js'
import { chapterCacheKeyPrefix, loadBrowserChapterContent } from '../src/utils/bookChapterCache.js'
import { newestBookProgress } from '../src/utils/bookOrder.js'
import { createMultiBookChapterMemoryCache } from '../src/utils/multiBookChapterMemoryCache.js'
import { readerRouteQueryFromBook } from '../src/utils/readerRoute.js'

// Contract: docs/compat/reader-shelf-switch-stale-catalog-fixed-baseline-second-audit-p1-contract.md R1-R5, 8.1(2).
// The real Reader composables are wired in Reader.vue setup order (Reader.vue keys the Reader only by session, so a
// shelf switch reuses the mounted instance and both route watchers run). useReaderShelf.select is mirrored here: its
// extensionless imports do not resolve under node --test. Layout and scroll restore are recorders, IndexedDB is a
// Map behind loadBrowserChapterContent, and the fake network holds the target book's catalog until the test releases it.

const SCOPE = 'user:1'
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))
// Long enough for a stale chapter load to fetch, commit, restore, preload and save with the fake 1-2ms timings.
const SETTLE_MS = 60

function catalogRows(bookId, titles) {
  // Real /books/:id/chapters rows and content chapters carry bookId (models.Chapter.BookID).
  return titles.map((title, index) => ({ id: bookId * 100 + index, bookId, index, title }))
}

function httpError(status, message) {
  return Object.assign(new Error(`Request failed with status code ${status}`), {
    response: { status, data: { error: message } },
  })
}

function abortError() {
  return Object.assign(new Error('canceled'), { name: 'CanceledError' })
}

function delay(ms, signal) {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(abortError())
      return
    }
    const timer = setTimeout(resolve, ms)
    signal?.addEventListener('abort', () => {
      clearTimeout(timer)
      reject(abortError())
    }, { once: true })
  })
}

function createGate() {
  let release
  const opened = new Promise(resolve => {
    release = resolve
  })
  return { opened, release }
}

function progressRecord(bookId, chapterIndex, totalChapters, extra = {}) {
  return {
    bookId,
    chapterId: bookId * 100 + chapterIndex,
    chapterIndex,
    offset: 0,
    chapterPercent: 0,
    percent: chapterIndex / totalChapters,
    updatedAt: '2026-10-01T00:00:00.000Z',
    ...extra,
  }
}

async function mountReader(definition) {
  const log = {
    phase: 'initial',
    applied: [],
    cacheWrites: [],
    catalogs: [],
    commits: [],
    content: [],
    errors: [],
    labels: [],
    marks: [],
    puts: [],
  }
  const catalogs = Object.fromEntries(
    Object.entries(definition.catalogs).map(([id, titles]) => [id, catalogRows(Number(id), titles)]),
  )
  const serverProgress = structuredClone(definition.serverProgress || {})
  const idb = new Map()
  const attempts = new Map()
  const catalogGates = new Map()

  const network = {
    async catalog(bookId) {
      const gate = catalogGates.get(Number(bookId))
      if (gate) await gate.opened
      else await sleep(1)
      log.catalogs.push({ bookId: Number(bookId), phase: log.phase })
      return { data: catalogs[bookId].map(row => ({ ...row })), fromCache: false }
    },
    async book(bookId) {
      await sleep(1)
      return { data: { ...definition.books[bookId] }, fromCache: false }
    },
    async chapterContent(bookId, index, options = {}) {
      const key = `${bookId}:${index}`
      const attempt = (attempts.get(key) || 0) + 1
      attempts.set(key, attempt)
      const request = { bookId: Number(bookId), index: Number(index), attempt, phase: log.phase, status: 0 }
      log.content.push(request)
      await delay(definition.contentDelay?.(Number(bookId), Number(index), attempt) ?? 2, options.signal)
      const row = catalogs[bookId]?.[index]
      if (!row) {
        request.status = 404
        throw httpError(404, 'chapter not found')
      }
      const failure = definition.contentFailure?.(Number(bookId), Number(index), attempt)
      if (failure) {
        request.status = failure
        throw httpError(failure, 'failed to load chapter content')
      }
      request.status = 200
      return {
        data: {
          chapter: { ...row },
          content: `${definition.books[bookId].title} ${row.title} 正文`,
          format: 'text',
        },
      }
    },
    async putProgress(payload) {
      log.puts.push({ ...payload, phase: log.phase })
      await sleep(1)
      // backend/services/readingprogress/service.go: the chapterIndex row must exist with the same chapterId.
      const row = catalogs[payload.bookId]?.[payload.chapterIndex]
      if (!row || row.id !== payload.chapterId) throw httpError(400, 'chapter does not match progress')
      const saved = { ...payload, updatedAt: new Date(Date.now() + 1).toISOString() }
      serverProgress[payload.bookId] = saved
      return saved
    },
  }

  const reader = {
    mode: 'page',
    progressByBook: structuredClone(definition.localProgress || {}),
    cachedProgress(bookId) {
      return this.progressByBook[bookId] || null
    },
    async loadProgress(bookId) {
      await sleep(1)
      return serverProgress[bookId] ? { ...serverProgress[bookId] } : null
    },
    applyServerProgress() {},
    applyProgress(progress) {
      log.applied.push({ ...progress, phase: log.phase })
      this.progressByBook[progress.bookId] = progress
    },
    saveProgress: payload => network.putProgress(payload),
    ensureClientId: () => 'shelf-switch-test',
  }
  const bookshelf = {
    books: Object.values(definition.books).map(book => ({ ...book })),
    applyBookProgress() {},
    upsertBook(book) {
      const index = this.books.findIndex(item => Number(item.id) === Number(book.id))
      if (index >= 0) this.books[index] = book
    },
  }
  const mergeShelfBook = (current, incoming) => ({ ...(current || {}), ...(incoming || {}) })

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/books/:id/read', name: 'reader', component: {} },
      { path: '/reader/remote/:sessionId', name: 'remote-reader', component: {} },
    ],
  })
  // useRoute() injects the same reactive route object that router.install provides.
  const provided = new Map()
  router.install({
    component() {},
    config: { globalProperties: {} },
    provide: (key, value) => provided.set(key, value),
    unmount() {},
  })
  const route = provided.get(routeLocationKey)
  await router.push(definition.start)

  const scope = effectScope()
  const reader$ = {}
  scope.run(() => {
    const remoteSessionId = computed(() => (
      route.name === 'remote-reader' ? String(route.params.sessionId || '') : ''
    ))
    const isTemporaryRemoteReader = computed(() => Boolean(remoteSessionId.value))
    const bookId = computed(() => (
      isTemporaryRemoteReader.value ? `remote:${remoteSessionId.value}` : Number(route.params.id)
    ))
    const book = ref(null)
    const readerBookDeleted = ref(false)
    const chapters = ref([])
    const chapter = ref(null)
    const currentIndex = ref(Number(route.query.chapter || 0))
    const { mergeLoadedBook } = useReaderBookState({ book, bookId, bookshelf, mergeBook: mergeShelfBook })
    const content = ref('')
    const chapterBlocks = ref([])
    const chapterLoading = ref(true)
    const chapterLoadError = ref('')
    const chapterLoaded = ref(false)
    const progressVersion = ref(0)
    const view = { offset: 0, percent: 0 }
    const scopeKey = () => [
      bookId.value,
      remoteSessionId.value,
      book.value?.url || book.value?.bookUrl || book.value?.libraryPath || '',
      book.value?.sourceId || '',
    ].join('|')

    let saveCurrentProgress = () => Promise.resolve()
    const chapterContentCache = createMultiBookChapterMemoryCache(3)
    const cacheMap = useReaderChapterCache({
      book,
      bookId,
      chapters,
      currentIndex,
      isTemporaryReader: isTemporaryRemoteReader,
      getContextKey: scopeKey,
      getCacheScope: () => SCOPE,
      listCachedChapters: async () => ({}),
    })
    const chapterContent = useReaderChapterContent({
      book,
      bookId,
      chapters,
      memoryCache: chapterContentCache,
      markCached: index => {
        log.marks.push({ bookId: bookId.value, index, phase: log.phase })
        cacheMap.markCached(index)
      },
      preloadRadius: 2,
      shouldCache: () => !isTemporaryRemoteReader.value,
      loadBrowserContent: (targetBook, targetBookId, index, options) => loadBrowserChapterContent(
        targetBook,
        targetBookId,
        index,
        {
          ...options,
          scope: SCOPE,
          getCache: async key => idb.get(key) ?? null,
          setCache: async (key, value) => {
            log.cacheWrites.push({
              key,
              chapterBookId: value.chapter.bookId,
              chapterIndex: value.chapter.index,
              phase: log.phase,
            })
            idb.set(key, value)
          },
          getChapterContent: network.chapterContent,
        },
      ),
    })
    const localProgress = useReaderLocalProgress({
      reader,
      bookshelf,
      bookId,
      book,
      chapter,
      chapters,
      currentIndex,
      getVisibleSnapshot: () => null,
      getCurrentPayload: () => null,
      getCurrentOffset: () => view.offset,
      getCurrentPercent: () => view.percent,
      mergeBook: mergeShelfBook,
      isTemporaryReader: isTemporaryRemoteReader,
    })
    const progressControls = useReaderProgressControls({
      scrollAnimator: { cancel() {}, scrollTo() {} },
      contentEl: ref(null),
      contentBody: ref(null),
      chapters,
      currentIndex,
      page: ref(0),
      pageCount: ref(1),
      progressVersion,
      isContinuousScrollRead: ref(false),
      getMode: () => 'page',
      getAnimateDuration: () => 0,
      getCurrentChapterPercent: () => view.percent,
      navigate: async () => {},
      applyLocalProgress: () => localProgress.apply(),
      saveProgress: () => saveCurrentProgress(),
      scheduleProgressSave: () => {},
    })
    const persistence = useReaderProgressPersistence({
      minimumInterval: 1200,
      isBlocked: () => readerBookDeleted.value,
      getPayload: () => chapter.value ? localProgress.currentPayload() : null,
      getBaseUpdatedAt: localProgress.serverBaseUpdatedAt,
      applyLocal: localProgress.apply,
      saveRemote: payload => reader.saveProgress(payload),
      onSaved: progress => localProgress.upsert(progress, { replace: true }),
      getMode: () => 'page',
      getStoredProgress: targetBookId => reader.progressByBook[targetBookId],
      ensureClientId: () => reader.ensureClientId(),
    })
    saveCurrentProgress = persistence.save
    const loader = useReaderChapterLoader({
      chapters,
      currentIndex,
      mobileChromeVisible: ref(true),
      restoringPosition: ref(false),
      chapterLoaded,
      chapterLoadError,
      chapterLoading,
      chapter,
      content,
      cachedImages: ref({}),
      chapterFormat: ref('text'),
      epubResource: ref(null),
      audioResource: ref(null),
      page: ref(0),
      chapterBlocks,
      progressVersion,
      isContinuousScrollRead: false,
      getScopeKey: scopeKey,
      cancelProgressSave: persistence.cancelScheduled,
      getMemoryContent: index => chapterContent.get(index),
      loadContent: (index, options) => chapterContent.load(index, options),
      makeChapterBlock: (index, row, text) => ({ index, id: row?.id, title: row?.title, text }),
      updateLayout: () => {},
      restorePosition: async (offset, options = {}) => {
        view.offset = offset
        view.percent = options.restorePercent ?? 0
      },
      preloadNearby: index => chapterContent.preload(index),
      saveProgress: options => persistence.save(options),
      markProgressSaved: persistence.markSaved,
      getCurrentProgress: () => localProgress.currentPayload(),
      computeChapterWindow: async () => {},
      invalidateChapterWindow: () => {},
      captureScrollAnchor: () => null,
      formatError: error => error?.response?.data?.error || error?.message || '章节加载失败',
      nextFrame: () => sleep(0),
    })
    const bookLoad = useReaderBookLoad({
      reader,
      bookshelf,
      bookId,
      book,
      chapters,
      currentIndex,
      getRouteQuery: () => route.query,
      isTemporaryReader: isTemporaryRemoteReader,
      cancelProgressSave: persistence.cancelScheduled,
      getShelfBook: targetBookId => bookshelf.books.find(item => Number(item.id) === Number(targetBookId)),
      loadCachedBook: targetBookId => network.book(targetBookId),
      loadCachedChapters: targetBookId => network.catalog(targetBookId),
      refreshBook: targetBookId => network.book(targetBookId),
      refreshChapters: targetBookId => network.catalog(targetBookId),
      mergeLoadedBook,
      mergeBookProgress: (loadedBook, progress) => mergeShelfBook(loadedBook, { id: loadedBook.id, progress }),
      resetSourceCandidates: () => {},
      loadChapter: (index, offset, options) => loader.load(index, offset, options),
      progressKey: persistence.key,
      getCurrentProgress: () => localProgress.currentPayload(),
      navigate: async () => {},
      markProgressSaved: persistence.markSaved,
      jumpToRouteLine: async () => {},
    })
    useReaderRouteSync({
      bookId,
      currentIndex,
      positionQuery: () => [route.query.chapter, route.query.offset, route.query.percent],
      searchQuery: () => [route.query.line, route.query.match, route.query.q, route.query.bookmark],
      loadBook: () => bookLoad.load(),
      loadChapter: (index, offset, options) => loader.load(index, offset, options),
      jumpToRouteLine: async () => {},
      onBookLoadStart: () => {
        readerBookDeleted.value = false
        persistence.resume()
        chapterLoadError.value = ''
      },
      onBookLoadError: error => {
        chapterLoadError.value = error.message
        chapterLoading.value = false
      },
    })

    watch(chapter, value => log.commits.push({
      bookId: value?.bookId,
      index: value?.index,
      title: value?.title,
      phase: log.phase,
    }), { flush: 'sync' })
    watch(chapterLoadError, value => log.errors.push({ value, phase: log.phase }), { flush: 'sync' })
    watch(progressControls.bookProgressLabel, value => log.labels.push({ value, phase: log.phase }), { flush: 'sync' })

    Object.assign(reader$, {
      bookId,
      bookLoad,
      chapter,
      chapterBlocks,
      chapterLoadError,
      chapters,
      currentIndex,
      bookProgressLabel: progressControls.bookProgressLabel,
      saveProgress: persistence.save,
    })
  })

  // Reader mount: useReaderPageLifecycle calls loadBook() directly, without the route watchers.
  await reader$.bookLoad.load()
  await sleep(SETTLE_MS)

  async function switchFromShelf(targetId, { holdCatalog = true } = {}) {
    // Mirror of useReaderShelf.select: force-save the current book, then push the selected book's resume route.
    const item = bookshelf.books.find(row => Number(row.id) === Number(targetId))
    await reader$.saveProgress({ force: true })
    const gate = holdCatalog ? createGate() : null
    if (gate) catalogGates.set(Number(targetId), gate)
    log.phase = holdCatalog ? 'held' : 'switched'
    await router.push({
      name: 'reader',
      params: { id: item.id },
      query: readerRouteQueryFromBook(item, newestBookProgress(item, reader.progressByBook)),
    })
    return {
      item,
      async release() {
        log.phase = 'released'
        catalogGates.delete(Number(targetId))
        gate?.release()
        await sleep(SETTLE_MS)
      },
    }
  }

  return {
    idb,
    log,
    reader,
    reader$,
    route,
    serverProgress,
    stop: () => scope.stop(),
    switchFromShelf,
  }
}

function targetContent(log, bookId, phase) {
  return log.content.filter(request => request.bookId === bookId && (!phase || request.phase === phase))
}

function describeRequests(requests) {
  return JSON.stringify(requests.map(({ bookId, index, attempt, status, phase }) => ({ bookId, index, attempt, status, phase })))
}

function assertHeldCatalogState(reader, target, label) {
  const { log, reader$ } = reader
  const previousRows = reader$.chapters.value.filter(row => Number(row.bookId) !== Number(target.id))
  assert.equal(
    previousRows.length,
    0,
    `${label}: while book ${target.id}'s catalog is held the Reader catalog (TOC, preload and progress source) still lists another book's rows ${JSON.stringify(previousRows.map(row => [row.bookId, row.index]))}`,
  )
  assert.equal(
    reader$.bookProgressLabel.value,
    '',
    `${label}: N% must be empty until book ${target.id}'s own catalog is known, got ${JSON.stringify(reader$.bookProgressLabel.value)}`,
  )
  assert.deepEqual(
    log.labels.filter(entry => entry.phase === 'held' && entry.value !== ''),
    [],
    `${label}: N% must not be derived from the previous catalog during the switch`,
  )
}

function assertTargetCacheOwnership(reader, target, label) {
  const { log } = reader
  const prefix = `${chapterCacheKeyPrefix(target, target.id, SCOPE)}@chapterContent-`
  const writes = log.cacheWrites.filter(write => write.phase !== 'initial')
  for (const write of writes) {
    assert.ok(write.key.startsWith(prefix), `${label}: browser cache key ${write.key} does not belong to book ${target.id}`)
    assert.equal(write.chapterBookId, target.id, `${label}: browser cache value for ${write.key} is not book ${target.id}'s chapter`)
    assert.equal(write.key, `${prefix}${write.chapterIndex}`, `${label}: cache key and cached chapter index disagree`)
  }
}

test('A: a shorter book loads only after its catalog and never requests chapters beyond it', async () => {
  const reader = await mountReader({
    start: '/books/2/read?chapter=1',
    books: {
      2: { id: 2, title: '书二', author: '甲', libraryPath: 'library/2.txt', chapterCount: 3, progress: null },
      3: { id: 3, title: '书三', author: '甲', libraryPath: 'library/3.txt', chapterCount: 2, progress: null },
    },
    catalogs: { 2: ['序章', '第一章', '第二章'], 3: ['第一章', '第二章'] },
  })
  try {
    const state = reader.reader$
    assert.equal(state.chapter.value?.bookId, 2, 'fixture: book 2 is open before the switch')
    const { item, release } = await reader.switchFromShelf(3)
    assert.deepEqual(reader.route.query, { resume: '1' }, 'fixture: book 3 has no saved progress')
    await sleep(SETTLE_MS)

    const held = targetContent(reader.log, 3, 'held')
    assert.deepEqual(held, [], `A: no chapter content may be requested before book 3's catalog arrives, got ${describeRequests(held)}`)
    assertHeldCatalogState(reader, item, 'A')

    await release()
    const all = targetContent(reader.log, 3)
    assert.ok(all.length > 0, 'A: book 3 must load after its catalog arrives')
    assert.deepEqual(
      all.filter(request => request.index >= 2),
      [],
      `A: requests must stay inside book 3's 2-chapter catalog, got ${describeRequests(all)}`,
    )
    assert.deepEqual(all.filter(request => request.status === 404), [], `A: no 404 chapter request, got ${describeRequests(all)}`)
    assert.deepEqual(
      [state.chapter.value?.bookId, state.chapter.value?.index],
      [3, 0],
      'A: book 3 opens at its first chapter',
    )
    assert.deepEqual(
      reader.log.marks.filter(mark => mark.phase !== 'initial' && mark.index >= 2),
      [],
      'A: cached markers only come from requests inside book 3\'s catalog',
    )
    assertTargetCacheOwnership(reader, item, 'A')
  } finally {
    reader.stop()
  }
})

test('B: a saved chapter beyond the previous catalog is neither clamped, shown nor saved', async () => {
  const saved = progressRecord(3, 9, 12, { offset: 300, chapterPercent: 0.5, percent: 9.5 / 12 })
  const reader = await mountReader({
    start: '/books/1/read?chapter=2',
    books: {
      1: { id: 1, title: '书一', author: '甲', libraryPath: 'library/1.txt', chapterCount: 3, progress: null },
      3: { id: 3, title: '书三', author: '甲', libraryPath: 'library/3.txt', chapterCount: 12, progress: { ...saved } },
    },
    localProgress: { 3: { ...saved } },
    serverProgress: { 3: { ...saved } },
    catalogs: {
      1: ['甲一', '甲二', '甲三'],
      3: Array.from({ length: 12 }, (_, index) => `乙${index + 1}`),
    },
  })
  try {
    const state = reader.reader$
    const { item, release } = await reader.switchFromShelf(3)
    assert.deepEqual(
      reader.route.query,
      { resume: '1', chapter: '9', offset: '300', percent: '0.5' },
      'fixture: the shelf route carries book 3\'s saved position',
    )
    await sleep(SETTLE_MS)

    const held = targetContent(reader.log, 3, 'held')
    assert.deepEqual(held, [], `B: no chapter content may be requested before book 3's catalog arrives, got ${describeRequests(held)}`)
    const heldPuts = reader.log.puts.filter(put => put.bookId === 3 && put.phase === 'held')
    assert.deepEqual(heldPuts, [], `B: no progress PUT for book 3 before its catalog, got ${JSON.stringify(heldPuts)}`)
    const heldApplied = reader.log.applied.filter(progress => progress.bookId === 3 && progress.phase === 'held')
    assert.deepEqual(heldApplied, [], `B: no local progress for book 3 before its catalog, got ${JSON.stringify(heldApplied)}`)
    assertHeldCatalogState(reader, item, 'B')

    await release()
    const commits = reader.log.commits.filter(commit => commit.bookId === 3)
    assert.deepEqual(
      commits.filter(commit => commit.index !== 9),
      [],
      `B: only book 3's saved chapter 9 may be committed, got ${JSON.stringify(commits)}`,
    )
    assert.deepEqual([state.chapter.value?.bookId, state.chapter.value?.index], [3, 9], 'B: book 3 opens at chapter 9')
    const all = targetContent(reader.log, 3)
    assert.equal(all.filter(request => request.index === 9).length, 1, `B: chapter 9 is requested once, got ${describeRequests(all)}`)
    assert.deepEqual(
      all.filter(request => request.index < 7 || request.index > 11),
      [],
      `B: preloads stay around chapter 9 inside book 3's catalog, got ${describeRequests(all)}`,
    )
    assert.deepEqual(
      reader.log.puts.filter(put => put.bookId === 3),
      [],
      'B: the book load restores chapter 9 without rewriting progress',
    )
    assert.equal(reader.serverProgress[3].chapterIndex, 9, 'B: server progress for book 3 stays on chapter 9')
    assert.equal(reader.reader.progressByBook[3].chapterIndex, 9, 'B: local progress for book 3 stays on chapter 9')
    assert.deepEqual(
      reader.log.marks.filter(mark => mark.phase !== 'initial' && (mark.index < 7 || mark.index > 11)),
      [],
      'B: cached markers only come from chapter 9 and its preloads',
    )
    assertTargetCacheOwnership(reader, item, 'B')
  } finally {
    reader.stop()
  }
})

test('C: no stale request error appears while the catalog is held; the target is requested once', async () => {
  const saved = progressRecord(5, 1, 6)
  const reader = await mountReader({
    start: '/books/1/read?chapter=2',
    books: {
      1: { id: 1, title: '书一', author: '甲', libraryPath: 'library/1.txt', chapterCount: 3, progress: null },
      5: { id: 5, title: '书五', author: '甲', libraryPath: 'library/5.txt', chapterCount: 6, progress: { ...saved } },
    },
    localProgress: { 5: { ...saved } },
    catalogs: { 1: ['甲一', '甲二', '甲三'], 5: ['戊一', '戊二', '戊三', '戊四', '戊五', '戊六'] },
    contentFailure: (bookId, index, attempt) => (bookId === 5 && index === 1 && attempt === 1 ? 502 : 0),
  })
  try {
    const state = reader.reader$
    const { item, release } = await reader.switchFromShelf(5)
    await sleep(SETTLE_MS)

    assert.deepEqual(
      reader.log.errors.filter(entry => entry.phase === 'held' && entry.value),
      [],
      'C: no chapter error may show while book 5\'s catalog is held',
    )
    assert.equal(state.chapterLoadError.value, '', 'C: the content error surface is empty during the switch')
    const held = targetContent(reader.log, 5, 'held')
    assert.deepEqual(held, [], `C: no chapter content may be requested before book 5's catalog arrives, got ${describeRequests(held)}`)
    assertHeldCatalogState(reader, item, 'C')

    await release()
    const chapterOne = targetContent(reader.log, 5).filter(request => request.index === 1)
    assert.equal(chapterOne.length, 1, `C: book 5 chapter 1 is requested exactly once, got ${describeRequests(chapterOne)}`)
    // R4: the target's own failure after its catalog arrived uses the existing error surface.
    assert.notEqual(state.chapterLoadError.value, '', 'C: book 5\'s own chapter failure shows the existing error surface')
    assert.deepEqual(reader.log.puts.filter(put => put.bookId === 5), [], 'C: a failed target chapter writes no progress')
    assertTargetCacheOwnership(reader, item, 'C')
  } finally {
    reader.stop()
  }
})

test('D control: when the catalog wins the race the target chapter is requested once and nothing is saved', async () => {
  const saved = progressRecord(3, 1, 2)
  const reader = await mountReader({
    start: '/books/1/read?chapter=2',
    books: {
      1: { id: 1, title: '书一', author: '甲', libraryPath: 'library/1.txt', chapterCount: 3, progress: null },
      3: { id: 3, title: '书三', author: '甲', libraryPath: 'library/3.txt', chapterCount: 2, progress: { ...saved } },
    },
    localProgress: { 3: { ...saved } },
    catalogs: { 1: ['甲一', '甲二', '甲三'], 3: ['乙一', '乙二'] },
    contentDelay: (bookId, index, attempt) => (bookId === 3 && attempt === 1 ? 40 : 2),
  })
  try {
    const state = reader.reader$
    const { item } = await reader.switchFromShelf(3, { holdCatalog: false })
    await sleep(SETTLE_MS * 2)

    const chapterOne = targetContent(reader.log, 3).filter(request => request.index === 1)
    assert.equal(chapterOne.length, 1, `D: book 3 chapter 1 is requested once, got ${describeRequests(chapterOne)}`)
    assert.deepEqual([state.chapter.value?.bookId, state.chapter.value?.index], [3, 1], 'D: book 3 opens at chapter 1')
    assert.deepEqual(reader.log.puts.filter(put => put.bookId === 3), [], 'D: the book load writes no progress')
    assert.deepEqual(
      reader.log.errors.filter(entry => entry.phase !== 'initial' && entry.value),
      [],
      'D: no chapter error during the switch',
    )
    assertTargetCacheOwnership(reader, item, 'D')
  } finally {
    reader.stop()
  }
})

test('E: without a stale load the held switch exposes no previous catalog and saves nothing for the new book', async () => {
  const reader = await mountReader({
    start: '/books/1/read?chapter=0',
    books: {
      1: { id: 1, title: '书一', author: '甲', libraryPath: 'library/1.txt', chapterCount: 3, progress: null },
      3: { id: 3, title: '书三', author: '甲', libraryPath: 'library/3.txt', chapterCount: 2, progress: null },
    },
    catalogs: { 1: ['甲一', '甲二', '甲三'], 3: ['乙一', '乙二'] },
  })
  try {
    const state = reader.reader$
    const { item, release } = await reader.switchFromShelf(3)
    await sleep(SETTLE_MS)

    const held = targetContent(reader.log, 3, 'held')
    assert.deepEqual(held, [], `E: no chapter content may be requested before book 3's catalog arrives, got ${describeRequests(held)}`)
    assertHeldCatalogState(reader, item, 'E')

    // R3: a forced save in the window (page hide, visibility change, leaving the Reader) must not write book 3's
    // progress from book 1's committed chapter, index or catalog.
    await state.saveProgress({ force: true }).catch(() => {})
    const heldApplied = reader.log.applied.filter(progress => progress.bookId === 3 && progress.phase === 'held')
    assert.deepEqual(heldApplied, [], `E: a forced save before book 3's chapter commits writes no local progress, got ${JSON.stringify(heldApplied)}`)
    const heldPuts = reader.log.puts.filter(put => put.bookId === 3 && put.phase === 'held')
    assert.deepEqual(heldPuts, [], `E: a forced save before book 3's chapter commits sends no progress PUT, got ${JSON.stringify(heldPuts)}`)

    await release()
    assert.deepEqual([state.chapter.value?.bookId, state.chapter.value?.index], [3, 0], 'E: book 3 opens at its first chapter')
    assert.equal(state.bookProgressLabel.value, '0%', 'E: N% returns once book 3\'s catalog is applied')
    assertTargetCacheOwnership(reader, item, 'E')
  } finally {
    reader.stop()
  }
})
