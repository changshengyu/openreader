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
  throw new Error(`bookshelf refresh server did not start: ${lastError?.message || 'unknown'}\n${processOutput()}`)
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
  const tempRoot = await mkdtemp(join(tmpdir(), 'openreader-bookshelf-refresh-'))
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
      OPENREADER_JWT_SECRET: 'bookshelf-refresh-progress-contract-secret',
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

async function importProgressFixture(root, token, title) {
  const form = new FormData()
  const content = [
    '第一章 起点',
    '这是第一章正文。',
    '第二章 过渡',
    '这是第二章正文。',
    '第三章 服务器新位置',
    '这是第三章正文。',
  ].join('\n')
  form.append('file', new Blob([content], { type: 'text/plain' }), `${title}.txt`)
  form.append('title', title)
  const response = await fetch(`${root}/api/imports/books`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: form,
  })
  const text = await response.text()
  if (!response.ok) throw new Error(`fixture import failed with ${response.status}: ${text}`)
  return JSON.parse(text)
}

function userIDFromToken(token) {
  const payload = JSON.parse(Buffer.from(token.split('.')[1], 'base64url').toString('utf8'))
  return payload.userId || payload.sub
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

async function runViewport(browser, root, viewport) {
  const suffix = `${viewport.width}${viewport.height}`
  const registered = await api(root, '/auth/register', {
    method: 'POST',
    body: { username: `shelfprogress${suffix}${process.pid}`, password: 'shelf-progress-contract' },
  })
  const token = registered?.token
  assert(token, `${suffix}: registration did not return a token`)
  const userID = userIDFromToken(token)
  const title = `刷新进度测试 ${suffix}`
  const book = await importProgressFixture(root, token, title)
  const chapters = await api(root, `/books/${book.id}/chapters`, { token })
  assert(Array.isArray(chapters) && chapters.length >= 3, `${suffix}: fixture did not produce three chapters`)

  const initial = await api(root, '/progress', {
    token,
    method: 'PUT',
    body: {
      bookId: book.id,
      chapterId: chapters[0].id,
      chapterIndex: 0,
      offset: 0,
      percent: 0.05,
      chapterPercent: 0.15,
      chapterTitle: chapters[0].title,
      clientUpdatedAt: new Date().toISOString(),
      clientId: `initial-${suffix}`,
    },
  })

  const context = await browser.newContext({ viewport, isMobile: viewport.width <= 750, hasTouch: viewport.width <= 750 })
  await context.addInitScript(({ tokenValue }) => {
    localStorage.setItem('openreader_token', tokenValue)
    class SilentWebSocket {
      static CONNECTING = 0
      static OPEN = 1
      static CLOSING = 2
      static CLOSED = 3
      constructor(url) {
        this.url = url
        this.readyState = SilentWebSocket.CONNECTING
      }
      addEventListener() {}
      removeEventListener() {}
      close() { this.readyState = SilentWebSocket.CLOSED }
      send() {}
    }
    window.WebSocket = SilentWebSocket
  }, { tokenValue: token })

  const page = await context.newPage()
  const errors = collectPageErrors(page, suffix)
  let navigationCount = 0
  page.on('framenavigated', frame => {
    if (frame === page.mainFrame()) navigationCount += 1
  })
  try {
    await page.goto(root, { waitUntil: 'networkidle' })
    await page.getByText(title, { exact: true }).first().waitFor()
    await page.getByText(`已读：${chapters[0].title}`, { exact: true }).waitFor()
    const navigationBaseline = navigationCount

    const localKey = `openreader_chapter_progress@user:${userID}@${book.id}`
    await page.evaluate(({ key, progress }) => {
      localStorage.setItem(key, JSON.stringify(progress))
    }, {
      key: localKey,
      progress: {
        ...initial,
        chapterId: chapters[0].id,
        chapterIndex: 0,
        chapterTitle: chapters[0].title,
        updatedAt: '2099-07-22T00:00:00Z',
      },
    })

    const serverProgress = await api(root, '/progress', {
      token,
      method: 'PUT',
      body: {
        bookId: book.id,
        chapterId: chapters[2].id,
        chapterIndex: 2,
        offset: 21,
        percent: 0.8,
        chapterPercent: 0.4,
        chapterTitle: chapters[2].title,
        baseUpdatedAt: initial.updatedAt,
        clientUpdatedAt: new Date().toISOString(),
        clientId: `remote-${suffix}`,
      },
    })
    assert(serverProgress.chapterIndex === 2, `${suffix}: server did not commit the remote position`)
    assert(await page.getByText(`已读：${chapters[2].title}`, { exact: true }).count() === 0, `${suffix}: silent client received an unexpected live update`)

    await page.getByRole('button', { name: '刷新', exact: true }).click()
    await page.getByText(`已读：${chapters[2].title}`, { exact: true }).waitFor({ timeout: 10_000 })
    assert(navigationCount === navigationBaseline, `${suffix}: refresh reloaded or replaced the document`)

    const stored = await page.evaluate(key => JSON.parse(localStorage.getItem(key) || 'null'), localKey)
    assert(stored?.chapterIndex === 2, `${suffix}: scoped local progress did not converge to the server snapshot`)
    assert(stored?.updatedAt === serverProgress.updatedAt, `${suffix}: future-dated stale progress still owns localStorage`)
    const geometry = await page.evaluate(() => ({ body: document.body.scrollWidth, viewport: window.innerWidth }))
    assert(geometry.body <= geometry.viewport + 1, `${suffix}: shelf refresh introduced horizontal overflow`)
    assert(errors.length === 0, errors.join('\n'))
    console.log(`${viewport.width}x${viewport.height}: refresh replaced future-dated cached progress without page reload`)
  } finally {
    await context.close()
  }
}

const targetRoot = String(process.env.TARGET_URL || '').replace(/\/$/, '')
const app = targetRoot
  ? { root: targetRoot, output: () => '', close: async () => {} }
  : await startOpenReader()
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
