import assert from 'node:assert/strict'
import test from 'node:test'

// PWA service worker contract W1/W6/W7/W17/S8 and user decisions Q2-A/Q3-A
// (docs/compat/pwa-service-worker-fixed-baseline-second-audit-p2-contract.md §10):
// register only production builds in secure contexts, treat any `nopwa` query as an
// escape hatch that unregisters and clears OpenReader caches, and make the version
// click activate the waiting worker (not the active one) before reloading once.

async function loadRegistration() {
  try {
    return await import('../src/utils/serviceWorker.js')
  } catch (error) {
    assert.fail(`frontend/src/utils/serviceWorker.js must provide the offline-shell registration: ${error.message}`)
  }
}

function createEventTarget() {
  const listeners = new Map()
  return {
    addEventListener(type, listener, options = {}) {
      listeners.set(type, [...(listeners.get(type) || []), { listener, once: Boolean(options?.once) }])
    },
    removeEventListener(type, listener) {
      listeners.set(type, (listeners.get(type) || []).filter(entry => entry.listener !== listener))
    },
    dispatch(type) {
      for (const entry of [...(listeners.get(type) || [])]) {
        if (entry.once) this.removeEventListener(type, entry.listener)
        entry.listener({ type })
      }
    },
  }
}

function fakeWorker(state) {
  const worker = { ...createEventTarget(), state, messages: [] }
  worker.postMessage = message => worker.messages.push(message)
  return worker
}

function fakeTimers() {
  let next = 1
  const pending = new Map()
  return {
    set: (callback, delay) => {
      const id = next++
      pending.set(id, { callback, delay })
      return id
    },
    clear: id => pending.delete(id),
    runAll() {
      for (const [id, timer] of [...pending]) {
        pending.delete(id)
        timer.callback()
      }
    },
    get size() {
      return pending.size
    },
  }
}

function fakeWindow({
  secure = true,
  supported = true,
  search = '',
  readyState = 'complete',
  registrations = [],
  cacheNames = [],
  register = async () => ({ scope: 'https://reader.test/' }),
} = {}) {
  const calls = { register: [], reloads: 0, deletedCaches: [] }
  const windowEvents = createEventTarget()
  const container = supported
    ? {
        ...createEventTarget(),
        async register(url, options) {
          calls.register.push({ url, options })
          return register(url, options)
        },
        async getRegistrations() {
          return registrations
        },
        async getRegistration() {
          return registrations[0]
        },
      }
    : undefined
  const win = {
    isSecureContext: secure,
    navigator: supported ? { serviceWorker: container } : {},
    location: {
      search,
      reload() {
        calls.reloads += 1
      },
    },
    document: { readyState },
    addEventListener: windowEvents.addEventListener,
    removeEventListener: windowEvents.removeEventListener,
    caches: {
      async keys() {
        return cacheNames
      },
      async delete(name) {
        calls.deletedCaches.push(name)
        return true
      },
    },
  }
  return { win, calls, container, fireLoad: () => windowEvents.dispatch('load') }
}

const tick = () => new Promise(resolve => setTimeout(resolve, 0))

test('treats any nopwa query parameter as the escape hatch, case-insensitively', async () => {
  const { hasNoPwaFlag } = await loadRegistration()
  for (const search of ['?nopwa', '?nopwa=', '?nopwa=1', '?NOPWA=true', '?workspace=sources&nopwa=0']) {
    assert.equal(hasNoPwaFlag(search), true, search)
  }
  for (const search of ['', '?', '?workspace=sources', '?nopwax=1', '?pwa=0']) {
    assert.equal(hasNoPwaFlag(search), false, search)
  }
})

test('registers only production builds in secure contexts that support service workers', async () => {
  const { shouldRegisterServiceWorker } = await loadRegistration()
  const base = { isProduction: true, isSecureContext: true, hasServiceWorker: true, search: '' }
  assert.equal(shouldRegisterServiceWorker(base), true)
  assert.equal(shouldRegisterServiceWorker({ ...base, isProduction: false }), false)
  assert.equal(shouldRegisterServiceWorker({ ...base, isSecureContext: false }), false)
  assert.equal(shouldRegisterServiceWorker({ ...base, hasServiceWorker: false }), false)
  assert.equal(shouldRegisterServiceWorker({ ...base, search: '?nopwa=1' }), false)
})

test('registers /sw.js once at the root scope after the page has loaded', async () => {
  const { setupServiceWorker } = await loadRegistration()
  const { win, calls, fireLoad } = fakeWindow({ readyState: 'loading' })
  const pending = setupServiceWorker({ win, isProduction: true })
  await tick()
  assert.equal(calls.register.length, 0, 'registration waits for the load event like upstream register-service-worker')
  fireLoad()
  const result = await pending
  assert.equal(result.status, 'registered')
  assert.deepEqual(calls.register, [{ url: '/sw.js', options: { scope: '/', updateViaCache: 'none' } }])
})

test('skips registration in development, insecure contexts and browsers without service workers', async () => {
  const { setupServiceWorker } = await loadRegistration()
  for (const [label, options, isProduction] of [
    ['development', {}, false],
    ['insecure HTTP', { secure: false }, true],
    ['unsupported', { supported: false }, true],
  ]) {
    const { win, calls } = fakeWindow(options)
    const result = await setupServiceWorker({ win, isProduction })
    assert.equal(result.status, 'skipped', label)
    assert.equal(calls.register.length, 0, label)
  }
})

test('nopwa unregisters existing workers and deletes only OpenReader caches', async () => {
  const { setupServiceWorker } = await loadRegistration()
  const unregistered = []
  const registrations = ['first', 'second'].map(name => ({
    async unregister() {
      unregistered.push(name)
      return true
    },
  }))
  const { win, calls } = fakeWindow({
    search: '?nopwa',
    registrations,
    cacheNames: ['openreader-shell-0123456789abcdef', 'openreader-legacy', 'third-party-cache'],
  })
  const result = await setupServiceWorker({ win, isProduction: true })
  assert.equal(result.status, 'disabled')
  assert.deepEqual(unregistered, ['first', 'second'])
  assert.deepEqual([...calls.deletedCaches].sort(), ['openreader-legacy', 'openreader-shell-0123456789abcdef'])
  assert.equal(calls.register.length, 0)
})

test('a blocked or failing registration settles without throwing', async () => {
  const { setupServiceWorker } = await loadRegistration()
  const blocked = fakeWindow({ register: async () => undefined })
  assert.equal((await setupServiceWorker({ win: blocked.win, isProduction: true })).status, 'blocked')

  const failing = fakeWindow({ register: async () => { throw new Error('SecurityError') } })
  const result = await setupServiceWorker({ win: failing.win, isProduction: true })
  assert.equal(result.status, 'failed')
})

test('version click without service worker support or registration keeps the build-info toast only', async () => {
  const { forceServiceWorkerUpdate } = await loadRegistration()
  const unsupported = fakeWindow({ supported: false })
  assert.equal((await forceServiceWorkerUpdate({ win: unsupported.win })).status, 'unsupported')
  assert.equal(unsupported.calls.reloads, 0)

  const unregistered = fakeWindow({ registrations: [] })
  assert.equal((await forceServiceWorkerUpdate({ win: unregistered.win })).status, 'unregistered')
  assert.equal(unregistered.calls.reloads, 0)
})

test('version click tells the waiting worker, not the active one, to skip waiting and reloads once', async () => {
  const { forceServiceWorkerUpdate } = await loadRegistration()
  const active = fakeWorker('activated')
  const waiting = fakeWorker('installed')
  let updates = 0
  const registration = {
    active,
    waiting,
    installing: null,
    async update() {
      updates += 1
      return registration
    },
  }
  const timers = fakeTimers()
  const { win, calls, container } = fakeWindow({ registrations: [registration] })

  const result = await forceServiceWorkerUpdate({ win, setTimer: timers.set, clearTimer: timers.clear })
  assert.equal(result.status, 'activating')
  assert.equal(updates, 1, 'the click checks for a newer worker first')
  assert.deepEqual(waiting.messages, [{ type: 'SKIP_WAITING' }])
  assert.deepEqual(active.messages, [], 'upstream posted SKIP_WAITING to the active worker, which never activated anything')
  assert.equal(calls.reloads, 0, 'the reload waits for the new worker to take control')

  container.dispatch('controllerchange')
  container.dispatch('controllerchange')
  timers.runAll()
  assert.equal(calls.reloads, 1, 'exactly one reload')
})

test('version click waits for an update that is still installing', async () => {
  const { forceServiceWorkerUpdate } = await loadRegistration()
  const active = fakeWorker('activated')
  const installing = fakeWorker('installing')
  const registration = {
    active,
    waiting: null,
    installing: null,
    async update() {
      registration.installing = installing
      return registration
    },
  }
  const timers = fakeTimers()
  const { win, calls, container } = fakeWindow({ registrations: [registration] })

  const pending = forceServiceWorkerUpdate({ win, setTimer: timers.set, clearTimer: timers.clear })
  await tick()
  assert.deepEqual(installing.messages, [], 'an installing worker cannot skip waiting yet')

  installing.state = 'installed'
  registration.installing = null
  registration.waiting = installing
  installing.dispatch('statechange')
  assert.equal((await pending).status, 'activating')
  assert.deepEqual(installing.messages, [{ type: 'SKIP_WAITING' }])

  container.dispatch('controllerchange')
  assert.equal(calls.reloads, 1)
})

test('version click with no newer worker still forces one reload, and a lost controllerchange falls back to one', async () => {
  const { forceServiceWorkerUpdate } = await loadRegistration()
  const current = fakeWorker('activated')
  const upToDate = {
    active: current,
    waiting: null,
    installing: null,
    async update() {
      return upToDate
    },
  }
  const first = fakeWindow({ registrations: [upToDate] })
  assert.equal((await forceServiceWorkerUpdate({ win: first.win })).status, 'reloaded')
  assert.equal(first.calls.reloads, 1)
  assert.deepEqual(current.messages, [])

  const waiting = fakeWorker('installed')
  const stalled = {
    active: fakeWorker('activated'),
    waiting,
    installing: null,
    async update() {
      return stalled
    },
  }
  const timers = fakeTimers()
  const second = fakeWindow({ registrations: [stalled] })
  await forceServiceWorkerUpdate({ win: second.win, setTimer: timers.set, clearTimer: timers.clear })
  assert.equal(second.calls.reloads, 0)
  timers.runAll()
  assert.equal(second.calls.reloads, 1, 'a missing controllerchange must not leave the page stale')
})
