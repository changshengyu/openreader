import assert from 'node:assert/strict'
import test, { after } from 'node:test'
import { createServer } from 'vite'

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

function deferred() {
  let resolve
  let reject
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function tokenFor(userId, nonce = '') {
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url')
  const payload = Buffer.from(JSON.stringify({ userId })).toString('base64url')
  return `${header}.${payload}.scope-${userId}-${nonce}`
}

const storage = new MemoryStorage()
const timerCallbacks = new Map()
let timerSerial = 0
globalThis.localStorage = storage
globalThis.window = {
  localStorage: storage,
  location: { protocol: 'http:', host: 'openreader.test' },
  addEventListener() {},
  removeEventListener() {},
  dispatchEvent() {},
  setTimeout(callback) {
    timerSerial += 1
    timerCallbacks.set(timerSerial, callback)
    return timerSerial
  },
  clearTimeout(id) {
    timerCallbacks.delete(id)
  },
}
globalThis.CustomEvent = class CustomEvent {
  constructor(type, options = {}) {
    this.type = type
    this.detail = options.detail
  }
}

const vite = await createServer({
  root: new URL('..', import.meta.url).pathname,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
})
after(() => vite.close())

const { createPinia, setActivePinia } = await import('pinia')
// Load the shared API module before stores so Vite's SSR graph gives every
// production import the same mutable client instance used by these contracts.
const { default: api } = await vite.ssrLoadModule('/src/api/client.js')
const { useBookshelfStore } = await vite.ssrLoadModule('/src/stores/bookshelf.js')
const { usePreferencesStore } = await vite.ssrLoadModule('/src/stores/preferences.js')
const { useReaderStore } = await vite.ssrLoadModule('/src/stores/reader.js')
const { useUserStore } = await vite.ssrLoadModule('/src/stores/user.js')
const { useSync } = await vite.ssrLoadModule('/src/composables/useSync.js')

function activateUser(userId, nonce = '') {
  const token = tokenFor(userId, nonce)
  storage.setItem('openreader_token', token)
  return token
}

function freshStores(userId = 1) {
  storage.clear()
  timerCallbacks.clear()
  activateUser(userId, 'initial')
  setActivePinia(createPinia())
  return {
    bookshelf: useBookshelfStore(),
    preferences: usePreferencesStore(),
    reader: useReaderStore(),
    user: useUserStore(),
  }
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

test('a delayed reader-settings load cannot commit into a later authenticated scope', { concurrency: false }, async () => {
  const request = deferred()
  const { reader } = freshStores(1)

  await withAPI('get', () => request.promise, async () => {
    const loading = reader.loadReaderSettings()
    activateUser(2, 'next')
    reader.resetReaderSettingsState()
    reader.theme = 'blue'
    reader.settingsSyncBaseUpdatedAt = 'user-b-base'
    reader.settingsUpdatedAt = 'user-b-updated'
    reader.settingsSyncError = 'user-b-error'

    request.resolve({
      data: {
        value: { theme: 'dark', themeType: 'night', fontSize: 31 },
        updatedAt: 'user-a-updated',
      },
    })
    await loading

    assert.equal(reader.theme, 'blue')
    assert.equal(reader.fontSize, 18)
    assert.equal(reader.settingsSyncBaseUpdatedAt, 'user-b-base')
    assert.equal(reader.settingsUpdatedAt, 'user-b-updated')
    assert.equal(reader.settingsSyncError, 'user-b-error')
  })
})

test('a delayed preference load cannot mix user A into user B persisted preferences', { concurrency: false }, async () => {
  const request = deferred()
  const { preferences } = freshStores(1)

  await withAPI('get', () => request.promise, async () => {
    const loading = preferences.loadPreference('shelf')
    activateUser(2, 'next')
    preferences.resetPreferenceState()
    preferences.shelf = { view: 'list', layoutVersion: 2 }
    preferences.syncBaseUpdatedAt.shelf = 'user-b-base'
    preferences.syncError.shelf = 'user-b-error'

    request.resolve({
      data: {
        value: { view: 'grid', layoutVersion: 2 },
        updatedAt: 'user-a-updated',
      },
    })
    await loading

    assert.equal(preferences.shelf.view, 'list')
    assert.equal(preferences.syncBaseUpdatedAt.shelf, 'user-b-base')
    assert.equal(preferences.syncError.shelf, 'user-b-error')
  })
})

test('an older preference save cannot settle a newer same-key operation in another scope', { concurrency: false }, async () => {
  const requestA = deferred()
  const requestB = deferred()
  let requestCount = 0
  const { preferences } = freshStores(1)
  preferences.shelf = { view: 'grid', layoutVersion: 2 }
  preferences.syncBaseUpdatedAt.shelf = 'user-a-base'

  await withAPI('put', () => {
    requestCount += 1
    return requestCount === 1 ? requestA.promise : requestB.promise
  }, async () => {
    const savingA = preferences.savePreference('shelf')
    activateUser(2, 'next')
    preferences.resetPreferenceState()
    preferences.shelf = { view: 'list', layoutVersion: 2 }
    const savingB = preferences.savePreference('shelf')

    requestA.resolve({
      data: { value: { view: 'grid', layoutVersion: 2 }, updatedAt: 'user-a-updated' },
      headers: { 'x-openreader-setting-conflict': '1' },
    })
    await savingA
    assert.equal(preferences.shelf.view, 'list')
    assert.equal(preferences.syncing.shelf, true)
    assert.notEqual(preferences.syncBaseUpdatedAt.shelf, 'user-a-updated')

    requestB.resolve({
      data: { value: { view: 'list', layoutVersion: 2 }, updatedAt: 'user-b-updated' },
      headers: {},
    })
    await savingB
    assert.equal(preferences.shelf.view, 'list')
    assert.equal(preferences.syncBaseUpdatedAt.shelf, 'user-b-updated')
    assert.equal(preferences.syncing.shelf, false)
  })
})

test('a delayed progress response cannot create a local progress key in the next user scope', { concurrency: false }, async () => {
  const request = deferred()
  const { reader } = freshStores(1)

  await withAPI('get', () => request.promise, async () => {
    const loading = reader.loadProgress(101)
    activateUser(2, 'next')
    reader.ensureProgressScope()
    reader.applyProgress({
      bookId: 202,
      chapterIndex: 2,
      offset: 20,
      percent: 0.2,
      updatedAt: '2026-07-18T00:00:00Z',
    })

    request.resolve({
      data: {
        bookId: 101,
        chapterIndex: 8,
        offset: 88,
        percent: 0.8,
        updatedAt: '2026-07-18T01:00:00Z',
      },
    })
    const result = await loading

    assert.equal(result, null)
    assert.equal(reader.progressByBook[101], undefined)
    assert.equal(storage.getItem('openreader_chapter_progress@user:2@101'), null)
    assert.equal(reader.progressByBook[202]?.offset, 20)
  })
})

test('a delayed category response cannot replace the next user shelf state', { concurrency: false }, async () => {
  const request = deferred()
  const { bookshelf } = freshStores(1)

  await withAPI('get', () => request.promise, async () => {
    const loading = bookshelf.loadCategories({ force: true })
    activateUser(2, 'next')
    bookshelf.resetShelfState()
    bookshelf.categories = [{ id: 202, name: '用户 B 分组' }]

    request.resolve({ data: [{ id: 101, name: '用户 A 分组' }] })
    await loading

    assert.deepEqual(bookshelf.categories, [{ id: 202, name: '用户 B 分组' }])
  })
})

test('a delayed profile response cannot overwrite a later login profile', { concurrency: false }, async () => {
  const request = deferred()
  const tokenB = tokenFor(2, 'next')
  const { user } = freshStores(1)

  await withAPI('get', () => request.promise, async () => {
    const loading = user.loadMe()
    storage.setItem('openreader_token', tokenB)
    user.token = tokenB
    user.profile = { id: 2, username: 'user-b', canAccessStore: false }

    request.resolve({ data: { id: 1, username: 'user-a', canAccessStore: true } })
    await loading

    assert.deepEqual(user.profile, { id: 2, username: 'user-b', canAccessStore: false })
  })
})

test('callbacks from a superseded websocket cannot close, clear, reconnect, or dispatch into the new session', { concurrency: false }, async () => {
  class FakeWebSocket {
    static OPEN = 1
    static instances = []

    constructor(url) {
      this.url = url
      this.readyState = 0
      this.closeCalls = 0
      this.listeners = new Map()
      FakeWebSocket.instances.push(this)
    }

    addEventListener(type, listener) {
      const listeners = this.listeners.get(type) || []
      listeners.push(listener)
      this.listeners.set(type, listeners)
    }

    close() {
      this.closeCalls += 1
      this.readyState = 3
    }

    send() {}

    emit(type, value = {}) {
      for (const listener of this.listeners.get(type) || []) listener(value)
    }
  }

  globalThis.WebSocket = FakeWebSocket
  window.WebSocket = FakeWebSocket
  const { bookshelf } = freshStores(1)
  bookshelf.loadBooks = async () => []
  bookshelf.loadCategories = async () => []
  const sync = useSync()

  sync.connect()
  const socketA = FakeWebSocket.instances[0]
  socketA.readyState = FakeWebSocket.OPEN
  socketA.emit('open')
  sync.disconnect()

  activateUser(2, 'next')
  bookshelf.resetShelfState()
  bookshelf.categories = [{ id: 202, name: '用户 B 分组' }]
  sync.connect()
  const socketB = FakeWebSocket.instances[1]
  socketB.readyState = FakeWebSocket.OPEN
  socketB.emit('open')

  socketA.emit('error')
  socketA.emit('close')
  socketA.emit('message', {
    data: JSON.stringify({ type: 'category_update', payload: { id: 101, name: '用户 A 分组' } }),
  })
  for (const callback of [...timerCallbacks.values()]) callback()

  assert.deepEqual({
    socketBCloseCalls: socketB.closeCalls,
    connected: sync.connected.value,
    socketCount: FakeWebSocket.instances.length,
    categories: bookshelf.categories,
  }, {
    socketBCloseCalls: 0,
    connected: true,
    socketCount: 2,
    categories: [{ id: 202, name: '用户 B 分组' }],
  })

  sync.disconnect()
})
