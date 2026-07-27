#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'
const fixtureImage = 'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%221%22 height=%221%22/%3E'

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

function rssSource(id = 1) {
  return {
    id,
    title: id === 1 ? '契约 RSS 源' : '即时 RSS 源',
    url: `https://rss.example/feed-${id}.xml`,
    customOrder: id,
    enabled: true,
    singleUrl: true,
  }
}

function rssArticle(id = 7, sourceId = 1) {
  return {
    id,
    sourceId,
    title: sourceId === 1 ? '契约 RSS 文章' : '即时 RSS 文章',
    summary: sourceId === 1 ? '契约文章摘要' : '即时文章摘要',
    author: 'OpenReader',
    pubDate: '2026-07-12',
    image: fixtureImage,
    isRead: false,
    favorite: false,
    link: 'https://rss.example/article/7',
  }
}

async function installApiMocks(page) {
  const refreshCalls = new Map()
  let delayNextSourceOneList = false
  let delayedSourceOnePending = false
  await page.exposeFunction('__rssSmokeRefreshCalls', () => [...refreshCalls.values()].reduce((sum, count) => sum + count, 0))
  await page.exposeFunction('__rssSmokeRefreshCallsBySource', () => Object.fromEntries(refreshCalls))
  await page.exposeFunction('__rssSmokeStartSourceRace', () => {
    delayNextSourceOneList = true
  })
  await page.exposeFunction('__rssSmokeSourceRacePending', () => delayedSourceOnePending)
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
    if (path === '/rss/sources' && method === 'GET') return route.fulfill(json([rssSource(1), rssSource(2)]))
    const refreshMatch = path.match(/^\/rss\/sources\/(\d+)\/refresh$/)
    if (refreshMatch && method === 'POST') {
      const sourceId = Number(refreshMatch[1])
      refreshCalls.set(sourceId, (refreshCalls.get(sourceId) || 0) + 1)
      return route.fulfill(json({ imported: 1, total: 1 }))
    }
    if (path === '/rss/articles' && method === 'GET') {
      const sourceId = Number(new URL(request.url()).searchParams.get('sourceId') || 1)
      if (sourceId === 1 && delayNextSourceOneList) {
        delayNextSourceOneList = false
        delayedSourceOnePending = true
        await new Promise(resolve => setTimeout(resolve, 1500))
        delayedSourceOnePending = false
      }
      return route.fulfill(json({
        items: [rssArticle(sourceId === 1 ? 7 : 8, sourceId)],
        page: 1,
        hasMore: false,
      }))
    }
    if (path === '/rss/articles/7/content' && method === 'GET') {
      return route.fulfill(json({
        content: `<p>契约 RSS 正文</p><img src="${fixtureImage}" alt="契约图片">`,
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
  if (viewport.width <= 750) {
    await page.waitForFunction(dialogSelector => {
      const node = document.querySelector(dialogSelector)
      if (!node) return false
      const rect = node.getBoundingClientRect()
      return Math.abs(rect.left) <= 1
        && Math.abs(rect.top) <= 1
        && Math.abs(rect.width - innerWidth) <= 1
        && Math.abs(rect.height - innerHeight) <= 1
    }, selector)
  }
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
  await page.locator(selector).locator('.el-dialog__headerbtn').first().click()
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

  await page.locator('.rss-source-card button').nth(0).click()
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
  await page.locator('.el-image-viewer__close').click()
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

  await page.evaluate(() => window.__rssSmokeStartSourceRace())
  await page.locator('.rss-source-card button').nth(0).click()
  await page.locator('.rss-article-list-dialog').waitFor({ state: 'visible', timeout: 10000 })
  await page.waitForFunction(() => window.__rssSmokeSourceRacePending())
  await closeDialog(page, '.rss-article-list-dialog')
  await page.locator('.rss-source-card button').nth(1).click()
  await page.locator('.rss-article-list-dialog').waitFor({ state: 'visible', timeout: 10000 })
  await page.getByText('即时 RSS 文章', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  await page.waitForTimeout(1600)
  assert(await page.locator('.rss-article-list-dialog').getByText('契约 RSS 文章', { exact: true }).count() === 0, `${viewport.width}: delayed source-one rows must not overwrite source two`)
  const refreshBySource = await page.evaluate(() => window.__rssSmokeRefreshCallsBySource())
  assert(refreshBySource['1'] === 1, `${viewport.width}: the stale source-one continuation must not trigger another refresh (${JSON.stringify(refreshBySource)})`)
  assert(refreshBySource['2'] === 1, `${viewport.width}: the active source-two selection must refresh exactly once (${JSON.stringify(refreshBySource)})`)
  await closeDialog(page, '.rss-article-list-dialog')
  await closeDialog(page, '.global-rss-dialog')

  assert(failures.length === 0, failures.join('\n'))
  await context.close()
  return `${viewport.width}x${viewport.height}`
}

async function run() {
  const browser = await openSmokeBrowser()
  try {
    const checks = []
    checks.push(await runViewport(browser, { width: 1440, height: 900 }))
    checks.push(await runViewport(browser, { width: 390, height: 844 }))
    checks.push(await runViewport(browser, { width: 360, height: 800 }))
    console.log(`rss-workspace: ok ${checks.join(', ')} sourceArticleContentDialogs=true refreshOnce=true staleSourceGuard=true`)
  } finally {
    await browser.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
