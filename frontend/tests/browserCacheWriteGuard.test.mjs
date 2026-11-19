import assert from 'node:assert/strict'
import test from 'node:test'
import {
  cacheFirstRequest,
  networkFirstRequest,
} from '../src/utils/browserCache.js'

function legacyStorage(t, indexedDB) {
  const values = new Map()
  const localStorage = {
    getItem: key => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  }
  const previousWindow = globalThis.window
  globalThis.window = { localStorage, indexedDB }
  t.after(() => { globalThis.window = previousWindow })
  return values
}

for (const request of [cacheFirstRequest, networkFirstRequest]) {
  test(request.name + ' preserves a newer cached response when an obsolete request completes', async t => {
    const values = legacyStorage(t)
    let release
    const pending = new Promise(resolve => { release = resolve })
    let current = true
    const first = request(() => pending, 'reader-response', { canWrite: () => current })
    current = false
    await request(async () => ({ data: ['latest'] }), 'reader-response')
    release({ data: ['obsolete'] })
    await first
    assert.deepEqual([...values.values()].map(JSON.parse), [['latest']])
    const offline = await networkFirstRequest(async () => { throw new Error('offline') }, 'reader-response')
    assert.deepEqual(offline, { data: ['latest'], fromCache: true })
  })
}

test('cache writes recheck ownership after IndexedDB opens', async t => {
  const openRequest = {}
  let notifyOpen
  const opening = new Promise(resolve => { notifyOpen = resolve })
  const values = legacyStorage(t, { open: () => { notifyOpen(); return openRequest } })
  let current = true
  let transactions = 0
  const write = networkFirstRequest(async () => ({ data: ['obsolete'] }), 'reader-response', {
    canWrite: () => current,
  })
  await opening
  current = false
  openRequest.result = { transaction: () => { transactions++; throw new Error('write unavailable') } }
  openRequest.onsuccess()
  await write
  assert.equal(transactions, 0)
  assert.equal(values.size, 0)
})

test('an obsolete IndexedDB write failure cannot replace the localStorage fallback', async t => {
  // Each test fixture owns its connection because the cache module keeps the open database promise.
  const { networkFirstRequest: request } = await import('../src/utils/browserCache.js?failed-write')
  let markObsoletePut
  const obsoletePutStarted = new Promise(resolve => { markObsoletePut = resolve })
  const database = {
    transaction: () => ({
      objectStore: () => ({
        put: ({ value }) => {
          const putRequest = { error: new Error('write unavailable') }
          if (value[0] === 'obsolete') {
            markObsoletePut(putRequest)
          } else {
            queueMicrotask(() => putRequest.onerror())
          }
          return putRequest
        },
      }),
    }),
  }
  const values = legacyStorage(t, {
    open() {
      const openRequest = { result: database }
      queueMicrotask(() => openRequest.onsuccess())
      return openRequest
    },
  })
  let current = true
  const obsolete = request(async () => ({ data: ['obsolete'] }), 'reader-response', {
    canWrite: () => current,
  })
  const putRequest = await obsoletePutStarted
  current = false
  await request(async () => ({ data: ['latest'] }), 'reader-response')
  putRequest.onerror()
  await obsolete
  assert.deepEqual([...values.values()].map(JSON.parse), [['latest']])
})
