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
      console.error('Playwright is required for Index mobile sidebar contract smoke.')
      console.error(`Original import error: ${error.message}`)
      process.exit(2)
    }
  }
}

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function json(data, status = 200) {
  return {
    status,
    contentType: 'application/json',
    body: JSON.stringify(data),
  }
}

function fakeToken() {
  const payload = Buffer.from(JSON.stringify({ userId: 1, sub: '1' })).toString('base64url')
  return `open.${payload}.reader`
}

async function installApiMocks(page) {
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()

    if (path === '/me') return route.fulfill(json({ id: 1, username: 'smoke', role: 'admin' }))
    if (path === '/health') return route.fulfill(json({ version: 'smoke', commit: 'sidebar-contract' }))
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(json({
        key: 'reader',
        updatedAt: '2026-07-07T00:00:00Z',
        value: { theme: 'parchment', mode: 'page', pageMode: 'auto' },
      }))
    }
    if (path === '/settings/reader' && method === 'PUT') {
      return route.fulfill(json({ key: 'reader', updatedAt: '2026-07-07T00:00:01Z', value: {} }))
    }
    if (path === '/settings/preferences') {
      return route.fulfill(json({ key: 'preferences', value: {} }))
    }
    if (path === '/books') {
      return route.fulfill(json([
        { id: 1, title: '侧栏契约测试', author: 'OpenReader', chapterCount: 1, updatedAt: '2026-07-07T00:00:00Z' },
      ]))
    }
    if (path === '/categories') return route.fulfill(json([]))
    if (path === '/sources') return route.fulfill(json([{ id: 1, name: '测试书源', enabled: true }]))
    if (path.startsWith('/cache')) return route.fulfill(json({ total: 0, books: 0, chapters: 0 }))
    return route.fulfill(json({}))
  })
}

async function rect(page, selector) {
  return page.locator(selector).evaluate((node) => {
    const r = node.getBoundingClientRect()
    return { left: r.left, top: r.top, width: r.width, height: r.height }
  })
}

async function sidebarMargin(page) {
  return page.locator('.app-sidebar').evaluate(node => Number.parseFloat(window.getComputedStyle(node).marginLeft))
}

async function waitForSidebarMargin(page, expected) {
  await page.waitForFunction((value) => {
    const node = document.querySelector('.app-sidebar')
    if (!node) return false
    return Math.abs(Number.parseFloat(window.getComputedStyle(node).marginLeft) - value) < 0.5
  }, expected)
}

async function dispatchTouch(page, type, x, y) {
  await page.locator('.app-shell').dispatchEvent(type, {
    touches: type === 'touchend' ? [] : [{ clientX: x, clientY: y, identifier: 1 }],
    changedTouches: [{ clientX: x, clientY: y, identifier: 1 }],
  })
}

async function assertShelfGeometry(page, viewport) {
  await page.waitForSelector('.book-row', { timeout: 10000 })
  const geometry = await page.evaluate(() => {
    const title = document.querySelector('.shelf-title')
    const group = document.querySelector('.book-group-wrapper')
    const row = document.querySelector('.book-row')
    const cover = document.querySelector('.list-cover')
    const titleStyle = window.getComputedStyle(title)
    const groupRect = group.getBoundingClientRect()
    const rowStyle = window.getComputedStyle(row)
    const coverRect = cover.getBoundingClientRect()
    return {
      titlePaddingLeft: Number.parseFloat(titleStyle.paddingLeft),
      titlePaddingRight: Number.parseFloat(titleStyle.paddingRight),
      titleFontSize: Number.parseFloat(window.getComputedStyle(title.querySelector('strong')).fontSize),
      groupLeft: groupRect.left,
      groupRightInset: window.innerWidth - groupRect.right,
      rowPaddingLeft: Number.parseFloat(rowStyle.paddingLeft),
      rowPaddingRight: Number.parseFloat(rowStyle.paddingRight),
      coverWidth: coverRect.width,
      coverHeight: coverRect.height,
      scrollWidth: document.documentElement.scrollWidth,
      innerWidth: window.innerWidth,
    }
  })

  assert(Math.abs(geometry.titlePaddingLeft - 24) < 0.5, `${viewport.width}: title left padding should be 24px, got ${geometry.titlePaddingLeft}`)
  assert(Math.abs(geometry.titlePaddingRight - 24) < 0.5, `${viewport.width}: title right padding should be 24px, got ${geometry.titlePaddingRight}`)
  assert(Math.abs(geometry.titleFontSize - 20) < 0.5, `${viewport.width}: title font should be 20px, got ${geometry.titleFontSize}`)
  assert(Math.abs(geometry.groupLeft - 24) < 0.5, `${viewport.width}: group left inset should be 24px, got ${geometry.groupLeft}`)
  assert(Math.abs(geometry.groupRightInset - 24) < 0.5, `${viewport.width}: group right inset should be 24px, got ${geometry.groupRightInset}`)
  assert(Math.abs(geometry.rowPaddingLeft - 20) < 0.5, `${viewport.width}: row left padding should be 20px, got ${geometry.rowPaddingLeft}`)
  assert(Math.abs(geometry.rowPaddingRight - 20) < 0.5, `${viewport.width}: row right padding should be 20px, got ${geometry.rowPaddingRight}`)
  assert(Math.abs(geometry.coverWidth - 84) < 0.5, `${viewport.width}: cover width should be 84px, got ${geometry.coverWidth}`)
  assert(Math.abs(geometry.coverHeight - 112) < 0.5, `${viewport.width}: cover height should be 112px, got ${geometry.coverHeight}`)
  assert(geometry.scrollWidth <= geometry.innerWidth + 1, `${viewport.width}: page should not horizontally overflow: ${geometry.scrollWidth} > ${geometry.innerWidth}`)
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({
    viewport,
    isMobile: true,
    hasTouch: true,
  })
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() !== 'error') return
    const text = message.text()
    if (/WebSocket connection to .*\/ws\/sync/.test(text)) return
    failures.push(`console.error: ${text}`)
  })
  await page.addInitScript((token) => {
    window.localStorage.setItem('openreader_token', token)
  }, fakeToken())
  await installApiMocks(page)

  await page.goto(targetUrl, { waitUntil: 'networkidle' })
  await page.waitForSelector('.app-shell.mobile-shell .app-sidebar', { timeout: 10000 })
  await page.waitForSelector('.mobile-menu-trigger', { timeout: 10000 })
  await assertShelfGeometry(page, viewport)

  const closedMargin = await sidebarMargin(page)
  assert(Math.abs(closedMargin + 260) < 0.5, `${viewport.width}: sidebar should default hidden at -260px, got ${closedMargin}`)

  await page.locator('.mobile-menu-trigger').click()
  await waitForSidebarMargin(page, 0)
  const openedSidebar = await rect(page, '.app-sidebar')
  assert(Math.abs(openedSidebar.width - 260) < 0.5, `${viewport.width}: sidebar width should be 260px, got ${openedSidebar.width}`)

  const beforeScroll = await rect(page, '.sidebar-bottom-icons')
  await page.locator('.app-sidebar-scroll').evaluate(node => {
    node.scrollTop = 600
    node.dispatchEvent(new Event('scroll', { bubbles: true }))
  })
  await page.waitForTimeout(100)
  const afterScroll = await rect(page, '.sidebar-bottom-icons')
  assert(Math.abs(beforeScroll.top - afterScroll.top) < 0.5, `${viewport.width}: bottom icons should not move vertically on sidebar scroll: ${beforeScroll.top} -> ${afterScroll.top}`)
  assert(Math.abs(beforeScroll.left - afterScroll.left) < 0.5, `${viewport.width}: bottom icons should not move horizontally on sidebar scroll: ${beforeScroll.left} -> ${afterScroll.left}`)

  await page.locator('.theme-toggle').click()
  await waitForSidebarMargin(page, 0)

  await page.mouse.click(Math.max(300, viewport.width - 40), 420)
  await waitForSidebarMargin(page, -260)

  await dispatchTouch(page, 'touchstart', 60, 220)
  await dispatchTouch(page, 'touchmove', 330, 222)
  const duringDrag = await sidebarMargin(page)
  assert(Math.abs(duringDrag) < 0.5, `${viewport.width}: 270px drag should bring sidebar to 0px margin, got ${duringDrag}`)
  const dragIcons = await rect(page, '.sidebar-bottom-icons')
  assert(Math.abs(dragIcons.left - beforeScroll.left) < 0.5, `${viewport.width}: bottom icons should stay stable during drag, got ${dragIcons.left} vs ${beforeScroll.left}`)
  await dispatchTouch(page, 'touchend', 330, 222)
  await waitForSidebarMargin(page, 0)

  assert(failures.length === 0, failures.join('\n'))
  await context.close()
  return `${viewport.width}x${viewport.height}`
}

async function run() {
  const { chromium } = await loadPlaywright()
  const browser = await chromium.launch({
    headless: true,
    executablePath: process.env.CHROME_PATH || defaultChromePath,
  })
  try {
    const results = []
    results.push(await runViewport(browser, { width: 390, height: 844 }))
    results.push(await runViewport(browser, { width: 360, height: 800 }))
    console.log(`index-mobile-sidebar: ok ${results.join(', ')} width=260 dragLimit=270 fixedBottomIcons=true shelfGeometry=true`)
  } finally {
    await browser.close()
  }
}

run().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})
