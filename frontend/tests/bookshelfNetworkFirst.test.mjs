import assert from 'node:assert/strict'
import test from 'node:test'
import { resolveShelfNetworkFirst } from '../src/utils/shelfNetworkFirst.js'

function deferred() {
  let resolve
  let reject
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

test('does not read a stale persistent shelf while the authoritative network request is pending', async () => {
  const network = deferred()
  let fallbackReads = 0
  const loading = resolveShelfNetworkFirst({
    request: () => network.promise,
    readFallback: async () => {
      fallbackReads += 1
      return [{ id: 1, title: '旧缓存中的书' }]
    },
    isCurrent: () => true,
    hasCurrent: () => false,
  })

  await new Promise(resolve => setTimeout(resolve, 0))
  assert.equal(fallbackReads, 0, 'the browser cache is a network-failure fallback, not a first paint source')

  const serverBooks = [{ id: 2, title: '服务器中的新书' }]
  network.resolve(serverBooks)
  assert.deepEqual(await loading, {
    source: 'network',
    value: serverBooks,
  })
  assert.equal(fallbackReads, 0)
})

test('uses the scoped persistent shelf only after the network request fails', async () => {
  const fallbackBooks = [{ id: 9, title: '离线书架' }]
  const result = await resolveShelfNetworkFirst({
    request: async () => {
      throw new Error('offline')
    },
    readFallback: async () => fallbackBooks,
    isCurrent: () => true,
    hasCurrent: () => false,
  })

  assert.deepEqual(result, {
    source: 'fallback',
    value: fallbackBooks,
  })
})

test('discards a fallback read invalidated by a newer local shelf mutation', async () => {
  const fallback = deferred()
  let current = true
  let hasCurrent = false
  const loading = resolveShelfNetworkFirst({
    request: async () => {
      throw new Error('offline')
    },
    readFallback: () => fallback.promise,
    isCurrent: () => current,
    hasCurrent: () => hasCurrent,
  })

  await new Promise(resolve => setTimeout(resolve, 0))
  current = false
  hasCurrent = true
  fallback.resolve([{ id: 1, title: '不应恢复的旧缓存' }])

  assert.deepEqual(await loading, { source: 'discarded' })
})

test('rethrows the network error when no usable persistent fallback exists', async () => {
  await assert.rejects(
    resolveShelfNetworkFirst({
      request: async () => {
        throw new Error('offline')
      },
      readFallback: async () => null,
      isCurrent: () => true,
      hasCurrent: () => false,
    }),
    /offline/,
  )
})
