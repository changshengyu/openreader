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
    { id: 1, title: '远程书架书', author: 'OpenReader', sourceId: 1, categoryIds: [1], chapterCount: 25, cachedChapterCount: 1, lastChapter: '第二十五章' },
    { id: 2, title: '另一远程书', author: 'OpenReader', sourceId: 1, categoryIds: [1], chapterCount: 25, cachedChapterCount: 0, lastChapter: '第二十五章' },
    { id: 3, title: '本地书架书', author: 'OpenReader', sourceId: 0, chapterCount: 2 },
  ]
}

async function installApiMocks(page) {
  const state = { clearRequests: 0, browserChapterRequests: [] }
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
      return route.fulfill(json({ affected: payload.bookIds?.length || 0 }))
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
        if (signal?.aborted) {
          abort()
          return
        }
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
}

async function dismissVisibleDropdowns(page) {
  await page.locator('.book-manage-title').click()
  await page.waitForFunction(() => ![...document.querySelectorAll('.el-dropdown-menu')].some(menu => {
    const style = getComputedStyle(menu)
    return style.display !== 'none' && style.visibility !== 'hidden' && menu.getClientRects().length
  }))
}

async function clickBookStopButton(page, title) {
  await page.waitForFunction(bookTitle => {
    const rows = [...document.querySelectorAll('.mobile-manage-card, .desktop-manage-table tbody tr')]
      .filter(row => row.getClientRects().length && row.textContent?.includes(bookTitle))
    return rows.some(row => [...row.querySelectorAll('button')].some(button => button.textContent?.trim().startsWith('停止')))
  }, title)
  await page.evaluate(bookTitle => {
    const rows = [...document.querySelectorAll('.mobile-manage-card, .desktop-manage-table tbody tr')]
      .filter(row => row.getClientRects().length && row.textContent?.includes(bookTitle))
    const button = rows.flatMap(row => [...row.querySelectorAll('button')])
      .find(node => node.textContent?.trim().startsWith('停止'))
    if (!button) throw new Error(`stop button not found for ${bookTitle}`)
    button.click()
  }, title)
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

  const managedRow = title => viewport.width <= 750
    ? manager.locator('.mobile-manage-card').filter({ hasText: title })
    : manager.locator('.desktop-manage-table tbody tr').filter({ hasText: title })
  let firstRemoteRow = managedRow('远程书架书')
  let secondRemoteRow = managedRow('另一远程书')
  await firstRemoteRow.getByRole('button', { name: '缓存', exact: true }).click()
  await chooseVisibleMenuItem(page, '缓存到服务器')
  await dismissVisibleDropdowns(page)
  await firstRemoteRow.getByRole('button', { name: /^停止/ }).waitFor({ state: 'visible' })
  await secondRemoteRow.getByRole('button', { name: '缓存', exact: true }).click()
  await chooseVisibleMenuItem(page, '缓存到服务器')
  await dismissVisibleDropdowns(page)
  await page.waitForTimeout(100)
  const secondStart = await page.evaluate(() => ({
    requests: window.__bookManageCacheMock.requests,
    visibleMenus: [...document.querySelectorAll('.el-dropdown-menu')]
      .filter(menu => getComputedStyle(menu).display !== 'none' && menu.getClientRects().length)
      .map(menu => menu.textContent?.trim()),
  }))
  assert(secondStart.requests.some(item => item.bookId === 2), `${viewport.width}: second cache command did not reach its own job: ${JSON.stringify(secondStart)}`)
  await secondRemoteRow.getByRole('button', { name: /^停止/ }).waitFor({ state: 'visible' })

  await manager.getByRole('button', { name: '取消', exact: true }).click()
  await manager.waitFor({ state: 'hidden', timeout: 10000 })
  await page.getByRole('button', { name: '书籍管理', exact: true }).click()
  await manager.waitFor({ state: 'visible', timeout: 10000 })
  firstRemoteRow = managedRow('远程书架书')
  secondRemoteRow = managedRow('另一远程书')
  assert(await firstRemoteRow.getByRole('button', { name: /^停止/ }).isVisible(), `${viewport.width}: first cache job must survive BookManage reopen`)
  assert(await secondRemoteRow.getByRole('button', { name: /^停止/ }).isVisible(), `${viewport.width}: second cache job must survive BookManage reopen`)
  await firstRemoteRow.getByRole('button', { name: /^停止/ }).click()
  await page.getByText('已取消服务器缓存', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  await page.getByText('已缓存 25/25 章', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })

  const streamState = await page.evaluate(() => window.__bookManageCacheMock)
  assert(streamState.requests.length === 2, `${viewport.width}: expected two independent server cache requests, got ${JSON.stringify(streamState)}`)
  assert(streamState.requests.every(item => JSON.stringify(item.payload) === JSON.stringify({ all: true, chapterIndex: 0, refresh: false })), `${viewport.width}: BookManage server cache payload must cover the whole catalogue: ${JSON.stringify(streamState)}`)
  assert(streamState.aborted.join(',') === '1', `${viewport.width}: cancelling the first book must leave the second request running: ${JSON.stringify(streamState)}`)

  await firstRemoteRow.getByRole('button', { name: '缓存', exact: true }).click()
  await chooseVisibleMenuItem(page, '缓存到浏览器')
  const browserRequestsAtCancel = apiState.browserChapterRequests.length
  await clickBookStopButton(page, '远程书架书')
  await page.getByText('已取消浏览器缓存', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  await dismissVisibleDropdowns(page)
  assert(apiState.browserChapterRequests.length <= browserRequestsAtCancel + 2, `${viewport.width}: browser cancellation scheduled work beyond the two active workers: before=${browserRequestsAtCancel} after=${JSON.stringify(apiState.browserChapterRequests)}`)
  assert(apiState.browserChapterRequests.length < 25, `${viewport.width}: browser cancellation continued through the whole catalogue`)

  await firstRemoteRow.getByRole('button', { name: '缓存', exact: true }).click()
  await chooseVisibleMenuItem(page, '删除服务器缓存')
  const clearConfirm = page.locator('.el-message-box')
  await clearConfirm.waitFor({ state: 'visible' })
  await clearConfirm.getByRole('button', { name: '取消', exact: true }).click()
  await clearConfirm.waitFor({ state: 'hidden' })
  assert(apiState.clearRequests === 0, `${viewport.width}: cancelling server-cache deletion must send no request`)
  await firstRemoteRow.getByRole('button', { name: '缓存', exact: true }).click()
  await chooseVisibleMenuItem(page, '删除服务器缓存')
  await clearConfirm.getByRole('button', { name: '确定', exact: true }).click()
  await clearConfirm.waitFor({ state: 'hidden' })
  assert(apiState.clearRequests === 1, `${viewport.width}: confirmed server-cache deletion must send exactly one request`)

  assert(await manager.isVisible(), `${viewport.width}: cache jobs and confirmations must leave BookManage open`)
  await firstRemoteRow.getByRole('button', { name: '分组', exact: true }).click()
  const groupSet = page.locator('.global-book-group-dialog')
  await groupSet.waitFor({ state: 'visible', timeout: 10000 })
  await assertMobileFullscreen(page, viewport, '.global-book-group-dialog', 'BookGroup set')
  const groupCheckbox = groupSet.locator('.group-set-table .el-checkbox__input').first()
  assert(await groupCheckbox.evaluate(node => node.classList.contains('is-checked')), `${viewport.width}: BookGroup set must preselect the book's existing categories`)
  await groupCheckbox.click()
  await page.waitForFunction(() => !document.querySelector('.group-set-table .el-checkbox__input')?.classList.contains('is-checked'))
  await groupSet.getByRole('button', { name: '确认', exact: true }).click()
  await page.waitForTimeout(250)
  assert(await groupSet.isVisible(), `${viewport.width}: an empty BookGroup selection must keep its dialog open`)
  await groupSet.getByRole('button', { name: '取消', exact: true }).click()
  await groupSet.waitFor({ state: 'hidden', timeout: 10000 })
  assert(await manager.isVisible(), `${viewport.width}: closing BookGroup set must leave BookManage open`)

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
  const browser = await openSmokeBrowser()
  try {
    const checks = []
    const requested = process.env.SMOKE_VIEWPORT
    const viewports = requested
      ? [requested.split('x').map(Number)].map(([width, height]) => ({ width, height }))
      : [{ width: 1440, height: 900 }, { width: 390, height: 844 }, { width: 360, height: 800 }]
    for (const viewport of viewports) checks.push(await runViewport(browser, viewport))
    console.log(`book-management-dialog: ok ${checks.join(', ')} dialogs=true mobileFullscreen=true bookInfoCoexists=true groupManagement=true`)
  } finally {
    await browser.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
