#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'
const fixtureImage = 'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%2220%22 height=%2220%22/%3E'

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
  if (id === 2) {
    return {
      id,
      sourceName: '即时 RSS 源',
      sourceUrl: 'https://rss.example/feed-2.xml',
      sourceIcon: fixtureImage,
      customOrder: 2,
      enabled: true,
      singleUrl: true,
    }
  }
  return {
    id,
    sourceName: '契约 RSS 源',
    sourceUrl: 'https://rss.example/feed-1.xml',
    sourceIcon: fixtureImage,
    customOrder: 1,
    enabled: true,
    singleUrl: false,
    sortUrl: '新闻::https://rss.example/news?page={{page}}\n科技::https://rss.example/tech?page={{page}}',
  }
}

function rssArticle(id, sourceId, page = 1) {
  const sourceOne = sourceId === 1
  return {
    id,
    sourceId,
    title: sourceOne ? `契约 RSS 文章 ${page}` : '即时 RSS 文章',
    pubDate: `2026-08-0${Math.min(page, 9)}`,
    image: fixtureImage,
    link: `https://rss.example/article/${id}`,
  }
}

async function installApiMocks(page) {
  const refreshCalls = []
  const importPayloads = []
  let delayedSourceOne = false
  let delayedSourceOnePending = false
  let legacyArticleListCalls = 0

  await page.exposeFunction('__rssSmokeRefreshCalls', () => refreshCalls.map(call => ({ ...call })))
  await page.exposeFunction('__rssSmokeImportPayloads', () => importPayloads.map(payload => [...payload]))
  await page.exposeFunction('__rssSmokeLegacyArticleListCalls', () => legacyArticleListCalls)
  await page.exposeFunction('__rssSmokeStartSourceRace', () => { delayedSourceOne = true })
  await page.exposeFunction('__rssSmokeSourceRacePending', () => delayedSourceOnePending)

  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()

    if (path === '/me') return route.fulfill(json({ id: 1, username: 'rss-smoke', role: 'admin' }))
    if (path === '/health') return route.fulfill(json({ version: 'smoke', commit: 'rss-workspace' }))
    if (path === '/settings/reader' && method === 'GET') return route.fulfill(json({ key: 'reader', value: { theme: 'parchment', mode: 'page', pageMode: 'auto' } }))
    if (path === '/settings/reader' && method === 'PUT') return route.fulfill(json({ key: 'reader', value: {} }))
    if (path === '/settings/preferences') return route.fulfill(json({ key: 'preferences', value: {} }))
    if (path === '/books' || path === '/categories' || path === '/sources') return route.fulfill(json([]))
    if (path === '/cache/stats') return route.fulfill(json({ files: 0, size: 0, cachedChapters: 0 }))
    if (path === '/rss/sources' && method === 'GET') return route.fulfill(json([rssSource(1), rssSource(2)]))
    if (path === '/rss/sources/import' && method === 'POST') {
      const payload = request.postDataJSON?.() || []
      importPayloads.push(payload)
      return route.fulfill(json({ imported: payload.length, total: 2 }))
    }
    if (path === '/rss/articles' && method === 'GET') {
      legacyArticleListCalls += 1
      return route.fulfill(json({ items: [], page: 1, hasMore: false }))
    }

    const refreshMatch = path.match(/^\/rss\/sources\/(\d+)\/refresh$/)
    if (refreshMatch && method === 'POST') {
      const sourceId = Number(refreshMatch[1])
      const requestedPage = Number(url.searchParams.get('page') || 1)
      refreshCalls.push({ sourceId, page: requestedPage, sortName: url.searchParams.get('sortName') || '' })
      if (sourceId === 1 && requestedPage === 1 && delayedSourceOne) {
        delayedSourceOne = false
        delayedSourceOnePending = true
        await new Promise(resolve => setTimeout(resolve, 1200))
        delayedSourceOnePending = false
      }
      if (sourceId === 1) {
        const id = requestedPage === 1 ? 7 : 9
        return route.fulfill(json({
          items: [rssArticle(id, sourceId, requestedPage)],
          page: requestedPage,
          hasMore: requestedPage < 2,
          imported: 1,
          total: 1,
        }))
      }
      return route.fulfill(json({
        items: [rssArticle(8, sourceId)],
        page: requestedPage,
        hasMore: false,
        imported: 1,
        total: 1,
      }))
    }

    const contentMatch = path.match(/^\/rss\/articles\/(\d+)\/content$/)
    if (contentMatch && method === 'GET') {
      const id = Number(contentMatch[1])
      return route.fulfill(json({
        id,
        content: `<p>契约 RSS 正文</p><img src="${fixtureImage}" alt="契约图片">`,
        link: `https://rss.example/article/${id}`,
      }))
    }
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

async function dialogGeometry(page, selector) {
  return page.locator(selector).evaluate(node => {
    const rect = node.getBoundingClientRect()
    const overlay = node.closest('.el-overlay')
    const overlayDialog = node.closest('.el-overlay-dialog')
    const overlayRect = overlay?.getBoundingClientRect()
    const style = getComputedStyle(node)
    return {
      left: rect.left,
      top: rect.top,
      width: rect.width,
      height: rect.height,
      viewportWidth: innerWidth,
      viewportHeight: innerHeight,
      scrollY,
      marginTop: style.marginTop,
      position: style.position,
      styleTop: style.top,
      transform: style.transform,
      overlayTop: overlayRect?.top,
      overlayHeight: overlayRect?.height,
      overlayScrollTop: overlayDialog?.scrollTop,
    }
  })
}

async function assertDialogGeometry(page, selector, viewport, label, desktopWidth) {
  await page.locator(selector).waitFor({ state: 'visible', timeout: 10000 })
  await page.waitForFunction(({ dialogSelector, mobile, expectedWidth }) => {
    const node = document.querySelector(dialogSelector)
    if (!node) return false
    const rect = node.getBoundingClientRect()
    if (mobile) {
      return Math.abs(rect.left) <= 1
        && Math.abs(rect.top) <= 1
        && Math.abs(rect.width - innerWidth) <= 1
        && Math.abs(rect.height - innerHeight) <= 1
    }
    return Math.abs(rect.width - expectedWidth) <= 2
      && Math.abs(rect.left - (innerWidth - rect.width) / 2) <= 1
  }, { dialogSelector: selector, mobile: viewport.width <= 750, expectedWidth: desktopWidth })
  const geometry = await dialogGeometry(page, selector)
  if (viewport.width <= 750) {
    assert(
      Math.abs(geometry.left) <= 1 && Math.abs(geometry.top) <= 1,
      `${label}: mobile dialog must start at viewport origin (${JSON.stringify(geometry)})`,
    )
    assert(Math.abs(geometry.width - geometry.viewportWidth) <= 1, `${label}: mobile width ${geometry.width}/${geometry.viewportWidth}`)
    assert(Math.abs(geometry.height - geometry.viewportHeight) <= 1, `${label}: mobile height ${geometry.height}/${geometry.viewportHeight}`)
    return
  }
  assert(Math.abs(geometry.width - desktopWidth) <= 2, `${label}: desktop width ${geometry.width}/${desktopWidth}`)
  assert(Math.abs(geometry.left - (geometry.viewportWidth - geometry.width) / 2) <= 1, `${label}: desktop dialog must be centred`)
}

async function closeDialog(page, selector) {
  await page.locator(selector).locator('.el-dialog__headerbtn').first().click()
  await page.locator(selector).waitFor({ state: 'hidden', timeout: 10000 })
}

async function assertRootCoexists(page, viewport, childSelector) {
  assert(await page.locator('.global-rss-dialog').isVisible(), `${viewport.width}: root source dialog must remain visible with ${childSelector}`)
  assert(await page.locator(childSelector).isVisible(), `${viewport.width}: child dialog ${childSelector} must be visible`)
}

async function verifyEditor(page, viewport) {
  await page.locator('.global-rss-dialog').getByText('新增', { exact: true }).click()
  const editor = '.rss-source-editor-dialog'
  const expectedWidth = viewport.width >= 1440 ? 1000 : 750
  await assertDialogGeometry(page, editor, viewport, `${viewport.width} editor`, expectedWidth)
  await assertRootCoexists(page, viewport, editor)
  const draft = await page.locator(`${editor} textarea`).inputValue()
  assert(draft.includes('"sourceName": "新增RSS源"'), `${viewport.width}: editor must start from upstream JSON template`)
  assert(draft.includes('"singleUrl": true'), `${viewport.width}: editor template must preserve upstream defaults`)
  await page.locator(editor).getByRole('button', { name: '取 消', exact: true }).click()
  await page.locator(editor).waitFor({ state: 'hidden', timeout: 10000 })
}

async function verifyImport(page, viewport) {
  const imported = [
    { sourceName: '第一个安全源', sourceUrl: 'https://safe.example/feed.xml' },
    { sourceName: '危险源', sourceUrl: 'https://risk.example/feed.xml', ruleArticles: '@js:return []' },
  ]
  await page.locator('.rss-source-import-input').setInputFiles({
    name: 'rss-import.json',
    mimeType: 'application/json',
    buffer: Buffer.from(JSON.stringify(imported)),
  })
  const importDialog = '.rss-source-import-dialog'
  const expectedWidth = viewport.width >= 1440 ? 1000 : 750
  await assertDialogGeometry(page, importDialog, viewport, `${viewport.width} import`, expectedWidth)
  await assertRootCoexists(page, viewport, importDialog)
  assert(await page.locator(`${importDialog} .el-checkbox__input.is-checked`).count() === 0, `${viewport.width}: import must initially select no sources`)
  await page.locator(importDialog).getByText('全选', { exact: true }).click()
  assert(await page.locator(`${importDialog} .rss-source-checkbox .el-checkbox__input.is-checked`).count() === 1, `${viewport.width}: safe select-all must select exactly one source`)
  assert(await page.locator(importDialog).getByText('@Javascript', { exact: false }).count() === 1, `${viewport.width}: risky source must be visibly labelled`)
  await page.locator(importDialog).getByRole('button', { name: '确定', exact: true }).click()
  await page.locator(importDialog).waitFor({ state: 'hidden', timeout: 10000 })
  const payloads = await page.evaluate(() => window.__rssSmokeImportPayloads())
  const latest = payloads.at(-1) || []
  assert(latest.length === 1 && latest[0]?.sourceName === '第一个安全源', `${viewport.width}: import must keep safe index zero and exclude risky sources`)
}

async function verifySourceArticleFlow(page, viewport) {
  const sourceCards = page.locator('.global-rss-dialog .rss-source')
  const sourceGeometry = await sourceCards.first().evaluate(node => {
    const rect = node.getBoundingClientRect()
    const icon = node.querySelector('.rss-icon')?.getBoundingClientRect()
    const parent = node.parentElement?.getBoundingClientRect()
    return { width: rect.width, parentWidth: parent?.width || 0, iconWidth: icon?.width || 0, iconHeight: icon?.height || 0 }
  })
  assert(Math.abs(sourceGeometry.width - sourceGeometry.parentWidth / 4) <= 1, `${viewport.width}: source tile must occupy 25% of the source grid`)
  assert(sourceGeometry.iconWidth === 50 && sourceGeometry.iconHeight === 50, `${viewport.width}: source icon must be 50x50`)

  await sourceCards.nth(0).click()
  await page.getByText('契约 RSS 文章 1', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  await assertDialogGeometry(page, '.rss-article-list-dialog', viewport, `${viewport.width} article-list`, 500)
  await assertRootCoexists(page, viewport, '.rss-article-list-dialog')
  assert(await page.locator('.rss-article-list-dialog .el-tabs__item').count() === 2, `${viewport.width}: two upstream sort rows must render two tabs`)
  assert(await page.locator('.rss-article-list-dialog').getByText('OpenReader', { exact: true }).count() === 0, `${viewport.width}: article list must not expose author metadata`)
  assert(await page.locator('.rss-article-list-dialog').getByText('收藏', { exact: false }).count() === 0, `${viewport.width}: article list must not expose hidden favourite controls`)

  await page.locator('.rss-article-list-dialog .load-more-rss').click()
  await page.getByText('契约 RSS 文章 2', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  const firstPages = (await page.evaluate(() => window.__rssSmokeRefreshCalls())).filter(call => call.sourceId === 1).map(call => call.page)
  assert(firstPages.join(',') === '1,2', `${viewport.width}: user actions must request exactly pages 1 then 2 (${firstPages.join(',')})`)
  assert(await page.evaluate(() => window.__rssSmokeLegacyArticleListCalls()) === 0, `${viewport.width}: visible flow must not reload articles through legacy list API`)

  await page.locator('.rss-article-list-dialog .rss-article').first().click()
  await page.getByText('契约 RSS 正文', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  await assertDialogGeometry(page, '.rss-article-content-dialog', viewport, `${viewport.width} article-content`, 500)
  await assertRootCoexists(page, viewport, '.rss-article-content-dialog')
  assert(await page.locator('.rss-article-list-dialog').isVisible(), `${viewport.width}: article list must coexist below article content`)
  await page.locator('.rss-article-content-dialog .rss-article-content img').click()
  await page.locator('.el-image-viewer__wrapper').waitFor({ state: 'visible', timeout: 10000 })
  await page.locator('.el-image-viewer__close').click()
  await page.locator('.el-image-viewer__wrapper').waitFor({ state: 'hidden', timeout: 10000 })
  await closeDialog(page, '.rss-article-content-dialog')
  await closeDialog(page, '.rss-article-list-dialog')
}

async function verifyLateSourceIsolation(page, viewport) {
  await page.evaluate(() => window.__rssSmokeStartSourceRace())
  await page.locator('.global-rss-dialog .rss-source').nth(0).click()
  await page.waitForFunction(() => window.__rssSmokeSourceRacePending())
  await closeDialog(page, '.rss-article-list-dialog')
  await page.locator('.global-rss-dialog .rss-source').nth(1).click()
  await page.getByText('即时 RSS 文章', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  await page.waitForTimeout(1300)
  assert(await page.locator('.rss-article-list-dialog').getByText('契约 RSS 文章 1', { exact: true }).count() === 0, `${viewport.width}: late source-one response must not replace source two`)
  await closeDialog(page, '.rss-article-list-dialog')
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
  await page.getByText('RSS订阅(2)', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  await assertDialogGeometry(page, '.global-rss-dialog', viewport, `${viewport.width} source`, 500)
  await assertNoHorizontalOverflow(page, `${viewport.width} source`)
  assert(await page.evaluate(() => window.__rssSmokeRefreshCalls()).then(calls => calls.length) === 0, `${viewport.width}: opening root must not refresh a source`)

  await verifyEditor(page, viewport)
  await verifyImport(page, viewport)
  await verifySourceArticleFlow(page, viewport)
  await verifyLateSourceIsolation(page, viewport)
  await assertNoHorizontalOverflow(page, `${viewport.width} completed`)
  await closeDialog(page, '.global-rss-dialog')
  await page.waitForFunction(() => new URLSearchParams(location.search).get('overlay') !== 'rss')

  assert(failures.length === 0, failures.join('\n'))
  await context.close()
  return `${viewport.width}x${viewport.height}`
}

async function run() {
  const browser = await openSmokeBrowser()
  try {
    const requestedWidths = new Set(
      String(process.env.SMOKE_VIEWPORTS || '')
        .split(',')
        .map(value => value.trim())
        .filter(Boolean)
        .map(Number)
        .filter(Number.isFinite),
    )
    const viewports = [
      { width: 1440, height: 900 },
      { width: 1024, height: 1366 },
      { width: 390, height: 844 },
      { width: 360, height: 800 },
    ].filter(viewport => !requestedWidths.size || requestedWidths.has(viewport.width))
    const checks = []
    for (const viewport of viewports) checks.push(await runViewport(browser, viewport))
    console.log(`rss-workspace: ok ${checks.join(', ')} upstreamDialogs=true jsonEditor=true guardedImport=true requestedPageOnly=true staleSourceGuard=true`)
  } finally {
    await browser.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
