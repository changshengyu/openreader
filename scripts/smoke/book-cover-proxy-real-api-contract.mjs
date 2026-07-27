#!/usr/bin/env node

import { execFile, spawn } from 'node:child_process'
import { createHash } from 'node:crypto'
import { access, mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
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

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function tinyPNG() {
  return Buffer.from(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
    'base64',
  )
}

async function reserveLocalPort() {
  const server = createServer()
  await new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', resolve)
  })
  const address = server.address()
  assert(address && typeof address === 'object', 'unable to reserve a local OpenReader test port')
  await new Promise(resolve => server.close(resolve))
  return address.port
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

async function waitForHealth(root, output) {
  const deadline = Date.now() + 60_000
  let lastError = null
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`${root}/api/health`)
      if (response.ok) return
      lastError = new Error(`health returned ${response.status}`)
    } catch (error) {
      lastError = error
    }
    await new Promise(resolve => setTimeout(resolve, 300))
  }
  throw new Error(`OpenReader cover-proxy test server did not start: ${lastError?.message || 'unknown error'}\n${output()}`)
}

async function startOpenReader() {
  await access(join(publicDir, 'index.html')).catch(() => {
    throw new Error('frontend/dist is missing; run `cd frontend && npm run build` before this smoke')
  })
  const tempRoot = await mkdtemp(join(tmpdir(), 'openreader-cover-proxy-real-api-'))
  const binary = join(tempRoot, 'openreader')
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
      OPENREADER_DATA_DIR: join(tempRoot, 'data'),
      OPENREADER_CACHE_DIR: join(tempRoot, 'cache'),
      OPENREADER_LIBRARY_DIR: join(tempRoot, 'library'),
      OPENREADER_LOCAL_STORE_DIR: join(tempRoot, 'library', 'localStore'),
      OPENREADER_DB: join(tempRoot, 'data', 'openreader.db'),
      OPENREADER_PUBLIC_DIR: publicDir,
      OPENREADER_JWT_SECRET: 'cover-proxy-real-api-contract-secret',
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
    tempRoot,
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
  let data = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    data = text
  }
  if (!response.ok) throw new Error(`${method} ${path} failed with ${response.status}: ${text}`)
  return data
}

async function seedWorkspace(server, viewport) {
  const suffix = `${viewport.width}x${viewport.height}`
  const registered = await api(server.root, '/auth/register', {
    method: 'POST',
    body: {
      username: `cover${viewport.width}${viewport.height}`,
      password: 'cover-proxy-contract',
    },
  })
  const token = registered?.token
  const userID = Number(registered?.user?.id)
  assert(token && Number.isInteger(userID) && userID > 0, `${suffix}: registration returned no identity`)

  const rawCoverURL = `https://browser-must-not-request.invalid/cover-${suffix}.png?credential=hidden`
  const cacheKey = createHash('sha256').update(rawCoverURL).digest('hex')
  const cacheRoot = join(server.tempRoot, 'cache', 'cover-images', `user-${userID}`)
  await mkdir(cacheRoot, { recursive: true, mode: 0o700 })
  await writeFile(join(cacheRoot, `${cacheKey}.img`), tinyPNG(), { mode: 0o600 })

  const valid = await api(server.root, '/books', {
    token,
    method: 'POST',
    body: {
      title: `安全代理封面 ${suffix}`,
      author: '真实缓存',
      sourceId: 0,
      url: `https://book.example/${suffix}/valid`,
      coverUrl: rawCoverURL,
    },
  })
  const unsafeCoverURL = `http://127.0.0.1:1/private-${suffix}.png`
  const unsafe = await api(server.root, '/books', {
    token,
    method: 'POST',
    body: {
      title: `不安全封面回退 ${suffix}`,
      author: '安全策略',
      sourceId: 0,
      url: `https://book.example/${suffix}/unsafe`,
      coverUrl: unsafeCoverURL,
    },
  })
  assert(valid.coverUrl === rawCoverURL, `${suffix}: valid raw cover URL was mutated`)
  assert(
    typeof valid.coverResourceUrl === 'string' && valid.coverResourceUrl.startsWith('/api/cover/'),
    `${suffix}: valid cover projection is missing`,
  )
  assert(unsafe.coverUrl === unsafeCoverURL, `${suffix}: unsafe raw cover URL was mutated`)
  assert(
    Object.hasOwn(unsafe, 'coverResourceUrl') && unsafe.coverResourceUrl === '',
    `${suffix}: unsafe projection must be present-empty`,
  )
  return { token, valid, unsafe, rawCoverURL, unsafeCoverURL }
}

function shelfRow(page, title) {
  return page.locator('.shelf-page .book-row').filter({ has: page.getByText(title, { exact: true }) })
}

async function assertNoHorizontalOverflow(page, suffix, scope = 'body') {
  const metrics = await page.locator(scope).evaluate(element => ({
    clientWidth: element.clientWidth,
    scrollWidth: element.scrollWidth,
  }))
  assert(
    metrics.scrollWidth <= metrics.clientWidth + 1,
    `${suffix}: ${scope} overflowed horizontally (${metrics.scrollWidth} > ${metrics.clientWidth})`,
  )
}

async function runViewport(browser, server, viewport) {
  const suffix = `${viewport.width}x${viewport.height}`
  const seeded = await seedWorkspace(server, viewport)
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 750,
    hasTouch: viewport.width <= 750,
  })
  const page = await context.newPage()
  const failures = []
  const browserRequests = []
  page.on('request', request => browserRequests.push(request.url()))
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error' && !/WebSocket connection to .*\/ws\/sync/.test(message.text())) {
      failures.push(`console.error: ${message.text()}`)
    }
  })

  try {
    await page.addInitScript(token => localStorage.setItem('openreader_token', token), seeded.token)
    await page.goto(server.root, { waitUntil: 'domcontentloaded' })

    const validRow = shelfRow(page, seeded.valid.title)
    const unsafeRow = shelfRow(page, seeded.unsafe.title)
    await validRow.waitFor({ state: 'visible', timeout: 15_000 })
    await unsafeRow.waitFor({ state: 'visible', timeout: 15_000 })

    const validCover = validRow.locator('.book-cover-shared')
    await validCover.locator('img').waitFor({ state: 'visible', timeout: 10_000 })
    await validCover.evaluate(element => new Promise((resolve, reject) => {
      const deadline = Date.now() + 10_000
      const check = () => {
        if (element.classList.contains('has-cover')) return resolve()
        if (Date.now() >= deadline) return reject(new Error('projected cover did not reach loaded state'))
        setTimeout(check, 50)
      }
      check()
    }))
    const shelfCoverSrc = await validCover.locator('img').getAttribute('src')
    assert(shelfCoverSrc?.startsWith('/api/cover/'), `${suffix}: shelf used non-capability cover ${shelfCoverSrc}`)

    const unsafeCover = unsafeRow.locator('.book-cover-shared')
    await unsafeCover.getByText('暂无封面', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 })
    assert(await unsafeCover.locator('img').count() === 0, `${suffix}: unsafe raw cover reached an img element`)

    await validCover.click()
    const dialog = page.locator('.book-info-dialog')
    await dialog.waitFor({ state: 'visible', timeout: 10_000 })
    const dialogCover = dialog.locator('.book-cover-shared')
    await dialogCover.locator('img').waitFor({ state: 'visible', timeout: 10_000 })
    const dialogCoverSrc = await dialogCover.locator('img').getAttribute('src')
    assert(dialogCoverSrc?.startsWith('/api/cover/'), `${suffix}: BookInfo used non-capability cover ${dialogCoverSrc}`)
    await assertNoHorizontalOverflow(page, suffix, '.book-info-dialog')

    const closeButton = dialog.locator('.el-dialog__headerbtn')
    await closeButton.click()
    await dialog.waitFor({ state: 'hidden', timeout: 10_000 })
    await assertNoHorizontalOverflow(page, suffix)

    assert(
      !browserRequests.some(url => url.startsWith(seeded.rawCoverURL)),
      `${suffix}: browser requested the persisted remote cover URL`,
    )
    assert(
      !browserRequests.some(url => url.startsWith(seeded.unsafeCoverURL)),
      `${suffix}: browser requested the unsafe remote cover URL`,
    )
    const coverRequests = browserRequests.filter(url => new URL(url).pathname.startsWith('/api/cover/'))
    assert(coverRequests.length >= 1, `${suffix}: browser never requested a projected cover resource`)
    assert(
      !coverRequests.some(url => url.includes('credential=hidden') || url.includes('browser-must-not-request')),
      `${suffix}: capability URL leaked raw cover data`,
    )
    assert(failures.length === 0, `${suffix}: browser failures:\n${failures.join('\n')}`)
  } finally {
    await context.close()
  }
}

const server = await startOpenReader()
const browser = await openSmokeBrowser()
const browserSource = process.env.CDP_URL ? 'CDP Chromium' : 'headless Chromium'
try {
  for (const viewport of [
    { width: 1440, height: 900 },
    { width: 390, height: 844 },
    { width: 360, height: 800 },
  ]) {
    await runViewport(browser, server, viewport)
  }
  console.log(`book cover proxy real-API contract passed (${browserSource}): capability-only requests, safe fallback, shared BookInfo, 1440/390/360`)
} finally {
  try {
    await browser.close()
  } finally {
    await server.close()
  }
}
