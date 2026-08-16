#!/usr/bin/env node

import { execFile, spawn } from 'node:child_process'
import { mkdtemp, rm } from 'node:fs/promises'
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
const browserSourceCount = 70

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function reserveLocalPort() {
  const server = createServer()
  await new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', resolve)
  })
  const address = server.address()
  assert(address && typeof address === 'object', 'unable to reserve a local OpenReader port')
  const port = address.port
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
    await new Promise(resolve => setTimeout(resolve, 250))
  }
  throw new Error(`OpenReader remote-work server did not start: ${lastError?.message || 'unknown error'}\n${output()}`)
}

async function startOpenReader() {
  const tempRoot = await mkdtemp(join(tmpdir(), 'openreader-remote-work-'))
  const binary = join(tempRoot, 'openreader')
  const port = await reserveLocalPort()
  await execFileAsync('go', ['build', '-o', binary, '.'], {
    cwd: backendDir,
    env: { ...process.env, GOCACHE: join(backendDir, '.gocache') },
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
      OPENREADER_JWT_SECRET: 'remote-work-browser-contract-secret',
      OPENREADER_SOURCE_NETWORK_ALLOWLIST: '127.0.0.1',
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
    close: async () => {
      await stopProcess(child)
      await rm(tempRoot, { recursive: true, force: true })
    },
  }
}

function deferred() {
  let resolve
  const promise = new Promise(done => { resolve = done })
  return { promise, resolve }
}

async function startSourceFixture() {
  const requests = []
  const slowSignals = new Map()
  const server = createServer((request, response) => {
    const url = new URL(request.url || '/', 'http://fixture.local')
    requests.push(url.pathname + url.search)
    const parts = url.pathname.split('/').filter(Boolean)

    if (parts[0] === 'search' && parts.length === 3) {
      const [, suffix, ordinal] = parts
      const page = Math.max(1, Number(url.searchParams.get('page')) || 1)
      const keyword = url.searchParams.get('q') || ''
      const hasResult = keyword !== '多源边界' || ordinal === '0'
      const body = hasResult && page <= 2
        ? `<article class="book"><span class="name">${suffix} 源${ordinal} 第${page}页</span><span class="author">真实夹具</span><a href="/result/${suffix}/${ordinal}/${page}">详情</a></article>`
        : '<html></html>'
      response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
      response.end(body)
      return
    }
    if (parts[0] === 'book' && parts.length === 3) {
      const [, suffix, kind] = parts
      const title = kind === 'cancel' ? `${suffix} 取消缓存书` : `${suffix} 整本缓存书`
      response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
      response.end(`<h1 class="detail-title">${title}</h1><a class="toc" href="/toc/${suffix}/${kind}">目录</a>`)
      return
    }
    if (parts[0] === 'toc' && parts.length === 3) {
      const [, suffix, kind] = parts
      const body = kind === 'cancel'
        ? `<article class="chapter"><span class="chapter-name">慢章节</span><a href="/chapter/${suffix}/cancel/0">阅读</a></article>`
        : [0, 1].map(index => `<article class="chapter"><span class="chapter-name">第${index + 1}章</span><a href="/chapter/${suffix}/success/${index}">阅读</a></article>`).join('')
      response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
      response.end(body)
      return
    }
    if (parts[0] === 'chapter' && parts[2] === 'cancel') {
      const suffix = parts[1]
      const signals = slowSignals.get(suffix)
      signals?.started.resolve()
      let settled = false
      const markAborted = () => {
        if (settled) return
        settled = true
        signals?.aborted.resolve()
      }
      request.once('aborted', markAborted)
      response.once('close', () => {
        if (!response.writableEnded) markAborted()
      })
      return
    }
    if (parts[0] === 'chapter' && parts[2] === 'success') {
      response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
      response.end(`<main class="content">${parts.join('-')} 正文</main>`)
      return
    }
    response.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' })
    response.end('missing fixture')
  })
  await new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', resolve)
  })
  const address = server.address()
  assert(address && typeof address === 'object', 'source fixture did not bind')
  return {
    root: `http://127.0.0.1:${address.port}`,
    requests,
    slowFor(suffix) {
      const signals = { started: deferred(), aborted: deferred() }
      slowSignals.set(suffix, signals)
      return signals
    },
    close: () => new Promise(resolve => {
      server.closeAllConnections?.()
      server.close(resolve)
    }),
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
  return { data, response }
}

async function rawRequest(root, path, token, body) {
  const response = await fetch(`${root}${path}`, {
    method: 'POST',
    headers: {
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      'Content-Type': 'application/json',
    },
    body,
  })
  return { response, text: await response.text() }
}

function sourcePayload(fixtureRoot, suffix, ordinal) {
  return {
    name: `${suffix} 边界书源 ${ordinal}`,
    baseUrl: fixtureRoot,
    charset: 'utf-8',
    enabled: true,
    enabledExplore: true,
    customOrder: ordinal + 1,
    rules: JSON.stringify({
      searchUrl: `${fixtureRoot}/search/${suffix}/${ordinal}?q={keyword}&page={page}`,
      bookListRule: '.book',
      bookNameRule: '.name|text',
      bookAuthorRule: '.author|text',
      bookUrlRule: 'a|attr:href',
      bookInfoNameRule: '.detail-title|text',
      tocUrlRule: '.toc|attr:href',
      chapterListRule: '.chapter',
      chapterNameRule: '.chapter-name|text',
      chapterUrlRule: 'a|attr:href',
      contentRule: '.content|text',
    }),
  }
}

async function seedWorkspace(root, fixture, viewport) {
  const suffix = `rw${viewport.width}x${viewport.height}`
  const auth = (await api(root, '/auth/register', {
    method: 'POST',
    body: { username: suffix, password: 'remote-work-contract' },
  })).data
  const sources = []
  for (let ordinal = 0; ordinal < browserSourceCount; ordinal++) {
    sources.push((await api(root, '/sources', {
      token: auth.token,
      method: 'POST',
      body: sourcePayload(fixture.root, suffix, ordinal),
    })).data)
  }
  const successBook = (await api(root, '/books/remote', {
    token: auth.token,
    method: 'POST',
    body: {
      title: `${suffix} 整本缓存书`,
      bookUrl: `${fixture.root}/book/${suffix}/success`,
      sourceId: sources[0].id,
    },
  })).data
  const cancelBook = (await api(root, '/books/remote', {
    token: auth.token,
    method: 'POST',
    body: {
      title: `${suffix} 取消缓存书`,
      bookUrl: `${fixture.root}/book/${suffix}/cancel`,
      sourceId: sources[0].id,
    },
  })).data
  return { suffix, token: auth.token, sources, successBook, cancelBook }
}

function padObject(body, totalBytes) {
  const prefix = body.slice(0, -1) + ',"padding":"'
  const suffix = '"}'
  const fixedBytes = Buffer.byteLength(prefix) + Buffer.byteLength(suffix)
  assert(totalBytes >= fixedBytes, 'target body is too small')
  return prefix + 'p'.repeat(totalBytes - fixedBytes) + suffix
}

async function assertWireBoundaries(root, fixture, seeded) {
  const sourceID = seeded.sources[0].id
  const bookID = seeded.successBook.id
  const routes = [
    { path: '/api/search', body: `{"keyword":"边界","sourceIds":[${sourceID}],"page":1}`, max: 64 << 10 },
    { path: `/api/sources/${sourceID}/test`, body: `{"keyword":"边界"}`, max: 16 << 10 },
    { path: `/api/sources/${sourceID}/test-chapter`, body: `{"bookUrl":"${fixture.root}/book/${seeded.suffix}/success"}`, max: 16 << 10 },
    { path: `/api/sources/${sourceID}/test-content`, body: `{"chapterUrl":"${fixture.root}/chapter/${seeded.suffix}/success/0"}`, max: 16 << 10 },
    { path: '/api/sources/batch-test', body: `{"sourceIds":[${sourceID}],"keyword":"边界"}`, max: 16 << 10 },
    { path: `/api/books/${bookID}/cache`, body: '{"all":true}', max: 16 << 10 },
    { path: `/api/books/${bookID}/cache/stream`, body: '{"all":true}', max: 16 << 10 },
  ]
  for (const route of routes) {
    const before = fixture.requests.length
    const overflow = await rawRequest(root, route.path, seeded.token, padObject(route.body, route.max + 1))
    assert(overflow.response.status === 413, `${route.path}: overflow status ${overflow.response.status} ${overflow.text}`)
    const trailing = await rawRequest(root, route.path, seeded.token, route.body + '{}')
    assert(trailing.response.status === 400, `${route.path}: trailing status ${trailing.response.status} ${trailing.text}`)
    assert(fixture.requests.length === before, `${route.path}: rejected body started remote work`)
  }

  const unauthorized = await rawRequest(root, '/api/search', '', 'x'.repeat((64 << 10) + 1))
  assert(unauthorized.response.status === 401, `auth precedence status ${unauthorized.response.status}`)

  const ids = seeded.sources.map(source => source.id).join(',')
  const beforeLegacy = fixture.requests.length
  const legacy = await rawRequest(root, '/api/search', seeded.token, `{"keyword":"旧数组","sourceIds":[${ids}],"concurrentCount":1}`)
  assert(legacy.response.status === 200, `legacy search status ${legacy.response.status}: ${legacy.text}`)
  assert(Array.isArray(JSON.parse(legacy.text)), 'legacy search body is no longer an array')
  assert(legacy.response.headers.get('x-openreader-search-truncated') === '1', 'legacy truncation header missing')
  assert(legacy.response.headers.get('x-openreader-search-last-index') === '7', 'legacy truncation cursor mismatch')
  assert(fixture.requests.length - beforeLegacy === 8, 'legacy search did not stop after eight stable windows')

  const exact = await rawRequest(root, '/api/search', seeded.token, padObject(`{"keyword":"精确边界","sourceIds":[${sourceID}],"page":1}`, 64 << 10))
  assert(exact.response.status === 200, `exact-limit search status ${exact.response.status}: ${exact.text}`)
}

async function openMobileNavigation(page, viewport) {
  if (viewport.width > 750) return
  const alreadyOpen = await page.locator('.app-sidebar').evaluate(sidebar => Math.abs(Number.parseFloat(getComputedStyle(sidebar).marginLeft)) < 0.5)
  if (alreadyOpen) return
  await page.locator('.mobile-menu-trigger').click()
  await page.waitForFunction(() => {
    const sidebar = document.querySelector('.app-sidebar')
    return sidebar && Math.abs(Number.parseFloat(getComputedStyle(sidebar).marginLeft)) < 0.5
  })
}

async function chooseServerCache(page, row, buttonName) {
  await row.getByRole('button', { name: buttonName, exact: true }).click()
  const item = page.locator('.el-dropdown-menu:visible').getByRole('menuitem', { name: '缓存到服务器', exact: true })
  await item.waitFor({ state: 'visible' })
  await item.click({ force: true })
}

async function runViewport(browser, root, fixture, viewport) {
  const seeded = await seedWorkspace(root, fixture, viewport)
  if (viewport.width === 1440) await assertWireBoundaries(root, fixture, seeded)
  const slow = fixture.slowFor(seeded.suffix)
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 750,
    hasTouch: viewport.width <= 750,
  })
  await context.addInitScript(token => localStorage.setItem('openreader_token', token), seeded.token)
  const page = await context.newPage()
  const failures = []
  const searchBodies = []
  const healthBodies = []
  const cacheBodies = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error' && !/WebSocket connection to .*\/ws\/sync/.test(message.text())) {
      failures.push(`console.error: ${message.text()}`)
    }
  })
  page.on('request', request => {
    const path = new URL(request.url()).pathname
    if (request.method() !== 'POST') return
    try {
      if (path === '/api/search') searchBodies.push(request.postDataJSON())
      if (path === '/api/sources/batch-test') healthBodies.push(request.postDataJSON())
      if (path.endsWith('/cache/stream')) cacheBodies.push({ path, body: request.postDataJSON() })
    } catch {
      failures.push(`unable to decode browser request ${path}`)
    }
  })

  try {
    const ids = seeded.sources.map(source => source.id)
    const beforeMultiFixture = fixture.requests.length
    await page.goto(`${root}/?workspace=search&q=${encodeURIComponent('多源边界')}&searchType=all&concurrent=8`, { waitUntil: 'domcontentloaded' })
    await page.waitForFunction(() => document.querySelectorAll('.result-shelf-page .remote-result-book').length === 1, undefined, { timeout: 60_000 })
    assert(searchBodies[0]?.lastIndex === -1 && searchBodies[0]?.searchSize === 20, `${viewport.width}: initial multi cursor payload ${JSON.stringify(searchBodies[0])}`)
    assert(searchBodies[0]?.concurrentCount === 8, `${viewport.width}: multi concurrency payload ${JSON.stringify(searchBodies[0])}`)
    assert(JSON.stringify(searchBodies[0]?.sourceIds) === JSON.stringify(ids), `${viewport.width}: multi source order changed`)
    assert(fixture.requests.length - beforeMultiFixture === 64, `${viewport.width}: first multi request count ${fixture.requests.length - beforeMultiFixture}`)
    await page.getByRole('button', { name: '加载更多', exact: true }).click()
    await page.waitForFunction(() => [...document.querySelectorAll('button')]
      .some(button => button.textContent?.trim() === '没有更多了' && button.disabled), undefined, { timeout: 30_000 })
    assert(searchBodies[1]?.lastIndex === 63, `${viewport.width}: continuation cursor ${JSON.stringify(searchBodies[1])}`)
    assert(fixture.requests.length - beforeMultiFixture === browserSourceCount, `${viewport.width}: continuation request count ${fixture.requests.length - beforeMultiFixture}`)

    const beforeSingle = searchBodies.length
    await page.goto(`${root}/?workspace=search&q=${encodeURIComponent('单源边界')}&searchType=single&sourceId=${ids[0]}&concurrent=24`, { waitUntil: 'domcontentloaded' })
    await page.waitForFunction(() => document.querySelectorAll('.result-shelf-page .remote-result-book').length === 1)
    assert(searchBodies.length > beforeSingle, `${viewport.width}: single search request did not start`)
    const singleFirst = searchBodies.at(-1)
    assert(singleFirst?.page === 1 && !Object.hasOwn(singleFirst || {}, 'lastIndex'), `${viewport.width}: single first payload ${JSON.stringify(singleFirst)}`)
    await page.getByRole('button', { name: '加载更多', exact: true }).click()
    await page.waitForFunction(() => document.querySelectorAll('.result-shelf-page .remote-result-book').length === 2)
    const singleSecond = searchBodies.at(-1)
    assert(singleSecond?.page === 2 && !Object.hasOwn(singleSecond || {}, 'lastIndex'), `${viewport.width}: single continuation ${JSON.stringify(singleSecond)}`)

    await page.goto(`${root}/?overlay=sources&sourceAction=health`, { waitUntil: 'domcontentloaded' })
    const sourceManager = page.locator('.global-source-manage-dialog')
    await sourceManager.waitFor({ state: 'visible', timeout: 15_000 })
    await sourceManager.getByRole('button', { name: /检测书源/ }).click()
    await sourceManager.getByRole('button', { name: new RegExp(`检测书源 ${browserSourceCount}/${browserSourceCount}`) }).waitFor({ state: 'visible', timeout: 60_000 })
    assert(healthBodies.length === browserSourceCount / 5, `${viewport.width}: health request count ${healthBodies.length}`)
    assert(healthBodies.every(body => body.concurrent === 5 && body.sourceIds.length === 5), `${viewport.width}: health chunks ${JSON.stringify(healthBodies)}`)
    assert(healthBodies.flatMap(body => body.sourceIds).join(',') === ids.join(','), `${viewport.width}: health source order changed`)
    await sourceManager.getByRole('button', { name: '取消', exact: true }).click()
    await sourceManager.waitFor({ state: 'hidden' })

    await page.goto(root, { waitUntil: 'domcontentloaded' })
    await page.getByText(seeded.successBook.title, { exact: true }).waitFor({ state: 'visible', timeout: 15_000 })
    await openMobileNavigation(page, viewport)
    await page.getByRole('button', { name: '书籍管理', exact: true }).click()
    const manager = page.locator('.global-book-manage-dialog')
    await manager.waitFor({ state: 'visible', timeout: 15_000 })
    const successRow = manager.locator('.book-manage-table tbody tr').filter({ hasText: seeded.successBook.title })
    const cancelRow = manager.locator('.book-manage-table tbody tr').filter({ hasText: seeded.cancelBook.title })
    await successRow.waitFor({ state: 'visible' })
    await cancelRow.waitFor({ state: 'visible' })

    await chooseServerCache(page, successRow, '缓存')
    await page.getByText(`${seeded.successBook.title}缓存到服务器完成`, { exact: true }).waitFor({ state: 'visible', timeout: 15_000 })
    assert(JSON.stringify(cacheBodies[0]?.body) === JSON.stringify({ all: true, chapterIndex: 0, refresh: false }), `${viewport.width}: whole cache payload ${JSON.stringify(cacheBodies)}`)
    await page.waitForFunction(title => [...document.querySelectorAll('.book-manage-table tbody tr')]
      .find(row => row.textContent?.includes(title))?.textContent?.includes('服务器缓存： 2 章'), seeded.successBook.title)

    await manager.getByRole('button', { name: '取消', exact: true }).click()
    await manager.waitFor({ state: 'hidden' })
    await openMobileNavigation(page, viewport)
    await page.getByRole('button', { name: '书籍管理', exact: true }).click()
    await manager.waitFor({ state: 'visible', timeout: 15_000 })
    await cancelRow.waitFor({ state: 'visible' })

    const slowCacheRequest = page.waitForRequest(request => request.method() === 'POST'
      && new URL(request.url()).pathname === `/api/books/${seeded.cancelBook.id}/cache/stream`, { timeout: 10_000 })
    await chooseServerCache(page, cancelRow, '缓存')
    await slowCacheRequest
    await Promise.race([slow.started.promise, new Promise((_, reject) => setTimeout(() => reject(new Error(`${viewport.width}: slow cache did not start`)), 10_000))])
    await chooseServerCache(page, cancelRow, '缓存中')
    await Promise.race([slow.aborted.promise, new Promise((_, reject) => setTimeout(() => reject(new Error(`${viewport.width}: slow cache was not aborted`)), 10_000))])
    await page.waitForTimeout(150)
    assert(await page.getByText(`${seeded.cancelBook.title}缓存到服务器完成`, { exact: true }).count() === 0, `${viewport.width}: canceled cache emitted success`)
    assert(JSON.stringify(cacheBodies[1]?.body) === JSON.stringify({ all: true, chapterIndex: 0, refresh: false }), `${viewport.width}: cancel cache payload ${JSON.stringify(cacheBodies)}`)

    const geometry = await page.evaluate(() => ({ scrollWidth: document.documentElement.scrollWidth, innerWidth: window.innerWidth }))
    assert(geometry.scrollWidth <= geometry.innerWidth + 1, `${viewport.width}: horizontal overflow ${JSON.stringify(geometry)}`)
    assert(failures.length === 0, `${viewport.width}: ${failures.join('\n')}`)
    return `${viewport.width}x${viewport.height}`
  } finally {
    await context.close()
  }
}

let app
let fixture
let browser
try {
  fixture = await startSourceFixture()
  app = await startOpenReader()
  browser = await openSmokeBrowser()
  const completed = []
  for (const viewport of [
    { width: 1440, height: 900 },
    { width: 390, height: 844 },
    { width: 360, height: 800 },
  ]) {
    completed.push(await runViewport(browser, app.root, fixture, viewport))
  }
  console.log(`remote-work-request-boundary: ok ${completed.join(', ')} realGo=true apiMocked=false searchWindows=8 healthChunk=5 wholeCache=true cancellation=true`)
} finally {
  await browser?.close().catch(() => {})
  await app?.close().catch(() => {})
  await fixture?.close().catch(() => {})
}
