import assert from 'node:assert/strict'
import test from 'node:test'
import {
  createShelfForegroundReconciler,
  refreshShelfAfterSyncConnect,
} from '../src/utils/shelfSyncFreshness.js'

test('forces a server shelf refresh after sync reconnect instead of trusting a recent local cache', async () => {
  const calls = []
  await refreshShelfAfterSyncConnect({
    loadCategories: options => calls.push(['categories', options]),
    loadBooks: options => calls.push(['books', options]),
  })
  assert.deepEqual(calls, [
    ['categories', { force: true }],
    ['books', { force: true, all: true }],
  ])
})

test('reconciles a visible shelf even when the websocket still reports connected', async () => {
  let now = 100_000
  const calls = []
  const reconciler = createShelfForegroundReconciler({
    loadShelf: options => calls.push(options),
    isVisible: () => true,
    now: () => now,
    interval: 30_000,
  })

  assert.equal(await reconciler.refresh(), true)
  assert.deepEqual(calls, [{ force: true, all: true }])

  now += 10_000
  assert.equal(await reconciler.refresh(), false)
  assert.equal(calls.length, 1, 'a successful foreground reconciliation should be throttled')

  now += 21_000
  assert.equal(await reconciler.refresh(), true)
  assert.equal(calls.length, 2)
})

test('deduplicates concurrent foreground refreshes and retries immediately after a failure', async () => {
  let calls = 0
  let fail = true
  let release
  const pending = new Promise(resolve => {
    release = resolve
  })
  const reconciler = createShelfForegroundReconciler({
    loadShelf: async () => {
      calls += 1
      await pending
      if (fail) throw new Error('offline')
    },
    isVisible: () => true,
    now: () => 100_000,
    interval: 30_000,
  })

  const first = reconciler.refresh()
  const duplicate = reconciler.refresh()
  assert.equal(calls, 1)
  release()
  await assert.rejects(first, /offline/)
  await assert.rejects(duplicate, /offline/)

  fail = false
  assert.equal(await reconciler.refresh(), true)
  assert.equal(calls, 2, 'a failed attempt must not consume the successful throttle window')
})

test('does not reconcile while the document is hidden', async () => {
  let calls = 0
  const reconciler = createShelfForegroundReconciler({
    loadShelf: async () => {
      calls += 1
    },
    isVisible: () => false,
    now: () => 100_000,
  })

  assert.equal(await reconciler.refresh(), false)
  assert.equal(calls, 0)
})
