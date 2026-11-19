#!/usr/bin/env node

import assert from 'node:assert/strict'
import { execFile, spawn } from 'node:child_process'
import { randomBytes } from 'node:crypto'
import { mkdtemp, rm } from 'node:fs/promises'
import { createServer } from 'node:http'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { setTimeout as delay } from 'node:timers/promises'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'
import { openSmokeBrowser } from './playwright-runtime.mjs'

const rootDir = join(dirname(fileURLToPath(import.meta.url)), '..', '..')
const backendDir = join(rootDir, 'backend')
const chapter = '.reader-body .chapter-content[data-index="0"]'
const viewports = [
  { width: 1440, height: 900 },
  { width: 390, height: 844 },
  { width: 360, height: 800 },
]

async function listen(server) {
  await new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', resolve)
  })
  return server.address().port
}

function closeServer(server) {
  return new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve()))
}

async function startSource() {
  const books = new Map()
  const server = createServer((request, response) => {
    const [, key, part] = new URL(request.url, 'http://localhost').pathname.split('/')
    const book = books.get(key)
    if (!book || !['book', 'toc', 'chapter'].includes(part)) {
      response.writeHead(404).end()
      return
    }
    if (part === 'chapter') {
      book.fetches += 1
      if (book.failed) {
        response.writeHead(503).end()
        return
      }
    }
    response.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8', 'Cache-Control': 'no-store' })
    if (part === 'book') {
      response.end('<h1 class="title">重载测试书</h1><a class="toc" href="toc">目录</a>')
    } else if (part === 'toc') {
      response.end('<article class="chapter"><span>当前章节</span><a href="chapter">阅读</a></article>')
    } else {
      const blocks = Array.from({ length: 80 }, (_, index) =>
        '<p>' + book.version + ' 第' + (index + 1) + '段 这是用于核对阅读位置的测试正文，两版段落的长度和换行一致。</p>')
      response.end('<main class="content">' + blocks.join('') + '</main>')
    }
  })
  const port = await listen(server)
  return { root: 'http://127.0.0.1:' + port, books, close: () => closeServer(server) }
}

async function startApp() {
  const directory = await mkdtemp(join(tmpdir(), 'openreader-chapter-reload-'))
  let child
  let output = ''
  async function close() {
    if (child?.pid && child.exitCode === null && child.signalCode === null) {
      const exited = new Promise(resolve => child.once('exit', resolve))
      child.kill()
      await Promise.race([exited, delay(5_000, undefined, { ref: false })])
      if (child.exitCode === null && child.signalCode === null) {
        child.kill('SIGKILL')
        await exited
      }
    }
    await rm(directory, { recursive: true, force: true })
  }
  try {
    const binary = process.env.OPENREADER_RELOAD_BIN || join(directory, 'openreader')
    if (!process.env.OPENREADER_RELOAD_BIN) {
      await promisify(execFile)('go', ['build', '-o', binary, '.'], { cwd: backendDir })
    }
    const reservation = createServer()
    const port = await listen(reservation)
    await closeServer(reservation)
    const root = 'http://127.0.0.1:' + port
    child = spawn(binary, [], {
      cwd: backendDir,
      env: {
        ...process.env,
        OPENREADER_ADDR: '127.0.0.1:' + port,
        OPENREADER_DATA_DIR: join(directory, 'data'),
        OPENREADER_CACHE_DIR: join(directory, 'cache'),
        OPENREADER_LIBRARY_DIR: join(directory, 'library'),
        OPENREADER_LOCAL_STORE_DIR: join(directory, 'library', 'localStore'),
        OPENREADER_DB: join(directory, 'data', 'openreader.db'),
        OPENREADER_PUBLIC_DIR: process.env.OPENREADER_RELOAD_PUBLIC_DIR || join(rootDir, 'frontend', 'dist'),
        OPENREADER_JWT_SECRET: randomBytes(32).toString('hex'),
        OPENREADER_SOURCE_NETWORK_ALLOWLIST: '127.0.0.1',
        OPENREADER_CHECK_INTERVAL: '24h',
      },
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    child.stdout.on('data', chunk => { output += chunk })
    child.stderr.on('data', chunk => { output += chunk })
    child.on('error', error => { output += error.message })
    const deadline = Date.now() + 15_000
    while (Date.now() < deadline && child.exitCode === null) {
      const response = await fetch(root + '/api/health').catch(() => null)
      if (response?.ok) return { root, close }
      await delay(100)
    }
    throw new Error('OpenReader test instance did not start:\n' + output)
  } catch (error) {
    await close()
    throw error
  }
}

async function api(root, path, { token, body, method = 'GET' } = {}) {
  const response = await fetch(root + '/api' + path, {
    method,
    headers: {
      ...(token ? { Authorization: 'Bearer ' + token } : {}),
      ...(body ? { 'Content-Type': 'application/json' } : {}),
    },
    body: body && JSON.stringify(body),
  })
  assert.ok(response.ok, method + ' ' + path + ' returned HTTP ' + response.status)
  return response.json()
}

async function seedBook(root, source, fixtureKey, mode) {
  const auth = await api(root, '/auth/register', {
    method: 'POST',
    body: { username: fixtureKey, password: randomBytes(16).toString('hex') },
  })
  assert.ok(auth.token && auth.user?.id, 'isolated reader account did not return user and token')
  await api(root, '/settings/reader', {
    token: auth.token,
    method: 'PUT',
    body: { value: { mode, autoTheme: false } },
  })
  const remote = await api(root, '/sources', {
    token: auth.token,
    method: 'POST',
    body: {
      name: fixtureKey,
      baseUrl: source.root,
      enabled: true,
      rules: JSON.stringify({
        bookInfoNameRule: '.title|text',
        tocUrlRule: '.toc|attr:href',
        chapterListRule: '.chapter',
        chapterNameRule: 'span|text',
        chapterUrlRule: 'a|attr:href',
        contentRule: '.content p|text',
      }),
    },
  })
  const book = await api(root, '/books/remote', {
    token: auth.token,
    method: 'POST',
    body: { title: '重载测试书', bookUrl: source.root + '/' + fixtureKey + '/book', sourceId: remote.id },
  })
  assert.ok(book.id, 'test book was not added to the shelf')
  return { ...auth, book }
}

async function createReaderContext(browser, viewport, auth, mode) {
  const mobile = viewport.width <= 750
  const context = await browser.newContext({ viewport, isMobile: mobile, hasTouch: mobile })
  // Leave each tab's reading position independent while testing shared body storage.
  await context.routeWebSocket(url => new URL(url).pathname === '/ws/sync', () => {})
  await context.addInitScript(({ token, scope, mode }) => {
    localStorage.setItem('openreader_token', token)
    if (!localStorage.getItem('reader')) {
      localStorage.setItem('reader', JSON.stringify({ mode, autoTheme: false, settingsScope: scope, progressScope: scope }))
    }
  }, { token: auth.token, scope: 'user:' + auth.user.id, mode })
  return context
}

async function assertReaderBody(page, mode, version) {
  const body = page.locator('.reader-shell.' + mode).locator(chapter)
  const firstParagraph = body.getByText(/^v[123] 第1段/, { exact: false })
  await firstParagraph.waitFor({ timeout: 10_000 })
  const text = await firstParagraph.textContent()
  assert.ok(text.startsWith(version + ' 第1段'), 'expected body ' + version + ', opened with ' + text.slice(0, 12))
}

async function openReader(context, url, mode, version) {
  const page = await context.newPage()
  await page.goto(url, { waitUntil: 'domcontentloaded' })
  await assertReaderBody(page, mode, version)
  return page
}

async function visibleParagraphIndex(page) {
  return page.evaluate(chapterSelector => {
    const content = document.querySelector('.reader-content')
    const scrollsDocument = document.querySelector('.reader-shell')?.classList.contains('document-scroll')
    const top = (scrollsDocument ? 0 : content.getBoundingClientRect().top) + 36
    const blocks = [...document.querySelectorAll(chapterSelector + ' [data-reader-block]')]
    return blocks.findIndex(block => block.getBoundingClientRect().bottom > top)
  }, chapter)
}

async function positionedReader(context, url, mode) {
  const page = await openReader(context, url + '&offset=1200', mode, 'v1')
  await page.waitForFunction(() => {
    const scroller = document.querySelector('.reader-shell.document-scroll')
      ? document.scrollingElement
      : document.querySelector('.reader-content')
    return scroller?.scrollTop > 100
  }, null, { timeout: 5_000 })
  const before = await visibleParagraphIndex(page)
  assert.ok(before > 3, 'reader did not restore a position beyond the first screen')
  return { page, before }
}

async function newScenario(browser, root, source, viewport, mode) {
  const fixtureKey = 'reload' + viewport.width + randomBytes(4).toString('hex')
  const fixture = { version: 'v1', fetches: 0, failed: false }
  source.books.set(fixtureKey, fixture)
  const auth = await seedBook(root, source, fixtureKey, mode)
  const contentPath = '/books/' + auth.book.id + '/chapters/0/content'
  const matchesContent = url => new URL(url).pathname === '/api' + contentPath
  const mobile = viewport.width <= 750
  const reloadSelector = mobile
    ? '.reader-mobile-float-right.visible button[aria-label="重新载入章节"]'
    : '.reader-right-rail button[aria-label="重新载入章节"]'
  async function press(page, selector) {
    await page.bringToFront()
    const button = page.locator(selector)
    const [response] = await Promise.all([
      page.waitForResponse(response => matchesContent(response.url()) && response.request().method() === 'GET', { timeout: 10_000 }),
      mobile ? button.tap({ timeout: 5_000 }) : button.click({ timeout: 5_000 }),
    ])
    return response
  }
  return {
    fixture,
    auth,
    contentPath,
    matchesContent,
    url: root + '/books/' + auth.book.id + '/read?chapter=0',
    context: await createReaderContext(browser, viewport, auth, mode),
    toolbarReload: page => press(page, reloadSelector),
    retryError: page => press(page, mode === 'scroll2'
      ? chapter + ' .chapter-inline-error button'
      : '.chapter-load-error button'),
  }
}

function assertReloadResponse(response, status, action) {
  const refresh = new URL(response.url()).searchParams.get('refresh')
  assert.equal(response.status(), status, action + ' returned HTTP ' + response.status() + ' (refresh=' + refresh + ')')
  assert.equal(refresh, '1', action + ' did not request explicit refresh')
}

async function assertToolbarReload(page, mode, scenario, version) {
  const fetches = scenario.fixture.fetches
  const response = await scenario.toolbarReload(page)
  assertReloadResponse(response, 200, 'toolbar reload')
  assert.match((await response.json()).content, new RegExp('^' + version + ' 第1段'))
  assert.equal(scenario.fixture.fetches, fetches + 1, 'toolbar reload should request the source once')
  await page.locator('.reader-toast').filter({ hasText: '章节已重新载入' }).waitFor({ timeout: 5_000 })
  await assertReaderBody(page, mode, version)
  assert.equal(new URL(page.url()).searchParams.get('chapter'), '0')
}

async function assertMode(browser, root, source, viewport, mode) {
  const scenario = await newScenario(browser, root, source, viewport, mode)
  const { fixture, auth, contentPath, url, context } = scenario
  try {
    // An equal-body reload supplies a control for the existing position restore.
    const control = await positionedReader(context, url, mode)
    assert.ok(fixture.fetches > 0, 'opening v1 did not warm the server cache')
    await assertToolbarReload(control.page, mode, scenario, 'v1')
    const controlAfter = await visibleParagraphIndex(control.page)
    await control.page.close()

    const { page, before } = await positionedReader(context, url, mode)
    fixture.version = 'v2'
    const cachedReads = fixture.fetches
    const v1 = await api(root, contentPath, { token: auth.token })
    assert.match(v1.content, /^v1 第1段/)
    assert.equal(fixture.fetches, cachedReads, 'ordinary API GET unexpectedly re-fetched the source')
    await assertToolbarReload(page, mode, scenario, 'v2')
    const after = await visibleParagraphIndex(page)
    assert.deepEqual([before, after], [control.before, controlAfter], 'changed text followed a different position restore than equal text')
    await page.close()

    const reopened = await openReader(context, url, mode, 'v2')
    fixture.failed = true
    assertReloadResponse(await scenario.toolbarReload(reopened), 502, 'toolbar reload with failing source')
    const errorSelector = mode === 'scroll2' ? chapter + ' .chapter-inline-error' : '.chapter-load-error'
    await reopened.locator('.reader-shell.' + mode + ' ' + errorSelector).waitFor({ timeout: 5_000 })
    assertReloadResponse(await scenario.retryError(reopened), 502, 'chapter error retry with failing source')
    await reopened.locator(errorSelector).waitFor({ timeout: 5_000 })
    assert.equal(await reopened.locator('.reader-toast').filter({ hasText: '章节已重新载入' }).count(), 0)
    await (await openReader(context, url, mode, 'v2')).close()

    fixture.failed = false
    fixture.version = 'v3'
    assertReloadResponse(await scenario.retryError(reopened), 200, 'chapter error retry after source recovery')
    await assertReaderBody(reopened, mode, 'v3')
    assert.equal(new URL(reopened.url()).searchParams.get('chapter'), '0')
    await reopened.close()
    await (await openReader(context, url, mode, 'v3')).close()

    const clean = await createReaderContext(browser, viewport, auth, mode)
    try {
      await openReader(clean, url, mode, 'v1')
      assert.equal(fixture.fetches, cachedReads + 4, 'ordinary reopens must use the last cached response')
    } finally {
      await clean.close()
    }
    console.log(viewport.width + 'x' + viewport.height + ' ' + mode + ': v1 cache, v2 reload, paragraph '
      + (before + 1) + ' → ' + (after + 1) + ' (equal-body control '
      + (control.before + 1) + ' → ' + (controlAfter + 1) + '), failed retry, v3 recovery, cached reopens passed')
  } finally {
    await context.close()
  }
}

async function assertTabWriteOrder(browser, root, source, viewport) {
  const mode = 'page'
  const scenario = await newScenario(browser, root, source, viewport, mode)
  const { fixture, auth, contentPath, matchesContent, url, context } = scenario
  const { promise: hold, resolve: release } = Promise.withResolvers()
  const { promise: captured, resolve: capture, reject: rejectCapture } = Promise.withResolvers()
  try {
    await api(root, contentPath, { token: auth.token })
    const olderTab = await context.newPage()
    await olderTab.route(matchesContent, async route => {
      if (new URL(route.request().url()).searchParams.has('refresh')) {
        await route.continue()
        return
      }
      try {
        const response = await route.fetch()
        assert.equal(response.status(), 200)
        assert.match((await response.json()).content, /^v1 第1段/)
        capture()
        await hold
        await route.fulfill({ response })
      } catch (error) {
        rejectCapture(error)
        await route.abort().catch(() => {})
      }
    })
    await olderTab.goto(url, { waitUntil: 'domcontentloaded' })
    await Promise.race([captured, delay(5_000, undefined, { ref: false }).then(() => {
      throw new Error('older tab did not capture its ordinary v1 response')
    })])
    const refreshTab = await openReader(context, url, mode, 'v1')
    fixture.version = 'v2'
    await assertToolbarReload(refreshTab, mode, scenario, 'v2')
    release()
    await assertReaderBody(olderTab, mode, 'v1')
    await openReader(context, url, mode, 'v2')
    assert.equal(fixture.fetches, 2, 'cross-tab scenario should fetch v1 once and v2 once')
    console.log(viewport.width + 'x' + viewport.height + ' tabs: paused v1 write, v2 reload, released v1, new tab kept v2')
  } finally {
    release()
    await context.close()
  }
}

async function main() {
  const source = await startSource()
  let app
  let browser
  try {
    app = await startApp()
    browser = await openSmokeBrowser()
    let failures = 0
    for (const viewport of viewports) {
      const scenarios = [
        ['page', () => assertMode(browser, app.root, source, viewport, 'page')],
        ['scroll2', () => assertMode(browser, app.root, source, viewport, 'scroll2')],
        ['tabs', () => assertTabWriteOrder(browser, app.root, source, viewport)],
      ]
      for (const [label, run] of scenarios) {
        try {
          await run()
        } catch (error) {
          failures += 1
          console.error(viewport.width + 'x' + viewport.height + ' ' + label + ': ' + error.stack)
        }
      }
    }
    if (failures) process.exitCode = 1
  } finally {
    await browser?.close()
    await app?.close()
    await source.close()
  }
}

main().catch(error => {
  console.error(error.stack)
  process.exitCode = 1
})
