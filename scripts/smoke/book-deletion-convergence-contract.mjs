#!/usr/bin/env node

import { execFile, spawn } from 'node:child_process'
import { access, mkdtemp, rm } from 'node:fs/promises'
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

async function reserveLocalPort() {
  const server = createServer()
  await new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', resolve)
  })
  const address = server.address()
  assert(address && typeof address === 'object', 'unable to reserve deletion-smoke port')
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
  let lastError
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
  throw new Error(`deletion-smoke server did not start: ${lastError?.message || 'unknown'}\n${output()}`)
}

async function startOpenReader() {
  await access(join(publicDir, 'index.html')).catch(() => {
    throw new Error('frontend/dist is missing; run `cd frontend && npm run build` first')
  })
  const tempRoot = await mkdtemp(join(tmpdir(), 'openreader-book-deletion-'))
  const dataDir = join(tempRoot, 'data')
  const binary = join(tempRoot, 'openreader')
  const port = await reserveLocalPort()
  await execFileAsync('go', ['build', '-o', binary, '.'], {
    cwd: backendDir,
    env: process.env,
    maxBuffer: 4 * 1024 * 1024,
  })
  let processOutput = ''
  const child = spawn(binary, [], {
    cwd: backendDir,
    env: {
      ...process.env,
      OPENREADER_ADDR: `127.0.0.1:${port}`,
      OPENREADER_DATA_DIR: dataDir,
      OPENREADER_CACHE_DIR: join(tempRoot, 'cache'),
      OPENREADER_LIBRARY_DIR: join(tempRoot, 'library'),
      OPENREADER_LOCAL_STORE_DIR: join(tempRoot, 'library', 'localStore'),
      OPENREADER_DB: join(dataDir, 'openreader.db'),
      OPENREADER_PUBLIC_DIR: publicDir,
      OPENREADER_JWT_SECRET: 'book-deletion-convergence-contract-secret',
      OPENREADER_CORS_ORIGIN: `http://127.0.0.1:${port}`,
      OPENREADER_CHECK_INTERVAL: '24h',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  child.stdout.on('data', chunk => { processOutput += chunk.toString() })
  child.stderr.on('data', chunk => { processOutput += chunk.toString() })
  const root = `http://127.0.0.1:${port}`
  try {
    await waitForHealth(root, () => processOutput)
  } catch (error) {
    await stopProcess(child)
    await rm(tempRoot, { recursive: true, force: true })
    throw error
  }
  return {
    root,
    output: () => processOutput,
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
  assert(response.ok, `${method} ${path} failed with ${response.status}: ${JSON.stringify(data)}`)
  return data
}

async function importTXT(root, token, title) {
  const paragraphs = Array.from(
    { length: 24 },
    (_, index) => `第 ${index + 1} 段：删除收敛合同正文，用于确认阅读器退出时不会重新写回已经删除的阅读进度。`,
  )
  const form = new FormData()
  form.append(
    'file',
    new Blob([`第一章 ${title}\n${paragraphs.join('\n')}`], { type: 'text/plain' }),
    `${title}.txt`,
  )
  form.append('title', title)
  const response = await fetch(`${root}/api/imports/books`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: form,
  })
  const text = await response.text()
  assert(response.ok, `TXT import failed with ${response.status}: ${text}`)
  const book = JSON.parse(text)
  const chapters = await api(root, `/books/${book.id}/chapters`, { token })
  assert(chapters.length > 0, `${title}: imported catalogue is empty`)
  return { book, chapter: chapters[0] }
}

function collectFailures(page, label) {
  const failures = []
  const progressRequests = []
  page.on('pageerror', error => failures.push(`${label} pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error' && !message.text().includes('favicon')) {
      failures.push(`${label} console.error: ${message.text()}`)
    }
  })
  page.on('response', response => {
    if (response.status() >= 400 && response.url().includes('/api/')) {
      failures.push(`${label} api ${response.status()}: ${response.request().method()} ${response.url()}`)
    }
  })
  page.on('request', request => {
    if (request.method() === 'PUT' && new URL(request.url()).pathname === '/api/progress') {
      progressRequests.push(request.url())
    }
  })
  return { failures, progressRequests }
}

async function newClient(browser, root, token, viewport, label) {
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 750,
    hasTouch: viewport.width <= 750,
  })
  await context.addInitScript((tokenValue) => {
    localStorage.setItem('openreader_token', tokenValue)
  }, token)
  const page = await context.newPage()
  const observed = collectFailures(page, label)
  await page.goto(root, { waitUntil: 'domcontentloaded' })
  await page.getByText(/书架 \(\d+\)/).first().waitFor({ timeout: 15_000 })
  return { context, page, ...observed }
}

async function deleteFromClient(page, bookId) {
  const result = await page.evaluate(async (targetBookId) => {
    const token = localStorage.getItem('openreader_token') || ''
    const response = await fetch(`/api/books/${targetBookId}`, {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${token}` },
    })
    return { ok: response.ok, status: response.status, text: await response.text() }
  }, bookId)
  assert(result.ok && result.status === 204, `browser delete failed: ${JSON.stringify(result)}`)
}

async function waitForReader(page) {
  await page.locator('.reader-content .chapter-content').first().waitFor({ timeout: 15_000 })
  await page.waitForFunction(() => {
    const content = document.querySelector('.reader-content .chapter-content')
    return content && !document.body.innerText.includes('正在加载章节')
  }, null, { timeout: 15_000 })
}

async function waitForLocalProgress(page, bookId) {
  await page.waitForFunction((targetBookId) => Object.keys(localStorage).some(key => (
    key.startsWith('openreader_chapter_progress@') && key.endsWith(`@${targetBookId}`)
  )), bookId, { timeout: 10_000 })
}

async function assertLocalProgressRemoved(page, bookId, label) {
  const remains = await page.evaluate((targetBookId) => Object.keys(localStorage).filter(key => (
    key.startsWith('openreader_chapter_progress@') && key.endsWith(`@${targetBookId}`)
  )), bookId)
  assert(remains.length === 0, `${label}: deleted local progress remained: ${remains.join(', ')}`)
}

async function assertNoOverflow(page, viewport, label) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  assert(overflow <= 1, `${label}: horizontal overflow ${overflow}px at ${viewport.width}x${viewport.height}`)
}

async function runViewport(browser, app, token, viewport) {
  const suffix = `${viewport.width}x${viewport.height}`
  const clientA = await newClient(browser, app.root, token, viewport, `${suffix}/A`)
  const clientB = await newClient(browser, app.root, token, viewport, `${suffix}/B`)
  try {
    const infoFixture = await importTXT(app.root, token, `删除信息 ${suffix}`)
    await clientA.page.reload({ waitUntil: 'domcontentloaded' })
    const infoRow = clientA.page.locator('.book-row').filter({ hasText: infoFixture.book.title }).first()
    await infoRow.waitFor({ timeout: 15_000 })
    await infoRow.locator('.list-cover').click()
    const infoDialog = clientA.page.locator('.book-info-dialog')
    await infoDialog.waitFor({ state: 'visible', timeout: 10_000 })
    await deleteFromClient(clientB.page, infoFixture.book.id)
    await infoDialog.waitFor({ state: 'hidden', timeout: 10_000 })
    await infoRow.waitFor({ state: 'hidden', timeout: 10_000 })

    const bookmarkFixture = await importTXT(app.root, token, `删除书签 ${suffix}`)
    await api(app.root, '/progress', {
      token,
      method: 'PUT',
      body: {
        bookId: bookmarkFixture.book.id,
        chapterId: bookmarkFixture.chapter.id,
        chapterIndex: bookmarkFixture.chapter.index,
        offset: 18,
        percent: 0.1,
        chapterPercent: 0.1,
        clientId: `delete-bookmark-${suffix}`,
      },
    })
    await clientA.page.goto(`${app.root}/books/${bookmarkFixture.book.id}/read?resume=1`, { waitUntil: 'domcontentloaded' })
    await waitForReader(clientA.page)
    await waitForLocalProgress(clientA.page, bookmarkFixture.book.id)
    await clientA.page.locator('button[title="书签"]').click()
    const bookmarkDialog = clientA.page.locator('.global-bookmark-dialog')
    await bookmarkDialog.waitFor({ state: 'visible', timeout: 10_000 })
    clientA.progressRequests.length = 0
    await deleteFromClient(clientB.page, bookmarkFixture.book.id)
    await clientA.page.waitForFunction(() => window.location.pathname === '/', null, { timeout: 10_000 })
    await bookmarkDialog.waitFor({ state: 'hidden', timeout: 10_000 })
    await clientA.page.waitForTimeout(700)
    await assertLocalProgressRemoved(clientA.page, bookmarkFixture.book.id, `${suffix}/bookmark`)
    assert(clientA.progressRequests.length === 0, `${suffix}: Reader wrote progress after deletion: ${clientA.progressRequests.join(', ')}`)

    const searchFixture = await importTXT(app.root, token, `删除搜索 ${suffix}`)
    await clientA.page.goto(`${app.root}/books/${searchFixture.book.id}/read`, { waitUntil: 'domcontentloaded' })
    await waitForReader(clientA.page)
    await clientA.page.locator('button[title="搜索正文"]').click()
    const searchDialog = clientA.page.locator('.global-content-search-dialog')
    await searchDialog.waitFor({ state: 'visible', timeout: 10_000 })
    clientA.progressRequests.length = 0
    await deleteFromClient(clientB.page, searchFixture.book.id)
    await clientA.page.waitForFunction(() => window.location.pathname === '/', null, { timeout: 10_000 })
    await searchDialog.waitFor({ state: 'hidden', timeout: 10_000 })
    await clientA.page.waitForTimeout(700)
    assert(clientA.progressRequests.length === 0, `${suffix}: search-open Reader wrote progress after deletion`)

    await assertNoOverflow(clientA.page, viewport, `${suffix}/A`)
    await assertNoOverflow(clientB.page, viewport, `${suffix}/B`)
    const failures = [...clientA.failures, ...clientB.failures]
    assert(failures.length === 0, failures.join('\n'))
    console.log(`${suffix}: BookInfo + bookmarks + content-search + Reader deletion convergence ok`)
  } finally {
    await clientA.context.close()
    await clientB.context.close()
  }
}

const app = await startOpenReader()
try {
  const browser = await openSmokeBrowser()
  try {
    const registered = await api(app.root, '/auth/register', {
      method: 'POST',
      body: { username: 'deletebrowseradmin', password: 'delete-browser-contract' },
    })
    const token = registered?.token
    assert(token, 'registration did not return a token')
    for (const viewport of [
      { width: 1440, height: 900 },
      { width: 390, height: 844 },
      { width: 360, height: 800 },
    ]) {
      await runViewport(browser, app, token, viewport)
    }
  } finally {
    await browser.close()
  }
} catch (error) {
  error.message = `${error.message}\nOpenReader output:\n${app.output()}`
  throw error
} finally {
  await app.close()
}
