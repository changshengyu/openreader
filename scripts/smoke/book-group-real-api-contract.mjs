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
  assert(address && typeof address === 'object', 'unable to reserve a local BookGroup test port')
  const port = address.port
  await new Promise(resolve => server.close(resolve))
  return port
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

async function waitForHealth(root, processOutput) {
  const deadline = Date.now() + 60_000
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`${root}/api/health`)
      if (response.ok) return
    } catch {
      // The process may still be binding its listener.
    }
    await new Promise(resolve => setTimeout(resolve, 300))
  }
  throw new Error(`OpenReader BookGroup server did not start\n${processOutput()}`)
}

async function startOpenReader() {
  await access(join(publicDir, 'index.html')).catch(() => {
    throw new Error('frontend/dist is missing; run `cd frontend && npm run build` before this smoke')
  })
  const tempRoot = await mkdtemp(join(tmpdir(), 'openreader-book-group-real-api-'))
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
      OPENREADER_JWT_SECRET: 'book-group-real-api-contract-secret',
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
  const data = text ? JSON.parse(text) : null
  if (!response.ok) throw new Error(`${method} ${path} failed with ${response.status}: ${text}`)
  return data
}

async function seedWorkspace(root, viewport) {
  const suffix = `${viewport.width}x${viewport.height}`
  const auth = await api(root, '/auth/register', {
    method: 'POST',
    body: { username: `bookgroup${viewport.width}${viewport.height}`, password: 'book-group-contract' },
  })
  assert(auth?.token, `${suffix}: registration did not return a token`)
  const category = await api(root, '/categories', {
    token: auth.token,
    method: 'POST',
    body: { name: `收藏 ${suffix}` },
  })
  const books = [
    { title: `本地收藏 ${suffix}`, url: `local://book-group/${suffix}/local`, sourceId: 0, type: 0, categoryIds: [category.id] },
    { title: `音频书 ${suffix}`, url: `https://book-group.example/${suffix}/audio`, sourceId: 8, type: 1 },
    { title: `未分组书 ${suffix}`, url: `https://book-group.example/${suffix}/plain`, sourceId: 8, type: 0 },
  ]
  for (const book of books) {
    await api(root, '/books', { token: auth.token, method: 'POST', body: book })
  }
  const groups = await api(root, '/book-groups', { token: auth.token })
  assert(groups.length === 5, `${suffix}: expected four built-ins plus one category, got ${groups.length}`)
  return { token: auth.token, category, suffix }
}

async function openMobileNavigation(page, viewport) {
  if (viewport.width > 750) return
  await page.locator('.mobile-menu-trigger').click()
  await page.waitForFunction(() => {
    const sidebar = document.querySelector('.app-sidebar')
    return sidebar && Math.abs(Number.parseFloat(getComputedStyle(sidebar).marginLeft)) < 0.5
  })
}

async function openGroupManager(page, viewport) {
  await openMobileNavigation(page, viewport)
  await page.getByRole('button', { name: '分组管理', exact: true }).click()
  const dialog = page.locator('.global-book-group-dialog')
  await dialog.waitFor({ state: 'visible', timeout: 15_000 })
  return dialog
}

function groupRow(dialog, text) {
  return dialog.locator('.group-manage-table tbody tr').filter({ hasText: text })
}

async function renameGroup(page, dialog, row, name) {
  await row.getByRole('button', { name: '编辑', exact: true }).click()
  const prompt = page.locator('.el-message-box').filter({ hasText: '重命名分组' })
  await prompt.waitFor({ state: 'visible', timeout: 10_000 })
  await prompt.locator('input').fill(name)
  await prompt.getByRole('button', { name: '确定', exact: true }).click()
  await prompt.waitFor({ state: 'hidden', timeout: 10_000 })
}

async function dragFirstRowToEnd(page, dialog) {
  const rows = dialog.locator('.group-manage-table tbody tr')
  const handle = rows.first().locator('.group-drag-handle')
  const target = rows.last()
  const from = await handle.boundingBox()
  const to = await target.boundingBox()
  assert(from && to, 'BookGroup drag geometry is unavailable')
  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2)
  await page.mouse.down()
  await page.mouse.move(to.x + to.width / 2, to.y + to.height - 4, { steps: 14 })
  await page.mouse.up()
}

async function assertVisibleTabs(page, expected, suffix) {
  const tabs = page.locator('.book-group-wrapper .group-chip')
  await tabs.first().waitFor({ state: 'visible', timeout: 15_000 })
  const names = (await tabs.allTextContents()).map(value => value.trim())
  for (const name of expected) assert(names.includes(name), `${suffix}: missing shelf group tab ${name}; got ${names.join(', ')}`)
}

async function runViewport(browser, root, viewport) {
  const seeded = await seedWorkspace(root, viewport)
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 750,
    hasTouch: viewport.width <= 750,
  })
  await context.addInitScript(token => localStorage.setItem('openreader_token', token), seeded.token)
  const pageA = await context.newPage()
  const pageB = await context.newPage()
  const failures = []
  for (const page of [pageA, pageB]) {
    page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
    page.on('console', message => {
      if (message.type() === 'error' && !/WebSocket connection to .*\/ws\/sync/.test(message.text())) {
        failures.push(`console.error: ${message.text()}`)
      }
    })
  }

  try {
    await Promise.all([
      pageA.goto(root, { waitUntil: 'domcontentloaded' }),
      pageB.goto(root, { waitUntil: 'domcontentloaded' }),
    ])
    const originalTabs = ['全部', '本地', '音频', '未分组', seeded.category.name]
    await assertVisibleTabs(pageA, originalTabs, seeded.suffix)
    await assertVisibleTabs(pageB, originalTabs, seeded.suffix)

    const dialog = await openGroupManager(pageA, viewport)
    const rows = dialog.locator('.group-manage-table tbody tr')
    await rows.first().waitFor({ state: 'visible', timeout: 15_000 })
    assert(await rows.count() === 5, `${seeded.suffix}: manager did not show four built-ins plus custom group`)
    assert(await groupRow(dialog, '全部(全部)').count() === 1, `${seeded.suffix}: built-in semantic suffix is missing`)

    const audioName = `有声 ${seeded.suffix}`
    await renameGroup(pageA, dialog, groupRow(dialog, '音频(音频)'), audioName)
    await pageA.getByText('分组已重命名', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 })
    await pageB.locator('.book-group-wrapper .group-chip').filter({ hasText: audioName }).waitFor({ state: 'visible', timeout: 10_000 })

    const customRow = groupRow(dialog, seeded.category.name)
    await customRow.locator('.el-switch').click()
    await pageA.getByText('分组已隐藏', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 })
    await pageB.locator('.book-group-wrapper .group-chip').filter({ hasText: seeded.category.name }).waitFor({ state: 'hidden', timeout: 10_000 })

    await dragFirstRowToEnd(pageA, dialog)
    const saveOrder = dialog.getByRole('button', { name: '保存排序', exact: true })
    await saveOrder.waitFor({ state: 'visible', timeout: 10_000 })
    await saveOrder.click()
    await pageA.getByText('分组排序已更新', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 })
    const persisted = await api(root, '/book-groups', { token: seeded.token })
    assert(persisted.at(-1)?.key === 'builtin:all', `${seeded.suffix}: mixed drag order was not persisted`)

    const hidden = persisted.find(group => group.key === `category:${seeded.category.id}`)
    assert(hidden?.show === false, `${seeded.suffix}: custom group visibility was not persisted`)
    assert(persisted.find(group => group.key === 'builtin:audio')?.name === audioName, `${seeded.suffix}: built-in rename was not persisted`)
    assert(failures.length === 0, `${seeded.suffix}: ${failures.join('\n')}`)
    return seeded.suffix
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
      console.log(`book-group-real-api: ok ${completed.join(', ')} realApi=true apiMocked=false multiClient=true mixedDrag=true`)
    } finally {
      await browser.close()
    }
  } finally {
    await app.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exitCode = 1
})
