#!/usr/bin/env node

import { access, mkdtemp, rm } from 'node:fs/promises'
import { createServer } from 'node:http'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'
import { execFile, spawn } from 'node:child_process'
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
  assert(address && typeof address === 'object', 'unable to reserve a local port')
  const port = address.port
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
    await new Promise(resolve => setTimeout(resolve, 250))
  }
  throw new Error(`bookshelf multi-client server did not start: ${lastError?.message || 'unknown'}\n${processOutput()}`)
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

async function startOpenReader() {
  await access(join(publicDir, 'index.html')).catch(() => {
    throw new Error('frontend/dist is missing; run `cd frontend && npm run build` first')
  })
  const tempRoot = await mkdtemp(join(tmpdir(), 'openreader-bookshelf-multiclient-'))
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
      OPENREADER_JWT_SECRET: 'bookshelf-multiclient-contract-secret',
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
    output: () => output,
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

function collectPageErrors(page, label) {
  const errors = []
  page.on('pageerror', error => errors.push(`${label} pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error' && !message.text().includes('favicon')) {
      errors.push(`${label} console.error: ${message.text()}`)
    }
  })
  page.on('response', response => {
    if (response.status() >= 500 && response.url().includes('/api/')) {
      errors.push(`${label} api ${response.status()}: ${response.url()}`)
    }
  })
  return errors
}

async function waitForSync(page) {
  await page.waitForFunction(() => document.body?.innerText.includes('同步在线'), null, { timeout: 10_000 })
}

async function importTXTFromClient(page, token, title) {
  const result = await page.evaluate(async ({ tokenValue, bookTitle }) => {
    const form = new FormData()
    const content = `第一章 ${bookTitle}\n这是 ${bookTitle} 的正文。`
    form.append('file', new File([content], `${bookTitle}.txt`, { type: 'text/plain' }))
    form.append('title', bookTitle)
    const response = await fetch('/api/imports/books', {
      method: 'POST',
      headers: { Authorization: `Bearer ${tokenValue}` },
      body: form,
    })
    return { ok: response.ok, status: response.status, text: await response.text() }
  }, { tokenValue: token, bookTitle: title })
  assert(result.ok, `client import failed with ${result.status}: ${result.text}`)
  return JSON.parse(result.text)
}

async function runViewport(browser, root, viewport) {
  const suffix = `${viewport.width}${viewport.height}`
  const registered = await api(root, '/auth/register', {
    method: 'POST',
    body: { username: `shelfsync${suffix}`, password: 'shelf-sync-contract' },
  })
  const token = registered?.token
  assert(token, `${suffix}: registration did not return a token`)
  const firstTitle = `缓存基线书 ${suffix}`
  const secondTitle = `冷启动新书 ${suffix}`
  const syncedTitle = `跨客户端导入 ${suffix}`
  await api(root, '/books', {
    token,
    method: 'POST',
    body: { title: firstTitle, author: 'OpenReader', url: `local://shelf/${suffix}/first`, sourceId: 0 },
  })

  const contextA = await browser.newContext({ viewport, isMobile: viewport.width <= 750, hasTouch: viewport.width <= 750 })
  const contextB = await browser.newContext({ viewport, isMobile: viewport.width <= 750, hasTouch: viewport.width <= 750 })
  await contextA.addInitScript(value => localStorage.setItem('openreader_token', value), token)
  await contextB.addInitScript(value => localStorage.setItem('openreader_token', value), token)
  let pageA
  let pageB
  let releaseBooks
  const errorLists = []
  try {
    pageA = await contextA.newPage()
    pageB = await contextB.newPage()
    errorLists.push(collectPageErrors(pageA, `${suffix}/A`), collectPageErrors(pageB, `${suffix}/B`))
    await Promise.all([
      pageA.goto(root, { waitUntil: 'networkidle' }),
      pageB.goto(root, { waitUntil: 'networkidle' }),
    ])
    await Promise.all([
      pageA.getByText(firstTitle, { exact: true }).first().waitFor(),
      pageB.getByText(firstTitle, { exact: true }).first().waitFor(),
      waitForSync(pageA),
      waitForSync(pageB),
    ])
    await pageB.waitForTimeout(250)
    await pageB.close()

    await api(root, '/books', {
      token,
      method: 'POST',
      body: { title: secondTitle, author: 'OpenReader', url: `local://shelf/${suffix}/second`, sourceId: 0 },
    })

    pageB = await contextB.newPage()
    errorLists.push(collectPageErrors(pageB, `${suffix}/B-reload`))
    let markBooksRequested
    const booksRequested = new Promise(resolve => { markBooksRequested = resolve })
    const booksBlocked = new Promise(resolve => { releaseBooks = resolve })
    await pageB.route('**/api/books', async (route) => {
      if (route.request().method() !== 'GET') return route.continue()
      markBooksRequested()
      await booksBlocked
      try {
        await route.continue()
      } catch {
        // A superseded force request may be cancelled during teardown.
      }
    })
    await pageB.goto(root, { waitUntil: 'domcontentloaded' })
    await booksRequested
    await pageB.waitForTimeout(150)
    assert(await pageB.getByText(firstTitle, { exact: true }).count() === 0, `${suffix}: stale persistent shelf rendered before the delayed network response`)
    releaseBooks()
    await pageB.getByText(secondTitle, { exact: true }).first().waitFor({ timeout: 10_000 })
    await waitForSync(pageB)

    await importTXTFromClient(pageA, token, syncedTitle)
    await pageB.getByText(syncedTitle, { exact: true }).first().waitFor({ timeout: 10_000 })

    const geometry = await pageB.evaluate(() => ({
      bodyWidth: document.body.scrollWidth,
      viewportWidth: window.innerWidth,
      count: document.querySelectorAll('.book-row, .book-card').length,
    }))
    assert(geometry.bodyWidth <= geometry.viewportWidth + 1, `${suffix}: shelf introduced horizontal overflow`)
    assert(geometry.count >= 3, `${suffix}: expected all three authoritative shelf rows`)
    const errors = errorLists.flat()
    assert(errors.length === 0, errors.join('\n'))
    console.log(`${viewport.width}x${viewport.height}: network-first cold load + live same-user import sync ok`)
  } finally {
    releaseBooks?.()
    await contextA.close()
    await contextB.close()
  }
}

const app = await startOpenReader()
const browser = await openSmokeBrowser()
try {
  await runViewport(browser, app.root, { width: 1440, height: 900 })
  await runViewport(browser, app.root, { width: 390, height: 844 })
  await runViewport(browser, app.root, { width: 360, height: 800 })
} catch (error) {
  error.message = `${error.message}\nOpenReader output:\n${app.output()}`
  throw error
} finally {
  await browser.close()
  await app.close()
}
