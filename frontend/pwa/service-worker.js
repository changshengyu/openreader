// OpenReader offline-shell service worker. See
// docs/compat/pwa-service-worker-fixed-baseline-second-audit-p2-contract.md.
// frontend/pwa/build-service-worker.mjs writes this file to dist/sw.js and replaces
// the config placeholder with the build id and the build's precache list.
//
// The scope is deliberately small:
// - install precaches exactly one build; a new build waits until its pages are closed
//   or the version button sends SKIP_WAITING (user decision Q3-A);
// - navigations are network-first and fall back to the precached shell only when the
//   server is unreachable (a network error, or a 502/503/504 from a reverse proxy);
// - only exact precached build files are answered from the cache;
// - API, upload, WebDAV, WebSocket, authenticated, non-GET, ranged and cross-origin
//   requests are never handled, and nothing is written outside install, so no account
//   data ever reaches Cache Storage.
'use strict'

const CONFIG = __OPENREADER_SW_CONFIG__
const CACHE_PREFIX = 'openreader-'
const SHELL_CACHE = `${CACHE_PREFIX}shell-${CONFIG.buildId}`
const SHELL_DOCUMENT = '/index.html'
const PRECACHED = new Set(CONFIG.precache)
const SERVER_ROOTS = new Set(['api', 'ws', 'webdav', 'reader3', 'uploads'])
const UNREACHABLE_STATUSES = new Set([502, 503, 504])

function sameOrigin(path) {
  return new URL(path, self.location.origin).href
}

function isServerNamespace(url) {
  return SERVER_ROOTS.has(url.pathname.split('/')[1] || '')
}

self.addEventListener('install', event => {
  event.waitUntil(caches.open(SHELL_CACHE).then(cache => cache.addAll(
    CONFIG.precache.map(path => new Request(sameOrigin(path), { cache: 'no-cache' })),
  )))
})

self.addEventListener('activate', event => {
  event.waitUntil(caches.keys().then(names => Promise.all(names
    .filter(name => name.startsWith(CACHE_PREFIX) && name !== SHELL_CACHE)
    .map(name => caches.delete(name)))))
})

self.addEventListener('message', event => {
  if (event.data && event.data.type === 'SKIP_WAITING') self.skipWaiting()
})

async function precachedShell() {
  const cache = await caches.open(SHELL_CACHE)
  return cache.match(sameOrigin(SHELL_DOCUMENT), { ignoreVary: true })
}

async function networkFirstNavigation(request) {
  let response
  try {
    response = await fetch(request)
  } catch (error) {
    const shell = await precachedShell()
    if (shell) return shell
    throw error
  }
  if (UNREACHABLE_STATUSES.has(response.status)) {
    const shell = await precachedShell()
    if (shell) return shell
  }
  return response
}

async function precachedFile(request) {
  const cache = await caches.open(SHELL_CACHE)
  const cached = await cache.match(request.url, { ignoreVary: true })
  return cached || fetch(request)
}

self.addEventListener('fetch', event => {
  const request = event.request
  if (request.method !== 'GET') return
  const url = new URL(request.url)
  if (url.origin !== self.location.origin || isServerNamespace(url)) return
  if (request.headers.has('authorization') || request.headers.has('range')) return
  if (request.mode === 'navigate') {
    event.respondWith(networkFirstNavigation(request))
  } else if (!url.search && PRECACHED.has(url.pathname)) {
    event.respondWith(precachedFile(request))
  }
})
