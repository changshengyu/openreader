import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('../src/layouts/AppLayout.vue', import.meta.url), 'utf8')
const bookshelfSource = readFileSync(new URL('../src/stores/bookshelf.js', import.meta.url), 'utf8')

test('foreground shelf reconciliation does not treat websocket connected as a freshness guarantee', () => {
  assert.doesNotMatch(
    source,
    /if \(syncConnected\.value && bookshelf\.books\.length\) return/,
    'a connected socket may have missed a backpressured state event',
  )
  assert.match(source, /createShelfForegroundReconciler/)
  assert.match(source, /function setOnline\(\)[\s\S]*?refreshShelfInForeground\(\)/)
})

test('the Pinia shelf uses the network-first resolver instead of eagerly committing persistent cache', () => {
  assert.match(bookshelfSource, /resolveShelfNetworkFirst/)
  assert.doesNotMatch(
    bookshelfSource,
    /if \(!force && this\.books\.length === 0\) \{\s*const cached = await readShelfCache/,
  )
})
