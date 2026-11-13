import assert from 'node:assert/strict'
import test from 'node:test'
import vm from 'node:vm'

// PWA service worker contract S1/S4/W3/W9–W14 (docs/compat/pwa-service-worker-fixed-baseline-second-audit-p2-contract.md):
// the generated worker runs here against in-memory Cache Storage and network fakes.
// It precaches one build, serves navigations network-first with the precached shell
// as the offline fallback, serves only exact precached files from cache, and never
// handles or stores API, authenticated, non-GET, ranged or cross-origin traffic.

const ORIGIN = 'https://reader.test'
const BUILD_ID = 'b1d0000000000001'
const SHELL_CACHE = `openreader-shell-${BUILD_ID}`
const SHELL_HTML = '<!doctype html><title>OpenReader shell</title>'
const PRECACHE = [
  '/assets/Reader-BBB.js',
  '/assets/index-AAA.js',
  '/bg/%E5%B1%B1%E6%B0%B4%E7%94%BB.jpg',
  '/index.html',
  '/manifest.webmanifest',
]

async function loadBuilder() {
  try {
    return await import('../pwa/build-service-worker.mjs')
  } catch (error) {
    assert.fail(`frontend/pwa/build-service-worker.mjs must generate the offline-shell service worker: ${error.message}`)
  }
}

function urlOf(request) {
  return new URL(typeof request === 'string' ? request : request.url, ORIGIN).href
}

class FakeCache {
  constructor(storage) {
    this.storage = storage
    this.entries = new Map()
  }

  async match(request) {
    const entry = this.entries.get(urlOf(request))
    return entry ? entry.clone() : undefined
  }

  async put(request, response) {
    this.storage.writes.push(urlOf(request))
    this.entries.set(urlOf(request), response.clone())
  }

  async addAll(requests) {
    const fetched = []
    for (const request of requests) {
      const response = await this.storage.network(request)
      if (!response.ok) throw new TypeError(`addAll: ${urlOf(request)} returned ${response.status}`)
      fetched.push([request, response])
    }
    for (const [request, response] of fetched) await this.put(request, response)
  }

  async keys() {
    return [...this.entries.keys()].map(url => new Request(url))
  }

  async delete(request) {
    return this.entries.delete(urlOf(request))
  }
}

class FakeCacheStorage {
  constructor(network) {
    this.network = network
    this.named = new Map()
    this.writes = []
  }

  async open(name) {
    if (!this.named.has(name)) this.named.set(name, new FakeCache(this))
    return this.named.get(name)
  }

  async has(name) {
    return this.named.has(name)
  }

  async keys() {
    return [...this.named.keys()]
  }

  async delete(name) {
    return this.named.delete(name)
  }

  async match(request) {
    for (const cache of this.named.values()) {
      const hit = await cache.match(request)
      if (hit) return hit
    }
    return undefined
  }
}

function createNetwork() {
  const routes = new Map()
  const calls = []
  const network = async request => {
    const url = urlOf(request)
    calls.push({ url, cache: request.cache, mode: request.mode })
    if (network.offline) throw new TypeError('Failed to fetch')
    const route = routes.get(url)
    if (!route) return new Response('{"error":{"code":"NOT_FOUND","message":"route not found"}}', { status: 404 })
    return new Response(route.body, { status: route.status || 200, headers: route.headers || {} })
  }
  network.offline = false
  network.routes = routes
  network.calls = calls
  network.route = (path, route) => routes.set(new URL(path, ORIGIN).href, route)
  for (const path of PRECACHE) {
    network.route(path, path === '/index.html'
      ? { body: SHELL_HTML, headers: { 'content-type': 'text/html; charset=utf-8' } }
      : { body: `asset:${path}` })
  }
  return network
}

function extendableEvent(extra = {}) {
  const pending = []
  return {
    event: { ...extra, waitUntil: promise => pending.push(Promise.resolve(promise)) },
    settled: () => Promise.all(pending),
  }
}

async function startWorker() {
  const builder = await loadBuilder()
  const source = await builder.renderServiceWorker({ buildId: BUILD_ID, precache: PRECACHE })
  const network = createNetwork()
  const storage = new FakeCacheStorage(network)
  const listeners = new Map()
  const self = {
    location: new URL(`${ORIGIN}/sw.js`),
    skipWaitingCalls: 0,
    addEventListener(type, listener) {
      listeners.set(type, [...(listeners.get(type) || []), listener])
    },
    async skipWaiting() {
      self.skipWaitingCalls += 1
    },
  }
  const context = vm.createContext({ self, caches: storage, fetch: network, Request, Response, Headers, URL, console })
  vm.runInContext(source, context, { filename: 'sw.js' })

  function emit(type, event) {
    for (const listener of listeners.get(type) || []) listener(event)
  }

  return {
    self,
    network,
    storage,
    async install() {
      const { event, settled } = extendableEvent()
      emit('install', event)
      await settled()
    },
    async activate() {
      const { event, settled } = extendableEvent()
      emit('activate', event)
      await settled()
    },
    async fetch(request) {
      let responded = null
      const { event, settled } = extendableEvent({
        request,
        respondWith(promise) {
          assert.equal(responded, null, 'respondWith must be called at most once')
          responded = Promise.resolve(promise)
        },
      })
      emit('fetch', event)
      await settled()
      return responded ? { handled: true, response: await responded } : { handled: false }
    },
    message(data) {
      emit('message', { data })
    },
  }
}

function pageRequest(path, init = {}) {
  return new Request(new URL(path, ORIGIN).href, init)
}

// Request cannot be constructed with mode "navigate"; mimic the fields a worker reads.
function navigationRequest(path) {
  return {
    url: new URL(path, ORIGIN).href,
    method: 'GET',
    mode: 'navigate',
    destination: 'document',
    cache: 'default',
    headers: new Headers({ accept: 'text/html' }),
  }
}

test('installs every precache entry into a build-scoped openreader cache without skipping waiting', async () => {
  const worker = await startWorker()
  await worker.install()

  assert.deepEqual(await worker.storage.keys(), [SHELL_CACHE])
  const shell = await worker.storage.open(SHELL_CACHE)
  for (const path of PRECACHE) {
    assert.ok(await shell.match(`${ORIGIN}${path}`), `${path} must be precached`)
  }
  assert.equal(worker.network.calls.length, PRECACHE.length)
  assert.ok(worker.network.calls.every(call => new URL(call.url).origin === ORIGIN), 'precache must stay same-origin')
  assert.ok(worker.network.calls.every(call => call.cache === 'no-cache'), 'precache must revalidate HTTP-cached copies')
  assert.equal(worker.self.skipWaitingCalls, 0, 'a new worker waits for its pages to close (Q3-A)')
})

test('a failed precache entry fails the install so the previous worker keeps control', async () => {
  const worker = await startWorker()
  worker.network.route('/assets/Reader-BBB.js', { status: 404, body: 'missing' })
  await assert.rejects(worker.install())
  const shell = await worker.storage.open(SHELL_CACHE)
  assert.equal(await shell.match(`${ORIGIN}/index.html`), undefined, 'a partial precache must not be published')
})

test('activation deletes stale openreader caches and leaves other caches alone', async () => {
  const worker = await startWorker()
  await worker.storage.open('openreader-shell-0ld0000000000000')
  await worker.storage.open('openreader-legacy')
  await worker.storage.open('other-app-cache')
  await worker.install()
  await worker.activate()
  assert.deepEqual((await worker.storage.keys()).sort(), [SHELL_CACHE, 'other-app-cache'].sort())
})

test('navigations stay network-first and never write to Cache Storage', async () => {
  const worker = await startWorker()
  await worker.install()
  const writesAfterInstall = worker.storage.writes.length

  worker.network.route('/books/42/read', { body: '<!doctype html><title>fresh</title>', headers: { 'content-type': 'text/html' } })
  const fresh = await worker.fetch(navigationRequest('/books/42/read'))
  assert.equal(fresh.handled, true)
  assert.equal(await fresh.response.text(), '<!doctype html><title>fresh</title>')

  for (const status of [401, 404, 500]) {
    worker.network.route('/books/7', { status, body: `status ${status}` })
    const result = await worker.fetch(navigationRequest('/books/7'))
    assert.equal(result.response.status, status, `a ${status} navigation passes through unchanged`)
  }
  assert.equal(worker.storage.writes.length, writesAfterInstall, 'navigation responses are never cached')
})

test('offline and gateway-failed navigations fall back to the precached shell', async () => {
  const worker = await startWorker()
  await worker.install()

  worker.network.offline = true
  for (const path of ['/', '/books/42/read', '/login', '/settings?panel=reader', '/reader/remote/session-token']) {
    const result = await worker.fetch(navigationRequest(path))
    assert.equal(result.handled, true, path)
    assert.equal(await result.response.text(), SHELL_HTML, `${path} must open the offline shell`)
  }

  worker.network.offline = false
  for (const status of [502, 503, 504]) {
    worker.network.route('/books/9', { status, body: `gateway ${status}` })
    const result = await worker.fetch(navigationRequest('/books/9'))
    assert.equal(await result.response.text(), SHELL_HTML, `a ${status} from a reverse proxy means the server is unreachable`)
  }
})

test('serves exact precached build files from the shell cache and leaves other files to the browser', async () => {
  const worker = await startWorker()
  await worker.install()
  worker.network.calls.length = 0

  const entry = await worker.fetch(pageRequest('/assets/index-AAA.js'))
  assert.equal(entry.handled, true)
  assert.equal(await entry.response.text(), 'asset:/assets/index-AAA.js')
  const background = await worker.fetch(pageRequest('/bg/%E5%B1%B1%E6%B0%B4%E7%94%BB.jpg'))
  assert.equal(await background.response.text(), 'asset:/bg/%E5%B1%B1%E6%B0%B4%E7%94%BB.jpg')
  assert.equal(worker.network.calls.length, 0, 'precached files must not touch the network')

  assert.equal((await worker.fetch(pageRequest('/assets/index-NEW.js'))).handled, false, 'a newer build file is left to the browser')
  assert.equal((await worker.fetch(pageRequest('/index.html?refresh=1'))).handled, false, 'query variants are not precache hits')
})

test('never handles API, authenticated, non-GET, ranged or cross-origin requests', async () => {
  const worker = await startWorker()
  await worker.install()
  const writesAfterInstall = worker.storage.writes.length

  for (const request of [
    pageRequest('/api/books', { headers: { Authorization: 'Bearer secret-token' } }),
    pageRequest('/api/health'),
    pageRequest('/api/cover/opaque-capability'),
    pageRequest('/api/chapter-image/opaque-capability'),
    pageRequest('/api/books', { method: 'POST', body: '{}' }),
    pageRequest('/uploads/user-1/cover.png'),
    pageRequest('/reader3/getBookshelf'),
    pageRequest('/webdav/'),
    pageRequest('/ws/sync'),
    pageRequest('/assets/index-AAA.js', { headers: { Range: 'bytes=0-1' } }),
    pageRequest('/assets/index-AAA.js', { headers: { Authorization: 'Bearer secret-token' } }),
    pageRequest('/assets/index-AAA.js', { method: 'HEAD' }),
    new Request('https://cdn.example/cover.png', { mode: 'no-cors' }),
  ]) {
    const result = await worker.fetch(request)
    assert.equal(result.handled, false, `${request.method} ${request.url} must be left to the browser`)
  }
  for (const path of ['/api/health', '/uploads/user-1/cover.png', '/webdav/', '/reader3/getBookshelf']) {
    assert.equal((await worker.fetch(navigationRequest(path))).handled, false, `navigation to ${path} must not get the shell`)
  }
  assert.equal(worker.storage.writes.length, writesAfterInstall, 'nothing outside the install step is ever written')
})

test('only an explicit SKIP_WAITING message activates the waiting worker', async () => {
  const worker = await startWorker()
  worker.message({ type: 'CLEAR_HOME_CACHE' })
  worker.message(null)
  worker.message('SKIP_WAITING')
  assert.equal(worker.self.skipWaitingCalls, 0)
  worker.message({ type: 'SKIP_WAITING' })
  assert.equal(worker.self.skipWaitingCalls, 1)
})
