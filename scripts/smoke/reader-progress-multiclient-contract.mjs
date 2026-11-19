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
import { createServer, request as httpRequest } from 'node:http'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'
import { openSmokeBrowser } from './playwright-runtime.mjs'

const execFileAsync = promisify(execFile)
const rootDir = join(dirname(fileURLToPath(import.meta.url)), '..', '..')
const backendDir = join(rootDir, 'backend')
const publicDir = join(rootDir, 'frontend', 'dist')
const progressRequestLimit = 16 * 1024

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

function rawProgressRequest(root, { token = '', contentLength, body = Buffer.alloc(0) } = {}) {
  const target = new URL('/api/progress', root)
  return new Promise((resolve, reject) => {
    const request = httpRequest({
      hostname: target.hostname,
      port: target.port,
      path: target.pathname,
      method: 'PUT',
      headers: {
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        'Content-Type': 'application/json',
        ...(contentLength === undefined ? {} : { 'Content-Length': String(contentLength) }),
      },
    }, response => {
      const chunks = []
      response.on('data', chunk => chunks.push(chunk))
      response.on('end', () => {
        const text = Buffer.concat(chunks).toString()
        let data = null
        try {
          data = text ? JSON.parse(text) : null
        } catch {
          data = text
        }
        resolve({
          status: response.statusCode,
          data,
          text,
          conflict: response.headers['x-openreader-progress-conflict'] === '1',
        })
      })
    })
    request.once('error', reject)
    request.setTimeout(15_000, () => request.destroy(new Error('progress raw request timed out')))
    request.end(body)
  })
}

function paddedProgressPayload(bookID, size) {
  const prefix = Buffer.from(`{"bookId":${bookID},"chapterIndex":0,"padding":"`)
  const suffix = Buffer.from('"}')
  assert(prefix.length + suffix.length <= size, `progress payload size ${size} is too small`)
  return Buffer.concat([prefix, Buffer.alloc(size - prefix.length - suffix.length, 0x61), suffix])
}

function assertProgressBoundaryError(result, status, message, label) {
  assert(result.status === status, `${label}: ${result.status} ${result.text}`)
  assert(result.data?.error === message, `${label}: error ${JSON.stringify(result.data)}`)
}

async function runRequestBoundary(root, token) {
  const book = await importTXT(root, token, '进度请求边界合同')
  const chapters = await api(root, `/books/${book.id}/chapters`, { token })
  assert(Array.isArray(chapters) && chapters.length > 0, 'progress boundary book has no chapter')
  const chapter = chapters[0]

  const declaredBody = paddedProgressPayload(book.id, progressRequestLimit + 1)
  const unauthorized = await rawProgressRequest(root, {
    contentLength: declaredBody.length,
    body: declaredBody,
  })
  assertProgressBoundaryError(unauthorized, 401, 'missing bearer token', 'unauthorized declared overflow')

  const declared = await rawProgressRequest(root, {
    token,
    contentLength: declaredBody.length,
    body: declaredBody,
  })
  assertProgressBoundaryError(declared, 413, 'request body too large', 'declared overflow')

  const exactBody = paddedProgressPayload(book.id, progressRequestLimit)
  const exact = await rawProgressRequest(root, {
    token,
    contentLength: exactBody.length,
    body: exactBody,
  })
  assert(exact.status === 200 && exact.data?.bookId === book.id, `exact 16 KiB progress: ${exact.status} ${exact.text}`)
  const baseline = await api(root, `/progress/${book.id}`, { token })

  const minimal = Buffer.from(JSON.stringify({ bookId: book.id, chapterIndex: 0, offset: 99 }))
  const invalidUTF8Prefix = Buffer.from(`{"bookId":${book.id},"chapterIndex":0,"offset":99,"padding":"`)
  const invalidUTF8 = Buffer.concat([invalidUTF8Prefix, Buffer.from([0xff]), Buffer.from('"}')])
  const rejected = [
    {
      label: 'actual chunked overflow',
      body: paddedProgressPayload(book.id, progressRequestLimit + 1),
      status: 413,
      error: 'request body too large',
    },
    {
      label: 'trailing JSON value',
      body: Buffer.concat([minimal, Buffer.from(' {"ignored":true}')]),
      status: 400,
      error: 'invalid progress payload',
    },
    { label: 'invalid UTF-8', body: invalidUTF8, status: 400, error: 'invalid progress payload' },
    {
      label: 'missing chapter index',
      body: Buffer.from(JSON.stringify({ bookId: book.id, offset: 99 })),
      status: 400,
      error: 'invalid progress payload',
    },
    {
      label: 'invalid conflict timestamps',
      body: Buffer.from(JSON.stringify({
        bookId: book.id,
        chapterIndex: 0,
        offset: 99,
        baseUpdatedAt: 'invalid-base',
        clientUpdatedAt: 'invalid-client',
      })),
      status: 400,
      error: 'invalid progress payload',
    },
    {
      label: 'oversized mode',
      body: Buffer.from(JSON.stringify({ bookId: book.id, chapterIndex: 0, offset: 99, mode: 'm'.repeat(21) })),
      status: 400,
      error: 'invalid progress payload',
    },
    {
      label: 'oversized client ID',
      body: Buffer.from(JSON.stringify({ bookId: book.id, chapterIndex: 0, offset: 99, clientId: 'c'.repeat(129) })),
      status: 400,
      error: 'invalid progress payload',
    },
  ]
  for (const test of rejected) {
    const result = await rawProgressRequest(root, { token, body: test.body })
    assertProgressBoundaryError(result, test.status, test.error, test.label)
  }

  const afterRejected = await api(root, `/progress/${book.id}`, { token })
  assert(
    afterRejected.offset === baseline.offset && afterRejected.updatedAt === baseline.updatedAt,
    'rejected progress boundary requests changed the durable winner',
  )

  const bounded = await request(root, '/progress', {
    token,
    method: 'PUT',
    body: {
      bookId: book.id,
      chapterId: chapter.id,
      chapterIndex: chapter.index,
      offset: 4,
      mode: 'm'.repeat(20),
      baseUpdatedAt: baseline.updatedAt,
      clientUpdatedAt: new Date().toISOString(),
      clientId: 'c'.repeat(128),
    },
  })
  assert(bounded.status === 200 && bounded.data?.offset === 4, `bounded progress fields: ${bounded.status} ${JSON.stringify(bounded.data)}`)
  console.log('progress request boundary: auth + 16 KiB + single UTF-8 JSON + field controls ok')
}

async function importTXT(root, token, title, chapterCount = 1) {
  const paragraphs = Array.from(
    { length: 90 },
    (_, index) => `第 ${index + 1} 段：这是用于验证双客户端阅读进度恢复的长正文。浏览器必须保存服务器确认的位置，而不是停留在各自的乐观缓存。`,
  )
  const form = new FormData()
  form.append(
    'file',
    new Blob([`第一章 ${title}\n${paragraphs.join('\n')}${chapterCount > 1 ? `\n第二章 外部接收\n${paragraphs.join('\n')}` : ''}`], { type: 'text/plain' }),
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

async function newReaderContext(browser, root, token, viewport, bookID, label, options = {}) {
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 750,
    hasTouch: viewport.width <= 750,
  })
  await context.addInitScript(({ tokenValue, oversizedClientId }) => {
    localStorage.setItem('openreader_token', tokenValue)
    if (oversizedClientId) sessionStorage.setItem('openreader_reader_client_id', 'x'.repeat(129))
    window.__openReaderProgressEvents = []
    window.addEventListener('openreader:progress-updated', (event) => {
      window.__openReaderProgressEvents.push(JSON.parse(JSON.stringify(event.detail?.progress || null)))
    })
  }, { tokenValue: token, oversizedClientId: options.oversizedClientId === true })
  const page = await context.newPage()
  const errors = collectPageErrors(page, label)
  await page.goto(`${root}/books/${bookID}/read?resume=1`, { waitUntil: 'domcontentloaded' })
  await page.locator('.reader-content .chapter-content').first().waitFor({ timeout: 15_000 })
  await page.waitForFunction(() => {
    const content = document.querySelector('.reader-content .chapter-content')
    return content && !document.body.innerText.includes('正在加载章节')
  }, null, { timeout: 15_000 })
  await page.waitForTimeout(450)
  if (options.oversizedClientId) {
    const clientId = await page.evaluate(() => sessionStorage.getItem('openreader_reader_client_id'))
    assert(clientId?.startsWith('web-') && Buffer.byteLength(clientId, 'utf8') <= 128, `${label}: oversized session client ID did not self-heal`)
  }
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

async function readWebDAVMirror(progressDir, book) {
  const files = (await readdir(progressDir)).filter(name => name.endsWith('.json'))
  const mirrors = []
  for (const name of files) {
    const parsed = JSON.parse(await readFile(join(progressDir, name), 'utf8'))
    if (parsed.bookUrl === book.url) mirrors.push({ name, parsed })
  }
  assert(mirrors.length === 1, `expected one WebDAV progress mirror for ${book.title}, got ${mirrors.length}`)
  return mirrors[0].parsed
}

async function assertWebDAVMirror(progressDir, book, progress, { afterClose = false } = {}) {
  let mirror = await readWebDAVMirror(progressDir, book)
  // The keepalive write can commit to SQLite while its WebDAV file is still being replaced.
  const deadline = Date.now() + 2_000
  while (afterClose && Number.isInteger(mirror.durChapterPos)
    && mirror.durChapterPos !== progress.offset && Date.now() < deadline) {
    await new Promise(resolve => setTimeout(resolve, 25))
    mirror = await readWebDAVMirror(progressDir, book)
  }
  assert(mirror.durChapterIndex === progress.chapterIndex, `${book.title}: mirrored chapter index diverged`)
  assert(mirror.durChapterPos === progress.offset, `${book.title}: mirrored offset ${mirror.durChapterPos} != ${progress.offset}`)
  assert(mirror.durChapterTitle === progress.chapterTitle, `${book.title}: mirrored title is not canonical`)
  assert(Number.isFinite(mirror.durChapterTime) && mirror.durChapterTime > 0, `${book.title}: mirrored timestamp is invalid`)
}

function readerScrollTop() {
  return document.querySelector('.reader-shell')?.classList.contains('document-scroll')
    ? document.scrollingElement?.scrollTop
    : document.querySelector('.reader-content')?.scrollTop
}

function captureProgressWrites(context, capture = request => request.postDataJSON()) {
  const writes = []
  context.on('request', request => {
    if (request.method() === 'PUT' && new URL(request.url()).pathname === '/api/progress') {
      writes.push(capture(request))
    }
  })
  return writes
}

async function scrollReaderBack(reader, viewport) {
  const start = await reader.page.evaluate(readerScrollTop)
  if (viewport.width <= 750) {
    const cdp = await reader.context.newCDPSession(reader.page)
    const x = Math.round(viewport.width / 2)
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchStart',
      touchPoints: [{ x, y: Math.round(viewport.height * 0.25) }],
    })
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchMove',
      touchPoints: [{ x, y: Math.round(viewport.height * 0.5) }],
    })
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchMove',
      touchPoints: [{ x, y: Math.round(viewport.height * 0.75) }],
    })
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
    await cdp.detach()
  } else {
    await reader.page.mouse.move(viewport.width / 2, viewport.height / 2)
    await reader.page.mouse.wheel(0, -Math.round(viewport.height / 2))
  }
  await reader.page.waitForFunction(previous => {
    const top = document.querySelector('.reader-shell')?.classList.contains('document-scroll')
      ? document.scrollingElement?.scrollTop
      : document.querySelector('.reader-content')?.scrollTop
    return top < previous - 40
  }, start, { timeout: 3000 })
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

  const readerA = await newReaderContext(browser, app.root, token, viewport, book.id, `${suffix}/A`, { oversizedClientId: true })
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
        offset: 2200,
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
        offset: 4400,
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
    assert([2200, 4400].includes(Number(winner?.offset)), `${suffix}: winner has an unexpected offset`)

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

    for (const [name, reader] of [['A', readerA], ['B', readerB]]) {
      const writes = captureProgressWrites(reader.context)
      await reader.context.close()
      const afterClose = await api(app.root, `/progress/${book.id}`, { token })
      assert(
        afterClose.offset === winner.offset && afterClose.updatedAt === winner.updatedAt,
        `${suffix}/${name}: untouched close changed the CAS winner from ${winner.offset}@${winner.updatedAt} to ${afterClose.offset}@${afterClose.updatedAt}`,
      )
      assert(writes.length === 0, `${suffix}/${name}: untouched close sent a progress write`)
    }
    const reopened = await newReaderContext(browser, app.root, token, viewport, book.id, `${suffix}/reopen`)
    const scrollWrites = captureProgressWrites(reopened.context)
    try {
      await waitForLocalProgress(reopened.page, book.id, winner.offset)
      const restored = await localProgress(reopened.page, book.id)
      assert(restored?.updatedAt === winner.updatedAt, `${suffix}: fresh context did not restore the server winner`)
      assert(await reopened.page.evaluate(readerScrollTop) > 0, `${suffix}: fresh context did not scroll to the server winner`)
      assert(reopened.errors.length === 0, reopened.errors.join('\n'))
      await scrollReaderBack(reopened, viewport)
      assert(scrollWrites.length === 0, `${suffix}: scroll fixture reached an ordinary save before closing the page`)
    } finally {
      await reopened.context.close()
    }
    const afterScroll = await api(app.root, `/progress/${book.id}`, { token })
    assert(
      afterScroll.chapterIndex === chapter.index && afterScroll.offset < winner.offset && afterScroll.updatedAt !== winner.updatedAt,
      `${suffix}: scrolling toward the previous paragraphs before close did not save a new position (${winner.offset} -> ${afterScroll.offset})`,
    )
    assert(
      scrollWrites.some(write => write.offset === afterScroll.offset && write.baseUpdatedAt === winner.updatedAt),
      `${suffix}: no close-time write matched the saved position and the restored server version`,
    )
    await assertWebDAVMirror(progressDir, book, afterScroll, { afterClose: true })
    const afterScrollReopen = await newReaderContext(browser, app.root, token, viewport, book.id, `${suffix}/scroll-reopen`)
    try {
      await waitForLocalProgress(afterScrollReopen.page, book.id, afterScroll.offset)
      const restored = await localProgress(afterScrollReopen.page, book.id)
      assert(restored.updatedAt === afterScroll.updatedAt, `${suffix}: fresh context did not restore the position saved on close`)
      assert(afterScrollReopen.errors.length === 0, afterScrollReopen.errors.join('\n'))
    } finally {
      await afterScrollReopen.context.close()
    }
    const errors = [...readerA.errors, ...readerB.errors]
    assert(errors.length === 0, errors.join('\n'))
    console.log(`${viewport.width}x${viewport.height}: client ID recovery + two-client CAS + WebSocket convergence + untouched close + cold restore + scroll and close + WebDAV mirror ok`)
  } finally {
    await readerA.context.close().catch(() => {})
    await readerB.context.close().catch(() => {})
  }
}

async function observeCloseParagraph(page) {
  // Measure when the keepalive starts because touch momentum can continue until close.
  await page.evaluate(() => {
    const originalFetch = window.fetch
    window.fetch = (url, options) => {
      if (!String(url).endsWith('/api/progress') || !options?.keepalive) return originalFetch(url, options)
      const reader = document.querySelector('.reader-content')
      const documentScroll = document.querySelector('.reader-shell.document-scroll')
      const rect = reader.getBoundingClientRect()
      const top = documentScroll ? 0 : rect.top
      const height = documentScroll ? window.innerHeight : rect.height
      const anchor = top + Math.min(height * 0.32, 180)
      const visible = [...reader.querySelectorAll('.chapter-content h3[data-pos], .chapter-content [data-reader-block][data-pos]')]
        .map(node => ({ node, rect: node.getBoundingClientRect() }))
        .filter(({ rect }) => rect.bottom >= top + 8 && rect.top <= top + height - 8)
      const paragraph = visible.find(({ rect }) => rect.top <= anchor && anchor <= rect.bottom)
        ?? visible.sort((a, b) => Math.abs(a.rect.top - anchor) - Math.abs(b.rect.top - anchor))[0]
      if (!paragraph) return originalFetch(url, options)
      const start = Number(paragraph.node.dataset.pos)
      const end = start + paragraph.node.textContent.length
      const headers = new Headers(options.headers)
      headers.set('X-Smoke-Visible-Paragraph', `${start},${end}`)
      return originalFetch(url, { ...options, headers })
    }
  })
}

async function runPendingRestoreViewport(browser, app, token, progressDir, viewport, scenario) {
  const label = `pending-restore-${viewport.width}x${viewport.height}/${scenario}`
  const shouldScroll = scenario !== 'untouched'
  const book = await importTXT(app.root, token, `等待恢复 ${label}`)
  const [chapter] = await api(app.root, `/books/${book.id}/chapters`, { token })
  assert(chapter, `${label}: imported book has no chapter`)
  const remoteOffset = 4400
  const newerOffset = 8000
  async function writeProgress(offset, chapterPercent, baseUpdatedAt) {
    return api(app.root, '/progress', {
      token,
      method: 'PUT',
      body: {
        bookId: book.id, chapterId: chapter.id, chapterIndex: chapter.index,
        offset, percent: chapterPercent, chapterPercent, mode: 'page', baseUpdatedAt,
        clientId: 'pending-restore-server',
      },
    })
  }
  const initial = await writeProgress(2200, 0.25)
  assert(initial?.updatedAt, `${label}: baseline progress is missing updatedAt`)
  const reader = await newReaderContext(browser, app.root, token, viewport, book.id, label)
  const writes = captureProgressWrites(reader.context, request => ({
    payload: request.postDataJSON(),
    visibleParagraph: request.headers()['x-smoke-visible-paragraph'],
  }))
  try {
    await waitForLocalProgress(reader.page, book.id, initial.offset)
    await reader.page.evaluate(offsets => {
      const router = document.querySelector('#app').__vue_app__.config.globalProperties.$router
      const gate = new Promise(() => {})
      router.beforeEach(to => {
        const offset = Number(to.query.offset)
        if (!offsets.includes(offset)) return
        window.__openReaderRestoreWaiting = offset
        return gate
      })
    }, [remoteOffset, newerOffset])
    const waitForRestoreNavigation = offset => reader.page.waitForFunction(
      expected => window.__openReaderRestoreWaiting === expected, offset, { timeout: 10_000 },
    )
    if (shouldScroll) await observeCloseParagraph(reader.page)
    const remote = await writeProgress(remoteOffset, 0.5, initial.updatedAt)
    assert(remote.offset === remoteOffset, `${label}: remote progress did not reach the server`)
    await waitForRestoreNavigation(remoteOffset)
    if (shouldScroll) await scrollReaderBack(reader, viewport)
    let latest = remote
    if (scenario === 'superseded') {
      latest = await writeProgress(newerOffset, 0.75, remote.updatedAt)
      assert(latest.offset === newerOffset && latest.updatedAt !== remote.updatedAt,
        `${label}: second remote progress did not reach the server`)
      await waitForRestoreNavigation(newerOffset)
    }
    const routeOffset = Number(new URL(reader.page.url()).searchParams.get('offset'))
    assert(routeOffset !== remoteOffset && routeOffset !== newerOffset,
      `${label}: restoration navigation committed before close`)
    assert(writes.length === 0, `${label}: progress write occurred before close`)
    await reader.context.close()
    const durable = await api(app.root, `/progress/${book.id}`, { token })
    if (shouldScroll) {
      const write = writes.find(({ payload }) => payload.offset === durable.offset && payload.baseUpdatedAt === latest.updatedAt)
      assert(write?.visibleParagraph && durable.updatedAt !== latest.updatedAt,
        `${label}: scrolling before restore did not save against the confirmed version on close (offset ${durable.offset})`)
      const [start, end] = write.visibleParagraph.split(',').map(Number)
      assert(durable.chapterIndex === chapter.index && durable.offset >= start && durable.offset <= end,
        `${label}: saved offset ${durable.offset} is outside the visible paragraph ${start}–${end}`)
    } else {
      assert(writes.length === 0 && durable.offset === latest.offset && durable.updatedAt === latest.updatedAt,
        `${label}: untouched close changed the pending restored position`)
    }
    await assertWebDAVMirror(progressDir, book, durable, { afterClose: shouldScroll })
    if (scenario === 'superseded') {
      const reopened = await newReaderContext(browser, app.root, token, viewport, book.id, `${label}/reopen`)
      try {
        await waitForLocalProgress(reopened.page, book.id, durable.offset)
        const restored = await localProgress(reopened.page, book.id)
        assert(restored.updatedAt === durable.updatedAt && await reopened.page.evaluate(readerScrollTop) > 0,
          `${label}: reopened reader did not restore the saved position`)
        assert(reopened.errors.length === 0, reopened.errors.join('\n'))
      } finally {
        await reopened.context.close()
      }
    }
    assert(reader.errors.length === 0, reader.errors.join('\n'))
    console.log(`${label}: close while remote route waits + durable offset ${durable.offset} + WebDAV mirror ok`)
  } finally {
    await reader.context.close().catch(() => {})
  }
}

async function runIngressViewport(browser, app, token, viewport) {
  const label = `ingress-${viewport.width}x${viewport.height}`
  await api(app.root, '/settings/reader', { token, method: 'PUT', body: { value: {
    mode: 'scroll', autoTheme: false, theme: 'parchment', fontSize: 18,
  } } })
  const book = await importTXT(app.root, token, `WebDAV 接收 ${label}`, 2)
  const chapters = await api(app.root, `/books/${book.id}/chapters`, { token })
  assert(chapters.length === 2, `${label}: expected two imported chapters`)
  const reader = await newReaderContext(browser, app.root, token, viewport, book.id, label)
  const writes = []
  reader.page.on('request', request => {
    if (request.method() === 'PUT' && new URL(request.url()).pathname === '/api/progress') writes.push(request.postData())
  })
  const path = `/reader3/webdav/bookProgress/${label}.json`
  async function upload(index, offset) {
    const body = JSON.stringify({ bookUrl: book.url, name: book.title, author: book.author,
      durChapterIndex: index, durChapterPos: offset, durChapterTime: Date.now(),
      durChapterTitle: '外部非权威标题', externalMetadata: 'keep these exact bytes',
    })
    const response = await fetch(`${app.root}${path}`, { method: 'PUT', headers: {
      Authorization: `Basic ${Buffer.from('progressbrowseradmin:progress-browser-contract').toString('base64')}`,
    }, body })
    assert(response.status === 201 && !response.headers.has('x-openreader-progress-sync') && await response.text() === '', `${label}: DAV upload/sync failed ${response.status}`)
    const raw = await fetch(`${app.root}${path.replace('/reader3/webdav', '/webdav')}`, { headers: { Authorization: `Bearer ${token}` } })
    assert(await raw.text() === body, `${label}: ingress rewrote raw payload`)
    const progress = await api(app.root, `/progress/${book.id}`, { token })
    assert(progress.chapterIndex === index && progress.offset === offset && progress.chapterId === chapters[index].id && progress.chapterTitle === chapters[index].title, `${label}: canonical progress missing`)
    const shelf = await api(app.root, '/books', { token })
    const shelfProgress = shelf.find(row => row.id === book.id)?.progress
    assert(shelfProgress?.offset === offset && shelfProgress?.chapterId === chapters[index].id, `${label}: shelf projection diverged`)
    await waitForEvent(reader.page, book.id, offset)
    await reader.page.waitForFunction(({ index, offset }) => {
      const url = new URL(location.href)
      const percentMatches = offset > 0 ? !url.searchParams.has('percent') : Number(url.searchParams.get('percent')) === 0
      return Number(url.searchParams.get('chapter')) === index && Number(url.searchParams.get('offset')) === offset && percentMatches
    }, { index, offset }, { timeout: 10_000 })
    return progress
  }
  async function assertExactParagraph(page, index, offset) {
    await page.waitForFunction(({ index, offset }) => {
      const body = document.querySelector(`.chapter-content[data-index="${index}"]`)
      const nodes = [...(body?.querySelectorAll('h3[data-pos], [data-reader-block][data-pos]') || [])]
      const target = nodes.filter(node => Number(node.dataset.pos) <= offset).at(-1)
      if (!target) return false
      const viewport = document.querySelector('.reader-content')
      const documentScroll = document.querySelector('.reader-shell.document-scroll')
      const viewportTop = documentScroll ? 0 : viewport.getBoundingClientRect().top
      return Math.abs(target.getBoundingClientRect().top - viewportTop - 80) < 3
    }, { index, offset }, { timeout: 10_000 })
  }
  try {
    const received = await upload(1, 2400)
    await assertExactParagraph(reader.page, 1, 2400)
    await waitForLocalProgress(reader.page, book.id, 2400)
    await reader.page.waitForTimeout(900)
    assert(writes.length === 0, `${label}: online external update echoed a progress PUT`)
    const cold = await newReaderContext(browser, app.root, token, viewport, book.id, `${label}/cold`)
    const coldWrites = captureProgressWrites(cold.context)
    try {
      await assertExactParagraph(cold.page, 1, 2400)
      await waitForLocalProgress(cold.page, book.id, 2400)
      const durable = await api(app.root, `/progress/${book.id}`, { token })
      assert(durable.updatedAt === received.updatedAt, `${label}: cold restore rewrote progress`)
      assert(cold.errors.length === 0, cold.errors.join('\n'))
    } finally { await cold.context.close() }
    const afterClose = await api(app.root, `/progress/${book.id}`, { token })
    assert(afterClose.offset === received.offset && afterClose.updatedAt === received.updatedAt,
      `${label}: closing after a cold restore changed the server position ${received.offset} → ${afterClose.offset}`)
    assert(coldWrites.length === 0, `${label}: cold close echoed a progress PUT`)
    await upload(0, 0)
    try {
      await reader.page.waitForFunction(() => {
        const chapter = document.querySelector('.chapter-content[data-index="0"]')
        if (!chapter) return false
        const viewportTop = document.querySelector('.reader-shell.document-scroll')
          ? 0 : document.querySelector('.reader-content').getBoundingClientRect().top
        return Math.abs(chapter.getBoundingClientRect().top - viewportTop) < 2
      }, null, { timeout: 3_000 })
    } catch (error) {
      const detail = `\n${label} reset geometry: ${JSON.stringify(await reader.page.evaluate(() => ({
        documentScroll: Boolean(document.querySelector('.reader-shell.document-scroll')),
        documentTop: document.scrollingElement?.scrollTop,
        contentTop: document.querySelector('.reader-content')?.scrollTop,
        chapterTop: document.querySelector('.chapter-content[data-index="0"]')?.getBoundingClientRect().top,
        viewportTop: document.querySelector('.reader-content')?.getBoundingClientRect().top,
        url: location.href,
      })))}`
      throw new Error(`${error.message}${detail}`, { cause: error })
    }
    assert(writes.length === 0, `${label}: explicit reset echoed a PUT`)
    assert(reader.errors.length === 0, reader.errors.join('\n'))
    console.log(`${label}: raw Basic upload + canonical shelf + WebSocket online exact paragraph + cold exact paragraph and close + chapter-zero reset + no echo ok`)
  } finally { await reader.context.close() }
}

const app = await startOpenReader()
let browser
try {
  browser = await openSmokeBrowser()
  const registered = await api(app.root, '/auth/register', {
    method: 'POST',
    body: { username: 'progressbrowseradmin', password: 'progress-browser-contract' },
  })
  const token = registered?.token
  assert(token, 'registration did not return a token')
  const progressDir = join(app.dataDir, 'webdav', 'bookProgress')
  await mkdir(progressDir, { recursive: true })
  await runRequestBoundary(app.root, token)
  if (!process.env.PROGRESS_SMOKE_INGRESS_ONLY) {
    for (const viewport of [{ width: 1440, height: 900 }, { width: 390, height: 844 }, { width: 360, height: 800 }]) {
      for (const scenario of ['untouched', 'scroll', 'superseded']) {
        await runPendingRestoreViewport(browser, app, token, progressDir, viewport, scenario)
      }
      await runViewport(browser, app, token, progressDir, viewport)
    }
  }
  await runIngressViewport(browser, app, token, { width: 1440, height: 900 })
  await runIngressViewport(browser, app, token, { width: 390, height: 844 })
  await runIngressViewport(browser, app, token, { width: 360, height: 800 })
} catch (error) {
  error.message = `${error.message}\nOpenReader output:\n${app.output()}`
  throw error
} finally {
  await browser?.close()
  await app.close()
}
