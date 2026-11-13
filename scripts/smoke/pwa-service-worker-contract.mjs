#!/usr/bin/env node

// Real-browser contract for the offline-shell service worker
// (docs/compat/pwa-service-worker-fixed-baseline-second-audit-p2-contract.md §7, E1–E7).
// Builds the Go server, serves a private copy of frontend/dist on 127.0.0.1 (a secure
// context), and checks registration, precache contents, the offline shell with the
// server stopped, account isolation, the A→B release path, the version-click force
// update, the nopwa escape hatch and the insecure-context fallback.
// Run `cd frontend && npm run build` first; Playwright must run outside the sandbox.

import { execFile, spawn } from 'node:child_process'
import { cp, mkdtemp, readdir, readFile, rename, rm, writeFile } from 'node:fs/promises'
import { existsSync } from 'node:fs'
import { createServer } from 'node:http'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { promisify } from 'node:util'

import { openSmokeBrowser } from './playwright-runtime.mjs'

const execFileAsync = promisify(execFile)
const rootDir = join(dirname(fileURLToPath(import.meta.url)), '..', '..')
const backendDir = join(rootDir, 'backend')
const builtDist = join(rootDir, 'frontend', 'dist')
const builderURL = pathToFileURL(join(rootDir, 'frontend', 'pwa', 'build-service-worker.mjs')).href
const VIEWPORTS = [
  { width: 1440, height: 900 },
  { width: 390, height: 844 },
  { width: 360, height: 800 },
]

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function reserveLocalPort() {
  const server = createServer()
  await new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', resolve)
  })
  const { port } = server.address()
  await new Promise(resolve => server.close(resolve))
  return port
}

async function stopProcess(child) {
  if (!child || child.exitCode !== null) return
  const exited = new Promise(resolve => child.once('exit', resolve))
  child.kill('SIGTERM')
  await Promise.race([exited, new Promise(resolve => setTimeout(resolve, 5_000))])
  if (child.exitCode === null) {
    child.kill('SIGKILL')
    await Promise.race([exited, new Promise(resolve => setTimeout(resolve, 2_000))])
  }
}

async function createServerHarness() {
  assert(existsSync(join(builtDist, 'index.html')), 'frontend/dist is missing; run `cd frontend && npm run build` first')
  assert(existsSync(join(builtDist, 'sw.js')), 'frontend/dist/sw.js is missing: the build must generate the offline-shell worker (W1/W2)')
  const tempRoot = await mkdtemp(join(tmpdir(), 'openreader-pwa-contract-'))
  const publicDir = join(tempRoot, 'public')
  await cp(builtDist, publicDir, { recursive: true })
  const binary = join(tempRoot, 'openreader')
  await execFileAsync('go', ['build', '-o', binary, '.'], { cwd: backendDir, env: process.env, maxBuffer: 4 * 1024 * 1024 })
  const port = await reserveLocalPort()
  const root = `http://127.0.0.1:${port}`
  let child = null
  let output = ''

  async function start() {
    child = spawn(binary, [], {
      cwd: backendDir,
      env: {
        ...process.env,
        OPENREADER_ADDR: `127.0.0.1:${port}`,
        OPENREADER_DATA_DIR: join(tempRoot, 'data'),
        OPENREADER_CACHE_DIR: join(tempRoot, 'cache'),
        OPENREADER_LIBRARY_DIR: join(tempRoot, 'library'),
        OPENREADER_LOCAL_STORE_DIR: join(tempRoot, 'library', 'localStore'),
        OPENREADER_DB: join(tempRoot, 'data', 'openreader.db'),
        OPENREADER_PUBLIC_DIR: publicDir,
        OPENREADER_JWT_SECRET: 'pwa-service-worker-contract-secret',
        OPENREADER_CORS_ORIGIN: root,
        OPENREADER_CHECK_INTERVAL: '24h',
      },
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    child.stdout.on('data', chunk => { output += chunk.toString() })
    child.stderr.on('data', chunk => { output += chunk.toString() })
    const deadline = Date.now() + 60_000
    while (Date.now() < deadline) {
      try {
        if ((await fetch(`${root}/api/health`)).ok) return
      } catch {
        // still starting
      }
      await new Promise(resolve => setTimeout(resolve, 300))
    }
    throw new Error(`OpenReader PWA contract server did not start\n${output}`)
  }

  await start()
  return {
    root,
    port,
    publicDir,
    start,
    stop: () => stopProcess(child),
    close: async () => {
      await stopProcess(child)
      await rm(tempRoot, { recursive: true, force: true })
    },
  }
}

async function api(root, path, { token = '', method = 'GET', body } = {}) {
  const response = await fetch(`${root}/api${path}`, {
    method,
    headers: {
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const text = await response.text()
  if (!response.ok) throw new Error(`${method} ${path} failed with ${response.status}: ${text}`)
  return text ? JSON.parse(text) : null
}

async function seedAccount(server, name) {
  const registered = await api(server.root, '/auth/register', {
    method: 'POST',
    body: { username: name, password: 'pwa-contract-password' },
  })
  assert(registered?.token, `${name}: registration returned no token`)
  const book = await api(server.root, '/books', {
    token: registered.token,
    method: 'POST',
    body: { title: `离线书架 ${name}`, author: 'PWA 合同', sourceId: 0, url: `https://book.example/${name}`, coverUrl: '' },
  })
  return { token: registered.token, book }
}

async function readWorkerConfig(publicDir) {
  const source = await readFile(join(publicDir, 'sw.js'), 'utf8')
  const match = source.match(/const CONFIG = (\{[^\n]*\})/)
  assert(match, 'sw.js does not embed its precache config')
  return JSON.parse(match[1])
}

function shelfRow(page, title) {
  return page.locator('.shelf-page .book-row').filter({ has: page.getByText(title, { exact: true }) })
}

function watchPage(page, label) {
  const failures = []
  page.on('pageerror', error => failures.push(`${label} pageerror: ${error.message}`))
  return failures
}

async function cacheSnapshot(page) {
  return page.evaluate(async () => {
    const names = await caches.keys()
    const entries = {}
    for (const name of names) {
      entries[name] = (await (await caches.open(name)).keys()).map(request => new URL(request.url).pathname)
    }
    return { names, entries }
  })
}

async function waitForController(page, label) {
  const ready = await page.evaluate(() => Promise.race([
    navigator.serviceWorker.ready.then(registration => registration.active?.state || 'none'),
    new Promise(resolve => setTimeout(() => resolve('timeout'), 20_000)),
  ]))
  assert(ready !== 'timeout', `${label}: the service worker never became ready (W1)`)
  if (!(await page.evaluate(() => Boolean(navigator.serviceWorker.controller)))) {
    await page.reload({ waitUntil: 'domcontentloaded' })
  }
  assert(await page.evaluate(() => Boolean(navigator.serviceWorker.controller)), `${label}: the reloaded page is not controlled`)
}

async function assertShellCache(page, config, label) {
  const snapshot = await cacheSnapshot(page)
  const shell = `openreader-shell-${config.buildId}`
  assert(snapshot.names.includes(shell), `${label}: missing ${shell} in ${snapshot.names.join(', ')}`)
  const cached = new Set(snapshot.entries[shell])
  for (const path of config.precache) assert(cached.has(path), `${label}: ${path} was not precached`)
  for (const [name, paths] of Object.entries(snapshot.entries)) {
    assert(name.startsWith('openreader-shell-'), `${label}: unexpected cache ${name}`)
    assert(!paths.some(path => path.startsWith('/api/')), `${label}: ${name} stored an API response`)
  }
  return snapshot
}

async function assertNoHorizontalOverflow(page, label) {
  const metrics = await page.evaluate(() => ({ client: document.body.clientWidth, scroll: document.body.scrollWidth }))
  assert(metrics.scroll <= metrics.client + 1, `${label}: body overflowed horizontally (${metrics.scroll} > ${metrics.client})`)
}

async function runViewport(browser, server, config, viewport) {
  const label = `${viewport.width}x${viewport.height}`
  const accountA = await seedAccount(server, `pwaa${viewport.width}${viewport.height}`)
  const accountB = await seedAccount(server, `pwab${viewport.width}${viewport.height}`)
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 750,
    hasTouch: viewport.width <= 750,
    serviceWorkers: 'allow',
  })
  const page = await context.newPage()
  const failures = watchPage(page, label)
  try {
    await page.addInitScript(token => {
      if (!sessionStorage.getItem('pwa-contract-seeded')) {
        localStorage.setItem('openreader_token', token)
        sessionStorage.setItem('pwa-contract-seeded', '1')
      }
    }, accountA.token)
    await page.goto(server.root, { waitUntil: 'domcontentloaded' })
    await shelfRow(page, accountA.book.title).waitFor({ state: 'visible', timeout: 15_000 })

    // E1: registered, controlling, exactly the build's precache in one shell cache.
    await waitForController(page, label)
    await shelfRow(page, accountA.book.title).waitFor({ state: 'visible', timeout: 15_000 })
    await assertShellCache(page, config, `${label} E1`)

    // E2: server stopped and browser offline → precached shell, cached shelf, no login prompt.
    await server.stop()
    await context.setOffline(true)
    try {
      await page.reload({ waitUntil: 'domcontentloaded' })
      await page.locator('.app-offline').waitFor({ state: 'visible', timeout: 15_000 })
      await shelfRow(page, accountA.book.title).waitFor({ state: 'visible', timeout: 15_000 })
      assert(await page.locator('.auth-dialog').isHidden(), `${label} E2: offline launch opened the login dialog`)
      assert(
        await page.evaluate(token => localStorage.getItem('openreader_token') === token, accountA.token),
        `${label} E2: offline launch dropped the session token`,
      )
      await assertNoHorizontalOverflow(page, `${label} E2 shelf`)

      await page.goto(`${server.root}/books/${accountA.book.id}/read`, { waitUntil: 'domcontentloaded' })
      await page.locator('.reader-shell').waitFor({ state: 'attached', timeout: 15_000 })
      assert(
        !failures.some(failure => /dynamically imported module|Importing a module script failed/i.test(failure)),
        `${label} E2: a lazy route chunk was not served offline:\n${failures.join('\n')}`,
      )
    } finally {
      await context.setOffline(false)
      await server.start()
    }

    // E4: switching accounts leaves no account data in Cache Storage.
    await page.goto(server.root, { waitUntil: 'domcontentloaded' })
    await shelfRow(page, accountA.book.title).waitFor({ state: 'visible', timeout: 15_000 })
    await page.evaluate(token => localStorage.setItem('openreader_token', token), accountB.token)
    await page.reload({ waitUntil: 'domcontentloaded' })
    await shelfRow(page, accountB.book.title).waitFor({ state: 'visible', timeout: 15_000 })
    assert(await shelfRow(page, accountA.book.title).count() === 0, `${label} E4: account A's book leaked into account B's shelf`)
    await assertShellCache(page, config, `${label} E4`)

    const unexpected = failures.filter(failure => !/Failed to fetch|NetworkError|ERR_INTERNET_DISCONNECTED|ERR_CONNECTION_REFUSED|Network Error/i.test(failure))
    assert(unexpected.length === 0, `${label}: browser failures:\n${unexpected.join('\n')}`)
  } finally {
    await context.close()
  }
}

// Simulate a real rebuild: every hashed asset gets a new name (Vite never reuses a
// hashed name for different bytes), references are rewritten, and the worker is
// regenerated, so build A's files disappear from the server.
async function publishBuildB(publicDir) {
  const assetDir = join(publicDir, 'assets')
  const assets = await readdir(assetDir)
  const readerChunk = assets.find(name => /^Reader-[\w-]+\.js$/.test(name))
  assert(readerChunk, 'build A has no Reader route chunk')
  const renamed = new Map(assets.map(name => [name, name.replace(/(\.[A-Za-z0-9]+)$/, '-pwab$1')]))
  const rewrite = text => {
    let next = text
    for (const [from, to] of renamed) next = next.split(from).join(to)
    return next
  }
  for (const name of assets) {
    const from = join(assetDir, name)
    if (/\.(js|css)$/.test(name)) await writeFile(from, rewrite(await readFile(from, 'utf8')))
    await rename(from, join(assetDir, renamed.get(name)))
  }
  const indexPath = join(publicDir, 'index.html')
  const index = rewrite(await readFile(indexPath, 'utf8'))
  await writeFile(indexPath, index.replace('<head>', '<head>\n    <meta name="openreader-pwa-contract-build" content="B" />'))
  const { writeServiceWorker } = await import(builderURL)
  const config = await writeServiceWorker(publicDir)
  return { config, oldReaderChunk: `/assets/${readerChunk}` }
}

async function runReleaseFlows(browser, server, configA) {
  const label = 'release 1440x900'
  const account = await seedAccount(server, 'pwarelease')
  const context = await browser.newContext({ viewport: VIEWPORTS[0], serviceWorkers: 'allow' })
  const page = await context.newPage()
  const failures = watchPage(page, label)
  try {
    await page.addInitScript(token => localStorage.setItem('openreader_token', token), account.token)
    await page.goto(server.root, { waitUntil: 'domcontentloaded' })
    await waitForController(page, label)
    await assertShellCache(page, configA, `${label} A`)

    // E3: build B replaces A on the server; the A page keeps its lazy chunks (W21),
    // B installs and waits (Q3-A), and navigations already get B's document.
    const { config: configB, oldReaderChunk } = await publishBuildB(server.publicDir)
    assert(configB.buildId !== configA.buildId, `${label} E3: build B reused build A's id`)
    assert((await fetch(`${server.root}${oldReaderChunk}`)).status === 404, `${label} E3: the server still serves A's Reader chunk`)
    const oldChunkStatus = await page.evaluate(url => fetch(url).then(response => response.status), oldReaderChunk)
    assert(oldChunkStatus === 200, `${label} E3: A's precached Reader chunk returned ${oldChunkStatus} after the release (W21)`)

    const waiting = await page.evaluate(async () => {
      const registration = await navigator.serviceWorker.getRegistration()
      await registration.update()
      const deadline = Date.now() + 20_000
      while (!registration.waiting && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 100))
      return Boolean(registration.waiting)
    })
    assert(waiting, `${label} E3: build B's worker never reached the waiting state`)
    await page.reload({ waitUntil: 'domcontentloaded' })
    assert(
      await page.locator('meta[name="openreader-pwa-contract-build"][content="B"]').count() === 1,
      `${label} E3: an online navigation did not fetch build B's document`,
    )
    assert(
      await page.evaluate(async () => Boolean((await navigator.serviceWorker.getRegistration()).waiting)),
      `${label} E3: build B took over without a version click or closed pages (Q3-A)`,
    )

    // E7: the version click shows build info, activates the waiting worker and reloads once.
    // The toast is recorded into sessionStorage so the reload cannot race the check.
    await page.evaluate(() => {
      new MutationObserver(() => {
        const text = [...document.querySelectorAll('.el-message')].map(node => node.textContent).join('|')
        if (text) sessionStorage.setItem('pwa-contract-toast', text)
      }).observe(document.body, { childList: true, subtree: true })
    })
    // Wait for the reloaded document's load event. framenavigated also fires for the
    // router's same-document replaceState, and waitForLoadState right after the commit can
    // still report the old document's state, so an evaluate could land in a dying context.
    let loads = 0
    page.on('load', () => { loads += 1 })
    const reloaded = page.waitForEvent('load', { timeout: 20_000 })
    await page.locator('.app-version-text').click()
    await reloaded
    await page.waitForTimeout(1_000)
    assert(loads === 1, `${label} E7: the version click loaded the page ${loads} times, want one reload`)
    const toast = await page.evaluate(() => sessionStorage.getItem('pwa-contract-toast') || '')
    assert(/构建/.test(toast), `${label} E7: the version click reloaded without showing build info first (toast: ${toast})`)
    const activated = await page.evaluate(async () => {
      const deadline = Date.now() + 20_000
      while (Date.now() < deadline) {
        const registration = await navigator.serviceWorker.getRegistration()
        if (registration && !registration.waiting && navigator.serviceWorker.controller) break
        await new Promise(resolve => setTimeout(resolve, 100))
      }
      return (await caches.keys()).filter(name => name.startsWith('openreader-'))
    })
    assert(
      activated.length === 1 && activated[0] === `openreader-shell-${configB.buildId}`,
      `${label} E7: after the version click caches were ${activated.join(', ')}, want only build B's shell`,
    )

    // E5: nopwa unregisters and clears OpenReader caches.
    await page.goto(`${server.root}/?nopwa=1`, { waitUntil: 'domcontentloaded' })
    const disabled = await page.evaluate(async () => {
      const deadline = Date.now() + 10_000
      while (Date.now() < deadline) {
        const registrations = await navigator.serviceWorker.getRegistrations()
        const names = (await caches.keys()).filter(name => name.startsWith('openreader-'))
        if (registrations.length === 0 && names.length === 0) return true
        await new Promise(resolve => setTimeout(resolve, 100))
      }
      return false
    })
    assert(disabled, `${label} E5: ?nopwa left a registration or an OpenReader cache behind`)
    assert(failures.length === 0, `${label}: browser failures:\n${failures.join('\n')}`)
  } finally {
    await context.close()
  }
}

// E6: over plain HTTP on a LAN address Chromium reports isSecureContext=false and hides
// navigator.serviceWorker (and caches). That API surface is reproduced on 127.0.0.1,
// because a made-up host name is not reachable through every environment's proxy.
async function runInsecureContext(browser, server) {
  const label = 'insecure context'
  const account = await seedAccount(server, 'pwainsecure')
  const context = await browser.newContext({ viewport: VIEWPORTS[0], serviceWorkers: 'allow' })
  const page = await context.newPage()
  const workerRequests = []
  page.on('request', request => {
    if (new URL(request.url()).pathname === '/sw.js') workerRequests.push(request.url())
  })
  try {
    await page.addInitScript(token => {
      delete Navigator.prototype.serviceWorker
      delete window.caches
      Object.defineProperty(window, 'isSecureContext', { configurable: true, get: () => false })
      localStorage.setItem('openreader_token', token)
    }, account.token)
    await page.goto(server.root, { waitUntil: 'load' })
    await shelfRow(page, account.book.title).waitFor({ state: 'visible', timeout: 15_000 })
    const state = await page.evaluate(() => ({ secure: window.isSecureContext, supported: 'serviceWorker' in navigator }))
    assert(state.secure === false && state.supported === false, `${label} E6: the insecure API surface was not reproduced: ${JSON.stringify(state)}`)

    let navigations = 0
    page.on('framenavigated', frame => { if (frame === page.mainFrame()) navigations += 1 })
    await page.locator('.app-version-text').click()
    await page.locator('.el-message').filter({ hasText: /构建|开发构建/ }).first().waitFor({ state: 'visible', timeout: 10_000 })
    await page.waitForTimeout(1_500)
    assert(navigations === 0, `${label} E6: the version click reloaded a page without a service worker`)
    assert(workerRequests.length === 0, `${label} E6: a page without service worker support requested ${workerRequests.join(', ')}`)
  } finally {
    await context.close()
  }
}

const server = await createServerHarness()
const browser = await openSmokeBrowser()
try {
  const configA = await readWorkerConfig(server.publicDir)
  for (const viewport of VIEWPORTS) await runViewport(browser, server, configA, viewport)
  await runReleaseFlows(browser, server, configA)
  await runInsecureContext(browser, server)
  console.log(`PWA service worker contract passed: build ${configA.buildId}, ${configA.precache.length} precached files, offline shell, account isolation, A→B release, version click, nopwa, insecure HTTP, 1440/390/360`)
} finally {
  await browser.close()
  await server.close()
}
