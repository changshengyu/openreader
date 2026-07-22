import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test, { after } from 'node:test'
import { createServer } from 'vite'
import { reconcileAuthoritativeShelfProgress } from '../src/utils/bookOrder.js'

const storage = new Map()
globalThis.localStorage = {
  getItem: key => storage.get(String(key)) ?? null,
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

const bookshelfSource = readFileSync(new URL('../src/stores/bookshelf.js', import.meta.url), 'utf8')
const homeSource = readFileSync(new URL('../src/views/Home.vue', import.meta.url), 'utf8')
const readerShelfSource = readFileSync(new URL('../src/composables/useReaderShelf.js', import.meta.url), 'utf8')

test('an authoritative shelf snapshot replaces newer non-pending client progress', () => {
  const local = {
    bookId: 7,
    chapterIndex: 2,
    chapterTitle: '旧客户端章节',
    updatedAt: '2099-07-22T00:00:00Z',
  }
  const server = {
    bookId: 7,
    chapterIndex: 8,
    chapterTitle: '服务器最新章节',
    updatedAt: '2026-07-22T00:00:00Z',
  }

  assert.deepEqual(reconcileAuthoritativeShelfProgress(local, server), {
    progress: server,
    retryPending: false,
  })
})

test('an authoritative shelf snapshot clears confirmed client progress absent from the server', () => {
  const local = {
    bookId: 7,
    chapterIndex: 2,
    updatedAt: '2099-07-22T00:00:00Z',
  }

  assert.deepEqual(reconcileAuthoritativeShelfProgress(local, null), {
    progress: null,
    retryPending: false,
  })
})

test('a genuinely newer pending local read survives the snapshot and requests a CAS retry', () => {
  const local = {
    bookId: 7,
    chapterIndex: 9,
    updatedAt: '2026-07-22T00:00:02Z',
    pendingSync: true,
    baseUpdatedAt: '2026-07-22T00:00:00Z',
  }
  const server = {
    bookId: 7,
    chapterIndex: 5,
    updatedAt: '2026-07-22T00:00:01Z',
  }

  assert.deepEqual(reconcileAuthoritativeShelfProgress(local, server), {
    progress: local,
    retryPending: true,
  })
})

test('a server snapshot newer than a pending client position resolves to the server winner', () => {
  const local = {
    bookId: 7,
    chapterIndex: 4,
    updatedAt: '2026-07-22T00:00:01Z',
    pendingSync: true,
    baseUpdatedAt: '2026-07-22T00:00:00Z',
  }
  const server = {
    bookId: 7,
    chapterIndex: 10,
    updatedAt: '2026-07-22T00:00:02Z',
  }

  assert.deepEqual(reconcileAuthoritativeShelfProgress(local, server), {
    progress: server,
    retryPending: false,
  })
})

test('network shelf commits reconcile the complete server snapshot before sorting and caching', () => {
  assert.match(
    bookshelfSource,
    /const reconciledBooks = await reconcileServerProgressFromBooks\(result\.value,[\s\S]*?awaitPending: settleProgress,[\s\S]*?this\.books = sortBooks\(reconciledBooks\)/,
  )
  assert.match(bookshelfSource, /await reader\.reconcileShelfProgress\(serverBooks, options\)/)
})

test('an explicit shelf refresh waits for the pending CAS winner before committing', async () => {
  setActivePinia(createPinia())
  const reader = useReaderStore()
  const pending = {
    bookId: 7,
    chapterIndex: 2,
    chapterTitle: '待确认旧位置',
    updatedAt: '2099-07-22T00:00:00Z',
    pendingSync: true,
    baseUpdatedAt: '2026-07-22T00:00:00Z',
  }
  const server = {
    bookId: 7,
    chapterIndex: 8,
    chapterTitle: '服务器位置',
    updatedAt: '2026-07-22T00:00:01Z',
  }
  const winner = {
    ...server,
    chapterIndex: 9,
    chapterTitle: 'CAS 最终位置',
    updatedAt: '2026-07-22T00:00:02Z',
  }
  reader.progressByBook[7] = pending
  let synchronizationOptions
  reader.syncLocalProgress = async (_progress, _baseUpdatedAt, options) => {
    synchronizationOptions = options
    reader.replaceProgress(winner)
    return winner
  }

  const reconciled = await reader.reconcileShelfProgress([
    { id: 7, progress: server },
  ], { awaitPending: true })

  assert.deepEqual(reconciled[7], winner)
  assert.deepEqual(reader.progressByBook[7], winner)
  assert.deepEqual(synchronizationOptions, {
    acceptConflict: true,
    throwOnError: true,
  })
})

test('a failed explicit pending sync remains local and rejects the visible refresh', async () => {
  setActivePinia(createPinia())
  const reader = useReaderStore()
  const pending = {
    bookId: 8,
    chapterIndex: 4,
    chapterTitle: '离线待同步位置',
    updatedAt: '2099-07-22T00:00:00Z',
    pendingSync: true,
    baseUpdatedAt: '2026-07-22T00:00:00Z',
  }
  reader.progressByBook[8] = pending
  reader.syncLocalProgress = async () => {
    throw new Error('offline')
  }

  await assert.rejects(
    reader.reconcileShelfProgress([
      {
        id: 8,
        progress: {
          bookId: 8,
          chapterIndex: 2,
          updatedAt: '2026-07-22T00:00:01Z',
        },
      },
    ], { awaitPending: true }),
    /offline/,
  )
  assert.deepEqual(reader.progressByBook[8], pending)
})

test('both visible refresh buttons request a settled full shelf snapshot', () => {
  assert.match(homeSource, /bookshelf\.loadBooks\(\{ force: true, all: true, settleProgress: true \}\)/)
  assert.match(readerShelfSource, /bookshelf\.loadBooks\(\{ force: true, all: true, settleProgress: true \}\)/)
})
