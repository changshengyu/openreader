#!/usr/bin/env node
// Contract: docs/compat/user-book-limit-fixed-baseline-second-audit-p2-contract.md
//
// Run against a real OpenReader server on a fresh data folder started with
//   OPENREADER_USER_BOOK_LIMIT=$USER_BOOK_LIMIT (default 2 here)
// e.g. TARGET_URL=http://127.0.0.1:8080 USER_BOOK_LIMIT=2 node scripts/smoke/user-book-limit-contract.mjs
//
// reader-dev BookController.kt#saveBook refuses a new book once the shelf holds
// userBookLimit books, and plugins/axios.js shows 超过用户书籍数上限 as an error
// message. Each viewport fills a fresh account's shelf to the limit through the
// API, then imports one more TXT through the real 导入书籍 dialog.
//
// USER_BOOK_LIMIT=unlimited checks the default since 2026-11-13 (contract section
// 12) on a server started WITHOUT OPENREADER_USER_BOOK_LIMIT: each viewport puts
// reader-dev's default of 200 books on a fresh shelf through the API, expects
// book 201 to be accepted, then imports one more TXT through the dialog.
import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetURL = (process.env.TARGET_URL || 'http://127.0.0.1:8080').replace(/\/$/, '')
const limitSetting = String(process.env.USER_BOOK_LIMIT || '2').trim()
const unlimited = limitSetting === 'unlimited'
const limit = unlimited ? 0 : Number.parseInt(limitSetting, 10)
const legacyDefaultLimit = 200
const runID = `${Date.now().toString(36)}${process.pid}`.replace(/[^a-z0-9]/gi, '').slice(-10)
const upstreamText = '超过用户书籍数上限'
const viewports = [
  ['desktop', { width: 1440, height: 900 }],
  ['mobile-390', { width: 390, height: 844 }],
  ['mobile-360', { width: 360, height: 800 }],
]

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

async function api(path, { method = 'GET', token = '', body } = {}) {
  const response = await fetch(`${targetURL}${path}`, {
    method,
    headers: {
      ...(body ? { 'Content-Type': 'application/json' } : {}),
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
    signal: AbortSignal.timeout(15_000),
  })
  const text = await response.text()
  let data = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    data = text
  }
  return { status: response.status, data }
}

async function fillShelf(name, count) {
  const username = `limit${name.replace(/[^a-z0-9]/g, '')}${runID}`.slice(0, 30)
  const registered = await api('/api/auth/register', { method: 'POST', body: { username, password: 'password88' } })
  assert(registered.status === 200 && registered.data?.token, `${name}: register ${registered.status} ${JSON.stringify(registered.data)}`)
  const token = registered.data.token
  const titles = []
  for (let index = 1; index <= count; index += 1) {
    const title = `上限书${index}-${name}`
    const created = await api('/api/books', { method: 'POST', token, body: { title } })
    assert(created.status === 201, `${name}: book ${index} of ${count} returned ${created.status} ${JSON.stringify(created.data)}`)
    titles.push(title)
  }
  return { token, titles }
}

// The server refuses the next new book with the agreed 403 body.
async function assertAPIRefusal(name, token) {
  const refused = await api('/api/books', { method: 'POST', token, body: { title: `超出-${name}` } })
  assert(refused.status === 403 && refused.data?.error === 'book limit reached',
    `${name}: book ${limit + 1} returned ${refused.status} ${JSON.stringify(refused.data)}, want 403 book limit reached`)
}

async function openImport(page) {
  const action = page.getByText('导入书籍', { exact: true })
  const mobileTrigger = page.getByLabel('打开侧边栏', { exact: true })
  const inViewport = async () => {
    const box = await action.boundingBox()
    const viewport = page.viewportSize()
    return Boolean(box && viewport && box.x < viewport.width && box.x + box.width > 0)
  }
  if (!await inViewport() && await mobileTrigger.isVisible()) {
    await mobileTrigger.click()
    await action.waitFor({ state: 'visible' })
    const actionElement = await action.elementHandle()
    await page.waitForFunction(element => {
      const rect = element?.getBoundingClientRect()
      return Boolean(rect && rect.left < innerWidth && rect.right > 0)
    }, actionElement)
    await action.scrollIntoViewIfNeeded()
  }
  assert(await inViewport(), 'import action is outside the viewport')
  await action.click()
  await page.locator('.direct-import-picker-dialog').waitFor()
}

async function assertNoHorizontalOverflow(page, label) {
  const geometry = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    viewportWidth: innerWidth,
  }))
  assert(geometry.scrollWidth <= geometry.viewportWidth + 1, `${label}: horizontal overflow ${geometry.scrollWidth} > ${geometry.viewportWidth}`)
}

async function runViewport(browser, name, viewport) {
  const { token, titles } = await fillShelf(name, limit)
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 750,
    hasTouch: viewport.width <= 750,
  })
  await context.addInitScript(value => localStorage.setItem('openreader_token', value), token)
  const page = await context.newPage()
  const pageErrors = []
  page.on('pageerror', error => pageErrors.push(String(error?.message || error)))
  const importStatuses = []
  page.on('response', response => {
    if (new URL(response.url()).pathname === '/api/imports/books') importStatuses.push(response.status())
  })
  try {
    await page.goto(targetURL, { waitUntil: 'networkidle' })
    await page.getByText(titles[0], { exact: true }).first().waitFor({ timeout: 15_000 })

    await openImport(page)
    const overTitle = `超出导入-${name}`
    await page.locator('.direct-import-picker-dialog input[type="file"]').setInputFiles([{
      name: `${overTitle}.txt`,
      mimeType: 'text/plain',
      buffer: Buffer.from('第一章 开始\n超过上限的书。', 'utf8'),
    }])
    const dialog = page.locator('.storage-import-single-dialog')
    await dialog.waitFor({ timeout: 15_000 })
    const before = await page.locator('.el-message--error .el-message__content').count()
    await dialog.getByRole('button', { name: '确定导入', exact: true }).click()
    const message = page.locator('.el-message--error .el-message__content', { hasText: upstreamText })
    try {
      await message.last().waitFor({ state: 'visible', timeout: 10_000 })
    } catch {
      const shown = await page.locator('.el-message__content').allTextContents()
      throw new Error(`${name}: expected the error message ${JSON.stringify(upstreamText)}, saw ${JSON.stringify(shown.slice(before))}; import statuses ${JSON.stringify(importStatuses)}`)
    }
    assert(importStatuses.at(-1) === 403, `${name}: import returned ${JSON.stringify(importStatuses)}, want 403`)
    // reader-dev keeps the import dialog open after a refused save.
    assert(await dialog.isVisible(), `${name}: the import dialog closed after the refusal`)
    await assertNoHorizontalOverflow(page, `${name} refused import`)
    await dialog.getByRole('button', { name: '取消', exact: true }).click()
    await dialog.waitFor({ state: 'hidden' })

    const shelf = await api('/api/books', { token })
    const shelfTitles = Array.isArray(shelf.data) ? shelf.data.map(book => book.title) : []
    assert(shelf.status === 200 && shelfTitles.length === limit, `${name}: shelf holds ${JSON.stringify(shelfTitles)}, want ${limit} books`)
    assert(!shelfTitles.includes(overTitle), `${name}: the refused book reached the shelf`)
    assert(await page.getByText(overTitle, { exact: true }).count() === 0, `${name}: the refused book is shown on the shelf`)
    await assertAPIRefusal(name, token)
    assert(pageErrors.length === 0, `${name}: page errors ${pageErrors.join('; ')}`)
  } finally {
    await context.close()
  }
}

// Default since 2026-11-13: no limit when OPENREADER_USER_BOOK_LIMIT is unset.
async function runUnlimitedViewport(browser, name, viewport) {
  const { token } = await fillShelf(name, legacyDefaultLimit)
  const extra = await api('/api/books', { method: 'POST', token, body: { title: `第${legacyDefaultLimit + 1}本-${name}` } })
  assert(extra.status === 201,
    `${name}: book ${legacyDefaultLimit + 1} with no limit returned ${extra.status} ${JSON.stringify(extra.data)}, want 201`)
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 750,
    hasTouch: viewport.width <= 750,
  })
  await context.addInitScript(value => localStorage.setItem('openreader_token', value), token)
  const page = await context.newPage()
  const pageErrors = []
  page.on('pageerror', error => pageErrors.push(String(error?.message || error)))
  const importStatuses = []
  page.on('response', response => {
    if (new URL(response.url()).pathname === '/api/imports/books') importStatuses.push(response.status())
  })
  try {
    await page.goto(targetURL, { waitUntil: 'networkidle' })
    await page.locator('.shelf-title', { hasText: `书架 (${legacyDefaultLimit + 1})` }).waitFor({ timeout: 20_000 })

    await openImport(page)
    const importedTitle = `不限导入-${name}`
    await page.locator('.direct-import-picker-dialog input[type="file"]').setInputFiles([{
      name: `${importedTitle}.txt`,
      mimeType: 'text/plain',
      buffer: Buffer.from('第一章 开始\n不限时可以继续导入。', 'utf8'),
    }])
    const dialog = page.locator('.storage-import-single-dialog')
    await dialog.waitFor({ timeout: 15_000 })
    await dialog.getByRole('button', { name: '确定导入', exact: true }).click()
    await dialog.waitFor({ state: 'hidden', timeout: 15_000 })
    assert(importStatuses.at(-1) === 201, `${name}: import returned ${JSON.stringify(importStatuses)}, want 201`)
    assert(await page.locator('.el-message__content', { hasText: upstreamText }).count() === 0,
      `${name}: ${upstreamText} was shown although no limit is set`)
    await page.locator('.shelf-title', { hasText: `书架 (${legacyDefaultLimit + 2})` }).waitFor({ timeout: 15_000 })
    await assertNoHorizontalOverflow(page, `${name} import with no limit`)

    const shelf = await api('/api/books', { token })
    const shelfTitles = Array.isArray(shelf.data) ? shelf.data.map(book => book.title) : []
    assert(shelf.status === 200 && shelfTitles.length === legacyDefaultLimit + 2,
      `${name}: shelf holds ${shelfTitles.length} books, want ${legacyDefaultLimit + 2}`)
    assert(shelfTitles.includes(importedTitle), `${name}: the imported book is missing from the shelf`)
    assert(pageErrors.length === 0, `${name}: page errors ${pageErrors.join('; ')}`)
  } finally {
    await context.close()
  }
}

async function main() {
  assert(unlimited || (Number.isInteger(limit) && limit > 0),
    `USER_BOOK_LIMIT must be a positive integer or "unlimited", got ${process.env.USER_BOOK_LIMIT}`)
  const health = await fetch(`${targetURL}/api/health`, { signal: AbortSignal.timeout(10_000) })
  assert(health.ok, `health returned ${health.status}`)
  const browser = await openSmokeBrowser()
  const checks = []
  try {
    for (const [name, viewport] of viewports) {
      if (unlimited) {
        await runUnlimitedViewport(browser, name, viewport)
        checks.push(`${name} ${viewport.width}x${viewport.height}: no limit, API accepts book ${legacyDefaultLimit + 1}, import ${legacyDefaultLimit + 2} succeeds without ${upstreamText}`)
      } else {
        await runViewport(browser, name, viewport)
        checks.push(`${name} ${viewport.width}x${viewport.height}: ${limit} books, API 403, import dialog shows ${upstreamText}, shelf unchanged`)
      }
    }
  } finally {
    await browser.close()
  }
  console.log(JSON.stringify({ status: 'pass', targetURL, limit: unlimited ? 'unlimited' : limit, checks }, null, 2))
}

main().catch(error => {
  console.error(error?.stack || error)
  process.exitCode = 1
})
