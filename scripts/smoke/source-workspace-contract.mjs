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
      console.error('Playwright is required for the source workspace smoke.')
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

function source(id = 1, name = '工作台书源') {
  return {
    id,
    name,
    group: '测试',
    baseUrl: `https://source-${id}.example`,
    searchUrl: `https://source-${id}.example/search?key={{key}}`,
    charset: 'utf-8',
    enabled: true,
    enabledExplore: true,
    usedBookCount: 0,
    rules: '',
  }
}

async function installApiMocks(page) {
  let sources = [source()]
  await page.exposeFunction('__sourceSmokeReplaceSources', names => {
    sources = Array.isArray(names)
      ? names.map((name, index) => source(index + 1, String(name)))
      : sources
  })
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname.replace(/^\/api/, '')
    const method = request.method()

    if (path === '/me') return route.fulfill(json({ id: 1, username: 'source-smoke', role: 'admin' }))
    if (path === '/health') return route.fulfill(json({ version: 'smoke', commit: 'source-workspace' }))
    if (path === '/settings/reader' && method === 'GET') return route.fulfill(json({ key: 'reader', value: { theme: 'parchment', mode: 'page', pageMode: 'auto' } }))
    if (path === '/settings/reader' && method === 'PUT') return route.fulfill(json({ key: 'reader', value: {} }))
    if (path === '/settings/preferences') return route.fulfill(json({ key: 'preferences', value: {} }))
    if (path === '/books') return route.fulfill(json([]))
    if (path === '/categories') return route.fulfill(json([]))
    if (path === '/sources' && method === 'GET') return route.fulfill(json(sources))
    if (path === '/sources/default') return route.fulfill(json({ configured: false, count: 0 }))
    if (path === '/sources/batch-test') {
      return route.fulfill(json({ results: [{ sourceId: 1, name: sources[0].name, group: '测试', enabled: true, ok: false, count: 0, message: '模拟失败' }] }))
    }
    if (path === '/sources/remote-preview') return route.fulfill(json({ count: 1, names: ['远程预览书源'], sources: [source(2, '远程预览书源')] }))
    if (path === '/sources/import') {
      sources = [...sources, source(2, '导入书源')]
      return route.fulfill(json({ imported: 1, updated: 0, skipped: 0 }))
    }
    if (path === '/sources/1/test') return route.fulfill(json({ results: [{ title: '调试结果', bookUrl: 'https://source.example/book/1' }], error: '' }))
    if (path === '/sources/1/test-chapter') return route.fulfill(json({ chapters: [{ title: '第一章', url: 'https://source.example/chapter/1' }], count: 1, error: '' }))
    if (path === '/sources/1/test-content') return route.fulfill(json({ content: '调试正文', fullLength: 4, error: '' }))
    if (path.startsWith('/cache')) return route.fulfill(json({ total: 0, books: 0, chapters: 0 }))
    return route.fulfill(json({}))
  })
}

async function assertNoHorizontalOverflow(page, name) {
  const geometry = await page.evaluate(() => ({ width: document.documentElement.scrollWidth, viewport: innerWidth }))
  assert(geometry.width <= geometry.viewport + 1, `${name}: horizontal overflow ${geometry.width} > ${geometry.viewport}`)
}

async function openMobileNavigation(page, viewport) {
  if (viewport.width > 750) return
  await page.locator('.mobile-menu-trigger').click()
  await page.waitForFunction(() => {
    const sidebar = document.querySelector('.app-sidebar')
    return sidebar && Math.abs(Number.parseFloat(getComputedStyle(sidebar).marginLeft)) < 0.5
  })
}

async function assertOverlayRoute(page, expectedAction, keep = '') {
  const state = await page.evaluate(() => ({
    path: location.pathname,
    overlay: new URLSearchParams(location.search).get('overlay'),
    sourceAction: new URLSearchParams(location.search).get('sourceAction'),
    keep: new URLSearchParams(location.search).get('keep'),
  }))
  assert(state.path === '/', 'legacy source URL must redirect to the root workspace')
  assert(state.overlay === 'sources', 'root query must retain overlay=sources')
  assert(state.sourceAction === expectedAction, `expected source action ${expectedAction}, got ${state.sourceAction}`)
  if (keep) assert(state.keep === keep, 'unrelated legacy query must survive the redirect')
}

async function waitForShelfWorkspace(page, failures) {
  try {
    await page.waitForSelector('.shelf-page', { timeout: 10000 })
  } catch (error) {
    const state = await page.evaluate(() => ({
      url: location.href,
      workspace: !!document.querySelector('.app-workspace'),
      text: document.body.innerText.slice(0, 800),
    }))
    throw new Error(`root workspace did not render its shelf: ${JSON.stringify(state)}\n${failures.join('\n')}\n${error.message}`)
  }
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
  await page.goto(`${root}/sources?panel=remote&keep=source-contract`, { waitUntil: 'networkidle' })
  await page.waitForSelector('.global-source-manage-dialog', { timeout: 10000 })
  await page.getByPlaceholder('输入书源 JSON 订阅地址').waitFor({ state: 'visible', timeout: 10000 })
  await assertOverlayRoute(page, 'remote', 'source-contract')
  await assertNoHorizontalOverflow(page, `${viewport.width} remote`)

  await page.goto(root, { waitUntil: 'networkidle' })
  await waitForShelfWorkspace(page, failures)
  await openMobileNavigation(page, viewport)
  await page.getByRole('button', { name: '书源管理' }).click()
  await page.waitForSelector('.global-source-manage-dialog', { timeout: 10000 })
  const visibleSourceList = viewport.width <= 750
    ? page.locator('.global-source-manage-dialog .mobile-source-card')
    : page.locator('.global-source-manage-dialog .source-table')
  await visibleSourceList.getByText('工作台书源', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  if (viewport.width <= 750) {
    const margin = await page.locator('.app-sidebar').evaluate(node => Number.parseFloat(getComputedStyle(node).marginLeft))
    assert(Math.abs(margin) < 0.5, `${viewport.width}: source manager must not implicitly close the mobile sidebar`)
    await page.locator('.global-source-manage-dialog .mobile-source-card p').click()
    const marginAfterPanelClick = await page.locator('.app-sidebar').evaluate(node => Number.parseFloat(getComputedStyle(node).marginLeft))
    assert(Math.abs(marginAfterPanelClick) < 0.5, `${viewport.width}: a source-manager click must not leak through and close the mobile sidebar`)
  }

  await page.evaluate(async () => {
    await window.__sourceSmokeReplaceSources(['同步刷新书源'])
    window.dispatchEvent(new CustomEvent('openreader:sources-update', { detail: { kind: 'import' } }))
  })
  await visibleSourceList.getByText('同步刷新书源', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })

  const uploadInput = page.locator('.global-source-manage-dialog input[type="file"]')
  await uploadInput.setInputFiles({
    name: 'bookSources.json',
    mimeType: 'application/json',
    buffer: Buffer.from(JSON.stringify([source(3, '本地预览书源')])),
  })
  await page.getByText('本地预览书源', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  await page.getByRole('button', { name: '确定导入' }).click()
  await page.waitForFunction(() => !Array.from(document.querySelectorAll('.el-dialog')).some(node => node.textContent?.includes('导入书源') && getComputedStyle(node).display !== 'none'))
  await assertNoHorizontalOverflow(page, `${viewport.width} import-preview`)

  await page.goto(`${root}/sources?action=health`, { waitUntil: 'networkidle' })
  await page.getByText('已检 1 · 可用 0 · 失败 1').waitFor({ state: 'visible', timeout: 10000 })
  await assertOverlayRoute(page, 'health')

  await page.goto(`${root}/sources?action=debug`, { waitUntil: 'networkidle' })
  await page.getByText('书源调试', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  await assertOverlayRoute(page, 'debug')
  await assertNoHorizontalOverflow(page, `${viewport.width} debug`)

  await page.locator('.global-source-manage-dialog .el-dialog__headerbtn').first().click()
  await page.waitForFunction(() => new URLSearchParams(location.search).get('overlay') !== 'sources')
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
    console.log(`source-workspace: ok ${checks.join(', ')} legacyRoutes=true previewImport=true health=true debug=true`)
  } finally {
    await browser.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
