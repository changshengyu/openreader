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
      await exited
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

async function seedBook(root, source, fixtureKey) {
  const auth = await api(root, '/auth/register', {
    method: 'POST',
    body: { username: fixtureKey, password: randomBytes(16).toString('hex') },
  })
  assert.ok(auth.token && auth.user?.id, 'isolated reader account did not return user and token')
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

async function createReaderContext(browser, viewport, auth) {
  const mobile = viewport.width <= 750
  const context = await browser.newContext({ viewport, isMobile: mobile, hasTouch: mobile })
  await context.addInitScript(({ token, scope }) => {
    localStorage.setItem('openreader_token', token)
    if (!localStorage.getItem('reader')) {
      localStorage.setItem('reader', JSON.stringify({ mode: 'scroll2', settingsScope: scope, progressScope: scope }))
    }
  }, { token: auth.token, scope: 'user:' + auth.user.id })
  return context
}

async function openReader(context, url, version) {
  const page = await context.newPage()
  await page.goto(url, { waitUntil: 'domcontentloaded' })
  await page.locator(chapter).getByText(version + ' 第1段', { exact: false }).waitFor({ timeout: 10_000 })
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

async function assertViewport(browser, root, source, viewport) {
  const fixtureKey = 'reload' + viewport.width + randomBytes(4).toString('hex')
  const fixture = { version: 'v1', fetches: 0, failed: false }
  source.books.set(fixtureKey, fixture)
  const auth = await seedBook(root, source, fixtureKey)
  const contentPath = '/books/' + auth.book.id + '/chapters/0/content'
  const isContentResponse = response => new URL(response.url()).pathname === '/api' + contentPath
    && response.request().method() === 'GET'
  const mobile = viewport.width <= 750
  const reloadSelector = mobile
    ? '.reader-mobile-float-right.visible button[aria-label="重新载入章节"]'
    : '.reader-right-rail button[aria-label="重新载入章节"]'
  const url = root + '/books/' + auth.book.id + '/read?chapter=0'
  const context = await createReaderContext(browser, viewport, auth)
  try {
    const page = await openReader(context, url + '&offset=1200', 'v1')
    await page.waitForFunction(() => {
      const shell = document.querySelector('.reader-shell')
      const scroller = shell?.classList.contains('document-scroll')
        ? document.scrollingElement
        : document.querySelector('.reader-content')
      return scroller?.scrollTop > 100
    }, null, { timeout: 5_000 })
    const before = await visibleParagraphIndex(page)
    assert.ok(before > 3, 'reader did not restore a position beyond the first screen')

    fixture.version = 'v2'
    const cachedReads = fixture.fetches
    assert.ok(cachedReads > 0, 'opening v1 did not warm the server cache')
    const v1 = await api(root, contentPath, { token: auth.token })
    assert.match(v1.content, /v1 第1段/)
    assert.equal(fixture.fetches, cachedReads, 'ordinary API GET unexpectedly re-fetched the source')

    const reload = page.locator(reloadSelector)
    const [refreshed] = await Promise.all([
      page.waitForResponse(isContentResponse, { timeout: 10_000 }),
      mobile ? reload.tap() : reload.click(),
    ])
    assert.equal(refreshed.status(), 200)
    const result = await refreshed.json()
    assert.ok(result.content.includes('v2 第1段'),
      'toolbar reload returned stale v1 (refresh=' + new URL(refreshed.url()).searchParams.get('refresh')
      + ', source fetches during reload=' + (fixture.fetches - cachedReads) + ')')
    assert.equal(new URL(refreshed.url()).searchParams.get('refresh'), '1')
    assert.equal(fixture.fetches, cachedReads + 1, 'toolbar reload should request the source once')
    await page.locator('.reader-toast').filter({ hasText: '章节已重新载入' }).waitFor({ timeout: 5_000 })
    await page.locator(chapter).getByText('v2 第1段', { exact: false }).waitFor({ timeout: 5_000 })
    assert.equal(new URL(page.url()).searchParams.get('chapter'), '0')
    const after = await visibleParagraphIndex(page)
    assert.ok(Math.abs(after - before) <= 1, 'visible paragraph changed from ' + before + ' to ' + after)
    await page.close()

    const reopened = await openReader(context, url, 'v2')
    fixture.failed = true
    const failedReload = reopened.locator(reloadSelector)
    const [failure] = await Promise.all([
      reopened.waitForResponse(isContentResponse, { timeout: 10_000 }),
      mobile ? failedReload.tap() : failedReload.click(),
    ])
    assert.equal(failure.status(), 502, 'source failure should surface as a failed reload')
    await reopened.locator('.chapter-inline-error, .chapter-load-error').waitFor({ timeout: 5_000 })
    assert.equal(await reopened.locator('.reader-toast').filter({ hasText: '章节已重新载入' }).count(), 0)
    const recovered = await reopened.reload({ waitUntil: 'domcontentloaded' })
    assert.equal(recovered.status(), 200)
    await reopened.locator(chapter).getByText('v2 第1段', { exact: false }).waitFor({ timeout: 5_000 })
    assert.equal(fixture.fetches, cachedReads + 2, 'ordinary reopens must preserve the browser cache after failure')

    const clean = await createReaderContext(browser, viewport, auth)
    try {
      await openReader(clean, url, 'v1')
      assert.equal(fixture.fetches, cachedReads + 2, 'fresh browser should read the original server cache')
    } finally {
      await clean.close()
    }
    console.log(viewport.width + 'x' + viewport.height + ': v1 cache, v2 reload, paragraph '
      + (before + 1) + ' → ' + (after + 1) + ', failed reload, cached reopens passed')
  } finally {
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
      try {
        await assertViewport(browser, app.root, source, viewport)
      } catch (error) {
        failures += 1
        console.error(viewport.width + 'x' + viewport.height + ': ' + error.stack)
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
