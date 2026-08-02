#!/usr/bin/env node

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
    { id: 1, title: '远程书架书', author: 'OpenReader', sourceId: 1, categoryIds: [1], chapterCount: 25, cachedChapterCount: 1, lastChapter: '第二十五章', originalFile: 'hidden-search-hit.epub' },
    { id: 2, title: '另一远程书', author: 'OpenReader', sourceId: 1, categoryIds: [1], chapterCount: 25, cachedChapterCount: 0, lastChapter: '第二十五章' },
    { id: 3, title: '本地书架书', author: '本地作者', sourceId: 0, chapterCount: 2 },
  ]
}

async function installApiMocks(page) {
  const state = { clearRequests: 0, browserChapterRequests: [], bookListRequests: 0 }
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname.replace(/^\/api/, '')
    const method = request.method()

    if (path === '/me') return route.fulfill(json({ id: 1, username: 'manage-smoke', role: 'admin' }))
    if (path === '/health') return route.fulfill(json({ version: 'smoke', commit: 'book-manage-fixed-baseline' }))
    if (path === '/settings/reader' && method === 'GET') return route.fulfill(json({ key: 'reader', value: { theme: 'parchment', mode: 'page', pageMode: 'auto', pageType: 'normal' } }))
    if (path === '/settings/reader' && method === 'PUT') return route.fulfill(json({ key: 'reader', value: {} }))
    if (path === '/settings/preferences') return route.fulfill(json({ key: 'preferences', value: {} }))
    if (path === '/books' && method === 'GET') {
      state.bookListRequests += 1
      return route.fulfill(json(shelfBooks()))
    }
    if (path === '/books/1') return route.fulfill(json(shelfBooks()[0]))
    if (path === '/books/2') return route.fulfill(json(shelfBooks()[1]))
    if (/^\/books\/[12]\/chapters$/.test(path)) {
      return route.fulfill(json(Array.from({ length: 25 }, (_, index) => ({ id: index + 1, index, title: `第${index + 1}章` }))))
    }
    const contentMatch = path.match(/^\/books\/(\d+)\/chapters\/(\d+)\/content$/)
    if (contentMatch) {
      const bookId = Number(contentMatch[1])
      const index = Number(contentMatch[2])
      state.browserChapterRequests.push({ bookId, index })
      await new Promise(resolve => setTimeout(resolve, 220))
      return route.fulfill(json({ chapter: { id: index + 1, index, title: `第${index + 1}章` }, content: `第${index + 1}章正文` }))
    }
    if (path === '/books/batch' && method === 'POST') {
      const payload = request.postDataJSON()
      if (payload.action === 'clear-cache') {
        state.clearRequests += 1
        return route.fulfill(json({ affected: payload.bookIds.length, cleared: 25 }))
      }
      return route.fulfill(json({ affected: payload.bookIds?.length || 0, books: [] }))
    }
    if (/^\/books\/\d+\/category$/.test(path) && method === 'PUT') {
      const id = Number(path.split('/')[2])
      return route.fulfill(json({ ...shelfBooks().find(book => book.id === id), categoryIds: request.postDataJSON().categoryIds || [] }))
    }
    if (path === '/categories') return route.fulfill(json([{ id: 1, name: '测试分组', show: true, sortOrder: 10 }]))
    if (path === '/sources') return route.fulfill(json([{ id: 1, name: '测试书源', enabled: true }]))
    if (path.startsWith('/cache')) return route.fulfill(json({ total: 0, books: 0, chapters: 0 }))
    return route.fulfill(json({}))
  })
  return state
}

async function installCacheStreamMock(page) {
  await page.addInitScript(() => {
    const nativeFetch = window.fetch.bind(window)
    window.__bookManageCacheMock = { requests: [], aborted: [] }
    window.fetch = (input, init = {}) => {
      const url = new URL(typeof input === 'string' ? input : input.url, location.href)
      const match = url.pathname.match(/^\/api\/books\/(\d+)\/cache\/stream$/)
      if (!match) return nativeFetch(input, init)
      const bookId = Number(match[1])
      const payload = JSON.parse(init.body || '{}')
      window.__bookManageCacheMock.requests.push({ bookId, payload })
      return new Promise((resolve, reject) => {
        const signal = init.signal
        const abort = () => {
          window.__bookManageCacheMock.aborted.push(bookId)
          reject(new DOMException('aborted', 'AbortError'))
        }
        if (signal?.aborted) return abort()
        signal?.addEventListener('abort', abort, { once: true })
        setTimeout(() => {
          if (signal?.aborted) return
          signal?.removeEventListener('abort', abort)
          const title = bookId === 1 ? '远程书架书' : '另一远程书'
          const body = [
            'event: message\n',
            `data: {"bookId":${bookId},"cachedCount":1,"successCount":1,"failedCount":0,"processed":1,"cached":1,"requested":1,"total":25,"chapterIndex":0,"failed":0}\n\n`,
            'event: end\n',
            `data: {"bookId":${bookId},"cachedCount":25,"successCount":24,"failedCount":0,"processed":25,"cached":25,"requested":25,"total":25,"failed":0,"book":{"id":${bookId},"title":"${title}","author":"OpenReader","sourceId":1,"categoryIds":[1],"chapterCount":25,"cachedChapterCount":25,"lastChapter":"第二十五章"}}\n\n`,
          ].join('')
          resolve(new Response(body, { status: 200, headers: { 'Content-Type': 'text/event-stream' } }))
        }, bookId === 1 ? 3000 : 2500)
      })
    }
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

async function assertDialogGeometry(page, viewport, selector, label) {
  await page.waitForFunction(dialogSelector => {
    const dialog = document.querySelector(dialogSelector)
    const overlay = dialog?.parentElement
    return dialog && overlay && Math.abs(overlay.getBoundingClientRect().top) < 0.5
  }, selector)
  const geometry = await page.locator(selector).evaluate(node => {
    const rect = node.getBoundingClientRect()
    const style = getComputedStyle(node)
    const parent = node.parentElement
    const parentStyle = parent ? getComputedStyle(parent) : null
    const parentRect = parent?.getBoundingClientRect()
    const table = node.querySelector('.book-manage-table')?.getBoundingClientRect()
    const headers = [...node.querySelectorAll('.el-table__header-wrapper th')].slice(0, 2).map(header => ({
      position: getComputedStyle(header).position,
      classes: header.className,
    }))
    return {
      left: rect.left,
      top: rect.top,
      width: rect.width,
      height: rect.height,
      tableHeight: table?.height || 0,
      marginTop: style.marginTop,
      marginBottom: style.marginBottom,
      transform: style.transform,
      dialogMarginTop: style.getPropertyValue('--el-dialog-margin-top').trim(),
      overlayScrollTop: node.parentElement?.scrollTop || 0,
      overlayRect: parentRect ? { top: parentRect.top, height: parentRect.height } : null,
      overlayStyle: parentStyle ? {
        display: parentStyle.display,
        position: parentStyle.position,
        paddingTop: parentStyle.paddingTop,
        alignItems: parentStyle.alignItems,
        justifyContent: parentStyle.justifyContent,
      } : null,
      windowScrollY: window.scrollY,
      headers,
    }
  })
  if (viewport.width <= 750) {
    assert(Math.abs(geometry.left) < 1 && Math.abs(geometry.top) < 1, `${viewport.width}: ${label} should start at fullscreen origin: ${JSON.stringify(geometry)}`)
    assert(Math.abs(geometry.width - viewport.width) < 1, `${viewport.width}: ${label} should fill viewport width: ${JSON.stringify(geometry)}`)
    assert(geometry.height >= viewport.height - 1, `${viewport.width}: ${label} should fill viewport height: ${JSON.stringify(geometry)}`)
    assert(Math.abs(geometry.tableHeight - (viewport.height - 226)) <= 3, `${viewport.width}: mobile table height drifted: ${JSON.stringify(geometry)}`)
    assert(geometry.headers.every(header => header.position === 'sticky' || header.classes.includes('is-left')), `${viewport.width}: selection/title columns must stay fixed: ${JSON.stringify(geometry.headers)}`)
    return
  }
  const expectedWidth = Math.min(Math.max(viewport.width * 0.7, 750), 1000)
  const contentHeight = Math.min(viewport.height * 0.7 - 184, 400)
  const expectedTop = (viewport.height - contentHeight - 184) / 2
  const expectedTable = contentHeight - 42
  assert(Math.abs(geometry.width - expectedWidth) <= 2, `${viewport.width}: desktop width drifted: ${JSON.stringify(geometry)}`)
  assert(Math.abs(geometry.top - expectedTop) <= 2, `${viewport.width}: desktop top drifted: ${JSON.stringify(geometry)}`)
  assert(Math.abs(geometry.tableHeight - expectedTable) <= 3, `${viewport.width}: desktop table height drifted: ${JSON.stringify(geometry)}`)
}

async function chooseVisibleMenuItem(page, name) {
  await page.waitForFunction(label => [...document.querySelectorAll('.el-dropdown-menu')].some(menu => {
    const style = getComputedStyle(menu)
    if (style.display === 'none' || style.visibility === 'hidden' || !menu.getClientRects().length) return false
    return [...menu.querySelectorAll('[role="menuitem"]')].some(item => item.textContent?.trim() === label)
  }), name)
  await page.evaluate(label => {
    const menus = [...document.querySelectorAll('.el-dropdown-menu')].filter(menu => {
      const style = getComputedStyle(menu)
      return style.display !== 'none' && style.visibility !== 'hidden' && menu.getClientRects().length
    })
    const item = [...(menus.at(-1)?.querySelectorAll('[role="menuitem"]') || [])]
      .find(node => node.textContent?.trim() === label)
    if (!item) throw new Error(`visible menu item not found: ${label}`)
    item.click()
  }, name)
  await page.waitForFunction(() => [...document.querySelectorAll('.el-dropdown-menu')].every(menu => {
    const style = getComputedStyle(menu)
    return style.display === 'none' || style.visibility === 'hidden' || !menu.getClientRects().length
  }))
}

async function chooseRowCacheCommand(page, row, buttonName, command) {
  await row.getByRole('button', { name: buttonName, exact: true }).click()
  await chooseVisibleMenuItem(page, command)
}

function managedRow(manager, title) {
  return manager.locator('.book-manage-table tbody tr').filter({ hasText: title })
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
  await installCacheStreamMock(page)
  const apiState = await installApiMocks(page)

  const root = targetUrl.replace(/\/$/, '')
  await page.goto(root, { waitUntil: 'networkidle' })
  await page.waitForSelector('.shelf-page .book-row', { timeout: 10000 })
  await openMobileNavigation(page, viewport)
  const beforeFirstOpen = apiState.bookListRequests
  await page.getByRole('button', { name: '书籍管理', exact: true }).click()
  const manager = page.locator('.global-book-manage-dialog')
  await manager.waitFor({ state: 'visible', timeout: 10000 })
  await assertDialogGeometry(page, viewport, '.global-book-manage-dialog', 'BookManage')
  await assertNoHorizontalOverflow(page, `${viewport.width} manage-open`)
  assert(apiState.bookListRequests > beforeFirstOpen, `${viewport.width}: opening manager must force a fresh full shelf read`)
  assert(await manager.locator('.book-manage-table').count() === 1, `${viewport.width}: BookManage must own one table`)
  assert(await manager.locator('.mobile-manage-card').count() === 0, `${viewport.width}: mobile cards must not survive fixed-upstream rebuild`)
  const headers = await manager.locator('.el-table__header-wrapper th').allTextContents()
  assert(headers.map(text => text.trim()).join('|').includes('书名名|作者|分组|章节|操作'), `${viewport.width}: table headers drifted: ${JSON.stringify(headers)}`)
  assert(!(await manager.textContent()).includes('阅读进度'), `${viewport.width}: manager must not add a reading-progress row`)

  const search = manager.getByPlaceholder('搜索书名或作者')
  await search.fill('hidden-search-hit')
  assert(await manager.locator('.book-manage-table tbody tr').count() === 0, `${viewport.width}: file-name-only search must not match`)
  await search.fill('本地作者')
  assert(await managedRow(manager, '本地书架书').count() === 1, `${viewport.width}: author search must match`)
  await search.fill('远程书架书')
  await manager.getByRole('button', { name: '取消', exact: true }).click()
  await manager.waitFor({ state: 'hidden', timeout: 10000 })
  const beforeReopen = apiState.bookListRequests
  await page.getByRole('button', { name: '书籍管理', exact: true }).click()
  await manager.waitFor({ state: 'visible', timeout: 10000 })
  assert(await search.inputValue() === '远程书架书', `${viewport.width}: closing manager must preserve search query`)
  assert(apiState.bookListRequests > beforeReopen, `${viewport.width}: reopening manager must force another shelf read`)
  await search.clear()

  await manager.getByRole('button', { name: '批量删除', exact: true }).click()
  await page.getByText('请选择需要删除的书籍', { exact: true }).waitFor({ state: 'visible' })
  await manager.getByRole('button', { name: '批量添加分组', exact: true }).click()
  await chooseVisibleMenuItem(page, '测试分组')
  await page.getByText('请选择需要添加分组的书籍', { exact: true }).waitFor({ state: 'visible' })
  await manager.getByRole('button', { name: '批量移除分组', exact: true }).click()
  await chooseVisibleMenuItem(page, '测试分组')
  await page.getByText('请选择需要移除分组的书籍', { exact: true }).waitFor({ state: 'visible' })

  if (viewport.width <= 750) {
    await managedRow(manager, '本地书架书').locator('td').nth(2).click()
    const sidebarMargin = await page.locator('.app-sidebar').evaluate(node => Number.parseFloat(getComputedStyle(node).marginLeft))
    assert(Math.abs(sidebarMargin) < 0.5, `${viewport.width}: BookManage table clicks must not close the mobile sidebar`)
  }

  await manager.getByRole('button', { name: '远程书架书', exact: true }).click()
  await page.waitForSelector('.book-info-dialog', { timeout: 10000 })
  assert(await manager.isVisible(), `${viewport.width}: BookInfo must coexist above BookManage`)
  await page.locator('.book-info-dialog .el-dialog__headerbtn').click()
  await page.waitForFunction(() => !document.querySelector('.book-info-dialog .el-dialog'))
  assert(await manager.isVisible(), `${viewport.width}: closing BookInfo must leave BookManage open`)

  let firstRemoteRow = managedRow(manager, '远程书架书')
  let secondRemoteRow = managedRow(manager, '另一远程书')
  await chooseRowCacheCommand(page, firstRemoteRow, '缓存', '缓存到服务器')
  await firstRemoteRow.getByRole('button', { name: '缓存中', exact: true }).waitFor({ state: 'visible' })
  await chooseRowCacheCommand(page, secondRemoteRow, '缓存', '缓存到服务器')
  await secondRemoteRow.getByRole('button', { name: '缓存中', exact: true }).waitFor({ state: 'visible' })

  await manager.getByRole('button', { name: '取消', exact: true }).click()
  await manager.waitFor({ state: 'hidden', timeout: 10000 })
  await page.getByRole('button', { name: '书籍管理', exact: true }).click()
  await manager.waitFor({ state: 'visible', timeout: 10000 })
  firstRemoteRow = managedRow(manager, '远程书架书')
  secondRemoteRow = managedRow(manager, '另一远程书')
  assert(await firstRemoteRow.getByRole('button', { name: '缓存中', exact: true }).isVisible(), `${viewport.width}: first cache job must survive reopen`)
  assert(await secondRemoteRow.getByRole('button', { name: '缓存中', exact: true }).isVisible(), `${viewport.width}: second cache job must survive reopen`)
  await chooseRowCacheCommand(page, firstRemoteRow, '缓存中', '缓存到服务器')
  const cancelMessage = page.getByText('已取消缓存', { exact: true })
  await cancelMessage.waitFor({ state: 'visible', timeout: 10000 })
  await page.getByText('另一远程书缓存到服务器完成', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })

  const streamState = await page.evaluate(() => window.__bookManageCacheMock)
  assert(streamState.requests.length === 2, `${viewport.width}: expected two independent cache requests: ${JSON.stringify(streamState)}`)
  assert(streamState.requests.every(item => JSON.stringify(item.payload) === JSON.stringify({ all: true, chapterIndex: 0, refresh: false })), `${viewport.width}: cache payload must cover whole catalogue: ${JSON.stringify(streamState)}`)
  assert(streamState.aborted.join(',') === '1', `${viewport.width}: cancelling first must leave second running: ${JSON.stringify(streamState)}`)
  await cancelMessage.waitFor({ state: 'hidden', timeout: 10000 })

  await chooseRowCacheCommand(page, firstRemoteRow, '缓存', '缓存到浏览器')
  await firstRemoteRow.getByRole('button', { name: '缓存中', exact: true }).waitFor({ state: 'visible' })
  const browserRequestsAtCancel = apiState.browserChapterRequests.length
  await chooseRowCacheCommand(page, firstRemoteRow, '缓存中', '缓存到浏览器')
  await cancelMessage.waitFor({ state: 'visible', timeout: 10000 })
  await page.waitForTimeout(500)
  assert(apiState.browserChapterRequests.length <= browserRequestsAtCancel + 2, `${viewport.width}: browser cancel scheduled beyond active workers: ${JSON.stringify(apiState.browserChapterRequests)}`)
  assert(apiState.browserChapterRequests.length < 25, `${viewport.width}: browser cancellation continued through whole catalogue`)

  await chooseRowCacheCommand(page, firstRemoteRow, '缓存', '删除服务器缓存')
  const clearConfirm = page.locator('.el-message-box').filter({ hasText: '确认要删除服务器上《远程书架书》的缓存章节吗?' })
  await clearConfirm.waitFor({ state: 'visible' })
  await clearConfirm.getByRole('button', { name: '取消', exact: true }).click()
  await clearConfirm.waitFor({ state: 'hidden' })
  assert(apiState.clearRequests === 0, `${viewport.width}: cancelled server cache deletion sent a request`)
  await chooseRowCacheCommand(page, firstRemoteRow, '缓存', '删除服务器缓存')
  await clearConfirm.getByRole('button', { name: '确定', exact: true }).click()
  await clearConfirm.waitFor({ state: 'hidden' })
  await page.getByText('删除服务器缓存成功', { exact: true }).waitFor({ state: 'visible' })
  assert(apiState.clearRequests === 1, `${viewport.width}: confirmed server cache deletion must send one request`)

  assert(await manager.isVisible(), `${viewport.width}: cache actions must leave BookManage open`)
  firstRemoteRow = managedRow(manager, '远程书架书')
  await firstRemoteRow.getByRole('button', { name: '分组', exact: true }).click()
  const groupSet = page.locator('.global-book-group-dialog')
  await groupSet.waitFor({ state: 'visible', timeout: 10000 })
  const groupCheckbox = groupSet.locator('.book-group-table .el-checkbox__input').first()
  await groupSet.locator('.book-group-table .el-checkbox__input.is-checked').first().waitFor({ state: 'visible', timeout: 10000 })
  assert(await groupCheckbox.evaluate(node => node.classList.contains('is-checked')), `${viewport.width}: BookGroup must preselect persisted category`)
  await groupCheckbox.click()
  await groupSet.getByRole('button', { name: '确认', exact: true }).click()
  await page.waitForTimeout(250)
  assert(await groupSet.isVisible(), `${viewport.width}: empty BookGroup selection must remain open`)
  await groupSet.getByRole('button', { name: '取消', exact: true }).click()
  await groupSet.waitFor({ state: 'hidden', timeout: 10000 })
  assert(await manager.isVisible(), `${viewport.width}: closing BookGroup must leave BookManage open`)

  await manager.getByRole('button', { name: '取消', exact: true }).click()
  await manager.waitFor({ state: 'hidden', timeout: 10000 })
  assert(await page.evaluate(() => location.pathname) === '/', `${viewport.width}: closing manager must retain root route`)
  await assertNoHorizontalOverflow(page, `${viewport.width} dialogs-close`)
  assert(failures.length === 0, failures.join('\n'))
  await context.close()
  return `${viewport.width}x${viewport.height}`
}

async function run() {
  const browser = await openSmokeBrowser()
  try {
    const checks = []
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
    for (const viewport of viewports) checks.push(await runViewport(browser, viewport))
    console.log(`book-management-dialog: ok ${checks.join(', ')} oneTable=true exactGeometry=true queryPersistence=true upstreamBatchErrors=true cacheJobs=true`)
  } finally {
    await browser.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
