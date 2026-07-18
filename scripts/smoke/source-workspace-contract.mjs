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
  let batchTestCalls = 0
  let invalidSourceCalls = 0
  let debugUnsupported = false
  await page.exposeFunction('__sourceSmokeReplaceSources', names => {
    sources = Array.isArray(names)
      ? names.map((name, index) => source(index + 1, String(name)))
      : sources
  })
  await page.exposeFunction('__sourceSmokeBatchTestCalls', () => batchTestCalls)
  await page.exposeFunction('__sourceSmokeInvalidSourceCalls', () => invalidSourceCalls)
  await page.exposeFunction('__sourceSmokeUseScriptSource', () => {
    sources = [{ ...source(), name: '调试脚本源', header: '<js>private-script</js>' }]
    debugUnsupported = true
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
    if (path === '/sources/invalid' && method === 'GET') {
      invalidSourceCalls += 1
      return route.fulfill(json([{ ...sources[0], errorMessage: '缓存失败', failedAt: '2026-07-12T00:00:00Z', expiresAt: '2026-07-12T00:10:00Z' }]))
    }
    if (path === '/sources/default') return route.fulfill(json({ configured: false, count: 0 }))
    if (path === '/sources/batch-test') {
      batchTestCalls += 1
      return route.fulfill(json({ results: [{ sourceId: 1, name: sources[0].name, group: '测试', enabled: true, ok: false, count: 0, message: '模拟失败' }] }))
    }
    if (path === '/sources/remote-preview') return route.fulfill(json({ count: 1, names: ['远程预览书源'], sources: [source(2, '远程预览书源')] }))
    if (path === '/sources/import') {
      sources = [...sources, { ...source(2, '导入脚本源'), header: '<js>private-header</js>' }]
      return route.fulfill(json({ imported: 1, updated: 0, skipped: 0 }))
    }
    if (path === '/sources/1/test') {
      return route.fulfill(json(debugUnsupported
        ? { error: 'book source rule is unsupported', code: 'source_rule_unsupported', stage: 'search' }
        : { results: [{ title: '调试结果', bookUrl: 'https://source.example/book/1' }], error: '' }))
    }
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
    buffer: Buffer.from(JSON.stringify([
      source(3, '本地预览书源'),
      { ...source(4, '动态头源'), header: '<js>private-header</js>' },
      { ...source(5, '登录检测源'), loginCheckJs: 'return privateLogin' },
      { ...source(6, '保留字段源'), ruleToc: { preUpdateJs: 'return chapterList' } },
      { ...source(7, '改名标志源'), ruleBookInfo: { canReName: '@js:presence-marker' } },
    ])),
  })
  await page.getByText('本地预览书源', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  const previewRows = page.locator('.source-import-item')
  assert(await previewRows.count() === 5, `${viewport.width}: source compatibility preview must keep every row`)
  const previewChecked = async name => previewRows.filter({ hasText: name }).locator('input[type="checkbox"]').isChecked()
  const previewState = await previewRows.evaluateAll(rows => rows.map(row => ({
    text: row.innerText,
    checked: row.querySelector('input[type="checkbox"]')?.checked,
    className: row.className,
  })))
  assert(await previewChecked('本地预览书源'), `${viewport.width}: supported source must be selected by default ${JSON.stringify(previewState)}`)
  assert(!(await previewChecked('动态头源')), `${viewport.width}: dynamic header source must not be selected by default`)
  assert(!(await previewChecked('登录检测源')), `${viewport.width}: login-check source must not be selected by default`)
  assert(await previewChecked('保留字段源'), `${viewport.width}: fixed-baseline dormant fields must not block import`)
  assert(await previewChecked('改名标志源'), `${viewport.width}: canReName presence marker must not be treated as executable script`)
  await previewRows.filter({ hasText: '动态头源' }).getByText('@Javascript', { exact: true }).waitFor({ state: 'visible' })
  await previewRows.filter({ hasText: '登录检测源' }).getByText('登录检测依赖 JavaScript', { exact: false }).waitFor({ state: 'visible' })
  await previewRows.filter({ hasText: '保留字段源' }).getByText('仅无损保存', { exact: false }).waitFor({ state: 'visible' })
  await previewRows.filter({ hasText: '动态头源' }).locator('.el-checkbox__inner').click()
  await page.getByText('已选择 4 / 5 个', { exact: true }).waitFor({ state: 'visible' })
  await page.getByRole('button', { name: '确定导入' }).click()
  await page.locator('.source-import-preview').waitFor({ state: 'hidden', timeout: 10000 })
  await assertNoHorizontalOverflow(page, `${viewport.width} import-preview`)

  const importedSourceRow = viewport.width <= 750
    ? page.locator('.mobile-source-card').filter({ hasText: '导入脚本源' })
    : page.locator('.desktop-source-table tbody tr').filter({ hasText: '导入脚本源' })
  await importedSourceRow.getByRole('button', { name: '编辑' }).click()
  const editorWarning = page.locator('.source-compatibility-warning').filter({ hasText: '当前服务不会执行' })
  await editorWarning.waitFor({ state: 'visible', timeout: 10000 })
  assert((await editorWarning.innerText()).includes('配置会保留'), `${viewport.width}: editor must explain lossless preservation`)
  await page.keyboard.press('Escape')
  await editorWarning.waitFor({ state: 'hidden', timeout: 10000 })

  await page.goto(`${root}/sources?action=health`, { waitUntil: 'networkidle' })
  await assertOverlayRoute(page, 'health')
  const cachedFailureMessage = viewport.width <= 750
    ? page.locator('.mobile-source-card').getByText('缓存失败', { exact: true })
    : page.locator('.desktop-source-table').getByText('缓存失败', { exact: true })
  await cachedFailureMessage.waitFor({ state: 'visible', timeout: 10000 })
  assert(await page.evaluate(() => window.__sourceSmokeInvalidSourceCalls()) === 1, `${viewport.width}: health intent must load the cached invalid-source list once`)
  assert(await page.evaluate(() => window.__sourceSmokeBatchTestCalls()) === 0, `${viewport.width}: health intent must not start a live batch test`)
  const manualHealthCommand = viewport.width <= 750
    ? page.locator('.source-batch-footer').getByRole('button', { name: '检测书源' })
    : page.getByRole('button', { name: '失效检测' })
  await manualHealthCommand.scrollIntoViewIfNeeded()
  await manualHealthCommand.click()
  await page.getByText('已检 1 · 可用 0 · 失败 1').waitFor({ state: 'visible', timeout: 10000 })
  assert(await page.evaluate(() => window.__sourceSmokeBatchTestCalls()) === 1, `${viewport.width}: explicit health command must start one live batch test`)

  await page.evaluate(() => window.__sourceSmokeUseScriptSource())
  await page.goto(`${root}/sources?action=debug`, { waitUntil: 'networkidle' })
  await page.getByText('书源调试', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  await assertOverlayRoute(page, 'debug')
  await assertNoHorizontalOverflow(page, `${viewport.width} debug`)
  const debugDialog = page.getByRole('dialog', { name: '书源调试' })
  await debugDialog.getByPlaceholder('搜索关键词').fill('脚本')
  await debugDialog.getByRole('button', { name: '测试搜索' }).click()
  const debugWarning = debugDialog.locator('.debug-compatibility-warning')
  await debugWarning.getByText('当前服务不会执行此书源在搜索阶段需要的 JavaScript 或 WebView', { exact: false }).waitFor({ state: 'visible', timeout: 10000 })
  await debugDialog.getByText('source_rule_unsupported', { exact: false }).waitFor({ state: 'visible' })
  assert(!(await debugDialog.innerText()).includes('private-script'), `${viewport.width}: debug must not echo the configured script`)

  await debugDialog.getByRole('button', { name: '关闭此对话框' }).click()
  await page.getByRole('dialog', { name: '书源调试' }).waitFor({ state: 'hidden', timeout: 10000 })
  await page.locator('.global-source-manage-dialog .el-dialog__headerbtn').first().click()
  await page.waitForFunction(() => new URLSearchParams(location.search).get('overlay') !== 'sources')
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
    console.log(`source-workspace: ok ${checks.join(', ')} legacyRoutes=true previewImport=true health=true debug=true`)
  } finally {
    await browser.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
