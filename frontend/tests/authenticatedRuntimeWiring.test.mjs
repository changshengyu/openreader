import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

function source(path) {
  return readFileSync(new URL(path, import.meta.url), 'utf8')
}

const reader = source('../src/stores/reader.js')
const preferences = source('../src/stores/preferences.js')
const bookshelf = source('../src/stores/bookshelf.js')
const user = source('../src/stores/user.js')
const sync = source('../src/composables/useSync.js')

test('every delayed authenticated store path owns a current operation before committing', () => {
  assert.match(reader, /readerSettingsOperations\.begin\('reader'\)/)
  assert.match(reader, /readerSettingsOperations\.canCommit\(operation\)/)
  assert.match(reader, /readerProgressOperations\.begin\(`book:\$\{(?:payload\.)?bookId\}`\)/)
  assert.match(reader, /readerProgressOperations\.canCommit\(operation\)/)
  assert.match(preferences, /preferenceOperations\.begin\(key\)/)
  assert.match(preferences, /preferenceOperations\.canCommit\(operation\)/)
  assert.match(bookshelf, /categoryOperations\.begin\('categories'\)/)
  assert.match(bookshelf, /scopedShelfCacheKey\(CATEGORY_CACHE_KEY, scope\)/)
  assert.match(user, /profileOperations\.begin\('profile'\)/)
  assert.match(user, /profileOperations\.canCommit\(operation\)/)
})

test('websocket callbacks are bound to their candidate generation and authenticated identity', () => {
  assert.match(sync, /const candidate = new WebSocket/)
  assert.match(sync, /const generation = socketGeneration \+ 1/)
  assert.match(sync, /if \(!isCurrentSocket\(candidate, generation, token, scope\)\) return/g)
  assert.match(sync, /candidate\.close\(\)/)
  assert.doesNotMatch(sync, /socket\?\.close\(\)/)
  assert.match(sync, /scheduleReconnect\(\{ generation, token, scope \}\)/)
  assert.match(sync, /if \(manualDisconnect \|\| socket \|\| !isExpectedSocketSession\(expected\)\) return/)
})
