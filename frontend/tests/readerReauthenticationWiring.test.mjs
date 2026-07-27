import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { safeReturnTo } from '../src/utils/authNavigation.js'

function source(path) {
  return readFileSync(new URL(path, import.meta.url), 'utf8')
}

const app = source('../src/App.vue')
const authDialog = source('../src/components/AuthDialog.vue')
const apiClient = source('../src/api/client.js')
const booksAPI = source('../src/api/books.js')
const reader = source('../src/views/Reader.vue')
const router = source('../src/router/index.js')
const user = source('../src/stores/user.js')

test('the root shell never renders authenticated Reader content while its session is blocked', () => {
  assert.match(app, /v-if="isReader && isLoggedIn && !readerSessionBlocked"/)
  assert.match(app, /<router-view\s+:key="readerSessionKey"/)
  assert.match(app, /v-else-if="isReader"[\s\S]*?reader-auth-blocked/)
  assert.match(app, /<router-view v-else-if="isLoginRoute"/)
  assert.match(app, /v-else class="workspace-auth-blocked"/)
  assert.match(app, /readerSessionKey/)
})

test('the active Reader suspends persistence synchronously when its session is invalidated', () => {
  assert.match(reader, /onSessionInvalidated:\s*\(\)\s*=>\s*\{[\s\S]*?suspendProgressSaving\(\)/)
})

test('session reauthentication is resolved without a hard reload or a dismissible invalid-session dialog', () => {
  assert.doesNotMatch(authDialog, /window\.location\.reload/)
  assert.match(authDialog, /:close-on-click-modal="user\.authReason !== 'session'"/)
  assert.match(authDialog, /:close-on-press-escape="user\.authReason !== 'session'"/)
  assert.match(authDialog, /:show-close="user\.authReason !== 'session'"/)
  assert.match(authDialog, /sameAuthenticatedScope/)
  assert.doesNotMatch(authDialog, /result\.previousScope[\s\S]*?!result\.sameAuthenticatedScope/)
  assert.match(authDialog, /completeReauthentication/)
})

test('session clearing dispatches invalidation before token removal and resets overlays', () => {
  assert.match(
    user,
    /dispatchSessionInvalidated\([\s\S]*?this\.token = ''[\s\S]*?useOverlayStore\(\)\.resetSessionState\(\)/,
  )
})

test('only the currently stored rejected token can create a new auth-required event', () => {
  assert.match(apiClient, /localStorage\.getItem\('openreader_token'\) !== rejectedToken[\s\S]*?return Promise\.reject\(error\)/)
  assert.match(booksAPI, /localStorage\.getItem\('openreader_token'\) === token[\s\S]*?openreader:auth-required/)
})

test('unauthenticated route redirects preserve a safe return path', () => {
  assert.match(router, /returnTo:\s*safeReturnTo\(to\.fullPath\)/)
  assert.match(router, /safeReturnTo/)
  assert.equal(safeReturnTo('/books/7/read?chapter=2'), '/books/7/read?chapter=2')
  assert.equal(safeReturnTo('//malicious.example/path'), '/')
  assert.equal(safeReturnTo('https://malicious.example/path'), '/')
  assert.equal(safeReturnTo(['/books/7/read', '//malicious.example/path']), '/')
  assert.equal(safeReturnTo('/\\malicious.example/path'), '/')
})
