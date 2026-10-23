import assert from 'node:assert/strict'
import test, { after } from 'node:test'
import { ref } from 'vue'
import { createServer } from 'vite'

// Local progress snapshots go through the real Pinia reader and bookshelf stores, so the
// bookshelf.upsertBook → reader.applyServerProgress path is part of every assertion (contract:
// docs/compat/reader-progress-pending-sync-keepalive-fixed-baseline-second-audit-p0-contract.md 8.1(7)).

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
const { useReaderLocalProgress } = await vite.ssrLoadModule('/src/composables/useReaderLocalProgress.js')

// Unsigned test value: the stores only decode its payload to scope the progress storage key.
const TOKEN = ['test', Buffer.from(JSON.stringify({ userId: 5 })).toString('base64url'), 'unsigned'].join('.')
const SERVER_UPDATED_AT = '2026-07-01T23:59:58.123456Z'

// Records the composable's own store calls and delegates them to the real store; calls the stores make
// internally (upsertBook → applyServerProgress → replaceProgress) still run but are not counted.
function recordingView(store, names, calls) {
  return new Proxy(store, {
    get(target, key) {
      if (!Object.hasOwn(names, key)) return target[key]
      return (...args) => {
        calls.push([names[key], ...args])
        return target[key](...args)
      }
    },
  })
}

function createController(overrides = {}) {
  storage.clear()
  storage.set('openreader_token', TOKEN)
  setActivePinia(createPinia())
  const reader = useReaderStore()
  const bookshelf = useBookshelfStore()
  reader.mode = 'scroll'
  const serverProgress = {
    bookId: 7,
    chapterId: 11,
    chapterIndex: 1,
    chapterTitle: '第二章',
    offset: 0,
    percent: 0.1,
    chapterPercent: 0,
    mode: 'scroll',
    updatedAt: SERVER_UPDATED_AT,
  }
  reader.replaceProgress(serverProgress)
  bookshelf.books = [{ id: 7, title: '测试书', progress: serverProgress }]
  const calls = []
  const book = ref({ id: 7, title: '测试书', progress: serverProgress })
  const chapter = ref({ id: 12, title: '第三章' })
  const controller = useReaderLocalProgress({
    reader: recordingView(reader, { applyProgress: 'apply' }, calls),
    bookshelf: recordingView(bookshelf, { upsertBook: 'upsert', applyBookProgress: 'shelf' }, calls),
    bookId: ref(7),
    book,
    chapter,
    chapters: ref(Array.from({ length: 10 }, (_, index) => ({ id: 10 + index, bookId: 7, index, title: `第${index + 1}章` }))),
    currentIndex: ref(2),
    getVisibleSnapshot: () => null,
    getCurrentOffset: () => 120,
    getCurrentPercent: () => 0.4,
    mergeBook: mergeShelfBook,
    now: () => new Date('2026-07-02T00:00:00Z'),
    getMode: () => reader.mode,
    ...overrides,
  })
  return { book, bookshelf, calls, chapter, controller, reader }
}

function storedProgress(bookId) {
  const raw = storage.get(`openreader_chapter_progress@user:5@${bookId}`)
  return raw ? JSON.parse(raw) : null
}

function pendingFields(progress) {
  return {
    pendingSync: Boolean(progress?.pendingSync),
    baseUpdatedAt: progress?.baseUpdatedAt || '',
  }
}

test('builds progress from the current chapter when no visible snapshot exists', () => {
  const fixture = createController()
  assert.deepEqual(fixture.controller.currentPayload(), {
    bookId: 7,
    chapterId: 12,
    chapterIndex: 2,
    offset: 120,
    percent: 0.24,
    chapterPercent: 0.4,
    chapterTitle: '第三章',
  })
})

test('deduplicates local progress while allowing forced snapshots', () => {
  const fixture = createController()
  fixture.controller.apply()
  fixture.controller.apply()
  fixture.controller.apply(undefined, { force: true })
  assert.equal(fixture.calls.filter(call => call[0] === 'apply').length, 2)
  assert.equal(fixture.calls.filter(call => call[0] === 'upsert').length, 2)
  assert.equal(fixture.book.value.shelfOrderAt, '2026-07-02T00:00:00.000Z')
  // The snapshot stays pending on its server base wherever the Reader, the shelf and the next session read it.
  const pending = { pendingSync: true, baseUpdatedAt: SERVER_UPDATED_AT }
  assert.deepEqual({
    book: pendingFields(fixture.book.value.progress),
    memory: pendingFields(fixture.reader.progressByBook[7]),
    stored: pendingFields(storedProgress(7)),
  }, {
    book: pending,
    memory: pending,
    stored: pending,
  })
})

test('preserves a pending server base and updates unrelated shelf progress', () => {
  const fixture = createController()
  fixture.reader.progressByBook[7] = {
    pendingSync: true,
    baseUpdatedAt: 'server-v1',
    updatedAt: 'local-v2',
  }
  assert.equal(fixture.controller.serverBaseUpdatedAt(), 'server-v1')
  fixture.controller.upsert({ bookId: 8, updatedAt: 'other-v1' }, { replace: true })
  assert.deepEqual(fixture.calls, [
    ['shelf', { bookId: 8, updatedAt: 'other-v1' }, { replace: true }],
  ])
  assert.deepEqual(pendingFields(fixture.reader.progressByBook[7]), { pendingSync: true, baseUpdatedAt: 'server-v1' })
})

// docs/compat/reader-desktop-flip-account-fixed-baseline-second-audit-p1-contract.md U8.
test('records the mode it is given rather than the raw setting', () => {
  const fixture = createController({ getMode: () => 'page' })
  fixture.reader.mode = 'flip'
  fixture.controller.apply()
  const applied = fixture.calls.filter(call => call[0] === 'apply').map(call => call[1].mode)
  assert.deepEqual(applied, ['page'])
})
