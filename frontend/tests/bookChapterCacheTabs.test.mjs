import assert from 'node:assert/strict'
import test from 'node:test'
import { chapterCacheKey } from '../src/utils/bookChapterCache.js'
import { getBrowserCache, setBrowserCache } from '../src/utils/browserCache.js'

const book = { id: 7, title: 'Scoped', author: 'Reader', url: 'book-url-7' }
const body = content => ({ chapter: { index: 0 }, content })
// browserCache keeps the opened connection. Give each scenario its own key on it.
const records = new Map()
let beforeCompare

function deferred() {
  let resolve
  const promise = new Promise(done => { resolve = done })
  return { promise, resolve }
}

function installSharedStorage(t, { indexed = true } = {}) {
  const fallback = new Map()
  const localStorage = {
    getItem: key => fallback.get(key) ?? null,
    setItem: (key, value) => fallback.set(key, value),
  }
  const indexedDB = indexed ? {
    open() {
      const request = { result: { transaction } }
      queueMicrotask(() => request.onsuccess())
      return request
    },
  } : undefined

  function transaction(_store, mode) {
    const tx = { objectStore: () => store }
    const store = {
      get(key) {
        return run(() => {
          if (mode === 'readwrite') beforeCompare?.()
          return records.get(key)
        })
      },
      put(record) { return run(() => records.set(record.key, structuredClone(record))) },
    }
    let pending = 0
    function run(operation) {
      const request = {}
      pending += 1
      queueMicrotask(() => {
        request.result = operation()
        request.onsuccess?.()
        pending -= 1
        if (pending === 0) tx.oncomplete?.()
      })
      return request
    }
    return tx
  }

  const oldWindow = globalThis.window
  globalThis.window = { localStorage, indexedDB }
  t.after(() => {
    globalThis.window = oldWindow
    beforeCompare = undefined
  })
  return { beforeWriteCompare(fn) { beforeCompare = fn } }
}

for (const indexed of [true, false]) {
  test('other-tab explicit reload survives a pending ordinary chapter read in ' + (indexed ? 'IndexedDB' : 'localStorage'), async t => {
    installSharedStorage(t, { indexed })
    const { loadBrowserChapterContent: tabA } = await import('../src/utils/bookChapterCache.js?tab-a')
    const { loadBrowserChapterContent: tabB } = await import('../src/utils/bookChapterCache.js?tab-b')
    const scope = indexed ? 'user:11' : 'user:22'
    const key = chapterCacheKey(book, book.id, 0, scope)
    const started = deferred()
    const pending = deferred()

    const old = tabA(book, book.id, 0, {
      scope,
      getChapterContent: () => {
        started.resolve()
        return pending.promise
      },
    })
    await started.promise
    await tabB(book, book.id, 0, {
      scope,
      refreshRemote: true,
      getChapterContent: async () => ({ data: body('fresh') }),
    })
    pending.resolve({ data: body('old') })

    assert.deepEqual(await old, body('old'))
    assert.deepEqual(await getBrowserCache(key), body('fresh'))
  })
}

test('refresh preserves the newest content but does not block later browser-only refreshes', async t => {
  installSharedStorage(t)
  const { loadBrowserChapterContent: tabA } = await import('../src/utils/bookChapterCache.js?tab-a')
  const { loadBrowserChapterContent: tabB } = await import('../src/utils/bookChapterCache.js?tab-b')
  const scope = 'user:33'
  const key = chapterCacheKey(book, book.id, 0, scope)
  await setBrowserCache(key, body('existing'))
  assert.deepEqual(await tabB(book, book.id, 0, { scope }), body('existing'))

  const started = deferred()
  const pending = deferred()
  const old = tabA(book, book.id, 0, {
    scope,
    refreshRemote: true,
    getChapterContent: () => {
      started.resolve()
      return pending.promise
    },
  })
  await started.promise
  await tabB(book, book.id, 0, {
    scope,
    refreshRemote: true,
    getChapterContent: async () => ({ data: body('fresh') }),
  })
  pending.resolve({ data: body('older reload') })
  await old
  assert.deepEqual(await getBrowserCache(key), body('fresh'))

  await tabA(book, book.id, 0, {
    scope,
    refresh: true,
    getChapterContent: async () => ({ data: body('rules updated') }),
  })
  assert.deepEqual(await getBrowserCache(key), body('rules updated'))
})

test('a read started while another tab is reloading keeps that reload cached after it succeeds', async t => {
  installSharedStorage(t)
  const { loadBrowserChapterContent: tabA } = await import('../src/utils/bookChapterCache.js?tab-a')
  const { loadBrowserChapterContent: tabB } = await import('../src/utils/bookChapterCache.js?tab-b')
  const scope = 'user:44'
  const key = chapterCacheKey(book, book.id, 0, scope)
  const source = deferred()
  const serverCache = deferred()
  const started = deferred()
  const reload = tabA(book, book.id, 0, {
    scope,
    refreshRemote: true,
    getChapterContent: () => source.promise,
  })
  const ordinary = tabB(book, book.id, 0, {
    scope,
    getChapterContent: () => {
      started.resolve()
      return serverCache.promise
    },
  })
  await started.promise
  source.resolve({ data: body('fresh') })
  await reload
  serverCache.resolve({ data: body('old') })
  await ordinary
  assert.deepEqual(await getBrowserCache(key), body('fresh'))
})

test('checks cancellation again before the IndexedDB transaction writes the chapter', async t => {
  const storage = installSharedStorage(t)
  const { loadBrowserChapterContent } = await import('../src/utils/bookChapterCache.js?tab-a')
  const scope = 'user:55'
  const key = chapterCacheKey(book, book.id, 0, scope)
  const controller = new AbortController()
  await setBrowserCache(key, body('existing'))
  storage.beforeWriteCompare(() => controller.abort())
  await loadBrowserChapterContent(book, book.id, 0, {
    scope,
    refreshRemote: true,
    signal: controller.signal,
    getChapterContent: async () => ({ data: body('cancelled update') }),
  })
  assert.equal(controller.signal.aborted, true)
  assert.deepEqual(await getBrowserCache(key), body('existing'))
})
