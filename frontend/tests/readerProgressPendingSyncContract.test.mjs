import assert from 'node:assert/strict'
import test, { after } from 'node:test'
import { ref } from 'vue'
import { createServer } from 'vite'

// Contract: docs/compat/reader-progress-pending-sync-keepalive-fixed-baseline-second-audit-p0-contract.md
// R1-R7, 8.1(1)-(6). The Pinia reader and bookshelf stores, the Reader progress composables and the axios
// client are the real modules wired as in Reader.vue; only the transport (axios adapter, keepalive fetch)
// and the clock are scripted.

const storage = new Map()
globalThis.localStorage = {
  get length() { return storage.size },
  getItem: key => storage.get(String(key)) ?? null,
  key: index => [...storage.keys()][index] ?? null,
  removeItem: key => storage.delete(String(key)),
  setItem: (key, value) => storage.set(String(key), String(value)),
}
globalThis.window = {
  localStorage: globalThis.localStorage,
  sessionStorage: globalThis.localStorage,
  location: { protocol: 'http:', host: 'openreader.test' },
  addEventListener() {},
  removeEventListener() {},
  dispatchEvent() {},
}

const vite = await createServer({
  root: new URL('..', import.meta.url).pathname,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
})
after(() => vite.close())

const { createPinia, setActivePinia } = await import('pinia')
const { useReaderStore } = await vite.ssrLoadModule('/src/stores/reader.js')
const { mergeShelfBook, useBookshelfStore } = await vite.ssrLoadModule('/src/stores/bookshelf.js')
const { default: api } = await vite.ssrLoadModule('/src/api/client.js')
const { useReaderLocalProgress } = await vite.ssrLoadModule('/src/composables/useReaderLocalProgress.js')
const { useReaderProgressPersistence } = await vite.ssrLoadModule('/src/composables/useReaderProgressPersistence.js')
const { useReaderScrollSync } = await vite.ssrLoadModule('/src/composables/useReaderScrollSync.js')

const USER_ID = 31
// Unsigned test value: the stores only decode its payload to scope the progress storage key.
const TOKEN = ['test', Buffer.from(JSON.stringify({ userId: USER_ID })).toString('base64url'), 'unsigned'].join('.')
const BOOK_ID = 41
const BOOK_TITLE = '待同步契约'
const CHAPTERS = ['第一章 开端', '第二章 已存', '第三章 离线', '第四章 预载', '第五章 后续', '第六章 终点']
  .map((title, index) => ({ id: 4100 + index, bookId: BOOK_ID, index, title }))
// Server versions carry microseconds like the Go API; client snapshots carry milliseconds.
const SERVER_V1 = '2026-10-18T02:42:20.276123Z'
const CLIENT_START = Date.parse('2026-10-18T02:42:26.000Z')
const PROGRESS_KEY = `openreader_chapter_progress@user:${USER_ID}@${BOOK_ID}`

function seededRow() {
  return {
    id: 1,
    userId: USER_ID,
    bookId: BOOK_ID,
    chapterId: CHAPTERS[1].id,
    chapterIndex: 1,
    offset: 1594,
    percent: 0.2083,
    chapterPercent: 0.25,
    chapterTitle: CHAPTERS[1].title,
    mode: 'page',
    updatedAt: SERVER_V1,
  }
}

function axiosResponse(config, data, headers = {}) {
  return { data, status: 200, statusText: 'OK', headers, config, request: {} }
}

function networkError(config) {
  const error = new Error('Network Error')
  error.code = 'ERR_NETWORK'
  error.config = config
  return error
}

// The Go API stamps a commit slightly after the request leaves the client, with microseconds.
function serverVersion() {
  return new Date(Date.now() + 5).toISOString().replace('Z', '123Z')
}

function installServer(t) {
  const server = {
    row: seededRow(),
    versions: [SERVER_V1],
    puts: [],
    shelfReads: 0,
    putMode: 'accept',
    held: null,
    commit(body) {
      const updatedAt = serverVersion()
      server.row = {
        id: 1,
        userId: USER_ID,
        bookId: body.bookId,
        chapterId: body.chapterId,
        chapterIndex: body.chapterIndex,
        offset: body.offset,
        percent: body.percent,
        chapterPercent: body.chapterPercent,
        chapterTitle: body.chapterTitle,
        mode: body.mode,
        updatedAt,
      }
      server.versions.push(updatedAt)
      return { ...server.row }
    },
  }
  const original = api.defaults.adapter
  api.defaults.adapter = async (config) => {
    if (config.method === 'get' && config.url === '/books') {
      server.shelfReads += 1
      return axiosResponse(config, [{ id: BOOK_ID, title: BOOK_TITLE, chapterCount: CHAPTERS.length, progress: { ...server.row } }])
    }
    if (config.method === 'get' && config.url === `/progress/${BOOK_ID}`) {
      return axiosResponse(config, { ...server.row })
    }
    if (config.method === 'put' && config.url === '/progress') {
      const body = JSON.parse(config.data)
      server.puts.push(body)
      if (server.putMode === 'offline') throw networkError(config)
      if (server.putMode === 'hold') {
        await new Promise((resolve, reject) => {
          server.held = { resolve, reject: () => reject(networkError(config)) }
        })
      }
      // Single client, no concurrent writer: every write is accepted.
      return axiosResponse(config, server.commit(body))
    }
    throw new Error(`unexpected API ${config.method} ${config.url}`)
  }
  t.after(() => { api.defaults.adapter = original })
  return server
}

function installKeepalive(t, server) {
  const previous = globalThis.fetch
  const keepalive = { outcome: 'accept', calls: [] }
  globalThis.fetch = (url, options) => {
    const body = JSON.parse(options.body)
    keepalive.calls.push({ url, keepalive: options.keepalive, body })
    if (keepalive.outcome === 'throw') throw new TypeError('keepalive unavailable')
    if (keepalive.outcome === 'offline') return Promise.reject(new TypeError('Failed to fetch'))
    if (keepalive.outcome === '503') {
      return Promise.resolve({ ok: false, status: 503, headers: { get: () => null }, json: async () => ({ error: 'unavailable' }) })
    }
    if (keepalive.outcome === 'hold') return new Promise(() => {})
    const saved = server.commit(body)
    return Promise.resolve({ ok: true, status: 200, headers: { get: () => null }, json: async () => saved })
  }
  t.after(() => { globalThis.fetch = previous })
  return keepalive
}

// Opens a book the way Reader.vue does: the shelf is loaded, the progress is read with preferLocal, and
// the local progress, persistence and scroll sync composables are wired with the same callbacks.
async function openReader(t, { mode = 'page', elapseBeforeRequest = 2 } = {}) {
  storage.clear()
  storage.set('openreader_token', TOKEN)
  t.mock.timers.enable({ apis: ['Date'], now: CLIENT_START })
  setActivePinia(createPinia())
  const reader = useReaderStore()
  const bookshelf = useBookshelfStore()
  reader.mode = mode
  const server = installServer(t)
  const keepalive = installKeepalive(t, server)
  await bookshelf.loadBooks({ force: true, all: true })
  await reader.loadProgress(BOOK_ID, { preferLocal: true })

  const book = ref({ ...bookshelf.books.find(item => Number(item.id) === BOOK_ID) })
  const chapter = ref(CHAPTERS[1])
  const currentIndex = ref(1)
  const position = { offset: 1594, chapterPercent: 0.25, visible: null }
  const local = useReaderLocalProgress({
    reader,
    bookshelf,
    bookId: ref(BOOK_ID),
    book,
    chapter,
    chapters: ref(CHAPTERS),
    currentIndex,
    getVisibleSnapshot: () => position.visible,
    getCurrentOffset: () => position.offset,
    getCurrentPercent: () => position.chapterPercent,
    mergeBook: mergeShelfBook,
  })
  const persistence = useReaderProgressPersistence({
    minimumInterval: 1200,
    getPayload: () => (chapter.value ? local.currentPayload() : null),
    getBaseUpdatedAt: local.serverBaseUpdatedAt,
    applyLocal: local.apply,
    // Real time passes between the local write and the request it starts (2 ms in contract 4.3).
    saveRemote: (payload) => {
      if (elapseBeforeRequest) t.mock.timers.tick(elapseBeforeRequest)
      return reader.saveProgress(payload)
    },
    onSaved: progress => local.upsert(progress, { replace: true }),
    getMode: () => reader.mode,
    getStoredProgress: targetBookId => reader.progressByBook[targetBookId],
    ensureClientId: () => reader.ensureClientId(),
  })
  t.after(() => persistence.cancelScheduled())
  return {
    book,
    bookshelf,
    chapter,
    currentIndex,
    elapse: ms => t.mock.timers.tick(ms),
    keepalive,
    local,
    persistence,
    position,
    reader,
    server,
  }
}

async function settle() {
  for (let round = 0; round < 8; round += 1) {
    await new Promise(resolve => setImmediate(resolve))
  }
}

function storedProgress() {
  const raw = storage.get(PROGRESS_KEY)
  return raw ? JSON.parse(raw) : null
}

function progressView(progress) {
  if (!progress) return null
  return {
    chapterIndex: progress.chapterIndex,
    offset: progress.offset,
    pendingSync: Boolean(progress.pendingSync),
    baseUpdatedAt: progress.baseUpdatedAt || '',
  }
}

function localState(r) {
  return {
    memory: progressView(r.reader.progressByBook[BOOK_ID]),
    stored: progressView(storedProgress()),
  }
}

function pendingAt(chapterIndex, offset, baseUpdatedAt = SERVER_V1) {
  return { chapterIndex, offset, pendingSync: true, baseUpdatedAt }
}

function confirmedAt(chapterIndex, offset) {
  return { chapterIndex, offset, pendingSync: false, baseUpdatedAt: '' }
}

function shelfProgress(r) {
  return r.bookshelf.books.find(item => Number(item.id) === BOOK_ID)?.progress
}

// 8.1(1), R1
test('a Reader local snapshot stays pending on its server base after the real shelf upsert', async (t) => {
  const r = await openReader(t)
  r.position.offset = 3918
  r.position.chapterPercent = 0.6
  r.local.apply()

  // The shelf and the Reader's book show the local position at once (contract 7 rejects skipping the upsert).
  assert.equal(shelfProgress(r)?.offset, 3918)
  assert.equal(r.book.value.progress?.offset, 3918)
  assert.equal(r.bookshelf.books.length, 1)
  assert.deepEqual(localState(r), {
    memory: pendingAt(1, 3918),
    stored: pendingAt(1, 3918),
  })
})

// 8.1(2), R1/R7: real-store version of the queued-keepalive test in readerProgressPersistence.test.mjs.
test('a queued keepalive leaves the snapshot pending until its response arrives', async (t) => {
  const r = await openReader(t)
  r.keepalive.outcome = 'hold'
  r.position.offset = 3192
  await r.persistence.save({ force: true, background: true })
  await settle()

  assert.equal(r.keepalive.calls.length, 1)
  assert.equal(r.keepalive.calls[0].keepalive, true)
  assert.equal(r.keepalive.calls[0].body.baseUpdatedAt, SERVER_V1)
  assert.equal(r.server.puts.length, 0, 'a queued keepalive must not be followed by an ordinary request')
  assert.deepEqual(localState(r), {
    memory: pendingAt(1, 3192),
    stored: pendingAt(1, 3192),
  })
})

// 8.1(2), R2
for (const [outcome, label] of [
  ['offline', 'rejects'],
  ['503', 'answers 503'],
]) {
  test(`a background save whose keepalive ${label} keeps the snapshot pending`, async (t) => {
    const r = await openReader(t)
    r.keepalive.outcome = outcome
    r.position.offset = 3918
    await r.persistence.save({ force: true, background: true })
    await settle()

    assert.equal(r.keepalive.calls.length, 1)
    assert.equal(r.server.puts.length, 0, 'a failed keepalive must not be retried with an ordinary request')
    assert.deepEqual(localState(r), {
      memory: pendingAt(1, 3918),
      stored: pendingAt(1, 3918),
    })
  })
}

// 8.1(2), R2/R7: the synchronous failure falls back once to the ordinary save, which fails offline as well.
test('a background save whose keepalive throws synchronously falls back once and stays pending', async (t) => {
  const r = await openReader(t)
  r.keepalive.outcome = 'throw'
  r.server.putMode = 'offline'
  r.position.offset = 3918
  let fallbackError = null
  await r.persistence.save({ force: true, background: true }).catch((error) => { fallbackError = error })
  await settle()

  assert.equal(r.keepalive.calls.length, 1)
  assert.equal(r.server.puts.length, 1, 'keepalive unavailability falls back to exactly one ordinary request')
  assert.equal(r.server.puts[0].baseUpdatedAt, SERVER_V1)
  assert.equal(fallbackError?.code, 'ERR_NETWORK')
  assert.deepEqual(localState(r), {
    memory: pendingAt(1, 3918),
    stored: pendingAt(1, 3918),
  })
})

// 8.1(2), R7: real-store version of the confirmed-keepalive test in readerProgressPersistence.test.mjs.
test('an accepted keepalive clears the pending snapshot through onSaved', async (t) => {
  const r = await openReader(t)
  r.position.offset = 3918
  await r.persistence.save({ force: true, background: true })
  await settle()

  assert.equal(r.keepalive.calls.length, 1)
  assert.equal(r.server.puts.length, 0)
  assert.deepEqual(localState(r), {
    memory: confirmedAt(1, 3918),
    stored: confirmedAt(1, 3918),
  })
  assert.equal(r.reader.progressByBook[BOOK_ID].updatedAt, r.server.row.updatedAt)
  assert.equal(storedProgress().updatedAt, r.server.row.updatedAt)
})

// 8.1(2), R2: Home button (goShelf), onBeforeRouteLeave and unmount each queue a background save.
test('the background saves of one Reader leave do not clear each other while offline', async (t) => {
  const r = await openReader(t)
  r.keepalive.outcome = 'offline'
  r.position.offset = 3918
  await r.persistence.save({ force: true, background: true })
  await settle()
  r.elapse(12)
  await r.persistence.save({ force: true, background: true })
  await settle()
  r.elapse(8)
  r.persistence.cancelScheduled()
  await r.persistence.save({ force: true, background: true })
  await settle()

  assert.ok(r.keepalive.calls.length >= 1)
  assert.ok(r.keepalive.calls.every(call => call.body.offset === 3918))
  assert.equal(r.server.puts.length, 0)
  assert.deepEqual(localState(r), {
    memory: pendingAt(1, 3918),
    stored: pendingAt(1, 3918),
  })
})

// 8.1(3), R3
test('a failed ordinary save followed by a failed background save keeps pending and the server base', async (t) => {
  const r = await openReader(t)
  r.server.putMode = 'offline'
  r.keepalive.outcome = 'offline'
  r.position.offset = 2480
  await r.persistence.save().catch(() => {})

  // Aligned (contract 5 #8): the optimistic write keeps the failed ordinary save pending.
  assert.equal(r.server.puts.length, 1)
  assert.deepEqual(localState(r), {
    memory: pendingAt(1, 2480),
    stored: pendingAt(1, 2480),
  })

  r.elapse(1900)
  r.position.offset = 3918
  await r.persistence.save({ force: true, background: true })
  await settle()

  assert.equal(r.keepalive.calls.length, 1)
  assert.equal(r.keepalive.calls[0].body.baseUpdatedAt, SERVER_V1)
  assert.deepEqual(localState(r), {
    memory: pendingAt(1, 3918),
    stored: pendingAt(1, 3918),
  })
})

// 8.1(4), R4: continuous mode applies every scroll sync locally before the idle save (Reader.vue:1846).
test('continuous scroll sync keeps the last server version as the base of the next save', async (t) => {
  const r = await openReader(t, { mode: 'scroll' })
  let scrollTop = 0
  const scheduled = []
  const scroll = useReaderScrollSync({
    isVerticalRead: true,
    restoringPosition: false,
    chapterLoading: false,
    windowBusy: false,
    progressVersion: ref(0),
    syncCurrentChapter() {},
    maybeExtendChapterWindow() {},
    updateLayout() {},
    captureProgressSnapshot: () => r.position.visible,
    applyLocalProgress: snapshot => r.local.apply(r.local.currentPayload(snapshot)),
    scheduleProgressSave: delay => scheduled.push(delay),
    isContinuousScrollRead: true,
    scrollPosition: () => scrollTop,
  })

  const observed = { baseAfterScroll: [], putBases: [] }
  for (const [top, offset, chapterPercent] of [[640, 1200, 0.3], [1280, 2400, 0.6]]) {
    r.elapse(400)
    scrollTop = top
    r.position.visible = { chapterIndex: 1, chapter: CHAPTERS[1], offset, chapterPercent }
    scroll.handle()
    observed.baseAfterScroll.push(r.local.serverBaseUpdatedAt())
    r.elapse(1300)
    await r.persistence.save()
    observed.putBases.push(r.server.puts.at(-1)?.baseUpdatedAt)
  }

  assert.deepEqual(scheduled, [1200, 1200])
  assert.equal(r.server.puts.length, 2)
  assert.deepEqual(localState(r).memory, confirmedAt(1, 2400))
  const [, firstSave] = r.server.versions
  assert.deepEqual(observed, {
    baseAfterScroll: [SERVER_V1, firstSave],
    putBases: [SERVER_V1, firstSave],
  })
})

// 8.1(4), R4: hidden then pagehide while offline (contract 4.3 timeline).
test('the second keepalive after a failed one still carries the server version', async (t) => {
  const r = await openReader(t)
  r.keepalive.outcome = 'offline'
  r.position.offset = 2207
  await r.persistence.save({ force: true, background: true })
  await settle()
  r.elapse(1500)
  await r.persistence.save({ force: true, background: true })
  await settle()

  assert.equal(r.keepalive.calls.length, 2)
  assert.deepEqual(r.keepalive.calls.map(call => call.body.baseUpdatedAt), [SERVER_V1, SERVER_V1])
})

// 8.1(5), R6: no time passes between applyLocal and the optimistic write in reader.saveProgress.
test('an optimistic write in the same millisecond as the local snapshot stays pending', async (t) => {
  const r = await openReader(t, { elapseBeforeRequest: 0 })
  r.server.putMode = 'hold'
  r.position.offset = 1663
  const saving = r.persistence.save({ force: true })
  await settle()

  assert.equal(r.server.puts.length, 1)
  assert.equal(r.server.puts[0].clientUpdatedAt, new Date(CLIENT_START).toISOString())
  assert.equal(storedProgress().updatedAt, new Date(CLIENT_START).toISOString())
  const inFlight = localState(r)
  r.server.held.reject()
  await saving.catch(() => {})
  await settle()

  assert.deepEqual({ inFlight, afterFailure: localState(r) }, {
    inFlight: { memory: pendingAt(1, 1663), stored: pendingAt(1, 1663) },
    afterFailure: { memory: pendingAt(1, 1663), stored: pendingAt(1, 1663) },
  })
})

// 8.1(6), R5: the snapshot is produced by the Reader path (failed leave keepalive), not written by hand.
test('a Reader-produced pending snapshot survives an older network shelf snapshot and is retried on its server base', async (t) => {
  const r = await openReader(t)
  r.keepalive.outcome = 'offline'
  r.position.offset = 3918
  await r.persistence.save({ force: true, background: true })
  await settle()

  r.elapse(4000)
  const putsBeforeShelf = r.server.puts.length
  await r.bookshelf.loadBooks({ force: true, all: true })
  await settle()

  assert.equal(r.server.shelfReads, 2)
  assert.deepEqual({
    retries: r.server.puts.slice(putsBeforeShelf).map(body => ({ baseUpdatedAt: body.baseUpdatedAt, offset: body.offset })),
    local: localState(r),
    shelfOffset: shelfProgress(r)?.offset,
    serverOffset: r.server.row.offset,
  }, {
    retries: [{ baseUpdatedAt: SERVER_V1, offset: 3918 }],
    local: { memory: confirmedAt(1, 3918), stored: confirmedAt(1, 3918) },
    shelfOffset: 3918,
    serverOffset: 3918,
  })
})

// 8.1(6), R5: reopening the Reader (useReaderBookLoad → loadProgress preferLocal) after the failed leave.
test('reopening the Reader prefers the Reader-produced pending snapshot over the older server row', async (t) => {
  const r = await openReader(t)
  r.keepalive.outcome = 'offline'
  r.position.offset = 3918
  await r.persistence.save({ force: true, background: true })
  await settle()

  r.elapse(4000)
  const opened = await r.reader.loadProgress(BOOK_ID, { preferLocal: true })
  await settle()

  assert.ok(r.server.puts.every(body => body.baseUpdatedAt === SERVER_V1), 'a retry must carry the server base')
  assert.deepEqual({ opened: progressView(opened), memoryOffset: r.reader.progressByBook[BOOK_ID]?.offset }, {
    opened: pendingAt(1, 3918),
    memoryOffset: 3918,
  })
})

// 8.1(6), R7: confirmed (non-pending) progress is still replaced by the authoritative shelf without a retry.
test('a confirmed Reader save is replaced by a newer shelf row from another device without a retry', async (t) => {
  const r = await openReader(t)
  r.position.offset = 3918
  await r.persistence.save({ force: true, background: true })
  await settle()
  assert.equal(r.reader.progressByBook[BOOK_ID].pendingSync, undefined)

  r.elapse(5000)
  r.server.commit({
    bookId: BOOK_ID,
    chapterId: CHAPTERS[3].id,
    chapterIndex: 3,
    offset: 77,
    percent: 0.52,
    chapterPercent: 0.1,
    chapterTitle: CHAPTERS[3].title,
    mode: 'page',
  })
  const putsBeforeShelf = r.server.puts.length
  await r.bookshelf.loadBooks({ force: true, all: true })
  await settle()

  assert.equal(r.server.puts.length, putsBeforeShelf)
  assert.deepEqual(localState(r), {
    memory: confirmedAt(3, 77),
    stored: confirmedAt(3, 77),
  })
  assert.equal(shelfProgress(r)?.chapterIndex, 3)
})
