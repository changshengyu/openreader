#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'

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
  const searchRequests = []
  await page.exposeFunction('__workspaceRemoteCreateCount', () => remoteCreateCount)
  await page.exposeFunction('__workspaceSearchRequests', () => searchRequests)
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
    if (path === '/sources') return route.fulfill(json([
      { id: 1, name: '工作台测试书源一', enabled: true, group: '测试' },
      { id: 2, name: '工作台测试书源二', enabled: true, group: '测试' },
    ]))
    if (path === '/explore/sources') {
      return route.fulfill(json([{
        id: 1,
        name: '工作台测试书源',
        enabled: true,
        group: '测试',
        exploreGroups: [[{ name: '热门', url: 'https://source.example/explore' }]],
      }]))
    }
    if (path === '/explore/1') {
      const page = Number(url.searchParams.get('page') || 1)
      return route.fulfill(json({
        items: page > 1
          ? [remoteBook('工作台探索重复结果'), remoteBook('工作台探索续页结果')]
          : [remoteBook('工作台探索重复结果')],
        page,
        hasMore: page === 1,
      }))
    }
    if (path === '/search' && method === 'POST') {
      const body = request.postDataJSON() || {}
      searchRequests.push(body)
      if (body.keyword === '陈旧请求') {
        await new Promise(resolve => setTimeout(resolve, 500))
        return route.fulfill(json({ list: [remoteBook('陈旧结果')], page: 1, lastIndex: 1, hasMore: false }))
      }
      const sourceIDs = Array.isArray(body.sourceIds) ? body.sourceIds : []
      if (sourceIDs.length > 1) {
        const hasStarted = Number(body.lastIndex) >= 0
        return route.fulfill(json({
          list: hasStarted
            ? [remoteBook('多源重复结果'), remoteBook('多源续页结果')]
            : [remoteBook('多源重复结果')],
          page: 1,
          lastIndex: hasStarted ? 1 : 0,
          hasMore: !hasStarted,
        }))
      }
      const page = Number(body.page || 1)
      return route.fulfill(json({
        list: page > 1
          ? [remoteBook('单源重复结果'), remoteBook('单源续页结果')]
          : [remoteBook('单源重复结果')],
        page,
        lastIndex: -1,
        hasMore: page === 1,
      }))
    }
    if (path === '/books/remote' && method === 'POST') {
      remoteCreateCount += 1
      const payload = request.postDataJSON() || {}
      const created = {
        id: 99,
        title: payload.title || '已加入的工作台书籍',
        author: payload.author || 'OpenReader',
        sourceId: payload.sourceId || 1,
        sourceName: payload.sourceName || '工作台测试书源',
        url: payload.bookUrl,
        bookUrl: payload.bookUrl,
        chapterCount: 1,
        categoryIds: payload.categoryIds || [],
      }
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
  await page.goto(root, { waitUntil: 'networkidle' })
  await page.waitForSelector('.shelf-page .book-row', { timeout: 10000 })
  const shelfRoute = await page.url()
  await page.locator('.shelf-page .book-row .list-cover').first().click()
  await page.waitForSelector('.book-info-dialog', { timeout: 10000 })
  const shelfBookInfo = page.locator('.book-info-dialog')
  assert(await shelfBookInfo.getByText('书架测试书', { exact: true }).count() === 1, `${viewport.width}: shelf cover must open the shared BookInfo record`)
  assert(await shelfBookInfo.getByText('加入书架', { exact: true }).count() === 0, `${viewport.width}: shelf BookInfo must not expose an unshelved add action`)
  assert(await shelfBookInfo.getByText('开始阅读', { exact: true }).count() === 0, `${viewport.width}: shelf BookInfo must not expose a second read action`)
  await shelfBookInfo.locator('.el-dialog__headerbtn').click()
  await shelfBookInfo.waitFor({ state: 'hidden', timeout: 10000 })
  assert(await page.url() === shelfRoute, `${viewport.width}: closing shelf BookInfo must stay on the shelf route`)

  await page.getByRole('button', { name: '书海', exact: true }).click()
  await page.waitForSelector('.explore-workspace-popover', { timeout: 10000 })
  const shelfBeforeExploreSelection = await page.locator('.shelf-page').count()
  assert(shelfBeforeExploreSelection === 1, `${viewport.width}: opening 书海 must keep the shelf body visible before selecting an entry`)
  await page.getByRole('button', { name: '关闭书海' }).click()
  await page.locator('.explore-workspace-popover').waitFor({ state: 'hidden', timeout: 10000 })

  await page.goto(`${root}/discover?sourceId=1&url=https%3A%2F%2Fsource.example%2Fexplore&name=%E7%83%AD%E9%97%A8`, { waitUntil: 'networkidle' })
  await page.waitForSelector('.explore-workspace-popover', { timeout: 10000 })
  const legacyExploreState = await page.evaluate(() => ({
    path: location.pathname,
    workspace: new URLSearchParams(location.search).get('workspace'),
    shelfVisible: Boolean(document.querySelector('.shelf-page')),
  }))
  assert(legacyExploreState.path === '/', `${viewport.width}: /discover must redirect to the root workspace`)
  assert(legacyExploreState.workspace === 'explore', `${viewport.width}: legacy discover intent must retain workspace=explore`)
  assert(legacyExploreState.shelfVisible, `${viewport.width}: legacy discover must hydrate the chooser before replacing the shelf`)
  await page.getByRole('button', { name: '关闭书海' }).click()
  await page.locator('.explore-workspace-popover').waitFor({ state: 'hidden', timeout: 10000 })

  await page.goto(root, { waitUntil: 'networkidle' })
  await page.waitForSelector('.shelf-page .book-row', { timeout: 10000 })
  await openMobileNavigation(page, viewport)
  const freshSearchInput = page.locator('.app-shell-search input')
  await freshSearchInput.fill('默认侧栏搜索')
  await freshSearchInput.press('Enter')
  await page.waitForSelector('.workspace-result-page .result-card', { timeout: 10000 })
  const freshDefaultSearch = (await page.evaluate(() => window.__workspaceSearchRequests())).at(-1)
  assert(freshDefaultSearch.concurrentCount === 24, `${viewport.width}: fresh sidebar search must use the upstream default concurrency 24`)
  assert(!Object.hasOwn(freshDefaultSearch, 'page'), `${viewport.width}: multi-source search must not drive its continuation with page`)
  assert(freshDefaultSearch.lastIndex === -1, `${viewport.width}: fresh multi-source search must start at cursor -1`)
  await page.getByRole('button', { name: '加载更多' }).click()
  await page.waitForFunction(() => document.querySelectorAll('.workspace-result-page .result-card').length === 2)
  const multiContinuation = (await page.evaluate(() => window.__workspaceSearchRequests())).at(-1)
  assert(multiContinuation.lastIndex === 0, `${viewport.width}: multi-source continuation must use the returned cursor`)
  assert(!Object.hasOwn(multiContinuation, 'page'), `${viewport.width}: multi-source continuation must not send a page number`)
  const multiEndButton = page.getByRole('button', { name: '没有更多了' })
  assert(await multiEndButton.isDisabled(), `${viewport.width}: multi-source completion must remain visibly disabled`)

  await page.goto(`${root}/?workspace=search&q=单源续页&searchType=single&sourceId=1&concurrent=24`, { waitUntil: 'networkidle' })
  await page.waitForSelector('.workspace-result-page .result-card', { timeout: 10000 })
  const singleFirst = (await page.evaluate(() => window.__workspaceSearchRequests())).at(-1)
  assert(singleFirst.page === 1, `${viewport.width}: single-source search must begin at page one`)
  assert(!Object.hasOwn(singleFirst, 'lastIndex'), `${viewport.width}: single-source search must not send a multi-source cursor`)
  await page.getByRole('button', { name: '加载更多' }).click()
  await page.waitForFunction(() => document.querySelectorAll('.workspace-result-page .result-card').length === 2)
  const singleContinuation = (await page.evaluate(() => window.__workspaceSearchRequests())).at(-1)
  assert(singleContinuation.page === 2, `${viewport.width}: single-source continuation must advance page`)
  assert(!Object.hasOwn(singleContinuation, 'lastIndex'), `${viewport.width}: single-source continuation must keep the cursor absent`)

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
  const legacySearch = (await page.evaluate(() => window.__workspaceSearchRequests())).at(-1)
  assert(legacySearch.concurrentCount === 8, `${viewport.width}: legacy concurrency must remain 8 until the user changes it`)
  await assertNoHorizontalOverflow(page, `${viewport.width} search`)

  assert(await page.locator('.workspace-result-page .result-actions').count() === 0, `${viewport.width}: result cards must not add a non-upstream preview button`)
  await page.locator('.workspace-result-page .result-card .book-cover-shared').first().click()
  await page.waitForSelector('.book-info-dialog', { timeout: 10000 })
  const searchBookInfo = page.locator('.book-info-dialog')
  assert(await searchBookInfo.getByText('加入书架', { exact: true }).count() === 1, `${viewport.width}: search cover must open the single unshelved BookInfo action`)
  assert(await searchBookInfo.getByText('加入并阅读', { exact: true }).count() === 0, `${viewport.width}: search BookInfo must not expose add-and-read`)
  assert(await searchBookInfo.getByText('开始阅读', { exact: true }).count() === 0, `${viewport.width}: search BookInfo must not expose a read action`)
  const searchBookInfoURL = await page.url()
  await searchBookInfo.getByText('加入书架', { exact: true }).click()
  const categoryDialog = page.locator('.book-add-category-dialog')
  await categoryDialog.waitFor({ state: 'visible', timeout: 10000 })
  await categoryDialog.getByRole('button', { name: '取消' }).click()
  await categoryDialog.waitFor({ state: 'hidden', timeout: 10000 })
  assert(await page.evaluate(() => window.__workspaceRemoteCreateCount()) === 0, `${viewport.width}: cancelling BookInfo groups must not add a book`)
  await searchBookInfo.getByText('加入书架', { exact: true }).click()
  await categoryDialog.waitFor({ state: 'visible', timeout: 10000 })
  const createRequest = page.waitForRequest(request => {
    const requestURL = new URL(request.url())
    return request.method() === 'POST' && requestURL.pathname === '/api/books/remote'
  }, { timeout: 10000 })
  await categoryDialog.getByRole('button', { name: '确定' }).click()
  await createRequest
  assert(await page.evaluate(() => window.__workspaceRemoteCreateCount()) === 1, `${viewport.width}: confirming BookInfo groups must add exactly once`)
  assert(await searchBookInfo.getByText('加入书架', { exact: true }).count() === 0, `${viewport.width}: confirmed search BookInfo must become the shelf state`)
  assert(await searchBookInfo.getByText('分组：', { exact: false }).count() === 1, `${viewport.width}: confirmed search BookInfo must expose shelf properties`)
  assert(await page.url() === searchBookInfoURL, `${viewport.width}: confirming BookInfo add must not navigate to Reader`)
  await searchBookInfo.locator('.el-dialog__headerbtn').click()
  await searchBookInfo.waitFor({ state: 'hidden', timeout: 10000 })

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
  const legacyPreferenceSearch = (await page.evaluate(() => window.__workspaceSearchRequests())).at(-1)
  assert(legacyPreferenceSearch.concurrentCount === 8, `${viewport.width}: sidebar search must retain the active legacy concurrency until the user changes it`)
  await assertNoHorizontalOverflow(page, `${viewport.width} second-search`)

  await searchInput.fill('陈旧请求')
  await searchInput.press('Enter')
  await page.waitForTimeout(50)
  await page.getByRole('button', { name: '探索书源' }).click()
  await page.waitForSelector('.explore-workspace-popover', { timeout: 10000 })
  const chooserState = await page.evaluate(() => ({
    heading: document.querySelector('.workspace-result-head h1')?.textContent || '',
    sidebarMargin: Number.parseFloat(getComputedStyle(document.querySelector('.app-sidebar')).marginLeft),
    popover: (() => {
      const node = document.querySelector('.explore-workspace-popover')
      const rect = node?.getBoundingClientRect()
      return rect ? { left: rect.left, top: rect.top, width: rect.width, height: rect.height } : null
    })(),
  }))
  assert(chooserState.heading.includes('搜索'), `${viewport.width}: opening Explore must retain the current root result scene until an entry is selected`)
  if (viewport.width <= 750) {
    assert(Math.abs(chooserState.sidebarMargin + 260) < 0.5, `${viewport.width}: sidebar Explore trigger must close the compact navigation`)
    assert(Math.abs(chooserState.popover.left) <= 1 && Math.abs(chooserState.popover.top) <= 1, `${viewport.width}: compact Explore chooser must start at the viewport origin`)
    assert(Math.abs(chooserState.popover.width - viewport.width) <= 1 && Math.abs(chooserState.popover.height - viewport.height) <= 1, `${viewport.width}: compact Explore chooser must be fullscreen`)
    const hitOwner = await page.evaluate(() => document.elementFromPoint(innerWidth / 2, innerHeight / 2)?.closest('.explore-workspace-popover')?.className || '')
    assert(hitOwner.includes('explore-workspace-popover'), `${viewport.width}: compact Explore chooser must block clicks through to the workspace`)
  } else {
    assert(chooserState.popover.width <= 520.5, `${viewport.width}: desktop Explore chooser should remain a compact popover`)
  }
  await page.locator('.explore-workspace-popover .explore-entry-row button').first().click()
  await page.locator('.explore-workspace-popover').waitFor({ state: 'hidden', timeout: 10000 })
  await page.waitForSelector('.workspace-result-page .discover-results .result-card', { timeout: 10000 })
  await page.waitForTimeout(550)
  const exploreState = await page.evaluate(() => ({
    path: window.location.pathname,
    heading: document.querySelector('.workspace-result-head h1')?.textContent || '',
    text: document.querySelector('.workspace-result-page')?.textContent || '',
  }))
  assert(exploreState.path === '/', `${viewport.width}: Explore must remain in the root scene`)
  assert(exploreState.heading.includes('探索 (1)'), `${viewport.width}: Explore result heading is missing`)
  assert(!exploreState.text.includes('陈旧结果'), `${viewport.width}: stale search response must not overwrite Explore`)
  await page.locator('.workspace-result-page .discover-results .result-card .book-cover-shared').first().click()
  await page.waitForSelector('.book-info-dialog', { timeout: 10000 })
  const exploreBookInfo = page.locator('.book-info-dialog')
  assert(await exploreBookInfo.getByText('加入书架', { exact: true }).count() === 1, `${viewport.width}: explore cover must open the shared unshelved BookInfo action`)
  assert(await exploreBookInfo.getByText('加入并阅读', { exact: true }).count() === 0, `${viewport.width}: explore BookInfo must not expose add-and-read`)
  const exploreBookInfoURL = await page.url()
  await exploreBookInfo.locator('.el-dialog__headerbtn').click()
  await exploreBookInfo.waitFor({ state: 'hidden', timeout: 10000 })
  assert(await page.url() === exploreBookInfoURL, `${viewport.width}: closing explore BookInfo must preserve the workspace route`)
  await page.locator('.workspace-result-actions').getByRole('button', { name: '加载更多', exact: true }).click()
  await page.waitForFunction(() => document.querySelectorAll('.workspace-result-page .discover-results .result-card').length === 2)
  const exploreEndButton = page.locator('.workspace-result-actions').getByRole('button', { name: '没有更多了', exact: true })
  assert(await exploreEndButton.isDisabled(), `${viewport.width}: Explore completion must remain visibly disabled`)
  await assertNoHorizontalOverflow(page, `${viewport.width} explore`)

  await page.locator('.workspace-result-actions').getByRole('button', { name: '书架', exact: true }).click()
  await page.waitForSelector('.shelf-page .book-row', { timeout: 10000 })
  await assertNoHorizontalOverflow(page, `${viewport.width} shelf-return`)
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
    console.log(`index-workspace: ok ${checks.join(', ')} legacyRedirects=true sidebarSearch=true canonicalBookInfo=true exploreCoverInfo=true`)
  } finally {
    await browser.close()
  }
}

run().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})
