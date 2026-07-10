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
      console.error('Playwright is required for the Index workspace smoke.')
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

function remoteBook(title = '工作台搜索结果') {
  return {
    title,
    author: 'OpenReader',
    url: `https://source.example/${encodeURIComponent(title)}`,
    bookUrl: `https://source.example/${encodeURIComponent(title)}`,
    sourceId: 1,
    sourceName: '工作台测试书源',
    latestChapter: '第一章',
    intro: '用于验证 Index 工作台搜索、书籍信息和阅读流程。',
  }
}

async function installApiMocks(page) {
  let shelfBooks = [{ id: 1, title: '书架测试书', author: 'OpenReader', chapterCount: 1 }]
  let remoteCreateCount = 0
  await page.exposeFunction('__workspaceRemoteCreateCount', () => remoteCreateCount)
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()

    if (path === '/me') return route.fulfill(json({ id: 1, username: 'workspace-smoke', role: 'admin' }))
    if (path === '/health') return route.fulfill(json({ version: 'smoke', commit: 'index-workspace' }))
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(json({ key: 'reader', value: { theme: 'parchment', mode: 'page', pageMode: 'auto' } }))
    }
    if (path === '/settings/reader' && method === 'PUT') return route.fulfill(json({ key: 'reader', value: {} }))
    if (path === '/settings/preferences') return route.fulfill(json({ key: 'preferences', value: {} }))
    if (path === '/categories') return route.fulfill(json([]))
    if (path === '/sources') return route.fulfill(json([{ id: 1, name: '工作台测试书源', enabled: true, group: '测试' }]))
    if (path === '/explore/sources') {
      return route.fulfill(json([{
        id: 1,
        name: '工作台测试书源',
        enabled: true,
        group: '测试',
        exploreGroups: [[{ name: '热门', url: 'https://source.example/explore' }]],
      }]))
    }
    if (path === '/explore/1') return route.fulfill(json({ items: [remoteBook('工作台探索结果')], page: 1, hasMore: false }))
    if (path === '/search' && method === 'POST') {
      const body = request.postDataJSON() || {}
      return route.fulfill(json({ list: [remoteBook(body.keyword || '工作台搜索结果')], page: 1, lastIndex: -1, hasMore: false }))
    }
    if (path === '/books/remote' && method === 'POST') {
      remoteCreateCount += 1
      const created = { id: 99, title: '已加入的工作台书籍', author: 'OpenReader', sourceId: 1, chapterCount: 1 }
      shelfBooks = [created, ...shelfBooks]
      return route.fulfill(json(created))
    }
    if (path === '/books') return route.fulfill(json(shelfBooks))
    if (path === '/books/99') return route.fulfill(json(shelfBooks.find(book => book.id === 99) || {}))
    if (path === '/books/99/chapters') return route.fulfill(json([{ index: 0, title: '第一章' }]))
    if (path === '/books/99/chapters/0/content') return route.fulfill(json({ title: '第一章', content: '工作台阅读验证内容。' }))
    if (path === '/books/99/bookmarks') return route.fulfill(json([]))
    if (path === '/books/1') return route.fulfill(json(shelfBooks.find(book => book.id === 1) || {}))
    if (path.startsWith('/cache')) return route.fulfill(json({ total: 0, books: 0, chapters: 0 }))
    return route.fulfill(json({}))
  })
}

async function assertNoHorizontalOverflow(page, name) {
  const geometry = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    innerWidth: window.innerWidth,
  }))
  assert(geometry.scrollWidth <= geometry.innerWidth + 1, `${name}: horizontal overflow ${geometry.scrollWidth} > ${geometry.innerWidth}`)
}

async function openMobileNavigation(page, viewport) {
  if (viewport.width > 750) return
  await page.locator('.mobile-menu-trigger').click()
  await page.waitForFunction(() => {
    const node = document.querySelector('.app-sidebar')
    return node && Math.abs(Number.parseFloat(getComputedStyle(node).marginLeft)) < 0.5
  })
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
    if (message.type() === 'error' && !/WebSocket connection to .*\/ws\/sync/.test(message.text())) {
      failures.push(`console.error: ${message.text()}`)
    }
  })
  await page.addInitScript(token => window.localStorage.setItem('openreader_token', token), fakeToken())
  await installApiMocks(page)

  const root = targetUrl.replace(/\/$/, '')
  await page.goto(`${root}/search?q=旧链接搜索&searchType=all&concurrent=8`, { waitUntil: 'networkidle' })
  await page.waitForSelector('.workspace-result-page .result-card', { timeout: 10000 })
  const legacyState = await page.evaluate(() => ({
    path: window.location.pathname,
    workspace: new URLSearchParams(window.location.search).get('workspace'),
    heading: document.querySelector('.workspace-result-head h1')?.textContent || '',
  }))
  assert(legacyState.path === '/', `${viewport.width}: /search must redirect to /`)
  assert(legacyState.workspace === 'search', `${viewport.width}: redirected search must retain workspace=search`)
  assert(legacyState.heading.includes('搜索 (1)'), `${viewport.width}: search result heading is missing`) 
  await assertNoHorizontalOverflow(page, `${viewport.width} search`)

  await page.getByRole('button', { name: '查看信息' }).click()
  await page.waitForSelector('.book-info-dialog .overlay-actions', { timeout: 10000 })
  await page.getByRole('button', { name: '加入并阅读' }).click()
  const categoryDialog = page.locator('.book-add-category-dialog')
  await categoryDialog.waitFor({ state: 'visible', timeout: 10000 })
  await categoryDialog.getByRole('button', { name: '取消' }).click()
  await page.waitForFunction(() => !document.querySelector('.book-add-category-dialog .el-dialog'))
  assert(await page.evaluate(() => window.__workspaceRemoteCreateCount()) === 0, `${viewport.width}: cancelling BookInfo groups must not add a book`)
  await page.waitForSelector('.book-info-dialog .overlay-actions', { timeout: 10000 })
  await page.getByRole('button', { name: '加入并阅读' }).click()
  await categoryDialog.getByRole('button', { name: '确定' }).click()
  await page.waitForURL(/\/books\/99\/read/, { timeout: 10000 })
  assert(await page.evaluate(() => window.__workspaceRemoteCreateCount()) === 1, `${viewport.width}: confirming BookInfo groups must add exactly once`)

  await page.goto(root, { waitUntil: 'networkidle' })
  await page.waitForSelector('.shelf-page .book-row', { timeout: 10000 })
  await openMobileNavigation(page, viewport)
  const searchInput = page.locator('.app-shell-search input')
  await searchInput.fill('二次侧栏搜索')
  await searchInput.press('Enter')
  await page.waitForSelector('.workspace-result-page .result-card', { timeout: 10000 })
  const directSearchState = await page.evaluate(() => ({
    path: window.location.pathname,
    heading: document.querySelector('.workspace-result-head h1')?.textContent || '',
  }))
  assert(directSearchState.path === '/', `${viewport.width}: sidebar search must retain the root scene`)
  assert(directSearchState.heading.includes('搜索 (1)'), `${viewport.width}: second sidebar search did not refresh results`)
  await assertNoHorizontalOverflow(page, `${viewport.width} second-search`)

  await page.getByRole('button', { name: '探索书源' }).click()
  await page.waitForSelector('.workspace-result-page .discover-results .result-card', { timeout: 10000 })
  const exploreState = await page.evaluate(() => ({
    path: window.location.pathname,
    heading: document.querySelector('.workspace-result-head h1')?.textContent || '',
  }))
  assert(exploreState.path === '/', `${viewport.width}: Explore must remain in the root scene`)
  assert(exploreState.heading.includes('探索 (1)'), `${viewport.width}: Explore result heading is missing`)
  await assertNoHorizontalOverflow(page, `${viewport.width} explore`)

  await page.locator('.workspace-result-actions button').click()
  await page.waitForSelector('.shelf-page .book-row', { timeout: 10000 })
  await assertNoHorizontalOverflow(page, `${viewport.width} shelf-return`)
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
    const checks = []
    checks.push(await runViewport(browser, { width: 1440, height: 900 }))
    checks.push(await runViewport(browser, { width: 390, height: 844 }))
    checks.push(await runViewport(browser, { width: 360, height: 800 }))
    console.log(`index-workspace: ok ${checks.join(', ')} legacyRedirects=true sidebarSearch=true bookInfoGroupConfirm=true explore=true`)
  } finally {
    await browser.close()
  }
}

run().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})
