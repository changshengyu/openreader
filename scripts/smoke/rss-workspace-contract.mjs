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
      console.error('Playwright is required for the RSS workspace smoke.')
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

function rssSource() {
  return {
    id: 1,
    title: '契约 RSS 源',
    url: 'https://rss.example/feed.xml',
    customOrder: 1,
    enabled: true,
    singleUrl: true,
  }
}

function rssArticle() {
  return {
    id: 7,
    sourceId: 1,
    title: '契约 RSS 文章',
    summary: '契约文章摘要',
    author: 'OpenReader',
    pubDate: '2026-07-12',
    image: 'https://rss.example/cover.jpg',
    isRead: false,
    favorite: false,
    link: 'https://rss.example/article/7',
  }
}

async function installApiMocks(page) {
  let refreshCalls = 0
  await page.exposeFunction('__rssSmokeRefreshCalls', () => refreshCalls)
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname.replace(/^\/api/, '')
    const method = request.method()

    if (path === '/me') return route.fulfill(json({ id: 1, username: 'rss-smoke', role: 'admin' }))
    if (path === '/health') return route.fulfill(json({ version: 'smoke', commit: 'rss-workspace' }))
    if (path === '/settings/reader' && method === 'GET') return route.fulfill(json({ key: 'reader', value: { theme: 'parchment', mode: 'page', pageMode: 'auto' } }))
    if (path === '/settings/reader' && method === 'PUT') return route.fulfill(json({ key: 'reader', value: {} }))
    if (path === '/settings/preferences') return route.fulfill(json({ key: 'preferences', value: {} }))
    if (path === '/books' || path === '/categories' || path === '/sources') return route.fulfill(json([]))
    if (path === '/cache/stats') return route.fulfill(json({ files: 0, size: 0, cachedChapters: 0 }))
    if (path === '/rss/sources' && method === 'GET') return route.fulfill(json([rssSource()]))
    if (path === '/rss/sources/1/refresh' && method === 'POST') {
      refreshCalls += 1
      return route.fulfill(json({ imported: 1, total: 1 }))
    }
    if (path === '/rss/articles' && method === 'GET') return route.fulfill(json({ items: [rssArticle()], page: 1, hasMore: false }))
    if (path === '/rss/articles/7/content' && method === 'GET') {
      return route.fulfill(json({
        content: '<p>契约 RSS 正文</p><img src="https://rss.example/content.jpg" alt="契约图片">',
        link: 'https://rss.example/article/7',
      }))
    }
    if (path === '/rss/articles/7' && method === 'PUT') return route.fulfill(json({ ...rssArticle(), isRead: true }))
    if (path === '/local-store') return route.fulfill(json({ path: '', items: [] }))
    if (path === '/backup/list') return route.fulfill(json([]))
    if (path === '/webdav/list') return route.fulfill(json({ path: '', items: [] }))
    if (path === '/replace-rules' || path === '/admin/users') return route.fulfill(json([]))
    return route.fulfill(json({}))
  })
}

async function assertNoHorizontalOverflow(page, label) {
  const geometry = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: innerWidth }))
  assert(geometry.width <= geometry.viewport + 1, `${label}: horizontal overflow ${geometry.width} > ${geometry.viewport}`)
}

async function assertDialogGeometry(page, selector, viewport, label) {
  const geometry = await page.locator(selector).evaluate(node => {
    const rect = node.getBoundingClientRect()
    return { left: rect.left, top: rect.top, width: rect.width, height: rect.height, viewportWidth: innerWidth, viewportHeight: innerHeight }
  })
  if (viewport.width <= 750) {
    assert(Math.abs(geometry.left) <= 1 && Math.abs(geometry.top) <= 1, `${label}: mobile dialog must start at the viewport origin`)
    assert(Math.abs(geometry.width - geometry.viewportWidth) <= 1 && Math.abs(geometry.height - geometry.viewportHeight) <= 1, `${label}: mobile dialog must be fullscreen`)
  } else {
    assert(Math.abs(geometry.left - (geometry.viewportWidth - geometry.width) / 2) <= 1, `${label}: desktop dialog must be centred`)
  }
}

async function closeDialog(page, selector) {
  await page.locator(`${selector} > .el-dialog__headerbtn`).click()
  await page.locator(selector).waitFor({ state: 'hidden', timeout: 10000 })
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 750,
    hasTouch: viewport.width <= 750,
  })
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error' && !/WebSocket connection to .*\/ws\/sync/.test(message.text())) failures.push(`console.error: ${message.text()}`)
  })
  await page.addInitScript(token => localStorage.setItem('openreader_token', token), fakeToken())
  await installApiMocks(page)

  const root = targetUrl.replace(/\/$/, '')
  await page.goto(`${root}/settings?panel=rss&keep=rss-contract`, { waitUntil: 'networkidle' })
  await page.locator('.global-rss-dialog').waitFor({ state: 'visible', timeout: 10000 })
  await page.getByText('契约 RSS 源', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  await assertDialogGeometry(page, '.global-rss-dialog', viewport, `${viewport.width} source`)
  await assertNoHorizontalOverflow(page, `${viewport.width} source`)
  assert(await page.evaluate(() => window.__rssSmokeRefreshCalls()) === 0, `${viewport.width}: opening the source dialog must not refresh an article source`)
  assert(await page.locator('.rss-article-list-dialog').count() === 0, `${viewport.width}: source dialog must not skip directly to an article list`)

  await page.locator('.rss-source-card button').click()
  await page.locator('.rss-article-list-dialog').waitFor({ state: 'visible', timeout: 10000 })
  await page.getByText('契约 RSS 文章', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  await assertDialogGeometry(page, '.rss-article-list-dialog', viewport, `${viewport.width} article-list`)
  assert(await page.evaluate(() => window.__rssSmokeRefreshCalls()) === 1, `${viewport.width}: selecting one source must run one refresh`)

  await page.locator('.rss-article-list-dialog .rss-article-row > button').click()
  await page.locator('.rss-article-content-dialog').waitFor({ state: 'visible', timeout: 10000 })
  await page.getByText('契约 RSS 正文', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  await assertDialogGeometry(page, '.rss-article-content-dialog', viewport, `${viewport.width} article-content`)
  await page.locator('.rss-article-content-dialog .rss-reader-content img').click()
  await page.locator('.el-image-viewer__wrapper').waitFor({ state: 'visible', timeout: 10000 })
  await page.keyboard.press('Escape')
  await page.locator('.el-image-viewer__wrapper').waitFor({ state: 'hidden', timeout: 10000 })

  await closeDialog(page, '.rss-article-content-dialog')
  await page.locator('.rss-article-list-dialog').waitFor({ state: 'visible', timeout: 10000 })
  await closeDialog(page, '.rss-article-list-dialog')
  await page.locator('.global-rss-dialog').waitFor({ state: 'visible', timeout: 10000 })
  await closeDialog(page, '.global-rss-dialog')
  await page.waitForFunction(() => new URLSearchParams(location.search).get('overlay') !== 'rss')

  await page.goto(`${root}/?overlay=rss&keep=rss-contract`, { waitUntil: 'networkidle' })
  await page.locator('.global-rss-dialog').waitFor({ state: 'visible', timeout: 10000 })
  assert(await page.locator('.rss-article-list-dialog').count() === 0, `${viewport.width}: reopening RSS must not restore a stale article dialog`)
  await closeDialog(page, '.global-rss-dialog')

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
    console.log(`rss-workspace: ok ${checks.join(', ')} sourceArticleContentDialogs=true refreshOnce=true`)
  } finally {
    await browser.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
