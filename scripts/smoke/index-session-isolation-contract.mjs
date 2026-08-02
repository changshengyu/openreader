#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = (process.env.TARGET_URL || 'http://127.0.0.1:4173').replace(/\/$/, '')

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function deferred() {
  let resolve
  const promise = new Promise(done => {
    resolve = done
  })
  return { promise, resolve }
}

function tokenFor(userId, nonce) {
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url')
  const payload = Buffer.from(JSON.stringify({ userId, sub: String(userId) })).toString('base64url')
  return `${header}.${payload}.index-${userId}-${nonce}`
}

function json(data, status = 200) {
  return {
    status,
    contentType: 'application/json',
    body: JSON.stringify(data),
  }
}

function requestJSON(request) {
  try {
    return request.postDataJSON()
  } catch {
    return {}
  }
}

function authorizationToken(request) {
  const authorization = request.headers().authorization || ''
  return authorization.startsWith('Bearer ') ? authorization.slice(7) : ''
}

function remoteBook(title, sourceName = '用户 A 私有书源') {
  return {
    title,
    author: '会话隔离测试',
    url: `https://source.example/${encodeURIComponent(title)}`,
    bookUrl: `https://source.example/${encodeURIComponent(title)}`,
    sourceId: 11,
    sourceName,
    latestChapter: '第一章',
    intro: '验证旧账号的迟到结果不能进入新认证会话。',
  }
}

function shelfBook(id, title, owner) {
  return {
    id,
    title,
    author: owner,
    sourceId: id * 10,
    sourceName: `${owner} 私有书源`,
    url: `https://shelf.example/${id}`,
    bookUrl: `https://shelf.example/${id}`,
    chapterCount: 1,
    categoryIds: [],
    updatedAt: '2026-07-28T00:00:00Z',
  }
}

function createState() {
  return {
    tokenA: tokenFor(1, 'expired'),
    tokenARenewed: tokenFor(1, 'renewed'),
    tokenB: tokenFor(2, 'renewed'),
    userA: { id: 1, username: 'account-a', role: 'admin' },
    userB: { id: 2, username: 'account-b', role: 'user' },
    shelfA: shelfBook(1, 'A 账号书架', '用户 A'),
    shelfB: shelfBook(2, 'B 账号干净书架', '用户 B'),
    searchRequests: [],
    exploreRequests: [],
    delayedSearch: deferred(),
    delayedSearchStarted: deferred(),
    delayedExplore: deferred(),
    delayedExploreStarted: deferred(),
    intentionalStaleExploreError: false,
  }
}

function userForToken(token, state) {
  return token === state.tokenB ? state.userB : state.userA
}

async function installApiMocks(page, state) {
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    const requestToken = authorizationToken(request)
    const activeUser = userForToken(requestToken, state)

    if (path === '/auth/login' && method === 'POST') {
      const payload = requestJSON(request)
      const differentAccount = payload.username === state.userB.username
      return route.fulfill(json({
        token: differentAccount ? state.tokenB : state.tokenARenewed,
        user: differentAccount ? state.userB : state.userA,
      }))
    }
    if (path === '/me') return route.fulfill(json(activeUser))
    if (path === '/health') {
      return route.fulfill(json({ version: 'index-session-smoke', commit: 'index-session-smoke' }))
    }
    if (path.startsWith('/settings/') && method === 'GET') {
      const key = path.slice('/settings/'.length)
      return route.fulfill(json({
        key,
        updatedAt: '2026-07-28T00:00:00Z',
        value: key === 'reader'
          ? {
              theme: 'parchment',
              themeType: 'day',
              mode: 'page',
              pageMode: 'auto',
              autoTheme: false,
            }
          : {},
      }))
    }
    if (path.startsWith('/settings/') && method === 'PUT') {
      return route.fulfill(json({
        key: path.slice('/settings/'.length),
        updatedAt: '2026-07-28T00:00:01Z',
        value: requestJSON(request).value || {},
      }))
    }
    if (path === '/books') {
      return route.fulfill(json(activeUser.id === state.userB.id ? [state.shelfB] : [state.shelfA]))
    }
    if (path === '/categories' || path === '/book-groups') return route.fulfill(json([]))
    if (path === '/sources') {
      return route.fulfill(json([{
        id: 11,
        name: activeUser.id === state.userB.id ? '用户 B 私有书源' : '用户 A 私有书源',
        enabled: true,
        group: '会话隔离',
      }]))
    }
    if (path === '/explore/sources') {
      return route.fulfill(json([{
        id: 11,
        name: activeUser.id === state.userB.id ? '用户 B 探索书源' : '用户 A 探索书源',
        enabled: true,
        group: '会话隔离',
        exploreGroups: [[{
          name: '会话入口',
          url: 'https://source.example/session-explore',
        }]],
      }]))
    }
    if (path === '/search' && method === 'POST') {
      const payload = requestJSON(request)
      state.searchRequests.push({ token: requestToken, payload })
      if (payload.keyword === 'A 预热搜索') {
        return route.fulfill(json({
          list: [remoteBook('A 旧搜索结果')],
          page: 1,
          lastIndex: 0,
          hasMore: false,
        }))
      }
      if (payload.keyword === 'A 恢复搜索' && requestToken === state.tokenA) {
        state.delayedSearchStarted.resolve()
        await state.delayedSearch.promise
        return route.fulfill(json({
          list: [remoteBook('A 迟到搜索结果')],
          page: 1,
          lastIndex: 0,
          hasMore: false,
        }))
      }
      if (payload.keyword === 'A 恢复搜索' && requestToken === state.tokenARenewed) {
        return route.fulfill(json({
          list: [remoteBook('A 重新认证搜索结果')],
          page: 1,
          lastIndex: 0,
          hasMore: false,
        }))
      }
      return route.fulfill(json({ list: [], page: 1, lastIndex: -1, hasMore: false }))
    }
    if (path === '/explore/11') {
      state.exploreRequests.push({ token: requestToken, url: url.searchParams.get('url') })
      if (requestToken === state.tokenA) {
        state.delayedExploreStarted.resolve()
        await state.delayedExplore.promise
        return route.fulfill(json({
          error: { code: 'STALE_EXPLORE', message: 'A 迟到探索错误' },
        }, 500))
      }
      return route.fulfill(json({
        items: [remoteBook('B 不应自动恢复的探索结果', '用户 B 探索书源')],
        page: 1,
        hasMore: false,
      }))
    }
    if (path === '/cache/stats') return route.fulfill(json({ files: 0, size: 0 }))
    if (path === '/cache' && method === 'DELETE') {
      return route.fulfill(json({ clearedFiles: 0, clearedSize: 0 }))
    }
    return route.fulfill(json({}))
  })
}

async function openSession(browser, viewport) {
  const state = createState()
  const context = await browser.newContext({
    viewport,
    hasTouch: viewport.width <= 900,
    isMobile: viewport.width <= 900,
  })
  await context.addInitScript(token => localStorage.setItem('openreader_token', token), state.tokenA)
  const page = await context.newPage()
  const errors = []
  page.on('pageerror', error => errors.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (
      message.type() === 'error'
      && !/WebSocket connection to .*\/ws\/sync/.test(message.text())
    ) {
      if (
        state.intentionalStaleExploreError
        && /Failed to load resource: the server responded with a status of 500/.test(message.text())
      ) return
      errors.push(`console.error: ${message.text()}`)
    }
  })
  await installApiMocks(page, state)
  await page.goto(targetUrl, { waitUntil: 'networkidle' })
  await page.getByText(state.shelfA.title, { exact: true })
    .waitFor({ state: 'visible', timeout: 10_000 })
  return { context, errors, page, state }
}

async function openSidebar(page, viewport) {
  if (viewport.width > 900) return
  const sidebar = page.locator('.app-sidebar')
  const margin = await sidebar.evaluate(element => Number.parseFloat(getComputedStyle(element).marginLeft))
  if (Math.abs(margin) < 0.5) return
  const trigger = page.locator('.mobile-menu-trigger')
  if (await trigger.count()) {
    await trigger.click()
  } else {
    const shell = page.locator('.app-shell')
    await shell.dispatchEvent('touchstart', {
      touches: [{ identifier: 1, clientX: 30, clientY: 180 }],
    })
    await shell.dispatchEvent('touchmove', {
      touches: [{ identifier: 1, clientX: 250, clientY: 182 }],
    })
    await shell.dispatchEvent('touchend', {
      touches: [],
      changedTouches: [{ identifier: 1, clientX: 250, clientY: 182 }],
    })
  }
  await page.waitForFunction(() => {
    const node = document.querySelector('.app-sidebar')
    return node && Math.abs(Number.parseFloat(getComputedStyle(node).marginLeft)) < 0.5
  })
}

async function runSidebarSearch(page, viewport, keyword) {
  await openSidebar(page, viewport)
  const input = page.locator('.app-shell-search input')
  await input.fill(keyword)
  await input.press('Enter')
}

async function waitForMessagesToClose(page) {
  await page.waitForFunction(() => (
    [...document.querySelectorAll('.el-message')]
      .every(element => getComputedStyle(element).display === 'none' || !element.isConnected)
  ), null, { timeout: 6_000 })
}

async function invalidateSession(session, viewport) {
  const { page, state } = session
  await page.evaluate(rejectedToken => {
    localStorage.removeItem('openreader_token')
    window.__openreaderAuthRequired = { reason: 'session', rejectedToken }
    window.dispatchEvent(new CustomEvent('openreader:auth-required', {
      detail: window.__openreaderAuthRequired,
    }))
  }, state.tokenA)
  await page.waitForFunction(() => (
    Boolean(document.querySelector('.workspace-auth-blocked'))
    && Boolean(document.querySelector('.auth-dialog'))
    && !document.querySelector('.app-shell')
  ))
  assert(await page.locator('.app-shell').count() === 0, `${viewport.width}: invalidated Index remained mounted`)
  assert(await page.locator('.result-shelf-page').count() === 0, `${viewport.width}: invalidated result scene remained mounted`)
  assert(await page.getByText('登录状态已失效，请重新登录', { exact: true }).count() === 1, `${viewport.width}: reauthentication warning is missing`)
  assert(await page.locator('.auth-dialog .el-dialog__headerbtn').count() === 0, `${viewport.width}: invalid-session dialog exposed a close button`)
}

async function submitLogin(page, username) {
  await page.getByPlaceholder('请输入用户名').fill(username)
  await page.getByPlaceholder('请输入密码').fill('password')
  await page.locator('.auth-dialog button[type="submit"]').click()
  await page.locator('.auth-dialog').waitFor({ state: 'hidden', timeout: 10_000 })
}

async function assertNoHorizontalOverflow(page, viewport, scene) {
  const geometry = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    innerWidth: window.innerWidth,
  }))
  assert(
    geometry.scrollWidth <= geometry.innerWidth + 1,
    `${viewport.width}: ${scene} horizontal overflow ${geometry.scrollWidth} > ${geometry.innerWidth}`,
  )
}

async function assertSameAccountSearch(browser, viewport) {
  const session = await openSession(browser, viewport)
  const { page, state } = session
  try {
    await runSidebarSearch(page, viewport, 'A 预热搜索')
    await page.getByText('A 旧搜索结果', { exact: true })
      .waitFor({ state: 'visible', timeout: 10_000 })
    await waitForMessagesToClose(page)

    await runSidebarSearch(page, viewport, 'A 恢复搜索')
    await state.delayedSearchStarted.promise
    await invalidateSession(session, viewport)
    assert(await page.getByText('A 旧搜索结果', { exact: true }).count() === 0, `${viewport.width}: visible A search row survived invalidation`)

    state.delayedSearch.resolve()
    await page.waitForTimeout(150)
    assert(await page.getByText('A 迟到搜索结果', { exact: true }).count() === 0, `${viewport.width}: delayed A search row committed while blocked`)
    assert(await page.locator('.el-message').count() === 0, `${viewport.width}: delayed A search emitted a toast while blocked`)

    await submitLogin(page, state.userA.username)
    await page.getByText('A 重新认证搜索结果', { exact: true })
      .waitFor({ state: 'visible', timeout: 10_000 })
    assert(new URL(page.url()).pathname === '/', `${viewport.width}: same-account Search left the Index route`)
    assert(await page.getByText('A 旧搜索结果', { exact: true }).count() === 0, `${viewport.width}: old visible search row was restored`)
    assert(await page.getByText('A 迟到搜索结果', { exact: true }).count() === 0, `${viewport.width}: delayed search row replaced the renewed result`)
    assert(await page.locator('.book-info-dialog').count() === 0, `${viewport.width}: old Search reopened BookInfo`)
    assert(!new URL(page.url()).pathname.includes('/reader'), `${viewport.width}: old Search navigated to Reader`)
    assert(
      state.searchRequests.some(request => (
        request.token === state.tokenA
        && request.payload.keyword === 'A 恢复搜索'
      )),
      `${viewport.width}: expired Search request was not exercised`,
    )
    assert(
      state.searchRequests.some(request => (
        request.token === state.tokenARenewed
        && request.payload.keyword === 'A 恢复搜索'
      )),
      `${viewport.width}: same-account intent was not re-fetched with the renewed token`,
    )
    await assertNoHorizontalOverflow(page, viewport, 'same-account Search')
    assert(session.errors.length === 0, `${viewport.width}: ${session.errors.join('\n')}`)
  } finally {
    state.delayedSearch.resolve()
    await session.context.close()
  }
}

async function assertDifferentAccountExplore(browser, viewport) {
  const session = await openSession(browser, viewport)
  const { page, state } = session
  try {
    await openSidebar(page, viewport)
    await page.getByRole('button', { name: '探索书源', exact: true }).click()
    const chooser = page.locator('.explore-workspace-popover:visible')
    await chooser.waitFor({ state: 'visible', timeout: 10_000 })
    const entry = chooser.locator('.explore-entry-row button').first()
    if (!await entry.isVisible()) {
      await chooser.locator('.el-collapse-item__header').first().click()
      await entry.waitFor({ state: 'visible', timeout: 10_000 })
    }
    await entry.click()
    await state.delayedExploreStarted.promise
    await invalidateSession(session, viewport)

    state.intentionalStaleExploreError = true
    state.delayedExplore.resolve()
    await page.waitForTimeout(150)
    assert(await page.getByText('A 迟到探索错误', { exact: false }).count() === 0, `${viewport.width}: delayed Explore emitted an old-account error`)
    assert(await page.locator('.el-message').count() === 0, `${viewport.width}: delayed Explore emitted a toast while blocked`)

    await submitLogin(page, state.userB.username)
    await page.getByText(state.shelfB.title, { exact: true })
      .waitFor({ state: 'visible', timeout: 10_000 })
    assert(new URL(page.url()).pathname === '/', `${viewport.width}: different account retained a non-Index route`)
    assert(await page.locator('.result-shelf-page').count() === 0, `${viewport.width}: different account retained Search/Explore results`)
    assert(await page.locator('.explore-workspace-popover:visible').count() === 0, `${viewport.width}: different account reopened the Explore chooser`)
    assert(await page.getByText(state.shelfA.title, { exact: true }).count() === 0, `${viewport.width}: different account exposed A shelf data`)
    assert(await page.getByText('B 不应自动恢复的探索结果', { exact: true }).count() === 0, `${viewport.width}: different account automatically replayed A Explore intent`)
    assert(await page.locator('.book-info-dialog').count() === 0, `${viewport.width}: different account retained an old overlay`)
    assert(state.exploreRequests.length === 1, `${viewport.width}: different account unexpectedly replayed Explore (${state.exploreRequests.length})`)
    assert(state.exploreRequests[0].token === state.tokenA, `${viewport.width}: Explore request did not belong exclusively to expired A`)
    await assertNoHorizontalOverflow(page, viewport, 'different-account shelf')
    assert(session.errors.length === 0, `${viewport.width}: ${session.errors.join('\n')}`)
  } finally {
    state.delayedExplore.resolve()
    await session.context.close()
  }
}

async function main() {
  const browser = await openSmokeBrowser()
  try {
    const checked = []
    for (const viewport of [
      { width: 1440, height: 900 },
      { width: 390, height: 844 },
      { width: 360, height: 800 },
    ]) {
      await assertSameAccountSearch(browser, viewport)
      await assertDifferentAccountExplore(browser, viewport)
      checked.push(`${viewport.width}x${viewport.height}`)
    }
    console.log(
      `index-session-isolation: ok ${checked.join(', ')} searchSameAccount=true exploreDifferentAccount=true staleResults=0 staleToasts=0 staleOverlays=0`,
    )
  } finally {
    await browser.close()
  }
}

main().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
