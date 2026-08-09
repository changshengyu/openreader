#!/usr/bin/env node

import { spawn } from 'node:child_process'
import { mkdtemp, rm } from 'node:fs/promises'
import { createServer } from 'node:http'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { openSmokeBrowser } from './playwright-runtime.mjs'

const rootDir = join(dirname(fileURLToPath(import.meta.url)), '..', '..')
const backendDir = join(rootDir, 'backend')
const targetPort = Number(process.env.OPENREADER_SOURCE_DEBUG_PORT || 18093)
const target = `http://127.0.0.1:${targetPort}`

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function jsonRequest(path, { token = '', method = 'GET', body } = {}) {
  const response = await fetch(`${target}${path}`, {
    method,
    headers: {
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
    },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  })
  const data = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(`${method} ${path} = ${response.status} ${JSON.stringify(data)}`)
  return data
}

async function startSourceFixture() {
  const requests = []
  let slowStartedResolve
  let slowAbortedResolve
  const slowStarted = new Promise(resolve => { slowStartedResolve = resolve })
  const slowAborted = new Promise(resolve => { slowAbortedResolve = resolve })
  const server = createServer((request, response) => {
    const url = new URL(request.url || '/', 'http://fixture.local')
    requests.push(url.pathname + url.search)
    const pages = {
      '/search': '<article class="book"><span class="name">搜索候选</span><a href="/book/1">详情</a></article>',
      '/book/1': '<h1 class="detail-title">浏览器详情书</h1><a class="toc" href="/toc/1">目录</a>',
      '/toc/1': '<article class="chapter"><span class="chapter-name">第一章</span><a href="/chapter/1">阅读</a></article><article class="chapter"><span class="chapter-name">第二章</span><a href="/chapter/2">阅读</a></article>',
      '/chapter/1': '<main class="content">DO_NOT_LEAK_BROWSER_CONTENT</main>',
      '/chapter/2': '<main class="content">DO_NOT_CROSS_CHAPTER_BOUNDARY</main>',
    }
    if (url.pathname === '/slow') {
      slowStartedResolve()
      let settled = false
      const markAborted = () => {
        if (settled) return
        settled = true
        slowAbortedResolve()
      }
      request.once('aborted', markAborted)
      response.once('close', () => {
        if (!response.writableEnded) markAborted()
      })
      return
    }
    const body = pages[url.pathname]
    if (!body) {
      response.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' })
      response.end('missing fixture')
      return
    }
    response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8', 'Cache-Control': 'no-store' })
    response.end(body)
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
    slowStarted,
    slowAborted,
    close: () => new Promise(resolve => {
      server.closeAllConnections?.()
      server.close(resolve)
    }),
  }
}

async function waitForHealth(output) {
  const deadline = Date.now() + 60_000
  while (Date.now() < deadline) {
    try {
      if ((await fetch(`${target}/api/health`)).ok) return
    } catch {
      // Retry while the Go process starts.
    }
    await new Promise(resolve => setTimeout(resolve, 250))
  }
  throw new Error(`OpenReader source-debug server did not start\n${output()}`)
}

async function stopProcess(child) {
  if (!child || child.exitCode !== null) return
  const exited = new Promise(resolve => child.once('exit', resolve))
  child.kill('SIGTERM')
  await Promise.race([exited, new Promise(resolve => setTimeout(resolve, 5_000))])
  if (child.exitCode === null) child.kill('SIGKILL')
}

async function cleanupWithin(task, timeoutMs = 5_000) {
  if (!task) return
  await Promise.race([
    task,
    new Promise(resolve => setTimeout(resolve, timeoutMs)),
  ])
}

function sourcePayload(fixtureRoot, searchPath = '/search?q={keyword}') {
  return {
    name: '真实浏览器书源调试',
    baseUrl: fixtureRoot,
    charset: 'utf-8',
    enabled: true,
    enabledExplore: true,
    rules: JSON.stringify({
      searchUrl: fixtureRoot + searchPath,
      bookListRule: '.book',
      bookNameRule: '.name|text',
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

async function assertViewport(page, viewport) {
  await page.setViewportSize(viewport)
  await page.waitForTimeout(80)
  const state = await page.evaluate(() => {
    const workspace = document.querySelector('.source-debug-workspace')
    const layout = document.querySelector('.source-debug-layout')
    const rule = document.querySelector('.source-debug-rule-pane')?.getBoundingClientRect()
    const rail = document.querySelector('.source-debug-command-rail')?.getBoundingClientRect()
    const output = document.querySelector('.source-debug-output-pane')?.getBoundingClientRect()
    return {
      workspace: Boolean(workspace),
      sidebar: document.querySelectorAll('.app-sidebar').length,
      scrollWidth: document.documentElement.scrollWidth,
      innerWidth: window.innerWidth,
      layoutDisplay: layout ? getComputedStyle(layout).display : '',
      ruleWidth: Math.round(rule?.width || 0),
      railWidth: Math.round(rail?.width || 0),
      outputWidth: Math.round(output?.width || 0),
      commands: document.querySelectorAll('.source-debug-command-rail button').length,
    }
  })
  assert(state.workspace, `${viewport.width}: source-debug workspace missing`)
  assert(state.sidebar === 0, `${viewport.width}: standalone debugger was wrapped in Index sidebar`)
  assert(state.scrollWidth <= state.innerWidth + 1, `${viewport.width}: debugger overflows horizontally ${JSON.stringify(state)}`)
  assert(state.commands === 9, `${viewport.width}: debugger command count ${state.commands}`)
  if (viewport.width <= 980) {
    assert(state.layoutDisplay === 'flex', `${viewport.width}: mobile debugger is not stacked`)
    assert(state.ruleWidth >= viewport.width - 20, `${viewport.width}: mobile rule pane is too narrow ${state.ruleWidth}`)
  } else {
    assert(state.layoutDisplay === 'grid', `${viewport.width}: desktop debugger is not three-column grid`)
    assert(state.ruleWidth > state.railWidth && state.outputWidth > state.railWidth, `${viewport.width}: invalid three-column geometry ${JSON.stringify(state)}`)
  }
}

let browser
let backend
let fixture
let dataRoot
try {
  fixture = await startSourceFixture()
  dataRoot = await mkdtemp(join(tmpdir(), 'openreader-source-debug-'))
  let processOutput = ''
  backend = spawn('go', ['run', '.'], {
    cwd: backendDir,
    env: {
      ...process.env,
      OPENREADER_ADDR: `127.0.0.1:${targetPort}`,
      OPENREADER_DATA_DIR: join(dataRoot, 'data'),
      OPENREADER_CACHE_DIR: join(dataRoot, 'cache'),
      OPENREADER_LIBRARY_DIR: join(dataRoot, 'library'),
      OPENREADER_LOCAL_STORE_DIR: join(dataRoot, 'library', 'localStore'),
      OPENREADER_DB: join(dataRoot, 'data', 'openreader.db'),
      OPENREADER_PUBLIC_DIR: join(rootDir, 'frontend', 'dist'),
      OPENREADER_JWT_SECRET: 'source-debug-browser-contract-secret',
      OPENREADER_SOURCE_NETWORK_ALLOWLIST: '127.0.0.1',
      GOCACHE: join(backendDir, '.gocache'),
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  backend.stdout.on('data', chunk => { processOutput += chunk.toString() })
  backend.stderr.on('data', chunk => { processOutput += chunk.toString() })
  await waitForHealth(() => processOutput)

  const auth = await jsonRequest('/api/auth/register', {
    method: 'POST',
    body: { username: 'debugbrowser', password: 'debugpass123' },
  })
  const source = await jsonRequest('/api/sources', {
    token: auth.token,
    method: 'POST',
    body: sourcePayload(fixture.root),
  })

  browser = await openSmokeBrowser()
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  await context.addInitScript(token => localStorage.setItem('openreader_token', token), auth.token)
  const page = await context.newPage()
  const browserErrors = []
  page.on('pageerror', error => browserErrors.push(String(error?.message || error)))
  const apiOrder = []
  page.on('request', request => {
    const url = new URL(request.url())
    if (url.pathname.includes(`/api/sources/${source.id}`)) {
      apiOrder.push({ method: request.method(), path: url.pathname, search: url.search, authorization: request.headers().authorization || '' })
    }
  })

  await page.goto(`${target}/sources?action=debug&sourceId=${source.id}`, { waitUntil: 'networkidle' })
  await page.locator('.source-debug-workspace').waitFor({ state: 'visible' })
  assert(new URL(page.url()).pathname === '/source-debug', `legacy source-debug route did not translate: ${page.url()}`)
  for (const viewport of [
    { width: 1440, height: 900 },
    { width: 1024, height: 1366 },
    { width: 390, height: 844 },
    { width: 360, height: 800 },
  ]) {
    await assertViewport(page, viewport)
  }

  await page.setViewportSize({ width: 1440, height: 900 })
  await page.getByRole('button', { name: '生成源', exact: true }).click()
  const generatedSource = JSON.parse(await page.locator('.source-debug-output-pane textarea').inputValue())
  assert(generatedSource.bookSourceName === '真实浏览器书源调试', `generated JSON is not reader-dev compatible: ${JSON.stringify(generatedSource)}`)
  assert(generatedSource.bookSourceUrl === fixture.root, 'generated JSON lost the reader-dev source URL')
  assert(generatedSource.ruleSearch?.name === '.name@text', 'generated JSON lost the reader-dev search rule mapping')
  assert(JSON.parse(generatedSource.rules || '{}').contentRule === '.content|text', 'generated JSON lost OpenReader extension rules')
  await page.getByRole('button', { name: '开始调试' }).click()
  await page.locator('.event-end').waitFor({ state: 'visible', timeout: 15_000 })
  const consoleText = await page.locator('.source-debug-console').innerText()
  for (const label of ['搜索', '详情', '目录', '正文', '调试完成']) {
    assert(consoleText.includes(label), `source-debug console missing ${label}: ${consoleText}`)
  }
  for (const secret of ['DO_NOT_LEAK_BROWSER_CONTENT', auth.token, 'source-debug-browser-contract-secret']) {
    assert(!consoleText.includes(secret), `source-debug console leaked ${secret}`)
  }
  const updateIndex = apiOrder.findIndex(item => item.method === 'PUT')
  const streamIndex = apiOrder.findIndex(item => item.method === 'POST' && item.path.endsWith('/debug/stream'))
  assert(updateIndex >= 0 && streamIndex > updateIndex, `debug did not save before stream: ${JSON.stringify(apiOrder)}`)
  const streamRequest = apiOrder[streamIndex]
  assert(streamRequest.search === '', `debug stream leaked auth/query data: ${JSON.stringify(streamRequest)}`)
  assert(streamRequest.authorization === `Bearer ${auth.token}`, 'debug stream did not use Bearer authorization')
  assert(!fixture.requests.some(path => path.startsWith('/chapter/2')), `debug crossed adjacent chapter: ${fixture.requests}`)
  const invalid = await jsonRequest('/api/sources/invalid', { token: auth.token })
  assert(Array.isArray(invalid) && invalid.length === 0, `debug polluted failure cache: ${JSON.stringify(invalid)}`)

  await page.getByRole('tab', { name: '帮助信息' }).click()
  assert(new URL(page.url()).hash === '#tab=help', `debug tab hash was not persisted: ${page.url()}`)
  await page.reload({ waitUntil: 'networkidle' })
  await page.getByText('服务端日志只返回阶段、数量和耗时').waitFor({ state: 'visible' })

  const slowSource = await jsonRequest(`/api/sources/${source.id}`, {
    token: auth.token,
    method: 'PUT',
    body: sourcePayload(fixture.root, '/slow?q={keyword}'),
  })
  assert(slowSource.id === source.id, 'slow source update changed identity')
  await page.reload({ waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '开始调试' }).click()
  await Promise.race([fixture.slowStarted, new Promise((_, reject) => setTimeout(() => reject(new Error('slow source request did not start')), 10_000))])
  await page.getByRole('button', { name: '停止' }).click()
  await Promise.race([fixture.slowAborted, new Promise((_, reject) => setTimeout(() => reject(new Error('source debug cancellation did not abort transport')), 10_000))])
  await page.waitForTimeout(150)
  assert(await page.locator('.event-end').count() === 0, 'canceled source debug emitted a fake end')
  assert(browserErrors.length === 0, `source-debug browser errors: ${browserErrors.join('\n')}`)
  await context.close()

  console.log(JSON.stringify({
    ok: true,
    viewports: ['1440x900', '1024x1366', '390x844', '360x800'],
    apiOrder: apiOrder.map(item => `${item.method} ${item.path}`),
    sourceRequests: fixture.requests,
    cancellation: 'transport-aborted-without-end',
  }, null, 2))
} finally {
  await cleanupWithin(browser?.close().catch(() => {}))
  await stopProcess(backend)
  await cleanupWithin(fixture?.close().catch(() => {}))
  if (dataRoot) await cleanupWithin(rm(dataRoot, { recursive: true, force: true }))
}


// A force-killed Chromium can leave an internal Playwright transport handle
// alive even after every owned child/server has been cleaned up. Reaching this
// line means the complete contract passed and all explicit resources above
// have already received bounded cleanup.
process.exit(0)
