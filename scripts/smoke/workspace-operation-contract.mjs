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
      console.error('Playwright is required for the workspace-operation smoke.')
      console.error(`Original import error: ${error.message}`)
      process.exit(2)
    }
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

async function installApiMocks(page) {
  let webdavRootRequests = 0
  const savedSettingKeys = []
  await page.exposeFunction('__workspaceOperationWebDAVRootRequests', () => webdavRootRequests)
  await page.exposeFunction('__workspaceOperationSavedSettingKeys', () => [...savedSettingKeys])
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/webdav\/.*$/, async route => {
    const authorization = route.request().headers().authorization || ''
    if (!authorization.startsWith('Bearer ')) {
      return route.fulfill(json({ error: 'missing bearer token' }, 401))
    }
    if (route.request().method() === 'GET') {
      if (new URL(route.request().url()).pathname === '/webdav/') webdavRootRequests += 1
      return route.fulfill({
        status: 207,
        contentType: 'application/xml',
        body: '<multistatus><response><propstat><prop><displayname></displayname><iscollection>true</iscollection><getcontentlength>0</getcontentlength><lastmodified></lastmodified></prop></propstat></response><response><propstat><prop><displayname>comic.cbz</displayname><iscollection>false</iscollection><getcontentlength>128</getcontentlength><lastmodified>Wed, 01 Jan 2025 00:00:00 GMT</lastmodified></prop></propstat></response></multistatus>',
      })
    }
    return route.fulfill({ status: 204, body: '' })
  })
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname.replace(/^\/api/, '')
    const method = request.method()

    if (path === '/me') return route.fulfill(json({ id: 1, username: 'operation-smoke', role: 'admin' }))
    if (path === '/health') return route.fulfill(json({ version: 'smoke', commit: 'workspace-operation' }))
    if (['/settings/reader', '/settings/shelf', '/settings/search'].includes(path) && method === 'GET') {
      const key = path.split('/').at(-1)
      const values = {
        reader: { theme: 'parchment', mode: 'page', pageMode: 'auto' },
        shelf: { view: 'grid', layoutVersion: 2 },
        search: { type: 'all', concurrent: 1 },
      }
      return route.fulfill(json({ key, value: values[key], updatedAt: '2026-07-16T00:00:00Z' }))
    }
    if (['/settings/reader', '/settings/shelf', '/settings/search'].includes(path) && method === 'PUT') {
      const key = path.split('/').at(-1)
      savedSettingKeys.push(key)
      const body = request.postDataJSON?.() || {}
      return route.fulfill(json({ key, value: body.value || {}, updatedAt: '2026-07-16T00:00:01Z' }))
    }
    if (path === '/books') return route.fulfill(json([]))
    if (path === '/categories') return route.fulfill(json([]))
    if (path === '/sources') return route.fulfill(json([]))
    if (path === '/cache/stats') return route.fulfill(json({ files: 0, size: 0, cachedChapters: 0 }))
    if (path === '/local-store') return route.fulfill(json({ path: '', items: [{ name: 'comic.cbz', path: 'comic.cbz', extension: '.cbz', size: 128, isDir: false, importable: true }] }))
    if (path === '/backup/list') return route.fulfill(json([]))
    if (path === '/webdav/list') return route.fulfill(json({ path: '', items: [] }))
    if (path === '/rss/sources') return route.fulfill(json([]))
    if (path === '/replace-rules') return route.fulfill(json([]))
    if (path === '/admin/users') return route.fulfill(json([]))
    return route.fulfill(json({}))
  })
}

async function assertNoHorizontalOverflow(page, label) {
  const geometry = await page.evaluate(() => ({ scrollWidth: document.documentElement.scrollWidth, width: innerWidth }))
  assert(geometry.scrollWidth <= geometry.width + 1, `${label}: horizontal overflow ${geometry.scrollWidth} > ${geometry.width}`)
}

async function assertRouteIntent(page, expectedOverlay, keep = 'operation-contract') {
  const state = await page.evaluate(() => ({
    pathname: location.pathname,
    overlay: new URLSearchParams(location.search).get('overlay'),
    keep: new URLSearchParams(location.search).get('keep'),
  }))
  assert(state.pathname === '/', `legacy operation URL must redirect to root, got ${state.pathname}`)
  assert(state.overlay === expectedOverlay, `expected overlay=${expectedOverlay}, got ${state.overlay}`)
  assert(state.keep === keep, 'legacy redirect must preserve unrelated query values')
}

async function assertRootCompatibilityState(page, keep = 'operation-contract') {
  const state = await page.evaluate(() => ({
    pathname: location.pathname,
    overlay: new URLSearchParams(location.search).get('overlay'),
    workspaceFocus: new URLSearchParams(location.search).get('workspaceFocus'),
    workspaceNotice: new URLSearchParams(location.search).get('workspaceNotice'),
    keep: new URLSearchParams(location.search).get('keep'),
  }))
  assert(state.pathname === '/', `legacy Settings URL must redirect to root, got ${state.pathname}`)
  assert(!state.overlay, `sidebar/Reader compatibility intent must not open an overlay, got ${state.overlay}`)
  assert(!state.workspaceFocus && !state.workspaceNotice, 'one-shot sidebar/Reader compatibility intent must be consumed')
  assert(state.keep === keep, 'legacy redirect must preserve unrelated query values')
}

async function waitForVisibleExactText(page, selector, text) {
  await page.waitForFunction(({ selector: rootSelector, expectedText }) => {
    const root = document.querySelector(rootSelector)
    if (!root) return false
    return [...root.querySelectorAll('*')].some(node => {
      if (node.children.length || node.textContent?.trim() !== expectedText) return false
      const style = getComputedStyle(node)
      const rect = node.getBoundingClientRect()
      return style.visibility !== 'hidden' && style.display !== 'none' && rect.width > 0 && rect.height > 0
    })
  }, { selector, expectedText: text })
}

async function closeDialog(page, selector, expectedOverlay) {
  await page.locator(`${selector} .el-dialog__headerbtn`).click()
  await page.waitForFunction((overlay) => new URLSearchParams(location.search).get('overlay') !== overlay, expectedOverlay)
}

async function assertMobilePanelBlocksClickThrough(page, viewport, selector) {
  if (viewport.width > 750) return
  const pointerEvents = await page.locator(selector).evaluate(node => {
    const overlay = node.closest('.el-overlay')
    return overlay ? getComputedStyle(overlay).pointerEvents : 'none'
  })
  assert(pointerEvents !== 'none', `${viewport.width}: operation panel overlay must block pointer events from reaching the workspace`)
}

async function openLegacyOperation(page, root, viewport, path, selector, overlay, title, usesUpstreamDialog = false, expectedFileName = '') {
  await page.goto(`${root}${path}`, { waitUntil: 'networkidle' })
  await page.waitForFunction((expectedOverlay) => new URLSearchParams(location.search).get('overlay') === expectedOverlay, overlay)
  await page.waitForSelector(selector, { timeout: 10000 })
  await page.locator(selector).getByText(title, { exact: true }).first().waitFor({ state: 'visible', timeout: 10000 })
  if (expectedFileName) {
    await waitForVisibleExactText(page, selector, expectedFileName)
  }
  await assertRouteIntent(page, overlay)
  await assertNoHorizontalOverflow(page, `${viewport.width} ${overlay}`)
  if (usesUpstreamDialog) {
    const geometry = await page.locator(selector).evaluate(node => {
      const rect = node.getBoundingClientRect()
      return { left: rect.left, top: rect.top, width: rect.width, height: rect.height, viewportWidth: innerWidth, viewportHeight: innerHeight }
    })
    if (viewport.width <= 750) {
      assert(Math.abs(geometry.left) <= 1 && Math.abs(geometry.top) <= 1, `${viewport.width}: ${overlay} must start at the mobile viewport origin`)
      assert(Math.abs(geometry.width - geometry.viewportWidth) <= 1 && Math.abs(geometry.height - geometry.viewportHeight) <= 1, `${viewport.width}: ${overlay} must be fullscreen`)
    } else {
      assert(Math.abs(geometry.left - (geometry.viewportWidth - geometry.width) / 2) <= 1, `${viewport.width}: ${overlay} must be centered like the upstream dialog`)
    }
  }
  await assertMobilePanelBlocksClickThrough(page, viewport, selector)
  if (usesUpstreamDialog) {
    await closeDialog(page, selector, overlay)
  }
}

async function openLegacySidebarFocus(page, root, viewport, panel, section) {
  await page.goto(`${root}/settings?panel=${panel}&keep=operation-contract`, { waitUntil: 'networkidle' })
  const selector = `[data-sidebar-section="${section}"]`
  await page.waitForSelector(`${selector}.is-compat-focused`, { timeout: 10000 })
  await assertRootCompatibilityState(page)
  await assertNoHorizontalOverflow(page, `${viewport.width} legacy-${panel}`)
  const hasDrawer = await page.locator('.global-workspace-settings-drawer').count()
  assert(hasDrawer === 0, `${viewport.width}: ${panel} legacy link must not recreate a generic settings drawer`)
  if (viewport.width <= 750) {
    const shellClass = await page.locator('.app-shell').getAttribute('class')
    assert(shellClass?.includes('mobile-nav-open'), `${viewport.width}: sidebar focus must open compact navigation`)
    await page.locator('.app-workspace').click({ position: { x: 200, y: 200 } })
    await page.waitForFunction(() => !document.querySelector('.app-shell')?.classList.contains('mobile-nav-open'))
  }
}

async function openLegacyReaderSettingsNotice(page, root, viewport) {
  await page.goto(`${root}/settings?panel=reader&keep=operation-contract`, { waitUntil: 'networkidle' })
  await page.getByText('阅读设置已迁移至书籍阅读页，请打开书籍后使用阅读器中的设置。', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  await assertRootCompatibilityState(page)
  await assertNoHorizontalOverflow(page, `${viewport.width} legacy-reader`)
  const hasDrawer = await page.locator('.global-workspace-settings-drawer').count()
  assert(hasDrawer === 0, `${viewport.width}: reader legacy link must not recreate a generic settings drawer`)
}

async function verifyUserConfigurationActions(page) {
  await page.getByText('备份用户配置', { exact: true }).click()
  await page.getByRole('button', { name: '确定' }).last().click()
  await page.waitForFunction(async () => {
    const keys = await window.__workspaceOperationSavedSettingKeys()
    return ['reader', 'shelf', 'search'].every(key => keys.includes(key))
  })
  const keys = await page.evaluate(() => window.__workspaceOperationSavedSettingKeys())
  assert(['reader', 'shelf', 'search'].every(key => keys.includes(key)), 'configuration backup must flush reader, shelf, and search settings')

  await page.getByText('同步用户配置', { exact: true }).click()
  await page.getByRole('button', { name: '确定' }).last().click()
  await page.getByText('用户配置已同步', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
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
  await openLegacyOperation(page, root, viewport, '/local-store?keep=operation-contract', '.global-local-store-dialog', 'local-store', '书仓文件管理', true, 'comic.cbz')
  await openLegacyOperation(page, root, viewport, '/settings?panel=backup&keep=operation-contract', '.global-backup-dialog', 'backup', '备份恢复', true)
  await openLegacyOperation(page, root, viewport, '/settings?panel=webdav&keep=operation-contract', '.global-webdav-dialog', 'webdav', 'WebDAV文件管理', true, 'comic.cbz')
  const firstWebDAVRootRequestCount = await page.evaluate(() => window.__workspaceOperationWebDAVRootRequests())
  await page.goto(`${root}/settings?panel=webdav&keep=operation-contract`, { waitUntil: 'networkidle' })
  await page.waitForSelector('.global-webdav-dialog', { timeout: 10000 })
  const secondWebDAVRootRequestCount = await page.evaluate(() => window.__workspaceOperationWebDAVRootRequests())
  assert(secondWebDAVRootRequestCount === firstWebDAVRootRequestCount + 1, `${viewport.width}: reopening WebDAV must reload its root directory`)
  await closeDialog(page, '.global-webdav-dialog', 'webdav')
  await openLegacyOperation(page, root, viewport, '/settings?panel=replace&keep=operation-contract', '.global-replace-dialog', 'replace-rules', '替换规则', true)
  await openLegacyOperation(page, root, viewport, '/settings?panel=rss&keep=operation-contract', '.global-rss-dialog', 'rss', 'RSS 订阅', true)
  await openLegacyOperation(page, root, viewport, '/settings?panel=admin&keep=operation-contract', '.global-user-dialog', 'user-manage', '用户管理', true)
  await openLegacySidebarFocus(page, root, viewport, 'account', 'account')
  await openLegacySidebarFocus(page, root, viewport, 'cache', 'cache')
  await openLegacyReaderSettingsNotice(page, root, viewport)
  await verifyUserConfigurationActions(page)

  assert(failures.length === 0, failures.join('\n'))
  await context.close()
  return `${viewport.width}x${viewport.height}`
}

async function run() {
  const { chromium } = await loadPlaywright()
  const browser = await chromium.launch({ headless: true, executablePath: process.env.CHROME_PATH || defaultChromePath })
  try {
    const checks = []
    checks.push(await runViewport(browser, { width: 1440, height: 900 }))
    checks.push(await runViewport(browser, { width: 390, height: 844 }))
    checks.push(await runViewport(browser, { width: 360, height: 800 }))
    console.log(`workspace-operation: ok ${checks.join(', ')} legacyRoutes=true sharedBodies=true mobileSidebar=true`)
  } finally {
    await browser.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
