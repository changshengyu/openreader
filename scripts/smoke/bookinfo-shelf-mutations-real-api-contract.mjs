#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

import { access, mkdtemp, rm } from 'node:fs/promises'
import { createServer } from 'node:http'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'
import { execFile, spawn } from 'node:child_process'

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
  throw new Error(`OpenReader BookInfo test server did not start: ${lastError?.message || 'unknown error'}\n${output()}`)
}

async function startOpenReader() {
  await access(join(publicDir, 'index.html')).catch(() => {
    throw new Error('frontend/dist is missing; run `cd frontend && npm run build` before this smoke')
  })
  const tempRoot = await mkdtemp(join(tmpdir(), 'openreader-bookinfo-real-api-'))
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
      OPENREADER_JWT_SECRET: 'bookinfo-real-api-contract-secret',
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

async function importLocalBook(root, token, title) {
  const form = new FormData()
  form.append('file', new Blob(['第一章 开始\n本地刷新验收正文。'], { type: 'text/plain' }), `${title}.txt`)
  form.append('title', title)
  const response = await fetch(`${root}/api/imports/books`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: form,
  })
  const text = await response.text()
  if (!response.ok) throw new Error(`local import failed with ${response.status}: ${text}`)
  return JSON.parse(text)
}

async function seedWorkspace(root, viewport) {
  const suffix = `${viewport.width}x${viewport.height}`
  const registered = await api(root, '/auth/register', {
    method: 'POST',
    body: { username: `bookinfo${viewport.width}${viewport.height}`, password: 'book-info-contract' },
  })
  const token = registered?.token
  const userID = Number(registered?.user?.id)
  assert(token && Number.isFinite(userID) && userID > 0, `${suffix}: registration did not return a user identity`)
  const category = await api(root, '/categories', {
    token,
    method: 'POST',
    body: { name: `BookInfo 分组 ${suffix}` },
  })
  const remoteTitle = `BookInfo 远程书 ${suffix}`
  const remote = await api(root, '/books', {
    token,
    method: 'POST',
    body: {
      title: remoteTitle,
      author: '真实 API',
      sourceId: 1,
      url: `https://bookinfo.example/${suffix}`,
      canUpdate: true,
    },
  })
  const localTitle = `BookInfo 本地书 ${suffix}`
  const local = await importLocalBook(root, token, localTitle)
  assert(Number(remote.id) > 0 && Number(local.id) > 0, `${suffix}: seed did not return shelf records`)
  return { token, userID, category, remote, local }
}

function shelfRow(page, title) {
  return page.locator('.shelf-page .book-row').filter({ has: page.getByText(title, { exact: true }) })
}

async function captureRequest(page, predicate, action) {
  const request = page.waitForRequest(predicate, { timeout: 15_000 })
  await action()
  return request
}

async function waitFor(check, description) {
  const deadline = Date.now() + 15_000
  while (Date.now() < deadline) {
    const value = await check()
    if (value) return value
    await new Promise(resolve => setTimeout(resolve, 120))
  }
  throw new Error(`timed out: ${description}`)
}

async function runViewport(browser, root, viewport) {
  const seeded = await seedWorkspace(root, viewport)
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 750,
    hasTouch: viewport.width <= 750,
  })
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error' && !/WebSocket connection to .*\/ws\/sync/.test(message.text())) {
      failures.push(`console.error: ${message.text()}`)
    }
  })

  try {
    await page.addInitScript(token => localStorage.setItem('openreader_token', token), seeded.token)
    await page.goto(root, { waitUntil: 'domcontentloaded' })
    const remoteRow = shelfRow(page, seeded.remote.title)
    await remoteRow.waitFor({ state: 'visible', timeout: 15_000 })
    await shelfRow(page, seeded.local.title).waitFor({ state: 'visible', timeout: 15_000 })

    await remoteRow.locator('.list-cover').click()
    const dialog = page.locator('.book-info-dialog')
    await dialog.waitFor({ state: 'visible', timeout: 10_000 })

    const followRequest = await captureRequest(
      page,
      request => request.method() === 'PUT' && new URL(request.url()).pathname === `/api/books/${seeded.remote.id}`,
      () => dialog.locator('.inline-update-switch .el-switch').click(),
    )
    const followPayload = JSON.parse(followRequest.postData() || '{}')
    assert(JSON.stringify(followPayload) === JSON.stringify({ canUpdate: false }), `${viewport.width}: follow toggle payload must be precise, got ${JSON.stringify(followPayload)}`)
    const afterFollow = await waitFor(
      async () => {
        const book = await api(root, `/books/${seeded.remote.id}`, { token: seeded.token })
        return book.canUpdate === false ? book : null
      },
      `${viewport.width}: follow state persisted`,
    )
    assert(afterFollow.title === seeded.remote.title, `${viewport.width}: follow toggle changed the book title`)

    const coverRequest = await captureRequest(
      page,
      request => request.method() === 'PUT' && new URL(request.url()).pathname === `/api/books/${seeded.remote.id}`,
      () => dialog.locator('.cover-file-input').setInputFiles({
        name: 'bookinfo-cover.png',
        mimeType: 'image/png',
        buffer: Buffer.from([0x89, 0x50, 0x4e, 0x47]),
      }),
    )
    const coverPayload = JSON.parse(coverRequest.postData() || '{}')
    assert(Object.keys(coverPayload).length === 1 && typeof coverPayload.customCoverUrl === 'string', `${viewport.width}: cover payload must only contain customCoverUrl, got ${JSON.stringify(coverPayload)}`)
    const expectedCoverPrefix = `/uploads/users/${seeded.userID}/covers/`
    assert(coverPayload.customCoverUrl.startsWith(expectedCoverPrefix), `${viewport.width}: uploaded cover path is not user-scoped: ${coverPayload.customCoverUrl}`)
    const afterCover = await waitFor(
      async () => {
        const book = await api(root, `/books/${seeded.remote.id}`, { token: seeded.token })
        return book.customCoverUrl === coverPayload.customCoverUrl ? book : null
      },
      `${viewport.width}: cover state persisted`,
    )
    assert(afterCover.author === '真实 API', `${viewport.width}: cover update changed the author`)
    const coverResponse = await fetch(`${root}${coverPayload.customCoverUrl}`)
    assert(coverResponse.status === 200, `${viewport.width}: user-scoped cover must remain directly loadable, got ${coverResponse.status}`)

    await dialog.getByRole('button', { name: '设置分组', exact: true }).click()
    const groupDialog = page.locator('.global-book-group-dialog')
    await groupDialog.waitFor({ state: 'visible', timeout: 10_000 })
    const categoryRow = groupDialog.locator('.group-set-table .el-table__row').filter({ hasText: seeded.category.name })
    await categoryRow.waitFor({ state: 'visible', timeout: 10_000 })
    const groupRequest = await captureRequest(
      page,
      request => request.method() === 'PUT' && new URL(request.url()).pathname === `/api/books/${seeded.remote.id}/category`,
      async () => {
        await categoryRow.locator('.el-checkbox').click()
        await groupDialog.getByRole('button', { name: '确认', exact: true }).click()
      },
    )
    const groupPayload = JSON.parse(groupRequest.postData() || '{}')
    assert(JSON.stringify(groupPayload.categoryIds) === JSON.stringify([Number(seeded.category.id)]), `${viewport.width}: BookInfo group payload mismatch: ${JSON.stringify(groupPayload)}`)
    await groupDialog.waitFor({ state: 'hidden', timeout: 10_000 })
    await waitFor(
      async () => {
        const book = await api(root, `/books/${seeded.remote.id}`, { token: seeded.token })
        return Array.isArray(book.categoryIds) && book.categoryIds.includes(Number(seeded.category.id))
      },
      `${viewport.width}: group persisted`,
    )
    await dialog.locator('.el-dialog__headerbtn').click()
    await dialog.waitFor({ state: 'hidden', timeout: 10_000 })

    const localRow = shelfRow(page, seeded.local.title)
    await localRow.locator('.list-cover').click()
    await dialog.waitFor({ state: 'visible', timeout: 10_000 })
    const refreshRequest = await captureRequest(
      page,
      request => request.method() === 'POST' && new URL(request.url()).pathname === `/api/books/${seeded.local.id}/refresh-local`,
      () => dialog.getByRole('button', { name: '更新', exact: true }).click(),
    )
    assert((refreshRequest.postData() || '') === '', `${viewport.width}: local refresh must not send a stray body`)
    await page.getByText('本地书已刷新，共 1 章', { exact: true }).waitFor({ state: 'visible', timeout: 15_000 })
    await waitFor(
      async () => {
        const book = await api(root, `/books/${seeded.local.id}`, { token: seeded.token })
        return Number(book.chapterCount) === 1 ? book : null
      },
      `${viewport.width}: refreshed local chapter count`,
    )
    assert(await dialog.isVisible(), `${viewport.width}: local refresh must leave BookInfo open`)
    assert(failures.length === 0, `${viewport.width}: ${failures.join('\n')}`)
    return `${viewport.width}x${viewport.height}`
  } finally {
    await context.close()
  }
}

async function run() {
  const app = await startOpenReader()
  try {
    const browser = await openSmokeBrowser()
    try {
      const completed = []
      for (const viewport of [
        { width: 1440, height: 900 },
        { width: 390, height: 844 },
        { width: 360, height: 800 },
      ]) {
        completed.push(await runViewport(browser, app.root, viewport))
      }
      console.log(`bookinfo-shelf-mutations-real-api: ok ${completed.join(', ')} realApi=true precisePatches=true userAssets=true groupSet=true localRefresh=true`)
    } finally {
      await browser.close()
    }
  } finally {
    await app.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
