#!/usr/bin/env node

import { execFile, spawn } from 'node:child_process'
import {
  access,
  mkdir,
  mkdtemp,
  readFile,
  readdir,
  rm,
} from 'node:fs/promises'
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
  throw new Error(`reader progress server did not start: ${lastError?.message || 'unknown'}\n${processOutput()}`)
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
  const tempRoot = await mkdtemp(join(tmpdir(), 'openreader-reader-progress-'))
  const dataDir = join(tempRoot, 'data')
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
      OPENREADER_DATA_DIR: dataDir,
      OPENREADER_CACHE_DIR: join(tempRoot, 'cache'),
      OPENREADER_LIBRARY_DIR: join(tempRoot, 'library'),
      OPENREADER_LOCAL_STORE_DIR: join(tempRoot, 'library', 'localStore'),
      OPENREADER_DB: join(dataDir, 'openreader.db'),
      OPENREADER_PUBLIC_DIR: publicDir,
      OPENREADER_JWT_SECRET: 'reader-progress-multiclient-contract-secret',
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
    dataDir,
    output: () => output,
    close: async () => {
      await stopProcess(child)
      await rm(tempRoot, { recursive: true, force: true })
    },
  }
}

async function request(root, path, { token = '', method = 'GET', body } = {}) {
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
  return {
    ok: response.ok,
    status: response.status,
    data,
    conflict: response.headers.get('x-openreader-progress-conflict') === '1',
  }
}

async function api(root, path, options = {}) {
  const response = await request(root, path, options)
  if (!response.ok) {
    throw new Error(`${options.method || 'GET'} ${path} failed with ${response.status}: ${JSON.stringify(response.data)}`)
  }
  return response.data
}

async function importTXT(root, token, title) {
  const paragraphs = Array.from(
    { length: 90 },
    (_, index) => `第 ${index + 1} 段：这是用于验证双客户端阅读进度恢复的长正文。浏览器必须保存服务器确认的位置，而不是停留在各自的乐观缓存。`,
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
  return JSON.parse(text)
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

async function newReaderContext(browser, root, token, viewport, bookID, label) {
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 750,
    hasTouch: viewport.width <= 750,
  })
  await context.addInitScript((tokenValue) => {
    localStorage.setItem('openreader_token', tokenValue)
    window.__openReaderProgressEvents = []
    window.addEventListener('openreader:progress-updated', (event) => {
      window.__openReaderProgressEvents.push(JSON.parse(JSON.stringify(event.detail?.progress || null)))
    })
  }, token)
  const page = await context.newPage()
  const errors = collectPageErrors(page, label)
  await page.goto(`${root}/books/${bookID}/read?resume=1`, { waitUntil: 'domcontentloaded' })
  await page.locator('.reader-content .chapter-content').first().waitFor({ timeout: 15_000 })
  await page.waitForFunction(() => {
    const content = document.querySelector('.reader-content .chapter-content')
    return content && !document.body.innerText.includes('正在加载章节')
  }, null, { timeout: 15_000 })
  await page.waitForTimeout(450)
  return { context, page, errors }
}

async function waitForEvent(page, bookID, offset) {
  await page.waitForFunction(({ targetBookID, targetOffset }) => (
    window.__openReaderProgressEvents?.some(progress => (
      Number(progress?.bookId) === Number(targetBookID)
      && Number(progress?.offset) === Number(targetOffset)
    ))
  ), { targetBookID: bookID, targetOffset: offset }, { timeout: 10_000 })
}

async function waitForReaderRoute(page, offset) {
  await page.waitForFunction(targetOffset => (
    Number(new URL(window.location.href).searchParams.get('offset')) === Number(targetOffset)
  ), offset, { timeout: 10_000 })
}

async function localProgress(page, bookID) {
  return page.evaluate((targetBookID) => {
    const suffix = `@${targetBookID}`
    const key = Object.keys(localStorage).find(candidate => (
      candidate.startsWith('openreader_chapter_progress@') && candidate.endsWith(suffix)
    ))
    if (!key) return null
    try {
      return JSON.parse(localStorage.getItem(key))
    } catch {
      return null
    }
  }, bookID)
}

async function waitForLocalProgress(page, bookID, offset) {
  await page.waitForFunction(({ targetBookID, targetOffset }) => {
    const suffix = `@${targetBookID}`
    const key = Object.keys(localStorage).find(candidate => (
      candidate.startsWith('openreader_chapter_progress@') && candidate.endsWith(suffix)
    ))
    if (!key) return false
    try {
      return Number(JSON.parse(localStorage.getItem(key))?.offset) === Number(targetOffset)
    } catch {
      return false
    }
  }, { targetBookID: bookID, targetOffset: offset }, { timeout: 10_000 })
}

async function browserProgressWrite(page, token, payload, delay = 120) {
  return page.evaluate(async ({ tokenValue, progress, wait }) => {
    await new Promise(resolve => setTimeout(resolve, wait))
    const response = await fetch('/api/progress', {
      method: 'PUT',
      headers: {
        Authorization: `Bearer ${tokenValue}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(progress),
    })
    return {
      ok: response.ok,
      status: response.status,
      conflict: response.headers.get('x-openreader-progress-conflict') === '1',
      data: await response.json(),
    }
  }, { tokenValue: token, progress: payload, wait: delay })
}

async function assertWebDAVMirror(progressDir, book, progress) {
  const files = (await readdir(progressDir)).filter(name => name.endsWith('.json'))
  const mirrors = []
  for (const name of files) {
    const parsed = JSON.parse(await readFile(join(progressDir, name), 'utf8'))
    if (parsed.bookUrl === book.url) mirrors.push({ name, parsed })
  }
  assert(mirrors.length === 1, `expected one WebDAV progress mirror for ${book.title}, got ${mirrors.length}`)
  const mirror = mirrors[0].parsed
  assert(mirror.durChapterIndex === progress.chapterIndex, `${book.title}: mirrored chapter index diverged`)
  assert(mirror.durChapterPos === progress.offset, `${book.title}: mirrored offset ${mirror.durChapterPos} != ${progress.offset}`)
  assert(mirror.durChapterTitle === progress.chapterTitle, `${book.title}: mirrored title is not canonical`)
  assert(Number.isFinite(mirror.durChapterTime) && mirror.durChapterTime > 0, `${book.title}: mirrored timestamp is invalid`)
}

async function runViewport(browser, app, token, progressDir, viewport) {
  const suffix = `${viewport.width}${viewport.height}`
  const book = await importTXT(app.root, token, `进度并发合同 ${suffix}`)
  assert(book?.id, `${suffix}: import did not return a book`)
  const chapters = await api(app.root, `/books/${book.id}/chapters`, { token })
  assert(Array.isArray(chapters) && chapters.length > 0, `${suffix}: imported book has no chapters`)
  const chapter = chapters[0]

  const initial = await api(app.root, '/progress', {
    token,
    method: 'PUT',
    body: {
      bookId: book.id,
      chapterId: chapter.id,
      chapterIndex: chapter.index,
      offset: 20,
      percent: 0.05,
      chapterPercent: 0.05,
      mode: 'page',
      clientId: `setup-${suffix}`,
    },
  })
  assert(initial?.updatedAt, `${suffix}: baseline progress is missing updatedAt`)

  const readerA = await newReaderContext(browser, app.root, token, viewport, book.id, `${suffix}/A`)
  const readerB = await newReaderContext(browser, app.root, token, viewport, book.id, `${suffix}/B`)
  try {
    const baseline = await api(app.root, `/progress/${book.id}`, { token })
    const warmOffset = 40
    const warm = await api(app.root, '/progress', {
      token,
      method: 'PUT',
      body: {
        bookId: book.id,
        chapterId: chapter.id,
        chapterIndex: chapter.index,
        offset: warmOffset,
        percent: 0.1,
        chapterPercent: 0.1,
        mode: 'page',
        baseUpdatedAt: baseline.updatedAt,
        clientUpdatedAt: new Date().toISOString(),
        clientId: `warm-${suffix}`,
      },
    })
    assert(warm.offset === warmOffset, `${suffix}: WebSocket warm-up write did not win`)
    await Promise.all([
      waitForEvent(readerA.page, book.id, warmOffset),
      waitForEvent(readerB.page, book.id, warmOffset),
      waitForReaderRoute(readerA.page, warmOffset),
      waitForReaderRoute(readerB.page, warmOffset),
    ])

    const concurrentBase = await api(app.root, `/progress/${book.id}`, { token })
    const payloads = [
      {
        bookId: book.id,
        chapterId: chapter.id,
        chapterIndex: chapter.index,
        offset: 111,
        percent: 0.25,
        chapterPercent: 0.25,
        mode: 'page',
        baseUpdatedAt: concurrentBase.updatedAt,
        clientUpdatedAt: new Date().toISOString(),
        clientId: `browser-a-${suffix}`,
      },
      {
        bookId: book.id,
        chapterId: chapter.id,
        chapterIndex: chapter.index,
        offset: 222,
        percent: 0.5,
        chapterPercent: 0.5,
        mode: 'page',
        baseUpdatedAt: concurrentBase.updatedAt,
        clientUpdatedAt: new Date().toISOString(),
        clientId: `browser-b-${suffix}`,
      },
    ]
    const responses = await Promise.all([
      browserProgressWrite(readerA.page, token, payloads[0]),
      browserProgressWrite(readerB.page, token, payloads[1]),
    ])
    assert(responses.every(response => response.ok && response.status === 200), `${suffix}: concurrent writes were not compatible 200 responses`)
    assert(responses.filter(response => response.conflict).length === 1, `${suffix}: concurrent writes need exactly one conflict`)
    assert(responses.filter(response => !response.conflict).length === 1, `${suffix}: concurrent writes need exactly one winner`)
    const winner = responses.find(response => !response.conflict)?.data
    assert([111, 222].includes(Number(winner?.offset)), `${suffix}: winner has an unexpected offset`)

    await Promise.all([
      waitForEvent(readerA.page, book.id, winner.offset),
      waitForEvent(readerB.page, book.id, winner.offset),
      waitForReaderRoute(readerA.page, winner.offset),
      waitForReaderRoute(readerB.page, winner.offset),
      waitForLocalProgress(readerA.page, book.id, winner.offset),
      waitForLocalProgress(readerB.page, book.id, winner.offset),
    ])
    const [localA, localB, durable] = await Promise.all([
      localProgress(readerA.page, book.id),
      localProgress(readerB.page, book.id),
      api(app.root, `/progress/${book.id}`, { token }),
    ])
    assert(localA?.offset === winner.offset && localB?.offset === winner.offset, `${suffix}: active readers did not converge to the winner`)
    assert(durable.offset === winner.offset && durable.updatedAt === winner.updatedAt, `${suffix}: durable progress diverged from the CAS winner`)
    await assertWebDAVMirror(progressDir, book, durable)

    await readerA.context.close()
    await readerB.context.close()
    const reopened = await newReaderContext(browser, app.root, token, viewport, book.id, `${suffix}/reopen`)
    try {
      await waitForLocalProgress(reopened.page, book.id, winner.offset)
      const restored = await localProgress(reopened.page, book.id)
      assert(restored?.updatedAt === winner.updatedAt, `${suffix}: fresh context did not restore the server winner`)
      assert(await reopened.page.locator('.reader-content').evaluate(element => element.scrollTop) > 0, `${suffix}: fresh context did not restore a non-zero reading position`)
      assert(reopened.errors.length === 0, reopened.errors.join('\n'))
    } finally {
      await reopened.context.close()
    }
    const errors = [...readerA.errors, ...readerB.errors]
    assert(errors.length === 0, errors.join('\n'))
    console.log(`${viewport.width}x${viewport.height}: two-client CAS + WebSocket convergence + cold restore + WebDAV mirror ok`)
  } finally {
    await readerA.context.close().catch(() => {})
    await readerB.context.close().catch(() => {})
  }
}

const app = await startOpenReader()
const browser = await openSmokeBrowser()
try {
  const registered = await api(app.root, '/auth/register', {
    method: 'POST',
    body: { username: 'progressbrowseradmin', password: 'progress-browser-contract' },
  })
  const token = registered?.token
  assert(token, 'registration did not return a token')
  const progressDir = join(app.dataDir, 'webdav', 'bookProgress')
  await mkdir(progressDir, { recursive: true })
  await runViewport(browser, app, token, progressDir, { width: 1440, height: 900 })
  await runViewport(browser, app, token, progressDir, { width: 390, height: 844 })
  await runViewport(browser, app, token, progressDir, { width: 360, height: 800 })
} catch (error) {
  error.message = `${error.message}\nOpenReader output:\n${app.output()}`
  throw error
} finally {
  await browser.close()
  await app.close()
}
