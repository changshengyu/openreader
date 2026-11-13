import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import test, { after } from 'node:test'
import { createServer } from 'vite'

// Contract: docs/compat/remember-login-fixed-baseline-second-audit-p2-contract.md
// reader-dev App.vue shows 记住登录信息 (checked by default) and calls setToken,
// which writes localStorage.api_token, only when it is checked. OpenReader keeps
// a remembered token in localStorage and an unremembered one in this tab's
// sessionStorage, both under openreader_token.
const __dirname = dirname(fileURLToPath(import.meta.url))
const frontendRoot = resolve(__dirname, '..')
const read = path => readFileSync(resolve(frontendRoot, path), 'utf8')

class MemoryStorage {
  constructor() {
    this.values = new Map()
  }

  get length() {
    return this.values.size
  }

  clear() {
    this.values.clear()
  }

  getItem(key) {
    return this.values.has(String(key)) ? this.values.get(String(key)) : null
  }

  key(index) {
    return [...this.values.keys()][index] ?? null
  }

  removeItem(key) {
    this.values.delete(String(key))
  }

  setItem(key, value) {
    this.values.set(String(key), String(value))
  }
}

function tokenFor(userId, nonce = '') {
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url')
  const payload = Buffer.from(JSON.stringify({ userId })).toString('base64url')
  return `${header}.${payload}.remember-${userId}-${nonce}`
}

const local = new MemoryStorage()
const session = new MemoryStorage()
const dispatchedEvents = []
globalThis.localStorage = local
globalThis.sessionStorage = session
globalThis.window = {
  localStorage: local,
  sessionStorage: session,
  location: { protocol: 'http:', host: 'openreader.test' },
  addEventListener() {},
  removeEventListener() {},
  dispatchEvent(event) {
    dispatchedEvents.push({ type: event?.type || '', detail: event?.detail })
  },
  setTimeout: () => 0,
  clearTimeout() {},
}
globalThis.CustomEvent = class CustomEvent {
  constructor(type, options = {}) {
    this.type = type
    this.detail = options.detail
  }
}

const vite = await createServer({
  root: frontendRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
})
after(() => vite.close())

const authTokenModule = await vite.ssrLoadModule('/src/utils/authToken.js').catch(() => null)
const { createPinia, setActivePinia } = await import('pinia')
const { default: api } = await vite.ssrLoadModule('/src/api/client.js')
const { useUserStore } = await vite.ssrLoadModule('/src/stores/user.js')

function resetStorage() {
  local.clear()
  session.clear()
  dispatchedEvents.length = 0
  delete window.__openreaderAuthRequired
}

function requireTokenModule() {
  assert.ok(authTokenModule?.readAuthToken, 'src/utils/authToken.js must export readAuthToken')
  for (const name of ['writeAuthToken', 'discardAuthToken', 'clearAuthToken']) {
    assert.equal(typeof authTokenModule[name], 'function', `src/utils/authToken.js must export ${name}`)
  }
  return authTokenModule
}

async function withAPI(method, replacement, callback) {
  const original = api[method]
  api[method] = replacement
  try {
    return await callback()
  } finally {
    api[method] = original
  }
}

test('T1 a remembered token lives in localStorage and an unremembered one only in this tab', { concurrency: false }, () => {
  const { readAuthToken, writeAuthToken } = requireTokenModule()
  resetStorage()
  writeAuthToken('remembered-token', { remember: true })
  assert.equal(local.getItem('openreader_token'), 'remembered-token')
  assert.equal(session.getItem('openreader_token'), null)
  assert.equal(readAuthToken(), 'remembered-token')

  // Choosing not to remember also drops the remembered token (A3).
  writeAuthToken('session-token', { remember: false })
  assert.equal(session.getItem('openreader_token'), 'session-token')
  assert.equal(local.getItem('openreader_token'), null)
  assert.equal(readAuthToken(), 'session-token')

  // A remembered login replaces this tab's session token.
  writeAuthToken('remembered-again', { remember: true })
  assert.equal(local.getItem('openreader_token'), 'remembered-again')
  assert.equal(session.getItem('openreader_token'), null)
})

test('T1 this tab reads its own session token before a remembered one', { concurrency: false }, () => {
  const { readAuthToken } = requireTokenModule()
  resetStorage()
  local.setItem('openreader_token', 'other-tab-remembered')
  session.setItem('openreader_token', 'this-tab-session')
  assert.equal(readAuthToken(), 'this-tab-session')
  session.removeItem('openreader_token')
  assert.equal(readAuthToken(), 'other-tab-remembered')
  local.removeItem('openreader_token')
  assert.equal(readAuthToken(), '')
})

test('T1 discard removes only the rejected token and clear removes both', { concurrency: false }, () => {
  const { discardAuthToken, clearAuthToken, readAuthToken } = requireTokenModule()
  resetStorage()
  local.setItem('openreader_token', 'remembered-token')
  session.setItem('openreader_token', 'session-token')
  discardAuthToken('stale-token')
  assert.equal(local.getItem('openreader_token'), 'remembered-token')
  assert.equal(session.getItem('openreader_token'), 'session-token')
  discardAuthToken('session-token')
  assert.equal(session.getItem('openreader_token'), null)
  assert.equal(local.getItem('openreader_token'), 'remembered-token')
  session.setItem('openreader_token', 'session-token')
  clearAuthToken()
  assert.equal(local.getItem('openreader_token'), null)
  assert.equal(session.getItem('openreader_token'), null)
  assert.equal(readAuthToken(), '')
})

test('T1 missing or throwing storage reads as no token', { concurrency: false }, () => {
  const { readAuthToken } = requireTokenModule()
  const throwing = { getItem() { throw new Error('storage disabled') } }
  window.localStorage = throwing
  globalThis.localStorage = throwing
  window.sessionStorage = undefined
  globalThis.sessionStorage = undefined
  try {
    assert.equal(readAuthToken(), '')
  } finally {
    window.localStorage = local
    globalThis.localStorage = local
    window.sessionStorage = session
    globalThis.sessionStorage = session
  }
})

test('T2 the user store remembers by default and honours an unchecked box', { concurrency: false }, async () => {
  resetStorage()
  setActivePinia(createPinia())
  const user = useUserStore()
  assert.equal(user.rememberLogin, true, 'reader-dev data.remember defaults to true')

  await withAPI('post', async () => ({ data: { token: tokenFor(3, 'remembered'), user: { id: 3, username: 'reader03' } } }), async () => {
    await user.login('reader03', 'password88')
  })
  assert.equal(local.getItem('openreader_token'), tokenFor(3, 'remembered'))
  assert.equal(session.getItem('openreader_token'), null)

  await withAPI('post', async () => ({ data: null }), async () => {
    user.logout()
  })
  assert.equal(local.getItem('openreader_token'), null)
  assert.equal(session.getItem('openreader_token'), null)

  user.rememberLogin = false
  await withAPI('post', async () => ({ data: { token: tokenFor(3, 'session'), user: { id: 3, username: 'reader03' } } }), async () => {
    await user.login('reader03', 'password88')
  })
  assert.equal(session.getItem('openreader_token'), tokenFor(3, 'session'))
  assert.equal(local.getItem('openreader_token'), null)
  assert.equal(user.token, tokenFor(3, 'session'))
  assert.equal(user.rememberLogin, false, 'the choice stays for the page lifetime')

  user.clearSession({ suspendWorkspace: false })
  assert.equal(session.getItem('openreader_token'), null)
  assert.equal(local.getItem('openreader_token'), null)
})

test('T2 a reloaded page starts from this tab\'s session token', { concurrency: false }, () => {
  resetStorage()
  session.setItem('openreader_token', tokenFor(4, 'session'))
  setActivePinia(createPinia())
  const user = useUserStore()
  assert.equal(user.token, tokenFor(4, 'session'))
  assert.equal(user.rememberLogin, true, 'a reload restores the default checked box')
})

const okAdapter = async config => ({ config, data: {}, headers: {}, request: {}, status: 200, statusText: 'OK' })
const unauthorizedAdapter = async config => {
  const error = new Error('Request failed with status code 401')
  error.config = config
  error.response = { status: 401, data: { error: 'unauthorized' }, config }
  throw error
}

test('T3 requests carry an unremembered token, and its 401 drops it and asks to sign in', { concurrency: false }, async () => {
  resetStorage()
  const token = tokenFor(5, 'session')
  session.setItem('openreader_token', token)
  const response = await api.get('/me', { adapter: okAdapter })
  assert.equal(response.config.headers.Authorization, `Bearer ${token}`)

  await assert.rejects(api.get('/books', { adapter: unauthorizedAdapter }))
  assert.equal(session.getItem('openreader_token'), null)
  assert.deepEqual(dispatchedEvents.map(event => [event.type, event.detail?.rejectedToken]), [
    ['openreader:auth-required', token],
  ])
})

test('T3 a stale 401 leaves the current session token and sends nothing', { concurrency: false }, async () => {
  resetStorage()
  const current = tokenFor(6, 'current')
  session.setItem('openreader_token', current)
  await assert.rejects(api.get('/books', {
    headers: { Authorization: `Bearer ${tokenFor(6, 'old')}` },
    adapter: unauthorizedAdapter,
  }))
  assert.equal(session.getItem('openreader_token'), current)
  assert.deepEqual(dispatchedEvents, [])
})

function sourceFiles(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
    const path = join(dir, entry.name)
    if (entry.isDirectory()) return sourceFiles(path)
    return /\.(js|vue)$/.test(entry.name) ? [path] : []
  })
}

test('T4 only utils/authToken.js reads or writes openreader_token storage', () => {
  const offenders = []
  for (const file of sourceFiles(resolve(frontendRoot, 'src'))) {
    const name = relative(frontendRoot, file)
    if (name === join('src', 'utils', 'authToken.js')) continue
    const source = readFileSync(file, 'utf8')
    if (/(localStorage|sessionStorage)\??\.(getItem|setItem|removeItem)\(\s*['"]openreader_token['"]/.test(source)) {
      offenders.push(name)
    }
  }
  assert.deepEqual(offenders, [], 'read and write the token through utils/authToken.js')
})

test('T4 the auth form shows 记住登录信息 in both modes, after the fields and before submit', () => {
  const form = read('src/components/AuthForm.vue')
  const checkbox = form.search(/<el-checkbox[^>]*v-model="user\.rememberLogin"[^>]*>\s*记住登录信息\s*<\/el-checkbox>/)
  assert.notEqual(checkbox, -1, 'the form needs <el-checkbox v-model="user.rememberLogin">记住登录信息</el-checkbox>')
  const opening = form.slice(checkbox, form.indexOf('>', checkbox) + 1)
  assert.doesNotMatch(opening, /v-if|v-show/, 'the checkbox shows in login and register mode')
  const item = form.lastIndexOf('<el-form-item', checkbox)
  const itemClose = form.lastIndexOf('</el-form-item>', checkbox)
  if (item > itemClose) {
    const itemOpening = form.slice(item, form.indexOf('>', item) + 1)
    assert.doesNotMatch(itemOpening, /v-if|v-show/, 'the checkbox item shows in login and register mode')
  }
  assert.ok(checkbox > form.indexOf('label="密码"'), 'the checkbox sits below the password field')
  assert.ok(checkbox > form.indexOf('label="邀请码(没有则不填)"'), 'the checkbox sits below the register-only invite field')
  assert.ok(checkbox < form.indexOf('native-type="submit"'), 'the checkbox sits above the submit button')
})
