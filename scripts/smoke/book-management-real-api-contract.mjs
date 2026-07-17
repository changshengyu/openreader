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
  const port = address.port
  await new Promise(resolve => server.close(resolve))
  return port
}

async function waitForHealth(root, processOutput) {
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
  throw new Error(`OpenReader BookManage test server did not start: ${lastError?.message || 'unknown error'}\n${processOutput()}`)
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
    throw new Error('frontend/dist is missing; run `cd frontend && npm run build` before this smoke')
  })
  const tempRoot = await mkdtemp(join(tmpdir(), 'openreader-book-manage-real-api-'))
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
      OPENREADER_JWT_SECRET: 'book-manage-real-api-contract-secret',
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
  if (!response.ok) {
    throw new Error(`${method} ${path} failed with ${response.status}: ${text}`)
  }
  return data
}

function categoryIDs(book) {
  return (Array.isArray(book?.categoryIds) ? book.categoryIds : [])
    .map(Number)
    .filter(Number.isFinite)
    .sort((a, b) => a - b)
}

function equalIDs(actual, expected) {
  return actual.length === expected.length && actual.every((id, index) => id === expected[index])
}

async function seedWorkspace(root, viewport) {
  const suffix = `${viewport.width}x${viewport.height}`
  const registered = await api(root, '/auth/register', {
    method: 'POST',
    body: { username: `bookmanage${viewport.width}${viewport.height}`, password: 'book-manage-contract' },
  })
  const token = registered?.token
  assert(token, `${suffix}: registration did not return a token`)
  const primary = await api(root, '/categories', {
    token,
    method: 'POST',
    body: { name: `主分组 ${suffix}` },
  })
  const secondary = await api(root, '/categories', {
    token,
    method: 'POST',
    body: { name: `次分组 ${suffix}` },
  })
  const grouped = await api(root, '/books', {
    token,
    method: 'POST',
    body: {
      title: `验收分组书 ${suffix}`,
      author: '真实 API',
      url: `local://book-manage/${suffix}/grouped`,
      sourceId: 0,
      type: 0,
      categoryIds: [primary.id],
    },
  })
  const batchTarget = await api(root, '/books', {
    token,
    method: 'POST',
    body: {
      title: `验收批量书 ${suffix}`,
      author: '真实 API',
      url: `local://book-manage/${suffix}/batch`,
      sourceId: 0,
      type: 0,
    },
  })
  assert(equalIDs(categoryIDs(grouped), [Number(primary.id)]), `${suffix}: seeded grouped book did not retain its category`)
  assert(categoryIDs(batchTarget).length === 0, `${suffix}: seeded batch book should start ungrouped`)
  return { token, primary, secondary, grouped, batchTarget }
}

async function openMobileNavigation(page, viewport) {
  if (viewport.width > 750) return
  await page.locator('.mobile-menu-trigger').click()
  await page.waitForFunction(() => {
    const sidebar = document.querySelector('.app-sidebar')
    return sidebar && Math.abs(Number.parseFloat(getComputedStyle(sidebar).marginLeft)) < 0.5
  })
}

function managerRow(manager, title, viewport) {
  if (viewport.width <= 750) {
    return manager.locator('.mobile-manage-card').filter({ hasText: title })
  }
  return manager.locator('.desktop-manage-table tbody tr').filter({ hasText: title })
}

async function ensureRowSelected(page, row) {
  const checkbox = row.locator('.el-checkbox').first()
  const input = row.locator('.el-checkbox__input').first()
  if (await input.evaluate(node => node.classList.contains('is-checked'))) {
    await checkbox.click()
    await page.waitForTimeout(80)
  }
  await checkbox.click()
}

async function freshBook(root, token, bookID) {
  const books = await api(root, '/books', { token })
  return books.find(book => Number(book.id) === Number(bookID)) || null
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
  const mutationRequests = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error' && !/WebSocket connection to .*\/ws\/sync/.test(message.text())) {
      failures.push(`console.error: ${message.text()}`)
    }
  })
  page.on('request', request => {
    const path = new URL(request.url()).pathname
    if (path.startsWith('/api/books') && ['POST', 'PUT', 'DELETE'].includes(request.method())) {
      mutationRequests.push(`${request.method()} ${path}`)
    }
  })

  try {
    await page.addInitScript(token => localStorage.setItem('openreader_token', token), seeded.token)
    await page.goto(root, { waitUntil: 'domcontentloaded' })
    await page.waitForSelector('.shelf-page .book-row', { timeout: 15_000 })
    await page.getByText(seeded.grouped.title, { exact: true }).waitFor({ state: 'visible', timeout: 15_000 })
    await openMobileNavigation(page, viewport)
    await page.getByRole('button', { name: '书籍管理', exact: true }).click()

    const manager = page.locator('.global-book-manage-dialog')
    await manager.waitFor({ state: 'visible', timeout: 15_000 })
    const groupedRow = managerRow(manager, seeded.grouped.title, viewport)
    const batchRow = managerRow(manager, seeded.batchTarget.title, viewport)
    await groupedRow.waitFor({ state: 'visible', timeout: 15_000 })
    await batchRow.waitFor({ state: 'visible', timeout: 15_000 })

    await groupedRow.getByRole('button', { name: '分组', exact: true }).click()
    const groupSet = page.locator('.global-book-group-dialog')
    await groupSet.waitFor({ state: 'visible', timeout: 10_000 })
    const primaryGroupRow = groupSet.locator('.group-set-table .el-table__row').filter({ hasText: seeded.primary.name })
    const secondaryGroupRow = groupSet.locator('.group-set-table .el-table__row').filter({ hasText: seeded.secondary.name })
    await primaryGroupRow.waitFor({ state: 'visible', timeout: 10_000 })
    await secondaryGroupRow.waitFor({ state: 'visible', timeout: 10_000 })
    assert(
      await primaryGroupRow.locator('.el-checkbox__input').evaluate(node => node.classList.contains('is-checked')),
      `${viewport.width}: BookGroup must preselect the actual persisted category`,
    )

    const beforeEmptyAttempt = mutationRequests.length
    await primaryGroupRow.locator('.el-checkbox').click()
    await groupSet.getByRole('button', { name: '确认', exact: true }).click()
    await page.getByText('请选择书籍分组', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 })
    assert(await groupSet.isVisible(), `${viewport.width}: empty group selection should leave BookGroup open`)
    assert(mutationRequests.length === beforeEmptyAttempt, `${viewport.width}: empty group selection made a book mutation request`)
    const unchanged = await freshBook(root, seeded.token, seeded.grouped.id)
    assert(unchanged && equalIDs(categoryIDs(unchanged), [Number(seeded.primary.id)]), `${viewport.width}: empty group selection mutated persisted categories`)

    await secondaryGroupRow.locator('.el-checkbox').click()
    await groupSet.getByRole('button', { name: '确认', exact: true }).click()
    await groupSet.waitFor({ state: 'hidden', timeout: 10_000 })
    await page.waitForFunction(name => [...document.querySelectorAll('.global-book-manage-dialog tbody tr, .global-book-manage-dialog .mobile-manage-card')]
      .some(row => row.textContent?.includes(name)), seeded.secondary.name)
    const groupedAfterSet = await freshBook(root, seeded.token, seeded.grouped.id)
    assert(groupedAfterSet && equalIDs(categoryIDs(groupedAfterSet), [Number(seeded.secondary.id)]), `${viewport.width}: real group set did not persist the selected category`)
    assert(
      mutationRequests.some(entry => entry === `PUT /api/books/${seeded.grouped.id}/category`),
      `${viewport.width}: group set did not issue its real PUT request`,
    )

    await ensureRowSelected(page, batchRow)
    await manager.getByRole('button', { name: '批量添加分组', exact: true }).click()
    await page.getByRole('menuitem', { name: seeded.primary.name, exact: true }).click()
    await page.getByText(`已添加到“${seeded.primary.name}”分组`, { exact: true }).waitFor({ state: 'visible', timeout: 10_000 })
    const batchAfterCategory = await freshBook(root, seeded.token, seeded.batchTarget.id)
    assert(batchAfterCategory && equalIDs(categoryIDs(batchAfterCategory), [Number(seeded.primary.id)]), `${viewport.width}: real batch category mutation did not persist`)
    assert(
      mutationRequests.some(entry => entry === 'POST /api/books/batch'),
      `${viewport.width}: batch category mutation did not issue its real POST request`,
    )

    // The real table receives a fresh store row after the category write. Element
    // Plus then recalculates its table selection, so a later destructive action
    // must be selected deliberately again; upstream does not require selection
    // persistence across an already-completed batch mutation.
    await ensureRowSelected(page, batchRow)
    await page.waitForFunction(() => [...document.querySelectorAll('.global-book-manage-dialog button')]
      .some(button => button.textContent?.trim() === '批量删除' && !button.hasAttribute('disabled')))
    await manager.getByRole('button', { name: '批量删除', exact: true }).click()
    const confirm = page.locator('.el-message-box').filter({ hasText: '确定删除选中的 1 本书吗？' })
    await confirm.waitFor({ state: 'visible', timeout: 10_000 })
    await confirm.getByRole('button', { name: '确定', exact: true }).click()
    await batchRow.waitFor({ state: 'hidden', timeout: 10_000 })
    assert(await manager.isVisible(), `${viewport.width}: batch deletion should leave BookManage usable`)
    assert(!(await freshBook(root, seeded.token, seeded.batchTarget.id)), `${viewport.width}: deleted shelf record remained in a fresh real API response`)

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
      console.log(`book-management-real-api: ok ${completed.join(', ')} realApi=true apiMocked=false groupSet=true batchCategory=true batchDelete=true`)
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
