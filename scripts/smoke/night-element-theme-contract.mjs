#!/usr/bin/env node

// reader-dev App.vue#setTheme hangs its Element night overrides (App.vue 939–1141) off
// body.night-theme for every page and dialog (user decision 2026-10-30: global). This
// contract checks the home workspace: the book management dialog (table, checkbox,
// search input, buttons, row dropdown, selection counter), a sidebar message box and a
// sidebar select dropdown, day -> night -> day. The light sidebar select surface (D5) and
// the standalone login page (D6) keep their day colors at night.
// docs/compat/night-element-plus-theme-fixed-baseline-second-audit-p1-contract.md

import { appendFileSync } from 'node:fs'
import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function json(data, status = 200) {
  return { status, contentType: 'application/json', body: JSON.stringify(data) }
}

function fakeToken() {
  const payload = Buffer.from(JSON.stringify({ userId: 1, sub: '1' })).toString('base64url')
  return `open.${payload}.reader`
}

function shelfBooks() {
  return [
    { id: 1, title: '远程书架书', author: 'OpenReader', sourceId: 1, categoryIds: [1], chapterCount: 25, cachedChapterCount: 1, lastChapter: '第二十五章' },
    { id: 2, title: '另一远程书', author: 'OpenReader', sourceId: 1, categoryIds: [1], chapterCount: 25, cachedChapterCount: 0, lastChapter: '第二十五章' },
  ]
}

async function installApiMocks(page) {
  let readerSettings = { theme: 'parchment', mode: 'page', pageMode: 'auto', pageType: 'normal' }
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname.replace(/^\/api/, '')
    const method = request.method()
    if (path === '/me') return route.fulfill(json({ id: 1, username: 'night-smoke', role: 'admin' }))
    if (path === '/health') return route.fulfill(json({ version: 'smoke', commit: 'night-element-theme' }))
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(json({ key: 'reader', updatedAt: '2026-10-30T00:00:00Z', value: readerSettings }))
    }
    if (path === '/settings/reader' && method === 'PUT') {
      const payload = request.postDataJSON() || {}
      readerSettings = { ...readerSettings, ...(payload.value || payload) }
      return route.fulfill(json({ key: 'reader', updatedAt: new Date().toISOString(), value: readerSettings }))
    }
    if (path === '/settings/preferences') return route.fulfill(json({ key: 'preferences', value: {} }))
    if (path === '/books' && method === 'GET') return route.fulfill(json(shelfBooks()))
    if (path === '/categories') return route.fulfill(json([{ id: 1, name: '测试分组', show: true, sortOrder: 10 }]))
    if (path === '/sources') return route.fulfill(json([{ id: 1, name: '测试书源', enabled: true }]))
    if (path.startsWith('/cache')) return route.fulfill(json({ total: 0, books: 0, chapters: 0 }))
    return route.fulfill(json({}))
  })
}

async function ensureNavigation(page, viewport) {
  if (viewport.width > 750) return
  const open = await page.evaluate(() => {
    const sidebar = document.querySelector('.app-sidebar')
    return Boolean(sidebar) && Math.abs(Number.parseFloat(getComputedStyle(sidebar).marginLeft)) < 0.5
  })
  if (open) return
  await page.locator('.mobile-menu-trigger').click()
  await page.waitForFunction(() => {
    const sidebar = document.querySelector('.app-sidebar')
    return sidebar && Math.abs(Number.parseFloat(getComputedStyle(sidebar).marginLeft)) < 0.5
  })
}

async function renderedStyle(page, selector, properties) {
  return page.evaluate(({ selector, properties }) => {
    const element = [...document.querySelectorAll(selector)].find(node => node.getClientRects().length)
    if (!element) return null
    const style = window.getComputedStyle(element)
    return Object.fromEntries(properties.map(property => [property, style.getPropertyValue(property)]))
  }, { selector, properties })
}

async function waitForDialogSettled(page, selector) {
  await page.waitForFunction((target) => {
    const dialog = document.querySelector(target)
    const overlay = dialog?.parentElement
    if (!dialog || !overlay || Math.abs(overlay.getBoundingClientRect().top) >= 0.5) return false
    const settled = value => value === 'none' || value === 'matrix(1, 0, 0, 1, 0, 0)'
    return settled(getComputedStyle(dialog).transform) && Number(getComputedStyle(dialog).opacity || 1) === 1
  }, selector, { timeout: 10000 })
}

async function homeElementSnapshot(page, viewport) {
  const box = ['background-color', 'color', 'border-top-color']
  const snapshot = { body: await renderedStyle(page, 'body', ['background-color']) }

  await ensureNavigation(page, viewport)
  const sidebarSelect = '.app-sidebar .setting-select'
  snapshot.sidebarSelect = await renderedStyle(page, `${sidebarSelect} .el-select__wrapper`, ['background-color', 'box-shadow'])
  snapshot.sidebarSelectText = await renderedStyle(page, `${sidebarSelect} .el-select__placeholder`, ['color'])
  await page.locator(`${sidebarSelect} .el-select__wrapper`).first().click()
  const option = '.el-select__popper .el-select-dropdown__item'
  await page.waitForFunction(selector => [...document.querySelectorAll(selector)].some(node => node.getClientRects().length), option, { timeout: 10000 })
  await page.waitForTimeout(300)
  snapshot.sidebarDropdown = await renderedStyle(page, '.el-select__popper', ['background-color', 'border-top-color'])
  snapshot.sidebarOption = await renderedStyle(page, `${option}:not(.is-selected)`, ['color'])
  await page.keyboard.press('Escape')
  await page.waitForFunction(selector => ![...document.querySelectorAll(selector)].some(node => node.getClientRects().length), option, { timeout: 10000 })

  await page.getByRole('button', { name: '书籍管理', exact: true }).click()
  const dialog = '.global-book-manage-dialog'
  await page.waitForSelector(`${dialog} .book-manage-table tbody tr`, { timeout: 10000 })
  await waitForDialogSettled(page, dialog)
  await page.mouse.move(1, 1)
  snapshot.dialog = await renderedStyle(page, dialog, ['background-color'])
  snapshot.dialogTitle = await renderedStyle(page, `${dialog} .book-manage-title > span:first-child`, ['color'])
  snapshot.searchInput = await renderedStyle(page, `${dialog} .search-input .el-input__wrapper`, ['background-color', 'box-shadow'])
  snapshot.searchInputText = await renderedStyle(page, `${dialog} .search-input .el-input__inner`, ['color'])
  snapshot.tableHeader = await renderedStyle(page, `${dialog} th.el-table__cell`, ['background-color', 'color', 'border-bottom-color'])
  snapshot.tableRow = await renderedStyle(page, `${dialog} .el-table__body tr`, ['background-color'])
  snapshot.tableCell = await renderedStyle(page, `${dialog} .el-table__body td.el-table__cell`, ['color', 'border-bottom-color'])
  snapshot.checkbox = await renderedStyle(page, `${dialog} .el-table__body .el-checkbox__inner`, ['background-color'])
  snapshot.counter = await renderedStyle(page, `${dialog} .check-tip`, ['color'])
  snapshot.primaryButton = await renderedStyle(page, `${dialog} .manage-footer .el-button--primary`, box)
  snapshot.cancelButton = await renderedStyle(page, `${dialog} .manage-footer > .el-button:not(.el-button--primary)`, box)

  const row = page.locator(`${dialog} .book-manage-table tbody tr`).filter({ hasText: '远程书架书' }).first()
  await row.getByRole('button', { name: '缓存', exact: true }).click()
  const menuItem = '.el-dropdown__popper .el-dropdown-menu__item'
  await page.waitForFunction(selector => [...document.querySelectorAll(selector)].some(node => node.getClientRects().length), menuItem, { timeout: 10000 })
  await page.waitForTimeout(300)
  snapshot.dropdown = await renderedStyle(page, '.el-dropdown__popper', ['background-color', 'border-top-color'])
  snapshot.dropdownItem = await renderedStyle(page, `${menuItem}:not(:hover)`, ['color'])
  await page.locator(`${dialog} .book-manage-title > span:first-child`).click()
  await page.waitForFunction(selector => ![...document.querySelectorAll(selector)].some(node => node.getClientRects().length), menuItem, { timeout: 10000 })
  await page.locator(`${dialog} .manage-footer > .el-button:not(.el-button--primary)`).click()
  await page.waitForSelector(dialog, { state: 'hidden', timeout: 10000 })

  await ensureNavigation(page, viewport)
  await page.getByRole('button', { name: '备份用户配置', exact: true }).click()
  await page.waitForSelector('.el-message-box', { state: 'visible', timeout: 10000 })
  await page.waitForTimeout(350)
  snapshot.messageBox = await renderedStyle(page, '.el-message-box', ['background-color', 'border-top-color'])
  snapshot.messageTitle = await renderedStyle(page, '.el-message-box__title', ['color'])
  snapshot.messageContent = await renderedStyle(page, '.el-message-box__content', ['color'])
  snapshot.messageCancel = await renderedStyle(page, '.el-message-box__btns .el-button:not(.el-button--primary)', box)
  // Element focuses the confirm button; reader-dev styles :focus like :hover (#2b67bb).
  snapshot.messageConfirm = await renderedStyle(page, '.el-message-box__btns .el-button--primary', box)
  await page.locator('.el-message-box__btns .el-button:not(.el-button--primary)').click()
  await page.waitForSelector('.el-message-box', { state: 'hidden', timeout: 10000 })
  return snapshot
}

const plainButton = { 'background-color': 'rgb(136, 136, 136)', color: 'rgb(221, 221, 221)', 'border-top-color': 'rgb(136, 136, 136)' }
const NIGHT = {
  body: { 'background-color': 'rgb(34, 34, 34)' },
  dialog: { 'background-color': 'rgb(34, 34, 34)' },
  dialogTitle: { color: 'rgb(187, 187, 187)' },
  searchInput: { 'background-color': 'rgb(68, 68, 68)', 'box-shadow': 'rgb(68, 68, 68) 0px 0px 0px 1px inset' },
  searchInputText: { color: 'rgb(221, 221, 221)' },
  tableHeader: { 'background-color': 'rgb(34, 34, 34)', color: 'rgb(144, 147, 153)', 'border-bottom-color': 'rgb(85, 85, 85)' },
  tableRow: { 'background-color': 'rgb(34, 34, 34)' },
  tableCell: { color: 'rgb(136, 136, 136)', 'border-bottom-color': 'rgb(85, 85, 85)' },
  checkbox: { 'background-color': 'rgb(187, 187, 187)' },
  counter: { color: 'rgb(187, 187, 187)' },
  primaryButton: { 'background-color': 'rgb(24, 87, 152)', color: 'rgb(255, 255, 255)', 'border-top-color': 'rgb(24, 87, 152)' },
  cancelButton: plainButton,
  dropdown: { 'background-color': 'rgb(68, 68, 68)', 'border-top-color': 'rgb(68, 68, 68)' },
  dropdownItem: { color: 'rgb(187, 187, 187)' },
  messageBox: { 'background-color': 'rgb(33, 33, 33)', 'border-top-color': 'rgb(33, 33, 33)' },
  messageTitle: { color: 'rgb(136, 136, 136)' },
  messageContent: { color: 'rgb(119, 119, 119)' },
  messageCancel: plainButton,
  messageConfirm: { 'background-color': 'rgb(43, 103, 187)', color: 'rgb(255, 255, 255)', 'border-top-color': 'rgb(43, 103, 187)' },
  sidebarDropdown: { 'background-color': 'rgb(51, 51, 51)', 'border-top-color': 'rgb(51, 51, 51)' },
  sidebarOption: { color: 'rgb(221, 221, 221)' },
}
// D5: the sidebar keeps its light look until it follows reader-dev Index.vue `.night`.
const DAY_AT_NIGHT = ['sidebarSelect', 'sidebarSelectText']

async function toggleTheme(page, viewport, night) {
  await ensureNavigation(page, viewport)
  await page.locator('.theme-toggle').click()
  await page.waitForFunction(expected => document.documentElement.classList.contains('dark-reader') === expected, night, { timeout: 10000 })
  await page.waitForTimeout(300)
}

async function runViewport(browser, viewport) {
  const mobile = viewport.width <= 750
  const context = await browser.newContext({ viewport, isMobile: mobile, hasTouch: mobile })
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error' && !/WebSocket connection to .*\/ws\/sync/.test(message.text())) failures.push(`console.error: ${message.text()}`)
  })
  await page.addInitScript(token => localStorage.setItem('openreader_token', token), fakeToken())
  await installApiMocks(page)
  await page.goto(targetUrl.replace(/\/$/, ''), { waitUntil: 'networkidle' })
  await page.waitForSelector('.shelf-page .book-row', { timeout: 10000 })
  assert(!(await page.evaluate(() => document.documentElement.classList.contains('dark-reader'))), `${viewport.width}: smoke must start in day mode`)

  const day = await homeElementSnapshot(page, viewport)
  if (process.env.SMOKE_DAY_SNAPSHOT) {
    // Evidence for "day unchanged": compare these lines between builds.
    appendFileSync(process.env.SMOKE_DAY_SNAPSHOT, `${JSON.stringify({ viewport: `${viewport.width}x${viewport.height}`, day })}\n`)
  }
  await toggleTheme(page, viewport, true)
  const night = await homeElementSnapshot(page, viewport)
  for (const [element, properties] of Object.entries(NIGHT)) {
    assert(night[element], `${viewport.width}: night ${element} is missing`)
    for (const [property, value] of Object.entries(properties)) {
      assert(night[element][property] === value, `${viewport.width}: night ${element} ${property} expected ${value}, got ${night[element][property]}`)
    }
  }
  for (const element of DAY_AT_NIGHT) {
    assert(JSON.stringify(night[element]) === JSON.stringify(day[element]), `${viewport.width}: night ${element} must keep its day colors: ${JSON.stringify(day[element])} -> ${JSON.stringify(night[element])}`)
  }
  await toggleTheme(page, viewport, false)
  const restored = await homeElementSnapshot(page, viewport)
  assert(JSON.stringify(restored) === JSON.stringify(day), `${viewport.width}: day Element Plus colors must restore exactly: ${JSON.stringify(day)} -> ${JSON.stringify(restored)}`)
  assert(failures.length === 0, failures.join('\n'))
  await context.close()
  return `${viewport.width}x${viewport.height}`
}

// D6: the standalone login page keeps its light design at night; only the in-app login
// dialog follows the night theme.
async function loginSnapshot(browser, viewport, colorScheme) {
  const mobile = viewport.width <= 750
  const context = await browser.newContext({ viewport, isMobile: mobile, hasTouch: mobile, colorScheme })
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  await installApiMocks(page)
  await page.goto(`${targetUrl.replace(/\/$/, '')}/login`, { waitUntil: 'networkidle' })
  await page.waitForSelector('.auth-card .el-input__wrapper', { timeout: 10000 })
  await page.mouse.move(1, 1)
  const box = ['background-color', 'color', 'border-top-color']
  const snapshot = {
    classes: await page.evaluate(() => ({
      darkReader: document.documentElement.classList.contains('dark-reader'),
      nightTheme: document.body.classList.contains('night-theme'),
    })),
    input: await renderedStyle(page, '.auth-card .el-input__wrapper', ['background-color', 'box-shadow']),
    inputText: await renderedStyle(page, '.auth-card .el-input__inner', ['color']),
    submit: await renderedStyle(page, '.auth-card .el-button--primary', box),
  }
  assert(failures.length === 0, failures.join('\n'))
  await context.close()
  return snapshot
}

async function runLoginViewport(browser, viewport) {
  const day = await loginSnapshot(browser, viewport, 'light')
  const night = await loginSnapshot(browser, viewport, 'dark')
  assert(night.classes.darkReader && !day.classes.darkReader, `${viewport.width}: login page must follow the device color scheme: ${JSON.stringify([day.classes, night.classes])}`)
  assert(!night.classes.nightTheme, `${viewport.width}: the login page must not take body.night-theme`)
  for (const element of ['input', 'inputText', 'submit']) {
    assert(JSON.stringify(night[element]) === JSON.stringify(day[element]), `${viewport.width}: login ${element} must keep its day colors: ${JSON.stringify(day[element])} -> ${JSON.stringify(night[element])}`)
  }
}

async function run() {
  const browser = await openSmokeBrowser()
  try {
    const requested = process.env.SMOKE_VIEWPORT
    const viewports = requested
      ? [requested.split('x').map(Number)].map(([width, height]) => ({ width, height }))
      : [
          { width: 1440, height: 900 },
          { width: 390, height: 844 },
          { width: 360, height: 800 },
          { width: 1024, height: 1366 },
          { width: 1366, height: 1024 },
        ]
    const checks = []
    for (const viewport of viewports) {
      checks.push(await runViewport(browser, viewport))
      await runLoginViewport(browser, viewport)
    }
    console.log(`night-element-theme: ok ${checks.join(', ')}`)
  } finally {
    await browser.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
