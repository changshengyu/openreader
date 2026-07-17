#!/usr/bin/env node

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'
const defaultChromePath = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'

async function loadPlaywright() {
  try {
    const module = await import('playwright')
    return module.chromium ? module : module.default
  } catch {
    const module = await import('/Users/yuchangsheng/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright/index.js')
    return module.chromium ? module : module.default
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

function shelfBook(id, title, updatedAt) {
  return { id, title, author: 'OpenReader', sourceId: 0, chapterCount: 1, updatedAt }
}

async function installMocks(page) {
  let shelfRequests = 0
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    if (path === '/me') return route.fulfill(json({ id: 1, username: 'race', role: 'admin' }))
    if (path === '/health') return route.fulfill(json({ version: 'race-contract' }))
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(json({ key: 'reader', value: { theme: 'parchment', mode: 'page', pageMode: 'auto' } }))
    }
    if (path.startsWith('/settings/')) return route.fulfill(json({ key: path.split('/').at(-1), value: {} }))
    if (path === '/categories') return route.fulfill(json([]))
    if (path === '/sources') return route.fulfill(json([]))
    if (path.startsWith('/cache')) return route.fulfill(json({ total: 0, books: 0, chapters: 0 }))
    if (path === '/books') {
      shelfRequests += 1
      if (shelfRequests === 1) {
        await new Promise(resolve => setTimeout(resolve, 900))
        return route.fulfill(json([
          shelfBook(1, '旧请求中的书', '2026-07-17T00:00:00Z'),
        ]))
      }
      return route.fulfill(json([
        shelfBook(2, '刚刚导入的新书', '2026-07-17T00:01:00Z'),
        shelfBook(1, '旧请求中的书', '2026-07-17T00:00:00Z'),
      ]))
    }
    return route.fulfill(json({}))
  })
  return () => shelfRequests
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({ viewport })
  await context.addInitScript(token => localStorage.setItem('openreader_token', token), fakeToken())
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(error.message))
  page.on('console', message => {
    if (message.type() === 'error' && !message.text().includes('/ws/sync')) failures.push(message.text())
  })
  const requestCount = await installMocks(page)
  await page.goto(targetUrl, { waitUntil: 'domcontentloaded' })
  const refresh = page.locator('.shelf-title .title-actions button').filter({ hasText: '刷新' })
  await refresh.waitFor({ state: 'visible', timeout: 10000 })
  await refresh.click()
  await page.getByText('刚刚导入的新书', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  await page.waitForTimeout(1100)
  assert(requestCount() >= 2, `${viewport.width}: expected overlapping shelf requests`)
  assert(await page.getByText('刚刚导入的新书', { exact: true }).count() === 1, `${viewport.width}: a late old response removed the newest imported book`)
  assert((await page.locator('.shelf-title strong').textContent())?.includes('(2)'), `${viewport.width}: shelf count was overwritten by the old response`)
  assert(failures.length === 0, failures.join('\n'))
  await context.close()
}

const { chromium } = await loadPlaywright()
const browser = process.env.CDP_URL
  ? await chromium.connectOverCDP(process.env.CDP_URL)
  : await chromium.launch({
    headless: true,
    executablePath: process.env.CHROME_PATH || defaultChromePath,
    args: ['--disable-gpu'],
  })
try {
  await runViewport(browser, { width: 1440, height: 900 })
  await runViewport(browser, { width: 390, height: 844 })
  console.log('shelf refresh race contract smoke passed')
} finally {
  await browser.close()
}
