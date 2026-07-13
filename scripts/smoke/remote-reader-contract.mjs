#!/usr/bin/env node

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:4173'
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
      console.error('Playwright is required for the remote reader contract smoke.')
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

function remoteBook() {
  return {
    title: '临时阅读书籍',
    author: 'OpenReader',
    url: 'https://source.example/book/remote-reader',
    bookUrl: 'https://source.example/book/remote-reader',
    sourceId: 1,
    sourceName: '临时阅读测试书源',
    latestChapter: '第一章',
    intro: '用于验证搜索结果正文进入无持久化临时阅读器。',
  }
}

async function installApiMocks(page) {
  const requests = []
  let sessionCreates = 0
  let remoteBookCreates = 0
  let shelfBooks = []
  await page.exposeFunction('__remoteReaderRequests', () => requests)
  await page.exposeFunction('__remoteReaderSessionCreates', () => sessionCreates)
  await page.exposeFunction('__remoteReaderBookCreates', () => remoteBookCreates)
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    requests.push(`${method} ${path}`)

    if (path === '/me') return route.fulfill(json({ id: 1, username: 'remote-reader-smoke', role: 'admin' }))
    if (path === '/health') return route.fulfill(json({ version: 'smoke', commit: 'remote-reader' }))
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(json({
        key: 'reader',
        value: {
          theme: 'parchment',
          mode: 'scroll',
          pageMode: 'auto',
          fontSize: 18,
          lineHeight: 1.8,
          paragraphSpace: 0.2,
          columnWidth: 800,
        },
      }))
    }
    if (path === '/settings/reader' && method === 'PUT') return route.fulfill(json({ key: 'reader', value: {} }))
    if (path === '/settings/preferences') return route.fulfill(json({ key: 'preferences', value: {} }))
    if (path === '/categories') return route.fulfill(json([]))
    if (path === '/sources') return route.fulfill(json([{ id: 1, name: '临时阅读测试书源', enabled: true }]))
    if (path === '/books') return route.fulfill(json(shelfBooks))
    if (path === '/search' && method === 'POST') {
      return route.fulfill(json({ list: [remoteBook()], page: 1, lastIndex: -1, hasMore: false }))
    }
    if (path === '/reader/remote-sessions' && method === 'POST') {
      sessionCreates += 1
      const payload = request.postDataJSON()
      if (payload?.sourceId !== 1 || payload?.bookUrl !== remoteBook().bookUrl) {
        return route.fulfill(json({ error: 'invalid client payload' }, 400))
      }
      return route.fulfill(json({
        id: 'smoke-temporary-session',
        expiresAt: '2026-07-13T08:00:00Z',
        book: {
          id: 0,
          title: remoteBook().title,
          author: remoteBook().author,
          sourceId: 1,
          url: remoteBook().bookUrl,
          chapterCount: 1,
        },
        chapters: [{ id: 0, index: 0, title: '第一章', url: 'https://source.example/chapter/1' }],
      }, 201))
    }
    if (path === '/books/remote' && method === 'POST') {
      remoteBookCreates += 1
      const payload = request.postDataJSON()
      shelfBooks = [{
        id: 99,
        title: payload.title,
        author: payload.author,
        sourceId: payload.sourceId,
        url: payload.bookUrl,
        chapterCount: 1,
        categoryIds: payload.categoryIds || [],
      }]
      return route.fulfill(json(shelfBooks[0], 201))
    }
    if (path === '/reader/remote-sessions/smoke-temporary-session') {
      return route.fulfill(json({
        id: 'smoke-temporary-session',
        expiresAt: '2026-07-13T08:00:00Z',
        book: {
          id: 0,
          title: remoteBook().title,
          author: remoteBook().author,
          sourceId: 1,
          url: remoteBook().bookUrl,
          chapterCount: 1,
        },
        chapters: [{ id: 0, index: 0, title: '第一章', url: 'https://source.example/chapter/1' }],
      }))
    }
    if (path === '/reader/remote-sessions/smoke-temporary-session/chapters/0/content') {
      return route.fulfill(json({
        chapter: { id: 0, index: 0, title: '第一章' },
        content: '临时阅读正文验证内容。\n这段内容只能存在于远程会话，不能创建书架、进度或书签记录。',
        format: 'text',
      }))
    }
    return route.fulfill(json({}))
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
  await page.goto(`${root}/?workspace=search&q=临时阅读&searchType=single&sourceId=1`, { waitUntil: 'networkidle' })
  await page.waitForSelector('.workspace-result-page .result-card', { timeout: 10000 })

  await page.locator('.workspace-result-page .result-card .book-cover-shared').click()
  await page.waitForSelector('.book-info-dialog', { timeout: 10000 })
  assert(await page.evaluate(() => window.__remoteReaderSessionCreates()) === 0, `${viewport.width}: cover must only open BookInfo`)
  const bookInfo = page.locator('.book-info-dialog')
  assert(await bookInfo.getByText('加入书架', { exact: true }).count() === 1, `${viewport.width}: unshelved BookInfo must expose one add action`)
  assert(await bookInfo.getByText('加入并阅读', { exact: true }).count() === 0, `${viewport.width}: BookInfo must not expose an add-and-read branch`)
  assert(await bookInfo.getByText('继续阅读', { exact: true }).count() === 0, `${viewport.width}: BookInfo must not expose a read branch`)
  await bookInfo.getByText('加入书架', { exact: true }).click()
  const categoryDialog = page.locator('.book-add-category-dialog')
  await categoryDialog.waitFor({ state: 'visible', timeout: 10000 })
  await categoryDialog.getByRole('button', { name: '取消', exact: true }).click()
  await categoryDialog.waitFor({ state: 'hidden', timeout: 10000 })
  assert(await page.evaluate(() => window.__remoteReaderBookCreates()) === 0, `${viewport.width}: cancelled BookInfo add must not persist a shelf book`)
  await page.locator('.book-info-dialog .el-dialog__headerbtn').click()
  await page.locator('.book-info-dialog').waitFor({ state: 'hidden', timeout: 10000 })

  await page.locator('.workspace-result-page .result-card .result-main').click()
  await page.waitForURL(/\/reader\/remote\/smoke-temporary-session\?chapter=0/, { timeout: 10000 })
  await page.waitForSelector('.reader-body', { timeout: 10000 })
  await page.getByText('临时阅读正文验证内容。', { exact: false }).waitFor({ state: 'visible', timeout: 10000 })
  assert(await page.evaluate(() => window.__remoteReaderSessionCreates()) === 1, `${viewport.width}: result body must create exactly one temporary reader session`)

  const temporaryReaderURL = await page.url()
  const bookInfoButton = page.locator('button[title="书籍信息"]:visible').first()
  await bookInfoButton.click()
  await page.waitForSelector('.book-info-dialog', { timeout: 10000 })
  const temporaryReaderBookInfo = page.locator('.book-info-dialog')
  assert(await temporaryReaderBookInfo.getByText('加入书架', { exact: true }).count() === 1, `${viewport.width}: temporary Reader BookInfo must expose the shared unshelved add action`)
  assert(await temporaryReaderBookInfo.getByText('加入并阅读', { exact: true }).count() === 0, `${viewport.width}: temporary Reader BookInfo must not expose add-and-read`)
  if (viewport.width <= 750) {
    assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: temporary Reader BookInfo must preserve the mobile toolbar`)
  }
  await temporaryReaderBookInfo.getByText('加入书架', { exact: true }).click()
  await categoryDialog.waitFor({ state: 'visible', timeout: 10000 })
  await categoryDialog.getByRole('button', { name: '取消', exact: true }).click()
  await categoryDialog.waitFor({ state: 'hidden', timeout: 10000 })
  assert(await page.evaluate(() => window.__remoteReaderBookCreates()) === 0, `${viewport.width}: temporary Reader cancellation must not persist a shelf book`)
  await temporaryReaderBookInfo.getByText('加入书架', { exact: true }).click()
  await categoryDialog.waitFor({ state: 'visible', timeout: 10000 })
  const remoteCreateRequest = page.waitForRequest(request => {
    const url = new URL(request.url())
    return request.method() === 'POST' && url.pathname === '/api/books/remote'
  }, { timeout: 10000 })
  await categoryDialog.getByRole('button', { name: '确定', exact: true }).click()
  await remoteCreateRequest
  assert(await page.evaluate(() => window.__remoteReaderBookCreates()) === 1, `${viewport.width}: temporary Reader confirmation must create one shelf book`)
  assert(await temporaryReaderBookInfo.getByText('加入书架', { exact: true }).count() === 0, `${viewport.width}: saved temporary Reader BookInfo must leave the add state`)
  assert(await page.url() === temporaryReaderURL, `${viewport.width}: temporary Reader BookInfo add must not replace the temporary route`)
  await temporaryReaderBookInfo.locator('.el-dialog__headerbtn').click()
  await temporaryReaderBookInfo.waitFor({ state: 'hidden', timeout: 10000 })

  const requests = await page.evaluate(() => window.__remoteReaderRequests())
  const forbidden = requests.filter(value => (
    value === 'PUT /progress'
    || /^(POST|PUT|DELETE) \/(?:books\/[^/]+\/bookmarks|bookmarks)(?:$|\/)/.test(value)
    || /^(POST|DELETE) \/(?:books\/[^/]+\/cache|cache)(?:$|\/)/.test(value)
  ))
  assert(forbidden.length === 0, `${viewport.width}: temporary reader made persistent requests: ${forbidden.join(', ')}`)
  assert(await page.evaluate(() => window.__remoteReaderBookCreates()) === 1, `${viewport.width}: temporary reader must not create another shelf book after explicit BookInfo add`)
  if (viewport.width <= 750) {
    assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: remote reader must keep the default mobile toolbar visible`)
  } else {
    assert(await page.locator('.reader-left-rail').count() === 1, 'desktop: remote reader must retain reader rails')
  }
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
    console.log(`remote-reader: ok ${checks.join(', ')} coverInfo=true temporaryReaderInfo=true canonicalAdd=true noImplicitWrites=true`)
  } finally {
    await browser.close()
  }
}

run().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})
