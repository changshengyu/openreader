#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = (process.env.TARGET_URL || 'http://127.0.0.1:4173').replace(/\/$/, '')

const viewports = [
  { width: 1440, height: 900 },
  { width: 390, height: 844 },
  { width: 360, height: 800 },
]

const scenarioNames = [
  'book-info',
  'storage-import',
  'webdav-restore',
  'source-save',
  'rss-save',
  'user-create',
]

const privateOverlaySelectors = [
  '.book-info-dialog',
  '.book-add-category-dialog',
  '.storage-import-preflight-dialog',
  '.storage-import-mode-dialog',
  '.storage-import-groups-dialog',
  '.storage-import-single-dialog',
  '.global-local-store-dialog',
  '.global-webdav-dialog',
  '.global-source-manage-dialog',
  '.global-rss-dialog',
  '.global-user-dialog',
  '.rss-source-editor-dialog',
  '.el-drawer[aria-label="新增书源"]',
]

const restoreEventNames = [
  'openreader:sources-update',
  'openreader:bookmarks-updated',
  'openreader:replace-rules-updated',
  'openreader:rss-updated',
]

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function selectedScenarios() {
  const requested = String(process.env.OVERLAY_SCENARIOS || '')
    .split(',')
    .map(value => value.trim())
    .filter(Boolean)
  if (!requested.length) return scenarioNames
  const unknown = requested.filter(value => !scenarioNames.includes(value))
  assert(!unknown.length, `unknown OVERLAY_SCENARIOS: ${unknown.join(', ')}`)
  return requested
}

function selectedViewports() {
  const requested = String(process.env.OVERLAY_VIEWPORTS || '')
    .split(',')
    .map(value => value.trim())
    .filter(Boolean)
  if (!requested.length) return viewports
  const resolved = requested.map(value => (
    viewports.find(viewport => (
      value === String(viewport.width)
      || value === `${viewport.width}x${viewport.height}`
    ))
  ))
  const unknown = requested.filter((value, index) => !resolved[index])
  assert(!unknown.length, `unknown OVERLAY_VIEWPORTS: ${unknown.join(', ')}`)
  return resolved
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
  return `${header}.${payload}.overlay-${userId}-${nonce}`
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
    return request.postDataJSON() || {}
  } catch {
    return {}
  }
}

function authorizationToken(request) {
  const authorization = request.headers().authorization || ''
  return authorization.startsWith('Bearer ') ? authorization.slice(7) : ''
}

function shelfBook(id, title, owner) {
  return {
    id,
    title,
    author: owner,
    sourceId: 0,
    url: `local://${owner}/${id}`,
    bookUrl: `local://${owner}/${id}`,
    chapterCount: 1,
    categoryIds: [],
    updatedAt: '2026-07-28T00:00:00Z',
  }
}

function remoteBook(title, sourceName) {
  return {
    title,
    author: '会话隔离测试',
    sourceId: 11,
    sourceName,
    url: `https://source.example/${encodeURIComponent(title)}`,
    bookUrl: `https://source.example/${encodeURIComponent(title)}`,
    latestChapter: '第一章',
    intro: '旧账号请求不得提交到重新认证后的工作台。',
  }
}

function previewItem(path, title = 'A 延迟导入书籍') {
  return {
    path,
    importToken: 'a'.repeat(48),
    book: {
      title,
      author: '用户 A',
      chapterCount: 1,
      chapters: [{ index: 0, title: '第一章' }],
    },
  }
}

function webdavListing(name) {
  return [
    '<multistatus>',
    '<response><propstat><prop><displayname></displayname><iscollection>true</iscollection><getcontentlength>0</getcontentlength><lastmodified></lastmodified></prop></propstat></response>',
    `<response><propstat><prop><displayname>${name}</displayname><iscollection>false</iscollection><getcontentlength>1024</getcontentlength><lastmodified>Mon, 28 Jul 2026 00:00:00 GMT</lastmodified></prop></propstat></response>`,
    '</multistatus>',
  ].join('')
}

function createState(scenario) {
  return {
    scenario,
    tokenA: tokenFor(1, 'expired'),
    tokenARenewed: tokenFor(1, 'renewed'),
    tokenB: tokenFor(2, 'renewed'),
    userA: { id: 1, username: 'accounta', role: 'admin' },
    userB: { id: 2, username: 'accountb', role: 'admin' },
    shelfA: shelfBook(1, 'A 账号原有书籍', '用户 A'),
    shelfB: shelfBook(2, 'B 账号干净书架', '用户 B'),
    pending: deferred(),
    pendingStarted: deferred(),
    pendingFinished: deferred(),
    requests: [],
    phase: 'initial',
  }
}

function userForToken(token, state) {
  return token === state.tokenB ? state.userB : state.userA
}

function sourceForToken(token, state) {
  if (token === state.tokenB) {
    return { id: 22, name: 'B 专属书源', baseUrl: 'https://b.example', enabled: true }
  }
  if (token === state.tokenARenewed) {
    return { id: 12, name: 'A 续登书源', baseUrl: 'https://a-renewed.example', enabled: true }
  }
  return { id: 11, name: 'A 原会话书源', baseUrl: 'https://a.example', enabled: true }
}

function rssSourceForToken(token, state) {
  if (token === state.tokenB) {
    return { id: 22, title: 'B 专属 RSS', url: 'https://b.example/feed.xml', enabled: true }
  }
  if (token === state.tokenARenewed) {
    return { id: 12, title: 'A 续登 RSS', url: 'https://a-renewed.example/feed.xml', enabled: true }
  }
  return { id: 11, title: 'A 原会话 RSS', url: 'https://a.example/feed.xml', enabled: true }
}

function managedUserForToken(token, state) {
  if (token === state.tokenB) {
    return { id: 22, username: 'B 专属用户', role: 'user', canEditSources: true, canAccessStore: true, canAccessWebdav: true }
  }
  return { id: 11, username: 'A 原会话用户', role: 'user', canEditSources: true, canAccessStore: true, canAccessWebdav: true }
}

function pendingRequestForScenario(path, method, scenario) {
  const expected = {
    'book-info': ['/books/remote', 'POST'],
    'storage-import': ['/local-store/import', 'POST'],
    'webdav-restore': ['/backup/restore-webdav', 'POST'],
    'source-save': ['/sources', 'POST'],
    'rss-save': ['/rss/sources', 'POST'],
    'user-create': ['/admin/users', 'POST'],
  }[scenario]
  return path === expected[0] && method === expected[1]
}

function pendingResponse(state) {
  if (state.scenario === 'book-info') {
    return {
      id: 101,
      ...remoteBook('A 延迟加入书架', 'A 原会话书源'),
      categoryIds: [],
      chapterCount: 1,
    }
  }
  if (state.scenario === 'storage-import') {
    return {
      imported: [{
        path: 'a-delayed.txt',
        book: shelfBook(102, 'A 延迟导入书籍', '用户 A'),
      }],
    }
  }
  if (state.scenario === 'webdav-restore') {
    return {
      sources: 2,
      books: 3,
      progress: 4,
      categories: 1,
      settings: 1,
      bookmarks: 1,
      replaceRules: 1,
    }
  }
  if (state.scenario === 'source-save') {
    return { id: 101, name: 'A 延迟新增书源', baseUrl: 'https://stale.example', enabled: true }
  }
  if (state.scenario === 'rss-save') {
    return { id: 101, title: 'A 延迟新增 RSS', url: 'https://stale.example/feed.xml', enabled: true }
  }
  return { id: 101, username: 'staleuser', role: 'user' }
}

async function installApiMocks(page, state) {
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/webdav\/.*$/, async route => {
    const request = route.request()
    const token = authorizationToken(request)
    state.requests.push({
      path: new URL(request.url()).pathname,
      method: request.method(),
      token,
      phase: state.phase,
    })
    const name = token === state.tokenB ? 'B-专属备份.zip' : 'A-原会话备份.zip'
    return route.fulfill({
      status: 207,
      contentType: 'application/xml',
      body: webdavListing(name),
    })
  })
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    const token = authorizationToken(request)
    const record = { path, method, token, phase: state.phase, body: requestJSON(request) }
    state.requests.push(record)

    if (path === '/auth/login' && method === 'POST') {
      const payload = requestJSON(request)
      const differentAccount = payload.username === state.userB.username
      return route.fulfill(json({
        token: differentAccount ? state.tokenB : state.tokenARenewed,
        user: differentAccount ? state.userB : state.userA,
      }))
    }
    if (path === '/me') return route.fulfill(json(userForToken(token, state)))
    if (path === '/health') {
      return route.fulfill(json({ version: 'overlay-session-smoke', commit: 'overlay-session-smoke' }))
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

    if (pendingRequestForScenario(path, method, state.scenario) && token === state.tokenA) {
      state.pendingStarted.resolve()
      await state.pending.promise
      try {
        await route.fulfill(json(pendingResponse(state)))
      } finally {
        state.pendingFinished.resolve()
      }
      return
    }

    if (path === '/books' && method === 'GET') {
      return route.fulfill(json(token === state.tokenB ? [state.shelfB] : [state.shelfA]))
    }
    if (path === '/categories' || path === '/book-groups') return route.fulfill(json([]))
    if (path === '/sources/default') {
      return route.fulfill(json({ configured: false, count: 0, savedAt: '' }))
    }
    if (path === '/sources' && method === 'GET') {
      return route.fulfill(json([sourceForToken(token, state)]))
    }
    if (path === '/explore/sources') return route.fulfill(json([]))
    if (path === '/search' && method === 'POST') {
      const source = sourceForToken(token, state)
      const title = token === state.tokenARenewed ? 'A 续登搜索新数据' : 'A 原会话待加入书籍'
      return route.fulfill(json({
        list: [remoteBook(title, source.name)],
        page: 1,
        lastIndex: 0,
        hasMore: false,
      }))
    }
    if (path === '/books/remote' && method === 'POST') {
      return route.fulfill(json({
        id: 202,
        ...remoteBook('非旧会话加入书架', sourceForToken(token, state).name),
        categoryIds: [],
        chapterCount: 1,
      }))
    }
    if (path === '/cache/stats') {
      return route.fulfill(json({ files: 0, size: 0, cachedChapters: 0 }))
    }
    if (path === '/backup/list') return route.fulfill(json([]))
    if (path === '/local-store' && method === 'GET') {
      const isB = token === state.tokenB
      return route.fulfill(json({
        path: '',
        items: [{
          name: isB ? 'B-专属书籍.txt' : 'a-delayed.txt',
          path: isB ? 'B-专属书籍.txt' : 'a-delayed.txt',
          extension: '.txt',
          size: 128,
          isDir: false,
          importable: true,
        }],
      }))
    }
    if (path === '/local-store/import-preview' && method === 'POST') {
      return route.fulfill(json({ items: [previewItem('a-delayed.txt')] }))
    }
    if (path === '/local-store/import' && method === 'POST') {
      return route.fulfill(json({ imported: [] }))
    }
    if (path === '/backup/restore-webdav' && method === 'POST') {
      return route.fulfill(json({ books: 0, sources: 0, progress: 0 }))
    }
    if (path === '/rss/sources' && method === 'GET') {
      return route.fulfill(json([rssSourceForToken(token, state)]))
    }
    if (path === '/rss/sources' && method === 'POST') {
      return route.fulfill(json({ id: 202, ...requestJSON(request) }))
    }
    if (path === '/rss/articles') {
      return route.fulfill(json({ items: [], page: 1, hasMore: false }))
    }
    if (path === '/admin/users' && method === 'GET') {
      return route.fulfill(json([
        userForToken(token, state),
        managedUserForToken(token, state),
      ]))
    }
    if (path === '/admin/users' && method === 'POST') {
      return route.fulfill(json({ id: 202, ...requestJSON(request), role: 'user' }))
    }
    return route.fulfill(json({}))
  })
}

async function openSession(browser, viewport, scenario) {
  const state = createState(scenario)
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 750,
    hasTouch: viewport.width <= 750,
  })
  await context.addInitScript(({ token, events }) => {
    if (!localStorage.getItem('openreader_token')) {
      localStorage.setItem('openreader_token', token)
    }
    window.__overlaySessionEventCounts = Object.fromEntries(events.map(name => [name, 0]))
    for (const name of events) {
      window.addEventListener(name, () => {
        window.__overlaySessionEventCounts[name] += 1
      })
    }
  }, { token: state.tokenA, events: restoreEventNames })
  const page = await context.newPage()
  const errors = []
  page.on('pageerror', error => errors.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (
      message.type() === 'error'
      && !/WebSocket connection to .*\/ws\/sync/.test(message.text())
    ) {
      errors.push(`console.error: ${message.text()}`)
    }
  })
  await installApiMocks(page, state)
  return { context, errors, page, state, viewport }
}

async function openSidebar(page, viewport) {
  if (viewport.width > 750) return
  const sidebar = page.locator('.app-sidebar')
  const margin = await sidebar.evaluate(element => Number.parseFloat(getComputedStyle(element).marginLeft))
  if (Math.abs(margin) < 0.5) return
  await page.locator('.mobile-menu-trigger').click()
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
  if (viewport.width <= 750) {
    const workspace = page.locator('.app-workspace')
    const box = await workspace.boundingBox()
    assert(box, `${viewport.width}: workspace geometry missing while closing search sidebar`)
    await page.mouse.click(box.x + box.width - 8, box.y + Math.min(box.height / 2, viewport.height / 2))
    await page.waitForFunction(() => {
      const node = document.querySelector('.app-sidebar')
      return node && Number.parseFloat(getComputedStyle(node).marginLeft) <= -259.5
    })
  }
}

async function waitForMessagesToClose(page) {
  await page.waitForFunction(() => (
    [...document.querySelectorAll('.el-message')]
      .every(element => !element.isConnected || getComputedStyle(element).display === 'none')
  ), null, { timeout: 6_000 })
}

async function invalidateSession(session) {
  const { page, state, viewport } = session
  state.phase = 'invalidating'
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
  assert(await page.locator('.app-shell').count() === 0, `${viewport.width} ${state.scenario}: invalidated workspace remained mounted`)
  await assertPrivateOverlaysClosed(page, viewport, state.scenario, 'blocked')
}

async function submitLogin(session, account) {
  const { page, state } = session
  const user = account === 'same' ? state.userA : state.userB
  await page.getByPlaceholder('请输入用户名').fill(user.username)
  await page.getByPlaceholder('请输入密码').fill('password123')
  await page.locator('.auth-dialog button[type="submit"]').click()
  await page.locator('.auth-dialog').waitFor({ state: 'hidden', timeout: 10_000 })
  await page.locator('.app-shell').waitFor({ state: 'visible', timeout: 10_000 })
  state.phase = 'renewed'
}

async function assertPrivateOverlaysClosed(page, viewport, scenario, phase) {
  for (const selector of privateOverlaySelectors) {
    const visible = await page.locator(`${selector}:visible`).count()
    assert(visible === 0, `${viewport.width} ${scenario}: ${selector} remained visible after ${phase}`)
  }
  const sourceDrawer = page.locator('.el-drawer:visible').filter({ hasText: '新增书源' })
  assert(await sourceDrawer.count() === 0, `${viewport.width} ${scenario}: source editor survived ${phase}`)
}

async function assertNoStaleToast(page, viewport, scenario) {
  const forbidden = {
    'book-info': ['加入书架成功', 'A 原会话待加入书籍'],
    'storage-import': ['导入 1 本', 'A 延迟导入书籍'],
    'webdav-restore': ['恢复完成'],
    'source-save': ['书源已新增'],
    'rss-save': ['RSS 源已创建'],
    'user-create': ['新增用户成功'],
  }[scenario]
  const texts = await page.locator('.el-message').allTextContents()
  for (const fragment of forbidden) {
    assert(
      !texts.some(text => text.includes(fragment)),
      `${viewport.width} ${scenario}: stale toast leaked (${texts.join(' | ')})`,
    )
  }
}

async function assertNoHorizontalOverflow(page, viewport, scenario) {
  const geometry = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    innerWidth: window.innerWidth,
  }))
  assert(
    geometry.scrollWidth <= geometry.innerWidth + 1,
    `${viewport.width} ${scenario}: horizontal overflow ${geometry.scrollWidth} > ${geometry.innerWidth}`,
  )
}

function relevantReadCount(state) {
  const predicate = {
    'book-info': row => row.path === '/sources' && row.method === 'GET',
    'storage-import': row => row.path === '/local-store' && row.method === 'GET',
    'webdav-restore': row => row.path.startsWith('/webdav/') && row.method === 'GET',
    'source-save': row => row.path === '/sources' && row.method === 'GET',
    'rss-save': row => row.path === '/rss/sources' && row.method === 'GET',
    'user-create': row => row.path === '/admin/users' && row.method === 'GET',
  }[state.scenario]
  return state.requests.filter(predicate).length
}

async function beginBookInfoOperation(session) {
  const { page, state, viewport } = session
  await page.goto(targetUrl, { waitUntil: 'networkidle' })
  await page.getByText(state.shelfA.title, { exact: true }).waitFor({ state: 'visible', timeout: 10_000 })
  await runSidebarSearch(page, viewport, '会话隔离')
  const title = 'A 原会话待加入书籍'
  const row = page.locator('.result-card').filter({ has: page.getByText(title, { exact: true }) })
  await row.waitFor({ state: 'visible', timeout: 10_000 })
  await row.locator('.book-cover-shared').click()
  const dialog = page.locator('.book-info-dialog')
  await dialog.waitFor({ state: 'visible', timeout: 10_000 })
  await dialog.getByRole('button', { name: '加入书架', exact: true }).click()
}

async function beginStorageImportOperation(session) {
  const { page } = session
  await page.goto(`${targetUrl}/local-store?overlaySession=1`, { waitUntil: 'networkidle' })
  const dialog = page.locator('.global-local-store-dialog')
  await dialog.waitFor({ state: 'visible', timeout: 10_000 })
  await dialog.getByRole('button', { name: '加入书架', exact: true }).first().click()
  const confirm = page.locator('.storage-import-single-dialog')
  await confirm.waitFor({ state: 'visible', timeout: 10_000 })
  await confirm.getByRole('button', { name: '确定导入', exact: true }).click()
}

async function beginWebDAVRestoreOperation(session) {
  const { page } = session
  await page.goto(`${targetUrl}/settings?panel=webdav&overlaySession=1`, { waitUntil: 'networkidle' })
  const dialog = page.locator('.global-webdav-dialog')
  await dialog.waitFor({ state: 'visible', timeout: 10_000 })
  await dialog.getByRole('button', { name: '恢复', exact: true }).click()
  await page.getByRole('dialog', { name: '恢复 WebDAV 备份' })
    .getByRole('button', { name: '确定', exact: true })
    .click()
}

async function beginSourceSaveOperation(session) {
  const { page } = session
  await page.goto(`${targetUrl}/sources?overlaySession=1`, { waitUntil: 'networkidle' })
  const dialog = page.locator('.global-source-manage-dialog')
  await dialog.waitFor({ state: 'visible', timeout: 10_000 })
  await dialog.getByRole('button', { name: '新增', exact: true }).click()
  const drawer = page.locator('.el-drawer:visible').filter({ hasText: '新增书源' })
  await drawer.waitFor({ state: 'visible', timeout: 10_000 })
  await drawer.locator('.el-form-item').filter({ hasText: '名称' }).first().locator('input').fill('A 延迟新增书源')
  await drawer.getByRole('button', { name: '保存', exact: true }).click()
}

async function beginRSSSaveOperation(session) {
  const { errors, page } = session
  await page.goto(`${targetUrl}/settings?panel=rss&overlaySession=1`, { waitUntil: 'networkidle' })
  const dialog = page.locator('.global-rss-dialog')
  await dialog.waitFor({ state: 'visible', timeout: 10_000 })
  await dialog.getByRole('button', { name: '新增', exact: true }).click()
  const editor = page.locator('.rss-source-editor-dialog')
  try {
    await editor.waitFor({ state: 'visible', timeout: 10_000 })
  } catch (error) {
    const visibleDialogs = await page.locator('.el-dialog:visible').evaluateAll(elements => (
      elements.map(element => ({
        ariaLabel: element.getAttribute('aria-label'),
        title: element.querySelector('.el-dialog__title')?.textContent?.trim() || '',
        text: element.textContent?.trim().slice(0, 300) || '',
      }))
    ))
    const addButtons = await dialog.getByRole('button', { name: '新增', exact: true }).evaluateAll(elements => (
      elements.map(element => ({
        disabled: element.disabled,
        text: element.textContent?.trim() || '',
      }))
    ))
    const editorStates = await editor.evaluateAll(elements => (
      elements.map(element => ({
        display: getComputedStyle(element).display,
        visibility: getComputedStyle(element).visibility,
        title: element.querySelector('.el-dialog__title')?.textContent?.trim() || '',
      }))
    ))
    throw new Error(`${error.message}\nRSS visible dialogs: ${JSON.stringify(visibleDialogs)}\nRSS add buttons: ${JSON.stringify(addButtons)}\nRSS editor states: ${JSON.stringify(editorStates)}\nRSS browser errors: ${JSON.stringify(errors)}`)
  }
  const inputs = editor.locator('input')
  await inputs.nth(0).fill('A 延迟新增 RSS')
  await inputs.nth(1).fill('https://stale.example/feed.xml')
  await editor.getByRole('button', { name: '保存', exact: true }).click()
}

async function beginUserCreateOperation(session) {
  const { page } = session
  await page.goto(`${targetUrl}/settings?panel=admin&overlaySession=1`, { waitUntil: 'networkidle' })
  const dialog = page.locator('.global-user-dialog')
  await dialog.waitFor({ state: 'visible', timeout: 10_000 })
  await dialog.getByRole('button', { name: '新增', exact: true }).click()
  const editor = page.getByRole('dialog', { name: '新增用户' })
  await editor.waitFor({ state: 'visible', timeout: 10_000 })
  const inputs = editor.locator('input')
  await inputs.nth(0).fill('staleuser')
  await inputs.nth(1).fill('password123')
  await editor.getByRole('button', { name: '确定', exact: true }).click()
}

async function beginScenarioOperation(session) {
  const begin = {
    'book-info': beginBookInfoOperation,
    'storage-import': beginStorageImportOperation,
    'webdav-restore': beginWebDAVRestoreOperation,
    'source-save': beginSourceSaveOperation,
    'rss-save': beginRSSSaveOperation,
    'user-create': beginUserCreateOperation,
  }[session.state.scenario]
  await begin(session)
  await session.state.pendingStarted.promise
  await waitForMessagesToClose(session.page)
}

async function waitForRenewedWorkspace(session, account) {
  const { page, state } = session
  if (account === 'same') {
    await page.getByText('A 续登搜索新数据', { exact: true })
      .waitFor({ state: 'visible', timeout: 10_000 })
    return
  }
  await page.getByText(state.shelfB.title, { exact: true })
    .waitFor({ state: 'visible', timeout: 10_000 })
}

async function reopenCurrentOverlay(session) {
  const { page, state, viewport } = session
  state.phase = 'manual-reopen'
  if (state.scenario === 'book-info') {
    const row = page.locator('.result-card').filter({
      has: page.getByText('A 续登搜索新数据', { exact: true }),
    })
    await row.locator('.book-cover-shared').click()
    const dialog = page.locator('.book-info-dialog')
    await dialog.waitFor({ state: 'visible', timeout: 10_000 })
    assert(
      await dialog.getByText('A 续登搜索新数据', { exact: true }).count() === 1,
      `${viewport.width}: reopened BookInfo did not use renewed-account data`,
    )
    return
  }

  const reopenContract = {
    'storage-import': {
      route: '/local-store?overlaySession=reopen',
      selector: '.global-local-store-dialog',
      expected: 'B-专属书籍.txt',
      forbidden: ['a-delayed.txt', 'A 延迟导入书籍'],
    },
    'webdav-restore': {
      route: '/settings?panel=webdav&overlaySession=reopen',
      selector: '.global-webdav-dialog',
      expected: 'B-专属备份.zip',
      forbidden: ['A-原会话备份.zip'],
    },
    'source-save': {
      route: '/sources?overlaySession=reopen',
      selector: '.global-source-manage-dialog',
      expected: 'B 专属书源',
      forbidden: ['A 原会话书源', 'A 延迟新增书源'],
    },
    'rss-save': {
      route: '/settings?panel=rss&overlaySession=reopen',
      selector: '.global-rss-dialog',
      expected: 'B 专属 RSS',
      forbidden: ['A 原会话 RSS', 'A 延迟新增 RSS'],
    },
    'user-create': {
      route: '/settings?panel=admin&overlaySession=reopen',
      selector: '.global-user-dialog',
      expected: 'B 专属用户',
      forbidden: ['A 原会话用户', 'staleuser'],
    },
  }[state.scenario]
  await page.goto(`${targetUrl}${reopenContract.route}`, { waitUntil: 'networkidle' })
  const dialog = page.locator(reopenContract.selector)
  await dialog.waitFor({ state: 'visible', timeout: 10_000 })
  try {
    const expectedRows = dialog.getByText(reopenContract.expected, { exact: true })
    const visibleExpectedRows = expectedRows.filter({ visible: true })
    await visibleExpectedRows.first()
      .waitFor({ state: 'visible', timeout: 10_000 })
    assert(
      await visibleExpectedRows.count() >= 1,
      `${viewport.width} ${state.scenario}: visible current-account row was missing`,
    )
    for (const staleText of reopenContract.forbidden) {
      assert(
        await dialog.getByText(staleText, { exact: true }).count() === 0,
        `${viewport.width} ${state.scenario}: stale account data remained visible (${staleText})`,
      )
    }
  } catch (error) {
    const text = await dialog.innerText().catch(() => '')
    const requests = state.requests.filter(row => (
      row.phase === 'manual-reopen'
      || row.path === '/local-store'
      || row.path.startsWith('/webdav/')
      || row.path === '/rss/sources'
      || row.path === '/admin/users'
      || row.path === '/sources'
    ))
    throw new Error(
      `${error.message}\n${viewport.width} ${state.scenario} reopen text: ${text}\nrequests: ${JSON.stringify(requests)}`,
    )
  }
}

async function assertScenarioIsolation(browser, viewport, scenario) {
  const session = await openSession(browser, viewport, scenario)
  const { context, errors, page, state } = session
  try {
    await beginScenarioOperation(session)
    await invalidateSession(session)
    const account = scenario === 'book-info' ? 'same' : 'different'
    await submitLogin(session, account)
    await waitForRenewedWorkspace(session, account)
    await assertPrivateOverlaysClosed(page, viewport, scenario, 'reauthentication')
    const readCountBeforeOldResponse = relevantReadCount(state)
    const eventCountsBefore = await page.evaluate(() => ({ ...window.__overlaySessionEventCounts }))

    state.pending.resolve()
    await state.pendingFinished.promise
    await page.waitForTimeout(350)

    await assertPrivateOverlaysClosed(page, viewport, scenario, 'old response')
    await assertNoStaleToast(page, viewport, scenario)
    assert(
      relevantReadCount(state) === readCountBeforeOldResponse,
      `${viewport.width} ${scenario}: old response started a write-after-login reload`,
    )
    const eventCountsAfter = await page.evaluate(() => ({ ...window.__overlaySessionEventCounts }))
    assert(
      JSON.stringify(eventCountsAfter) === JSON.stringify(eventCountsBefore),
      `${viewport.width} ${scenario}: old response dispatched business events ${JSON.stringify(eventCountsAfter)}`,
    )
    if (scenario !== 'book-info') {
      assert(
        await page.getByText(state.shelfB.title, { exact: true }).count() === 1,
        `${viewport.width} ${scenario}: B shelf was replaced after old response`,
      )
      assert(
        await page.getByText(state.shelfA.title, { exact: true }).count() === 0,
        `${viewport.width} ${scenario}: A shelf leaked into B session`,
      )
    }
    assert(
      await page.getByText('A 延迟导入书籍', { exact: true }).count() === 0,
      `${viewport.width} ${scenario}: stale imported book became visible`,
    )
    assert(
      await page.getByText('A 延迟加入书架', { exact: true }).count() === 0,
      `${viewport.width} ${scenario}: stale remote book became visible`,
    )

    await reopenCurrentOverlay(session)
    await assertNoHorizontalOverflow(page, viewport, scenario)
    assert(errors.length === 0, `${viewport.width} ${scenario}: ${errors.join('\n')}`)
    return `${scenario}=ok`
  } finally {
    state.pending.resolve()
    await context.close()
  }
}

async function main() {
  const activeViewports = selectedViewports()
  const activeScenarios = selectedScenarios()
  const browser = await openSmokeBrowser()
  try {
    const completed = []
    for (const viewport of activeViewports) {
      const scenarios = []
      for (const scenario of activeScenarios) {
        scenarios.push(await assertScenarioIsolation(browser, viewport, scenario))
      }
      completed.push(`${viewport.width}x${viewport.height}[${scenarios.join(',')}]`)
    }
    console.log(
      `workspace-overlay-session-isolation: ok ${completed.join(' ')} staleToasts=0 staleEvents=0 staleReloads=0 manualReopen=current-account`,
    )
  } finally {
    await browser.close()
  }
}

main().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
