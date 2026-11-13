// Offline-shell service worker registration and the version-click update.
// See docs/compat/pwa-service-worker-fixed-baseline-second-audit-p2-contract.md §10:
// register only production builds in secure contexts (upstream registerServiceWorker.js),
// treat any `nopwa` query parameter as an escape hatch, and let a new worker take over
// only when the version button asks for it (Q2-A, Q3-A).

export const SERVICE_WORKER_URL = '/sw.js'
export const SERVICE_WORKER_SCOPE = '/'
export const OPENREADER_CACHE_PREFIX = 'openreader-'
const UPDATE_TIMEOUT_MS = 30_000

// Upstream only honoured a non-empty value (`?nopwa=1`); any `nopwa` key disables here.
export function hasNoPwaFlag(search = '') {
  for (const key of new URLSearchParams(String(search || '')).keys()) {
    if (key.toLowerCase() === 'nopwa') return true
  }
  return false
}

export function shouldRegisterServiceWorker({ isProduction, isSecureContext, hasServiceWorker, search }) {
  return Boolean(isProduction && isSecureContext && hasServiceWorker) && !hasNoPwaFlag(search)
}

function serviceWorkerContainer(win) {
  return win?.navigator && 'serviceWorker' in win.navigator ? win.navigator.serviceWorker : null
}

function waitForWindowLoad(win) {
  if (win.document?.readyState === 'complete') return Promise.resolve()
  return new Promise(resolve => win.addEventListener('load', () => resolve(), { once: true }))
}

export async function disableServiceWorkers(win = globalThis.window) {
  const container = serviceWorkerContainer(win)
  const registrations = container
    ? await Promise.resolve().then(() => container.getRegistrations()).catch(() => [])
    : []
  await Promise.all(registrations.map(registration => Promise.resolve()
    .then(() => registration.unregister())
    .catch(() => false)))
  const storage = win?.caches
  if (!storage) return
  const names = await Promise.resolve().then(() => storage.keys()).catch(() => [])
  await Promise.all(names
    .filter(name => name.startsWith(OPENREADER_CACHE_PREFIX))
    .map(name => Promise.resolve().then(() => storage.delete(name)).catch(() => false)))
}

export async function setupServiceWorker({ win = globalThis.window, isProduction = false } = {}) {
  const container = serviceWorkerContainer(win)
  const search = win?.location?.search || ''
  if (container && hasNoPwaFlag(search)) {
    await disableServiceWorkers(win)
    return { status: 'disabled' }
  }
  if (!shouldRegisterServiceWorker({
    isProduction,
    isSecureContext: Boolean(win?.isSecureContext),
    hasServiceWorker: Boolean(container),
    search,
  })) {
    return { status: 'skipped' }
  }
  await waitForWindowLoad(win)
  try {
    const registration = await container.register(SERVICE_WORKER_URL, {
      scope: SERVICE_WORKER_SCOPE,
      updateViaCache: 'none',
    })
    // Playwright's serviceWorkers: 'block' resolves registration with undefined.
    return registration ? { status: 'registered', registration } : { status: 'blocked' }
  } catch (error) {
    return { status: 'failed', error }
  }
}

function waitForInstalledWorker(registration, { timeoutMs, setTimer, clearTimer }) {
  if (registration.waiting) return Promise.resolve(registration.waiting)
  const installing = registration.installing
  if (!installing) return Promise.resolve(null)
  return new Promise(resolve => {
    let timer = null
    const settle = () => {
      clearTimer(timer)
      installing.removeEventListener('statechange', onStateChange)
      resolve(registration.waiting || null)
    }
    function onStateChange() {
      if (installing.state !== 'installing') settle()
    }
    timer = setTimer(settle, timeoutMs)
    installing.addEventListener('statechange', onStateChange)
  })
}

// Upstream Index.vue#updateForce posted SKIP_WAITING to the active worker, which never
// promoted the waiting one. Here the waiting worker is told to take over and the page
// reloads once when it does; with no newer worker the click still forces one reload.
export async function forceServiceWorkerUpdate({
  win = globalThis.window,
  timeoutMs = UPDATE_TIMEOUT_MS,
  setTimer = (callback, delay) => setTimeout(callback, delay),
  clearTimer = id => clearTimeout(id),
} = {}) {
  const container = serviceWorkerContainer(win)
  if (!container) return { status: 'unsupported' }
  const registration = await Promise.resolve().then(() => container.getRegistration()).catch(() => null)
  if (!registration) return { status: 'unregistered' }

  let reloaded = false
  const reload = () => {
    if (reloaded) return
    reloaded = true
    win.location.reload()
  }
  // An unreachable server only means there is nothing newer to install.
  await Promise.resolve().then(() => registration.update()).catch(() => {})
  const waiting = await waitForInstalledWorker(registration, { timeoutMs, setTimer, clearTimer })
  if (!waiting) {
    reload()
    return { status: 'reloaded' }
  }
  container.addEventListener('controllerchange', reload, { once: true })
  setTimer(reload, timeoutMs)
  waiting.postMessage({ type: 'SKIP_WAITING' })
  return { status: 'activating' }
}
