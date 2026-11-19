import assert from 'node:assert/strict'
import test from 'node:test'

function installStorage(t, { available = true } = {}) {
  const indexed = new Map()
  const fallback = new Map()
  const localStorage = {
    getItem: key => fallback.get(key) ?? null,
    setItem: (key, value) => fallback.set(key, value),
  }
  const indexedDB = {
    open() {
      const request = {}
      queueMicrotask(() => {
        if (available) {
          request.result = { transaction }
          request.onsuccess()
        } else {
          request.error = new Error('IndexedDB open is temporarily blocked')
          request.onerror()
        }
      })
      return request
    },
  }

  function transaction() {
    if (!available) throw new Error('IndexedDB is temporarily unavailable')
    const tx = { objectStore: () => store }
    const store = {
      get: key => run(() => indexed.get(key)),
      put: record => run(() => indexed.set(record.key, structuredClone(record))),
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

  const previousWindow = globalThis.window
  globalThis.window = { indexedDB, localStorage }
  t.after(() => { globalThis.window = previousWindow })
  return {
    indexed,
    setAvailable(value) { available = value },
  }
}

const key = 'user:15@title@book-url@chapterContent-0'
const body = content => ({ chapter: { index: 0 }, content })
const reload = startedAt => ({ refreshRemote: true, startedAt })
const normal = chapterReload => ({ refreshRemote: false, chapterReload })

test('an empty store carries over the last reload when IndexedDB recovers', async t => {
  const storage = installStorage(t)
  const cache = await import('../src/utils/browserCache.js?empty-store-recovery')
  await cache.getBrowserCache(key)
  storage.setAvailable(false)
  await cache.setBrowserCache(key, body('fallback body'), undefined, reload(20))
  const fallback = await cache.readBrowserCacheRecord(key)
  assert.deepEqual(fallback, { value: body('fallback body'), chapterReload: 20 })

  storage.setAvailable(true)
  await cache.setBrowserCache(key, body('updated body'), undefined, normal(fallback.chapterReload))
  assert.deepEqual(await cache.getBrowserCache(key), body('updated body'))
  assert.equal([...storage.indexed.values()][0].chapterReload, 20)
})

test('an IndexedDB open failure allows a later chapter write to recover in the same page', async t => {
  const storage = installStorage(t, { available: false })
  const cache = await import('../src/utils/browserCache.js?open-failure-recovery')
  await cache.setBrowserCache(key, body('fallback body'), undefined, reload(20))
  storage.setAvailable(true)

  await cache.setBrowserCache(key, body('new body'), undefined, reload(30))
  assert.deepEqual([...storage.indexed.values()].map(record => record.value), [body('new body')])
})

test('a newer fallback reload wins on recovery and prevents a pending older write', async t => {
  const storage = installStorage(t)
  const cache = await import('../src/utils/browserCache.js?newer-fallback-recovery')
  await cache.setBrowserCache(key, body('indexed body'), undefined, reload(10))
  const earlier = await cache.readBrowserCacheRecord(key)
  storage.setAvailable(false)
  await cache.setBrowserCache(key, body('fallback body'), undefined, reload(20))
  storage.setAvailable(true)

  const latest = await cache.readBrowserCacheRecord(key)
  assert.equal(latest.chapterReload, 20)
  assert.deepEqual(latest.value, body('fallback body'))
  await cache.setBrowserCache(key, body('old response'), undefined, normal(earlier.chapterReload))
  await cache.setBrowserCache(key, body('old reload'), undefined, reload(15))
  assert.deepEqual(await cache.getBrowserCache(key), body('fallback body'))
  assert.deepEqual([...storage.indexed.values()].map(record => record.value), [body('indexed body')])

  await cache.setBrowserCache(key, body('updated body'), undefined, normal(latest.chapterReload))
  assert.deepEqual(await cache.getBrowserCache(key), body('updated body'))
  assert.equal([...storage.indexed.values()][0].chapterReload, 20)
})
