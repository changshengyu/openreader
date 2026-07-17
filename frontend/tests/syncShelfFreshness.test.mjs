import assert from 'node:assert/strict'
import test from 'node:test'
import { refreshShelfAfterSyncConnect } from '../src/utils/shelfSyncFreshness.js'

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
