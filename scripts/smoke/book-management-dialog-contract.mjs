#!/usr/bin/env node

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'
const defaultChromePath = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'

async function loadPlaywright() {
  try {
    const module = await import('playwright')
    return module.chromium ? module : module.default
  } catch (error) {
    const bundled = '/Users/yuchangsheng/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright/index.js'
    try {
      const module = await import(bundled)
      return module.chromium ? module : module.default
    } catch {
      console.error('Playwright is required for the BookManage dialog smoke.')
      console.error(`Original import error: ${error.message}`)
      process.exit(2)
    }
  }
}

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
    { id: 1, title: '远程书架书', author: 'OpenReader', sourceId: 1, categoryIds: [1], chapterCount: 3, cachedChapterCount: 1, lastChapter: '第三章' },
    { id: 2, title: '本地书架书', author: 'OpenReader', sourceId: 0, chapterCount: 2 },
  ]
}

async function installApiMocks(page) {
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname.replace(/^\/api/, '')
    const method = request.method()

    if (path === '/me') return route.fulfill(json({ id: 1, username: 'manage-smoke', role: 'admin' }))
    if (path === '/health') return route.fulfill(json({ version: 'smoke', commit: 'book-manage-dialog' }))
    if (path === '/settings/reader' && method === 'GET') return route.fulfill(json({ key: 'reader', value: { theme: 'parchment', mode: 'page', pageMode: 'auto' } }))
    if (path === '/settings/reader' && method === 'PUT') return route.fulfill(json({ key: 'reader', value: {} }))
    if (path === '/settings/preferences') return route.fulfill(json({ key: 'preferences', value: {} }))
    if (path === '/books') return route.fulfill(json(shelfBooks()))
    if (path === '/books/1') return route.fulfill(json(shelfBooks()[0]))
    if (path === '/books/2') return route.fulfill(json(shelfBooks()[1]))
    if (path === '/categories') return route.fulfill(json([{ id: 1, name: '测试分组', show: true, sortOrder: 10 }]))
    if (path === '/sources') return route.fulfill(json([{ id: 1, name: '测试书源', enabled: true }]))
    if (path.startsWith('/cache')) return route.fulfill(json({ total: 0, books: 0, chapters: 0 }))
    return route.fulfill(json({}))
  })
}

async function assertNoHorizontalOverflow(page, label) {
  const geometry = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: innerWidth }))
  assert(geometry.width <= geometry.viewport + 1, `${label}: horizontal overflow ${geometry.width} > ${geometry.viewport}`)
}

async function openMobileNavigation(page, viewport) {
  if (viewport.width > 750) return
  await page.locator('.mobile-menu-trigger').click()
  await page.waitForFunction(() => {
    const sidebar = document.querySelector('.app-sidebar')
    return sidebar && Math.abs(Number.parseFloat(getComputedStyle(sidebar).marginLeft)) < 0.5
  })
}

async function assertMobileFullscreen(page, viewport, selector, label) {
  if (viewport.width > 750) return
  await page.waitForFunction(target => {
    const dialog = document.querySelector(target)
    const overlay = dialog?.closest('.el-overlay-dialog')
    return overlay && getComputedStyle(overlay).transform === 'none'
  }, selector)
  const geometry = await page.locator(selector).evaluate(node => {
    const rect = node.getBoundingClientRect()
    const overlay = node.closest('.el-overlay')
    const overlayRect = overlay?.getBoundingClientRect()
    const style = getComputedStyle(node)
    return {
      left: rect.left,
      top: rect.top,
      width: rect.width,
      height: rect.height,
      position: getComputedStyle(node).position,
      cssTop: style.top,
      marginTop: style.marginTop,
      transform: style.transform,
      scrollY,
      visualViewport: window.visualViewport && {
        offsetTop: window.visualViewport.offsetTop,
        pageTop: window.visualViewport.pageTop,
        height: window.visualViewport.height,
      },
      overlay: overlayRect && { left: overlayRect.left, top: overlayRect.top, width: overlayRect.width, height: overlayRect.height },
    }
  })
  assert(Math.abs(geometry.left) < 1 && Math.abs(geometry.top) < 1, `${viewport.width}: ${label} should start at the fullscreen origin, got ${JSON.stringify(geometry)}`)
  assert(Math.abs(geometry.width - viewport.width) < 1, `${viewport.width}: ${label} should fill the viewport width, got ${JSON.stringify(geometry)}`)
  assert(geometry.height >= viewport.height - 1, `${viewport.width}: ${label} should fill the viewport height, got ${JSON.stringify(geometry)}`)
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({ viewport, isMobile: viewport.width <= 750, hasTouch: viewport.width <= 750 })
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error' && !/WebSocket connection to .*\/ws\/sync/.test(message.text())) failures.push(`console.error: ${message.text()}`)
  })
  await page.addInitScript(token => localStorage.setItem('openreader_token', token), fakeToken())
  await installApiMocks(page)

  const root = targetUrl.replace(/\/$/, '')
  await page.goto(root, { waitUntil: 'networkidle' })
  await page.waitForSelector('.shelf-page .book-row', { timeout: 10000 })
  await openMobileNavigation(page, viewport)
  await page.getByRole('button', { name: '书籍管理' }).click()
  const manager = page.locator('.global-book-manage-dialog')
  await manager.waitFor({ state: 'visible', timeout: 10000 })
  await assertMobileFullscreen(page, viewport, '.global-book-manage-dialog', 'BookManage')
  await assertNoHorizontalOverflow(page, `${viewport.width} manage-open`)

  if (viewport.width <= 750) {
    await manager.locator('.mobile-manage-card p').first().click()
    const sidebarMargin = await page.locator('.app-sidebar').evaluate(node => Number.parseFloat(getComputedStyle(node).marginLeft))
    assert(Math.abs(sidebarMargin) < 0.5, `${viewport.width}: BookManage panel clicks must not close the mobile sidebar`)
    await manager.locator('.mobile-manage-card button').filter({ hasText: '远程书架书' }).click()
  } else {
    await manager.getByRole('button', { name: '远程书架书' }).click()
  }
  await page.waitForSelector('.book-info-dialog', { timeout: 10000 })
  assert(await manager.isVisible(), `${viewport.width}: BookInfo must coexist above the BookManage workspace dialog`)
  await page.locator('.book-info-dialog .el-dialog__headerbtn').click()
  await page.waitForFunction(() => !document.querySelector('.book-info-dialog .el-dialog'))
  assert(await manager.isVisible(), `${viewport.width}: closing BookInfo must leave BookManage open`)

  await manager.getByRole('button', { name: '取消', exact: true }).click()
  await manager.waitFor({ state: 'hidden', timeout: 10000 })

  await page.getByRole('button', { name: '分组管理' }).click()
  const groups = page.locator('.global-book-group-dialog')
  await groups.waitFor({ state: 'visible', timeout: 10000 })
  await assertMobileFullscreen(page, viewport, '.global-book-group-dialog', 'BookGroup')
  await assertNoHorizontalOverflow(page, `${viewport.width} groups-open`)
  assert(await groups.getByRole('button', { name: '添加分组', exact: true }).isVisible(), `${viewport.width}: group management must retain the create entry`)

  if (viewport.width <= 750) {
    await groups.locator('.group-table-name').first().click()
    const sidebarMargin = await page.locator('.app-sidebar').evaluate(node => Number.parseFloat(getComputedStyle(node).marginLeft))
    assert(Math.abs(sidebarMargin) < 0.5, `${viewport.width}: BookGroup panel clicks must not close the mobile sidebar`)
  }

  await groups.getByRole('button', { name: '取消', exact: true }).click()
  await groups.waitFor({ state: 'hidden', timeout: 10000 })
  assert(await page.evaluate(() => location.pathname) === '/', `${viewport.width}: closing BookManage must retain the root route`)
  await assertNoHorizontalOverflow(page, `${viewport.width} dialogs-close`)
  assert(failures.length === 0, failures.join('\n'))
  await context.close()
  return `${viewport.width}x${viewport.height}`
}

async function run() {
  const { chromium } = await loadPlaywright()
  const browser = await chromium.launch({ headless: true, executablePath: process.env.CHROME_PATH || defaultChromePath })
  try {
    const checks = []
    checks.push(await runViewport(browser, { width: 1440, height: 900 }))
    checks.push(await runViewport(browser, { width: 390, height: 844 }))
    checks.push(await runViewport(browser, { width: 360, height: 800 }))
    console.log(`book-management-dialog: ok ${checks.join(', ')} dialogs=true mobileFullscreen=true bookInfoCoexists=true groupManagement=true`)
  } finally {
    await browser.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
