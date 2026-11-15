#!/usr/bin/env node

// Contract: docs/compat/reader-progress-pending-sync-keepalive-fixed-baseline-second-audit-p0-contract.md
// R1-R7, 8.3(1)-(3). Real Go server, SQLite, WebSocket and the production frontend build. The browser is only
// taken offline (context.setOffline) or, in keepalive-503, has PUT /api/progress fulfilled with 503 while the
// Reader is left. Every run is driven to its end and records each failed contract assertion; the script exits
// non-zero when an assertion failed or a run could not be driven (reported as ERROR, which is not a red).
//
// TARGET_URL reuses a running server (SQLite rows are then read only when SMOKE_DB_PATH is set);
// SMOKE_VIEWPORTS (default 1440x900,390x844,360x800); SMOKE_SCENARIOS (default all, see SCENARIOS);
// SMOKE_REPORT writes a JSON report.

import { execFile, execFileSync, spawn } from 'node:child_process'
import { randomBytes } from 'node:crypto'
import { access, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { createServer } from 'node:http'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'
import { openSmokeBrowser } from './playwright-runtime.mjs'

const execFileAsync = promisify(execFile)
const rootDir = join(dirname(fileURLToPath(import.meta.url)), '..', '..')
const backendDir = join(rootDir, 'backend')
const publicDir = join(rootDir, 'frontend', 'dist')

const HEADINGS = ['第一章 开端', '第二章 已存', '第三章 离线', '第四章 预载', '第五章 后续', '第六章 终点']
const SAVED = 1 // chapter the server already has
const NEXT = 2 // chapter entered while offline (inside the preload radius of SAVED)
const OFFLINE_SCENARIOS = ['offline-home', 'offline-chapter', 'keepalive-503', 'offline-hidden-close']
const BASE_MODES = ['page', 'scroll', 'scroll2', 'flip']
const SCENARIOS = [...OFFLINE_SCENARIOS, ...BASE_MODES.map(mode => `base-${mode}`)]
// Test-only accounts on the isolated local server; never a real credential.
const CONTRACT_PASSWORD = 'offline-pending-contract'

const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))
const isMobile = viewport => viewport.width <= 750
const vpTag = viewport => `${viewport.width}x${viewport.height}`
const uniqueSuffix = () => `${Date.now().toString(36)}${randomBytes(2).toString('hex')}`

function smokeViewports() {
  return String(process.env.SMOKE_VIEWPORTS || '1440x900,390x844,360x800')
    .split(',')
    .map(value => value.trim())
    .filter(Boolean)
    .map((value) => {
      const [width, height] = value.toLowerCase().split('x').map(Number)
      if (!Number.isInteger(width) || !Number.isInteger(height) || width < 1 || height < 1) {
        throw new Error(`Invalid SMOKE_VIEWPORTS entry: ${value}`)
      }
      return { width, height }
    })
}

function smokeScenarios() {
  const requested = String(process.env.SMOKE_SCENARIOS || SCENARIOS.join(','))
    .split(',')
    .map(value => value.trim())
    .filter(Boolean)
  for (const scenario of requested) {
    if (!SCENARIOS.includes(scenario)) throw new Error(`Unknown SMOKE_SCENARIOS entry: ${scenario}`)
  }
  return requested
}

function redact(text) {
  return String(text || '').replace(/([?&](?:token|access_token)=)[^\s&"']+/gi, '$1<redacted>')
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

async function waitForHealth(root, processOutput) {
  const deadline = Date.now() + 60_000
  let lastError
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`${root}/api/health`)
      if (response.ok) return
      lastError = new Error(`health returned ${response.status}`)
    } catch (error) {
      lastError = error
    }
    await sleep(250)
  }
  throw new Error(`offline pending server did not start: ${lastError?.message || 'unknown'}\n${redact(processOutput())}`)
}

async function stopProcess(child) {
  if (!child || child.exitCode !== null) return
  const exited = new Promise(resolve => child.once('exit', resolve))
  child.kill('SIGTERM')
  await Promise.race([exited, sleep(5_000)])
  if (child.exitCode === null) {
    child.kill('SIGKILL')
    await Promise.race([exited, sleep(2_000)])
  }
}

async function startOpenReader() {
  await access(join(publicDir, 'index.html')).catch(() => {
    throw new Error('frontend/dist is missing; run `cd frontend && npm run build` first')
  })
  const tempRoot = await mkdtemp(join(tmpdir(), 'openreader-offline-pending-'))
  const binary = join(tempRoot, 'openreader')
  const dataDir = join(tempRoot, 'data')
  const dbPath = join(dataDir, 'openreader.db')
  const port = await reserveLocalPort()
  await execFileAsync('go', ['build', '-o', binary, '.'], {
    cwd: backendDir,
    env: process.env,
    maxBuffer: 4 * 1024 * 1024,
  })
  let output = ''
  const child = spawn(binary, [], {
    cwd: backendDir,
    env: {
      ...process.env,
      OPENREADER_ADDR: `127.0.0.1:${port}`,
      OPENREADER_DATA_DIR: dataDir,
      OPENREADER_CACHE_DIR: join(tempRoot, 'cache'),
      OPENREADER_LIBRARY_DIR: join(tempRoot, 'library'),
      OPENREADER_LOCAL_STORE_DIR: join(tempRoot, 'library', 'localStore'),
      OPENREADER_DB: dbPath,
      OPENREADER_PUBLIC_DIR: publicDir,
      // Random per run and never printed.
      OPENREADER_JWT_SECRET: randomBytes(32).toString('hex'),
      OPENREADER_CORS_ORIGIN: `http://127.0.0.1:${port}`,
      OPENREADER_CHECK_INTERVAL: '24h',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  child.stdout.on('data', chunk => { output += chunk.toString() })
  child.stderr.on('data', chunk => { output += chunk.toString() })
  const root = `http://127.0.0.1:${port}`
  try {
    await waitForHealth(root, () => output)
  } catch (error) {
    await stopProcess(child)
    await rm(tempRoot, { recursive: true, force: true })
    throw error
  }
  return {
    root,
    dbPath,
    output: () => redact(output),
    close: async () => {
      await stopProcess(child)
      await rm(tempRoot, { recursive: true, force: true })
    },
  }
}

async function api(root, path, { token = '', method = 'GET', body, form } = {}) {
  const response = await fetch(`${root}/api${path}`, {
    method,
    headers: {
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
    },
    body: form || (body === undefined ? undefined : JSON.stringify(body)),
  })
  const text = await response.text()
  let data = null
  try { data = text ? JSON.parse(text) : null } catch { data = null }
  // Auth responses carry tokens: report the status and the API error only.
  if (!response.ok) throw new Error(`${method} ${path} failed with ${response.status} ${data?.error || ''}`.trim())
  return data
}

async function register(root, label) {
  const username = `op${label}${uniqueSuffix()}`
  const result = await api(root, '/auth/register', { method: 'POST', body: { username, password: CONTRACT_PASSWORD } })
  if (!result?.token) throw new Error(`register ${label}: no token in the response`)
  return { username, token: result.token }
}

function txtBody(title) {
  return HEADINGS.map((heading) => {
    const paragraphs = Array.from({ length: 70 }, (_, index) => (
      `${heading}第${index + 1}段。${title}正文，用于验证离线或保存失败时本地待同步阅读进度的保留与重试。`
    ))
    return `${heading}\n${paragraphs.join('\n')}`
  }).join('\n')
}

async function importFixture(root, token, title) {
  const form = new FormData()
  form.append('file', new Blob([txtBody(title)], { type: 'text/plain' }), `${title}.txt`)
  form.append('title', title)
  const book = await api(root, '/imports/books', { token, method: 'POST', form })
  const chapters = await api(root, `/books/${book.id}/chapters`, { token })
  if (!Array.isArray(chapters) || chapters.length !== HEADINGS.length) {
    throw new Error(`${title}: expected ${HEADINGS.length} chapters, got ${chapters?.length}`)
  }
  return { book, chapters, title }
}

async function setMode(root, token, mode) {
  await api(root, '/settings/reader', {
    token,
    method: 'PUT',
    body: { value: { mode, autoTheme: false, fontSize: 18, animateDuration: 0 } },
  })
}

async function seedProgress(root, token, fixture, index) {
  const row = fixture.chapters[index]
  return api(root, '/progress', {
    token,
    method: 'PUT',
    body: {
      bookId: fixture.book.id,
      chapterId: row.id,
      chapterIndex: row.index,
      offset: 0,
      percent: row.index / fixture.chapters.length,
      chapterPercent: 0,
      chapterTitle: row.title,
      mode: 'page',
    },
  })
}

function sqliteRow(dbPath, bookId) {
  try {
    const output = execFileSync('sqlite3', ['-readonly', '-json', dbPath,
      `select chapter_index as chapterIndex, offset, updated_at as updatedAt from reading_progresses where book_id = ${Number(bookId)}`,
    ], { encoding: 'utf8' })
    const rows = output.trim() ? JSON.parse(output) : []
    return rows[0] || null
  } catch (error) {
    return { error: String(error.message || error).split('\n')[0] }
  }
}

async function serverRow(app, user, fixture) {
  if (app.dbPath) return sqliteRow(app.dbPath, fixture.book.id)
  const row = await api(app.root, `/progress/${fixture.book.id}`, { token: user.token }).catch(error => ({ error: error.message }))
  return row?.error ? row : { chapterIndex: row?.chapterIndex, offset: row?.offset, updatedAt: row?.updatedAt }
}

function createTraffic(bookId, seededUpdatedAt) {
  return {
    bookId: Number(bookId),
    puts: [],
    shelfLoads: [],
    // Every progress updatedAt the server sent for this book; R4 bases must come from this set.
    versions: [{ value: seededUpdatedAt, at: Date.now(), via: 'seed' }],
  }
}

function watchTraffic(context, traffic) {
  const entries = new WeakMap()
  context.on('request', (request) => {
    const url = new URL(request.url())
    if (request.method() === 'GET' && url.pathname === '/api/books') {
      traffic.shelfLoads.push({ at: Date.now() })
      return
    }
    if (request.method() !== 'PUT' || url.pathname !== '/api/progress') return
    let body = null
    try { body = JSON.parse(request.postData() || 'null') } catch {}
    if (Number(body?.bookId) !== traffic.bookId) return
    const entry = {
      at: Date.now(),
      type: request.resourceType(), // fetch: keepalive path, xhr: axios path
      chapterIndex: body.chapterIndex,
      offset: body.offset,
      baseUpdatedAt: body.baseUpdatedAt || '',
      clientUpdatedAt: body.clientUpdatedAt || '',
      status: null,
      failure: null,
    }
    traffic.puts.push(entry)
    entries.set(request, entry)
  })
  context.on('requestfailed', (request) => {
    const entry = entries.get(request)
    if (entry) entry.failure = request.failure()?.errorText || 'failed'
  })
  context.on('response', async (response) => {
    const request = response.request()
    const entry = entries.get(request)
    if (entry) entry.status = response.status()
    if (response.status() !== 200) return
    const url = new URL(response.url())
    const isShelf = request.method() === 'GET' && url.pathname === '/api/books'
    const isProgress = url.pathname === '/api/progress' || url.pathname === `/api/progress/${traffic.bookId}`
    if (!isShelf && !isProgress) return
    const data = await response.json().catch(() => null)
    const rows = isShelf
      ? (Array.isArray(data) ? data : data?.list || []).map(book => book?.progress)
      : [data]
    for (const row of rows) {
      if (row?.updatedAt && Number(row.bookId) === traffic.bookId) {
        traffic.versions.push({ value: row.updatedAt, at: Date.now(), via: isShelf ? 'shelf' : 'progress' })
      }
    }
  })
}

function latestVersion(traffic) {
  return traffic.versions.at(-1)?.value || ''
}

function briefPut(put) {
  if (!put) return null
  const outcome = put.failure || put.status || 'pending'
  return `${put.type === 'fetch' ? 'keepalive' : put.type} i${put.chapterIndex} off${put.offset} base=${put.baseUpdatedAt || '-'} client=${put.clientUpdatedAt || '-'} -> ${outcome}`
}

function watchPageErrors(page, sink) {
  page.on('pageerror', error => sink.push(`pageerror: ${String(error.message || error).split('\n')[0].slice(0, 200)}`))
}

async function newContext(browser, viewport, token) {
  const context = await browser.newContext({ viewport, isMobile: isMobile(viewport), hasTouch: isMobile(viewport) })
  await context.addInitScript(value => localStorage.setItem('openreader_token', value), token)
  await context.addInitScript(() => {
    // Scoped progress snapshots as this document found them, before the app ran.
    window.__progressAtStart = Object.fromEntries(Object.keys(localStorage)
      .filter(key => key.startsWith('openreader_chapter_progress@'))
      .map(key => [key.split('@').pop(), localStorage.getItem(key)]))
    // Diagnostic write log, deliberately without stack capture so it does not shift millisecond boundaries.
    window.__progressWrites = []
    const setItem = Storage.prototype.setItem
    Storage.prototype.setItem = function recordProgressWrite(key, value) {
      if (this === window.localStorage && String(key).startsWith('openreader_chapter_progress@')) {
        try {
          const data = JSON.parse(value)
          window.__progressWrites.push(`${Date.now()} i${data.chapterIndex} off${data.offset} ${data.pendingSync ? 'pending' : 'plain'} upd=${data.updatedAt} base=${data.baseUpdatedAt || '-'}`)
        } catch {}
      }
      return setItem.call(this, key, value)
    }
  })
  return context
}

async function drainWrites(page) {
  return page.evaluate(() => {
    const log = window.__progressWrites || []
    window.__progressWrites = []
    return log
  }).catch(() => [])
}

function pickLocal(value) {
  if (!value || typeof value !== 'object') return value ?? null
  return {
    chapterIndex: value.chapterIndex,
    offset: value.offset,
    pendingSync: Boolean(value.pendingSync),
    baseUpdatedAt: value.baseUpdatedAt || '',
    updatedAt: value.updatedAt || '',
  }
}

async function localSnapshot(page, bookId) {
  return page.evaluate((target) => {
    const key = Object.keys(localStorage).find(candidate => (
      candidate.startsWith('openreader_chapter_progress@') && candidate.endsWith(`@${target}`)
    ))
    if (!key) return null
    try { return JSON.parse(localStorage.getItem(key)) } catch { return null }
  }, bookId).then(pickLocal)
}

async function snapshotAtDocumentStart(page, bookId) {
  return page.evaluate((target) => {
    const raw = window.__progressAtStart?.[String(target)]
    try { return raw ? JSON.parse(raw) : null } catch { return null }
  }, bookId).then(pickLocal)
}

async function homeRow(page, title) {
  return page.evaluate(wanted => [...document.querySelectorAll('.book-row')]
    .map(row => ({
      name: row.querySelector('.name')?.textContent.trim(),
      dur: row.querySelector('.dur-chapter')?.textContent.trim() || null,
      unread: row.querySelector('.unread-num-badge .el-badge__content')?.textContent.trim() || null,
    }))
    .find(row => row.name === wanted) || null, title)
}

async function readerQuery(page) {
  return page.evaluate(() => {
    const params = new URLSearchParams(location.search)
    return { chapter: params.get('chapter'), offset: params.get('offset') }
  })
}

async function waitHome(page, title) {
  await page.locator('.book-row .name', { hasText: title }).first().waitFor({ timeout: 15_000 })
  await page.waitForTimeout(1200)
}

async function clickBookRow(page, title) {
  await page.locator('.book-row', { has: page.locator('.name', { hasText: title }) }).first().click()
}

async function waitChapter(page, index) {
  await page.waitForSelector(`.chapter-content[data-index="${index}"] [data-reader-block]`, { timeout: 15_000 })
}

async function waitAnyChapter(page) {
  await page.waitForSelector('.chapter-content [data-reader-block], .chapter-load-error', { timeout: 15_000 })
  await page.waitForTimeout(1200)
}

async function clickHome(page, viewport) {
  if (isMobile(viewport)) {
    if (!await page.locator('.reader-mobile-top.visible').count()) {
      await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
      await page.waitForTimeout(300)
    }
    await page.locator('.reader-mobile-top .mobile-tool-button', { hasText: '首页' }).click()
    return
  }
  await page.locator('.reader-left-rail .rail-item[aria-label="返回首页"]').click()
}

// Same location shape as useReaderNavigation.goChapter → Reader.vue navigate (router.replace).
async function routeToChapter(page, index) {
  await page.evaluate((target) => {
    const router = document.querySelector('#app').__vue_app__.config.globalProperties.$router
    const current = router.currentRoute.value
    return router.replace({ name: current.name, params: current.params, query: { chapter: target } })
  }, index)
}

async function hidePage(page) {
  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => true })
    Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'hidden' })
    document.dispatchEvent(new Event('visibilitychange'))
  })
}

function samePosition(value, expected) {
  return Boolean(value) && Number(value.chapterIndex) === expected.chapterIndex && Number(value.offset) === expected.offset
}

function createChecks() {
  const failures = []
  return {
    failures,
    expect(condition, message, observed) {
      if (!condition) failures.push(observed === undefined ? message : `${message}; observed ${JSON.stringify(observed)}`)
      return Boolean(condition)
    },
  }
}

// R4: every PUT /api/progress base is an updatedAt this client received from the server.
function expectServerBases(checks, traffic, label = 'every PUT /api/progress') {
  const known = new Set(traffic.versions.map(version => version.value))
  const offending = traffic.puts.filter(put => !put.baseUpdatedAt || !known.has(put.baseUpdatedAt))
  checks.expect(
    offending.length === 0,
    `R4 ${label} carries a server-issued baseUpdatedAt (${traffic.puts.length - offending.length}/${traffic.puts.length})`,
    offending.map(briefPut),
  )
}

function expectHomeRow(checks, row, expected, label) {
  const wanted = { dur: `已读：${HEADINGS[expected.chapterIndex]}`, unread: String(HEADINGS.length - 1 - expected.chapterIndex) }
  checks.expect(row?.dur === wanted.dur && row?.unread === wanted.unread, `${label} shows ${wanted.dur} / unread ${wanted.unread}`, row)
}

async function runOfflineScenario(browser, app, viewport, user, scenario) {
  await setMode(app.root, user.token, 'page')
  const title = `离线待同步${scenario.replace(/[^a-z0-9]/g, '')}${uniqueSuffix()}`
  const fixture = await importFixture(app.root, user.token, title)
  const seeded = await seedProgress(app.root, user.token, fixture, SAVED)
  const traffic = createTraffic(fixture.book.id, seeded.updatedAt)
  const checks = createChecks()
  const observed = {}
  const pageErrors = []
  const context = await newContext(browser, viewport, user.token)
  watchTraffic(context, traffic)
  let page = await context.newPage()
  watchPageErrors(page, pageErrors)
  const offline503 = url => url.pathname === '/api/progress'
  try {
    // 1. Open from the shelf (resume), move inside the saved chapter and let one throttled ordinary save land.
    await page.goto(`${app.root}/`, { waitUntil: 'domcontentloaded' })
    await waitHome(page, title)
    await clickBookRow(page, title)
    await waitChapter(page, SAVED)
    await page.waitForTimeout(800)
    await page.keyboard.press('PageDown')
    await page.waitForTimeout(400)
    await page.keyboard.press('PageDown')
    await page.waitForTimeout(2800)
    if (!traffic.puts.some(put => put.status === 200)) throw new Error('baseline: no ordinary progress save reached the server')
    const baseVersion = latestVersion(traffic)
    observed.baseline = { baseVersion, server: await serverRow(app, user, fixture) }

    // 2. Lose the network (or let only the keepalive fail) and keep reading.
    if (scenario === 'keepalive-503') {
      await context.route(offline503, (route) => {
        if (route.request().method() !== 'PUT') return route.continue()
        return route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'contract progress failure' }) })
      })
      // Leave inside the 1200 ms idle window: only the keepalive carries the new position.
      await page.keyboard.press('PageDown')
      await page.waitForTimeout(350)
      await page.keyboard.press('PageDown')
      await page.waitForTimeout(250)
    } else if (scenario === 'offline-chapter') {
      await page.waitForTimeout(800) // the preloads of SAVED±2 are in the memory cache by now
      await context.setOffline(true)
      await page.waitForTimeout(400)
      await routeToChapter(page, NEXT)
      await page.waitForTimeout(2500)
    } else {
      await context.setOffline(true)
      await page.waitForTimeout(400)
      const turns = scenario === 'offline-hidden-close' ? 2 : 3
      for (let turn = 0; turn < turns; turn += 1) {
        await page.keyboard.press('PageDown')
        await page.waitForTimeout(1900) // one ordinary save per turn; it fails offline
      }
    }

    observed.writes = { read: await drainWrites(page) }

    // 3. Leave the Reader.
    const leaveAt = Date.now()
    if (scenario === 'offline-hidden-close') {
      await hidePage(page)
      await page.waitForTimeout(1500)
      observed.afterHidden = await localSnapshot(page, fixture.book.id)
      observed.writes.leave = await drainWrites(page)
      await page.close({ runBeforeUnload: true })
      await sleep(1500)
    } else {
      await clickHome(page, viewport)
      await waitHome(page, title)
      await page.waitForTimeout(800)
      observed.writes.leave = await drainWrites(page)
    }
    const leaveKeepalives = traffic.puts.filter(put => put.at >= leaveAt && put.type === 'fetch')
    if (!leaveKeepalives.length) throw new Error('leave: no keepalive progress save was sent')
    const expected = { chapterIndex: leaveKeepalives.at(-1).chapterIndex, offset: leaveKeepalives.at(-1).offset }
    observed.expected = expected
    observed.leaveKeepalives = leaveKeepalives.map(briefPut)
    if (scenario === 'offline-chapter' && expected.chapterIndex !== NEXT) {
      throw new Error(`offline chapter change did not reach chapter ${NEXT}: ${JSON.stringify(expected)}`)
    }

    // 4. Recover: online event, manual refresh after the 503 is lifted, or a new page after the close.
    let afterLeave
    const recoverAt = Date.now()
    if (scenario === 'offline-hidden-close') {
      await context.setOffline(false)
      page = await context.newPage()
      watchPageErrors(page, pageErrors)
      await page.goto(`${app.root}/`, { waitUntil: 'domcontentloaded' })
      afterLeave = await snapshotAtDocumentStart(page, fixture.book.id)
      await waitHome(page, title)
      await page.waitForTimeout(2500)
    } else {
      afterLeave = await localSnapshot(page, fixture.book.id)
      observed.homeAfterLeave = await homeRow(page, title)
      if (scenario === 'keepalive-503') {
        await context.unroute(offline503)
        await page.getByRole('button', { name: '刷新', exact: true }).click()
        await page.waitForTimeout(3000)
      } else {
        expectHomeRow(checks, observed.homeAfterLeave, expected, 'R5 the offline home row after leaving')
        await context.setOffline(false)
        await page.waitForTimeout(4000)
      }
    }
    observed.afterLeave = afterLeave
    checks.expect(samePosition(afterLeave, expected), 'R1/R6 the local snapshot after leaving keeps the left position', afterLeave)
    checks.expect(afterLeave?.pendingSync === true, 'R1/R2 the local snapshot after the failed leave save is still pendingSync', afterLeave)
    checks.expect(afterLeave?.baseUpdatedAt === baseVersion, `R1/R3 the pending snapshot keeps the server base ${baseVersion}`, afterLeave)

    const retries = traffic.puts.filter(put => put.at >= recoverAt && put.type === 'xhr')
    observed.retries = retries.map(briefPut)
    const retry = retries.find(put => put.status === 200 && samePosition(put, expected))
    checks.expect(retry, 'R5 the next network shelf load retries the pending position with PUT /api/progress (xhr) -> 200', observed.retries)
    checks.expect(!retry || retry.baseUpdatedAt === baseVersion, `R4/R5 the retry carries the server base ${baseVersion}`, briefPut(retry))
    observed.recovered = {
      local: await localSnapshot(page, fixture.book.id),
      home: await homeRow(page, title),
      server: await serverRow(app, user, fixture),
    }
    observed.writes.recover = await drainWrites(page)
    checks.expect(samePosition(observed.recovered.server, expected), 'R5 the SQLite row after recovery is the left position', observed.recovered.server)
    checks.expect(samePosition(observed.recovered.local, expected), 'R5 the local progress after recovery is the left position', observed.recovered.local)
    expectHomeRow(checks, observed.recovered.home, expected, 'R5 the home row after recovery')

    // 5. Reload the shelf (the close scenario already started a new page).
    if (scenario !== 'offline-hidden-close') {
      await page.reload({ waitUntil: 'domcontentloaded' })
      await waitHome(page, title)
      await page.waitForTimeout(1500)
      observed.reloaded = { local: await localSnapshot(page, fixture.book.id), home: await homeRow(page, title) }
      checks.expect(samePosition(observed.reloaded.local, expected), 'R5 the local progress after a reload is the left position', observed.reloaded.local)
      expectHomeRow(checks, observed.reloaded.home, expected, 'R5 the home row after a reload')
    }

    // 6. Reopen the book from its shelf row.
    await clickBookRow(page, title)
    await waitAnyChapter(page)
    observed.reopen = await readerQuery(page)
    // readerRouteQueryFromBook omits a zero offset.
    const expectedOffset = expected.offset > 0 ? String(expected.offset) : null
    checks.expect(
      observed.reopen.chapter === String(expected.chapterIndex) && observed.reopen.offset === expectedOffset,
      `R5 reopening resumes chapter=${expected.chapterIndex}${expectedOffset ? `&offset=${expectedOffset}` : ' without offset'}`,
      observed.reopen,
    )
    expectServerBases(checks, traffic)
    checks.expect(pageErrors.length === 0, 'R7 no pageerror', pageErrors)
    observed.puts = traffic.puts.map(briefPut)
    return { failures: checks.failures, observed }
  } catch (error) {
    observed.puts = traffic.puts.map(briefPut)
    return { failures: checks.failures, observed, error: String(error.stack || error.message || error).split('\n').slice(0, 4).join('\n') }
  } finally {
    await context.close().catch(() => {})
  }
}

async function runBaseScenario(browser, app, viewport, user, mode) {
  await setMode(app.root, user.token, mode)
  const title = `基线${mode}${uniqueSuffix()}`
  const fixture = await importFixture(app.root, user.token, title)
  const seeded = await seedProgress(app.root, user.token, fixture, SAVED)
  const traffic = createTraffic(fixture.book.id, seeded.updatedAt)
  const checks = createChecks()
  const observed = {}
  const pageErrors = []
  const context = await newContext(browser, viewport, user.token)
  watchTraffic(context, traffic)
  const page = await context.newPage()
  watchPageErrors(page, pageErrors)
  try {
    await page.goto(`${app.root}/`, { waitUntil: 'domcontentloaded' })
    await waitHome(page, title)
    await clickBookRow(page, title)
    await waitChapter(page, SAVED)
    await page.waitForTimeout(1500)
    for (let turn = 0; turn < 3; turn += 1) {
      await page.keyboard.press('PageDown')
      await page.waitForTimeout(2600)
    }
    observed.puts = traffic.puts.map(briefPut)
    observed.writes = await drainWrites(page)
    if (!traffic.puts.length) throw new Error(`${mode}: no progress save was sent`)
    checks.expect(traffic.puts.every(put => put.status === 200), `${mode}: every online save returns 200`, observed.puts)
    expectServerBases(checks, traffic, `every ${mode} PUT /api/progress`)
    checks.expect(pageErrors.length === 0, 'R7 no pageerror', pageErrors)
    return { failures: checks.failures, observed }
  } catch (error) {
    observed.puts = traffic.puts.map(briefPut)
    return { failures: checks.failures, observed, error: String(error.stack || error.message || error).split('\n').slice(0, 4).join('\n') }
  } finally {
    await context.close().catch(() => {})
  }
}

async function main() {
  const viewports = smokeViewports()
  const scenarios = smokeScenarios()
  const targetRoot = String(process.env.TARGET_URL || '').replace(/\/$/, '')
  const app = targetRoot
    ? { root: targetRoot, dbPath: process.env.SMOKE_DB_PATH || '', output: () => '', close: async () => {} }
    : await startOpenReader()
  const report = { startedAt: new Date().toISOString(), viewports: viewports.map(vpTag), scenarios, results: [] }
  const browser = await openSmokeBrowser()
  try {
    // The first account of a fresh server is the administrator; the contract accounts are ordinary users.
    if (!targetRoot) await register(app.root, 'owner')
    for (const viewport of viewports) {
      const user = await register(app.root, vpTag(viewport).replace('x', 'v'))
      for (const scenario of scenarios) {
        const run = scenario.startsWith('base-')
          ? await runBaseScenario(browser, app, viewport, user, scenario.slice('base-'.length))
          : await runOfflineScenario(browser, app, viewport, user, scenario)
        const status = run.error ? 'ERROR' : run.failures.length ? 'FAIL' : 'PASS'
        report.results.push({ viewport: vpTag(viewport), scenario, status, ...run })
        console.log(`${status} ${vpTag(viewport)} ${scenario}`)
        for (const failure of run.failures) console.log(`  - ${failure}`)
        if (run.error) console.log(`  ! ${run.error.replace(/\n/g, '\n    ')}`)
      }
    }
  } finally {
    await browser.close().catch(() => {})
    report.finishedAt = new Date().toISOString()
    if (process.env.SMOKE_REPORT) await writeFile(process.env.SMOKE_REPORT, `${JSON.stringify(report, null, 2)}\n`)
    const failed = report.results.filter(result => result.status !== 'PASS')
    if (failed.some(result => result.status === 'ERROR')) console.log(`OpenReader output:\n${app.output().slice(-4000)}`)
    await app.close()
    console.log(`reader progress offline pending contract: ${report.results.length} runs, ${report.results.length - failed.length} passed, ${failed.filter(result => result.status === 'FAIL').length} failed, ${failed.filter(result => result.status === 'ERROR').length} errors`)
    if (failed.length) process.exitCode = 1
  }
}

main().catch((error) => {
  console.error(redact(error.stack || error.message))
  process.exit(1)
})
