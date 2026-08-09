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

function source(id, name, options = {}) {
  const group = options.group === undefined ? '测试' : options.group
  const usedBookNames = options.usedBookNames || []
  return {
    id,
    name,
    group,
    baseUrl: `https://source-${id}.example`,
    searchUrl: `https://source-${id}.example/search?key={{key}}`,
    charset: 'utf-8',
    enabled: true,
    enabledExplore: true,
    usedBookCount: usedBookNames.length,
    usedBookNames,
    header: options.header || '',
    rules: options.rules || '',
  }
}

async function installApiMocks(page) {
  let sources = [
    source(1, '书架引用源', { usedBookNames: ['引用书一', '引用书二'] }),
    source(2, '可编辑脚本源', { group: '第二组', header: '<js>private-script</js>' }),
    source(3, '未分组源', { group: '' }),
  ]
  let batchTestCalls = 0
  let invalidSourceCalls = 0
  let importCalls = 0

  await page.exposeFunction('__sourceSmokeReplaceSources', () => {
    sources = [source(4, '同步刷新书源', { group: '同步组' })]
  })
  await page.exposeFunction('__sourceSmokeStats', () => ({
    batchTestCalls,
    invalidSourceCalls,
    importCalls,
  }))

  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname.replace(/^\/api/, '')
    const method = request.method()

    if (path === '/me') return route.fulfill(json({ id: 1, username: 'source-smoke', role: 'admin' }))
    if (path === '/health') return route.fulfill(json({ version: 'smoke', commit: 'source-manager-second-audit' }))
    if (path === '/settings/reader' && method === 'GET') return route.fulfill(json({ key: 'reader', value: { theme: 'parchment', mode: 'page', pageMode: 'auto' } }))
    if (path === '/settings/reader' && method === 'PUT') return route.fulfill(json({ key: 'reader', value: {} }))
    if (path.startsWith('/settings/')) return route.fulfill(json({ key: path.split('/').at(-1), value: {} }))
    if (path === '/books') return route.fulfill(json([]))
    if (path === '/categories' || path === '/book-groups') return route.fulfill(json([]))
    if (path === '/sources' && method === 'GET') return route.fulfill(json(sources))
    if (path === '/sources' && method === 'POST') {
      const payload = request.postDataJSON()
      const created = source(Math.max(0, ...sources.map(item => item.id)) + 1, payload.name, { group: payload.group })
      sources.push({ ...created, ...payload })
      return route.fulfill(json(created, 201))
    }
    if (/^\/sources\/\d+$/.test(path) && method === 'GET') {
      const id = Number(path.split('/').at(-1))
      const item = sources.find(row => row.id === id)
      return route.fulfill(item ? json(item) : json({ error: 'source not found' }, 404))
    }
    if (/^\/sources\/\d+$/.test(path) && method === 'PUT') {
      const id = Number(path.split('/').at(-1))
      const payload = request.postDataJSON()
      const index = sources.findIndex(row => row.id === id)
      if (index < 0) return route.fulfill(json({ error: 'source not found' }, 404))
      sources[index] = { ...sources[index], ...payload, id }
      return route.fulfill(json(sources[index]))
    }
    if (path === '/sources/invalid' && method === 'GET') {
      invalidSourceCalls += 1
      const item = sources.find(row => row.id === 2) || sources[0]
      return route.fulfill(json([{
        ...item,
        errorMessage: 'timeout',
        failedAt: '2026-08-09T00:00:00Z',
        expiresAt: '2026-08-09T00:10:00Z',
      }]))
    }
    if (path === '/sources/batch-test') {
      batchTestCalls += 1
      const payload = request.postDataJSON()
      return route.fulfill(json({
        results: (payload.sourceIds || []).map(id => ({
          sourceId: id,
          name: sources.find(row => row.id === id)?.name || `源 ${id}`,
          group: sources.find(row => row.id === id)?.group || '',
          enabled: true,
          ok: id !== 2,
          count: id === 2 ? 0 : 1,
          message: id === 2 ? 'timeout' : '可用',
        })),
      }))
    }
    if (path === '/sources/remote-preview') {
      return route.fulfill(json({ sources: [source(20, '远程预览书源')] }))
    }
    if (path === '/sources/import') {
      importCalls += 1
      sources.push(source(21 + importCalls, '已导入书源'))
      return route.fulfill(json({ imported: 1, updated: 0, skipped: 0 }))
    }
    if (path === '/sources/batch') {
      const payload = request.postDataJSON()
      if (payload.action === 'delete') {
        const ids = new Set(payload.sourceIds || [])
        sources = sources.filter(item => !ids.has(item.id) || item.usedBookCount)
      }
      return route.fulfill(json({ affected: payload.sourceIds?.length || 0, skippedUsed: 0 }))
    }
    if (path === '/sources/default/restore') return route.fulfill(json({ imported: 0, updated: 0 }))
    if (path === '/sources/export') return route.fulfill(json(sources))
    if (path === '/sources' && method === 'DELETE') {
      sources = []
      return route.fulfill(json({ affected: 3 }))
    }
    if (path.startsWith('/cache')) return route.fulfill(json({ total: 0, books: 0, chapters: 0 }))
    return route.fulfill(json({}))
  })
}

async function assertNoHorizontalOverflow(page, name) {
  const geometry = await page.evaluate(() => ({
    width: document.documentElement.scrollWidth,
    viewport: innerWidth,
  }))
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

async function assertManagerGeometry(page, viewport) {
  const dialog = page.locator('.global-source-manage-dialog')
  const geometry = await dialog.evaluate(node => {
    const rect = node.getBoundingClientRect()
    return {
      x: Math.round(rect.x),
      width: Math.round(rect.width),
      fullscreen: node.classList.contains('is-fullscreen'),
    }
  })
  if (viewport.width <= 750) {
    assert(geometry.fullscreen, `${viewport.width}: source manager must be fullscreen`)
    assert(Math.abs(geometry.width - viewport.width) <= 1, `${viewport.width}: fullscreen width ${geometry.width}`)
    assert(Math.abs(geometry.x) <= 1, `${viewport.width}: fullscreen x ${geometry.x}`)
    return
  }
  const expected = Math.min(Math.max(viewport.width * 0.7, 750), 1000)
  assert(!geometry.fullscreen, `${viewport.width}: desktop/iPad manager must not be fullscreen`)
  assert(Math.abs(geometry.width - expected) <= 1, `${viewport.width}: manager width ${geometry.width}, want ${expected}`)
}

async function runRemoteImport(page, root, viewport) {
  await page.goto(`${root}/sources?panel=remote&keep=source-contract`, { waitUntil: 'networkidle' })
  await assertOverlayRoute(page, 'remote', 'source-contract')
  const prompt = page.locator('.el-message-box').filter({ hasText: '导入远程书源文件' })
  await prompt.waitFor({ state: 'visible', timeout: 10000 })
  assert(await page.locator('.global-source-manage-dialog').count() === 0, `${viewport.width}: remote import must not show the manager behind its prompt`)
  await prompt.locator('input').fill('https://remote.example/sources.json')
  await prompt.getByRole('button', { name: '确定' }).click()
  const preview = page.locator('.source-import-preview-dialog')
  await preview.getByText('远程预览书源', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  assert(await preview.locator('input[type="checkbox"]:checked').count() === 0, `${viewport.width}: remote preview must start with no selection`)
  await preview.getByRole('button', { name: '取消' }).click()
  await page.waitForFunction(() => new URLSearchParams(location.search).get('overlay') !== 'sources')
}

async function runManager(page, root, viewport) {
  await page.goto(root, { waitUntil: 'networkidle' })
  await openMobileNavigation(page, viewport)
  await page.getByRole('button', { name: '书源管理' }).click()
  const manager = page.locator('.global-source-manage-dialog')
  await manager.getByText('书架引用源', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  await assertManagerGeometry(page, viewport)

  assert(await manager.locator('.el-table').count() === 1, `${viewport.width}: manager must own one table`)
  assert(await manager.locator('.mobile-source-card, .source-batch-footer, .el-drawer').count() === 0, `${viewport.width}: old card/drawer structure must be absent`)
  const titleActions = await manager.locator('.source-title-actions .el-button').allTextContents()
  assert(JSON.stringify(titleActions.map(text => text.trim())) === JSON.stringify(['新增', '导出', '恢复默认', '清空']), `${viewport.width}: title actions ${JSON.stringify(titleActions)}`)
  for (const heading of ['书源名称', '书源链接', '书架书籍', '操作']) {
    await manager.getByRole('columnheader', { name: heading }).waitFor({ state: 'visible' })
  }
  const usedBooks = manager.locator('.source-used-books').filter({ hasText: '引用书一' })
  await usedBooks.waitFor({ state: 'visible' })
  const usedRow = manager.locator('tbody tr').filter({ hasText: '书架引用源' })
  assert(await usedRow.locator('input[type="checkbox"]').isDisabled(), `${viewport.width}: used source selection must be disabled`)
  const chips = await manager.locator('.source-group-btn').allTextContents()
  assert(JSON.stringify(chips.map(text => text.trim())) === JSON.stringify(['测试', '第二组', '未分组']), `${viewport.width}: group order ${JSON.stringify(chips)}`)

  if (viewport.width <= 750) {
    const margin = await page.locator('.app-sidebar').evaluate(node => Number.parseFloat(getComputedStyle(node).marginLeft))
    assert(Math.abs(margin) < 0.5, `${viewport.width}: source manager must not implicitly close the mobile sidebar`)
    await usedBooks.click()
    const afterClick = await page.locator('.app-sidebar').evaluate(node => Number.parseFloat(getComputedStyle(node).marginLeft))
    assert(Math.abs(afterClick) < 0.5, `${viewport.width}: manager clicks must not leak through to the sidebar`)
  }

  const editRow = manager.locator('tbody tr').filter({ hasText: '可编辑脚本源' })
  await editRow.getByRole('button', { name: '编辑' }).click()
  const editor = page.locator('.source-json-editor-dialog')
  await editor.waitFor({ state: 'visible', timeout: 10000 })
  const editorValue = await editor.locator('textarea').inputValue()
  assert(editorValue.includes('"bookSourceName": "可编辑脚本源"'), `${viewport.width}: editor must expose reader-dev JSON`)
  await editor.locator('.source-json-compatibility-warning').waitFor({ state: 'visible' })
  await editor.locator('textarea').fill('{bad json')
  await editor.getByRole('button', { name: '保 存' }).click()
  await page.getByText('书源必须是JSON格式', { exact: true }).waitFor({ state: 'visible' })
  await editor.locator('.el-dialog__headerbtn').click()

  await page.evaluate(async () => {
    await window.__sourceSmokeReplaceSources()
    window.dispatchEvent(new CustomEvent('openreader:sources-update', { detail: { kind: 'import' } }))
  })
  await manager.getByText('同步刷新书源', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  await assertNoHorizontalOverflow(page, `${viewport.width} manager`)
  await manager.locator('.el-dialog__headerbtn').click()
}

async function runLocalImport(page, root, viewport) {
  await page.goto(root, { waitUntil: 'networkidle' })
  await openMobileNavigation(page, viewport)
  const chooserPromise = page.waitForEvent('filechooser')
  await page.getByRole('button', { name: '导入书源' }).click()
  const chooser = await chooserPromise
  await chooser.setFiles({
    name: 'bookSources.json',
    mimeType: 'application/json',
    buffer: Buffer.from(JSON.stringify([
      { bookSourceName: '安全导入源', bookSourceUrl: 'https://safe-import.example' },
      { bookSourceName: '脚本导入源', bookSourceUrl: 'https://script-import.example', header: '<js>private</js>' },
    ])),
  })
  const preview = page.locator('.source-import-preview-dialog')
  await preview.getByText('安全导入源', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  assert(await page.locator('.global-source-manage-dialog').count() === 0, `${viewport.width}: local import must not show the manager behind preview`)
  assert(await preview.locator('input[type="checkbox"]:checked').count() === 0, `${viewport.width}: local preview must start empty`)
  await preview.getByText('全选', { exact: true }).click()
  await preview.getByText('已选择 1 个', { exact: true }).waitFor({ state: 'visible' })
  assert(await preview.locator('.source-import-item').filter({ hasText: '脚本导入源' }).locator('input[type="checkbox"]').isChecked() === false, `${viewport.width}: executable source must not be selected by safe select-all`)
  await preview.getByRole('button', { name: '确定' }).click()
  await preview.waitFor({ state: 'hidden', timeout: 10000 })
  const stats = await page.evaluate(() => window.__sourceSmokeStats())
  assert(stats.importCalls === 1, `${viewport.width}: confirmed preview must import once`)
}

async function runFailureManager(page, root, viewport) {
  await page.goto(`${root}/sources?action=health`, { waitUntil: 'networkidle' })
  await assertOverlayRoute(page, 'health')
  const manager = page.locator('.global-source-manage-dialog')
  await manager.getByText('失效书源管理', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
  await manager.getByText('timeout', { exact: true }).first().waitFor({ state: 'visible' })
  for (const label of ['搜索词：', '超时(ms)：', '并发数：']) {
    await manager.getByText(label, { exact: true }).waitFor({ state: 'visible' })
  }
  const before = await page.evaluate(() => window.__sourceSmokeStats())
  assert(before.invalidSourceCalls === 1, `${viewport.width}: failure entry must read its cache once, got ${before.invalidSourceCalls}`)
  assert(before.batchTestCalls === 0, `${viewport.width}: failure entry must not start a live test`)
  await manager.getByRole('button', { name: /检测书源/ }).click()
  await page.waitForFunction(() => window.__sourceSmokeStats().then(stats => stats.batchTestCalls === 1))
  await manager.getByText(/\d+\/\d+/).waitFor({ state: 'visible' })
  await assertManagerGeometry(page, viewport)
  await assertNoHorizontalOverflow(page, `${viewport.width} failure-manager`)
  await manager.locator('.el-dialog__headerbtn').click()
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 750,
    hasTouch: viewport.width <= 750,
    acceptDownloads: true,
  })
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error' && !/WebSocket connection to .*\/ws\/sync/.test(message.text())) {
      failures.push(`console.error: ${message.text()}`)
    }
  })
  await page.addInitScript(token => localStorage.setItem('openreader_token', token), fakeToken())
  await installApiMocks(page)
  const root = targetUrl.replace(/\/$/, '')

  await runRemoteImport(page, root, viewport)
  await page.goto(root, { waitUntil: 'networkidle' })
  await waitForShelfWorkspace(page, failures)
  await runManager(page, root, viewport)
  await runLocalImport(page, root, viewport)
  await runFailureManager(page, root, viewport)

  assert(failures.length === 0, failures.join('\n'))
  await context.close()
  return `${viewport.width}x${viewport.height}`
}

async function run() {
  const browser = await openSmokeBrowser()
  try {
    const checks = []
    checks.push(await runViewport(browser, { width: 1440, height: 900 }))
    checks.push(await runViewport(browser, { width: 1024, height: 1366 }))
    checks.push(await runViewport(browser, { width: 390, height: 844 }))
    checks.push(await runViewport(browser, { width: 360, height: 800 }))
    console.log(`source-workspace: ok ${checks.join(', ')} singleTable=true jsonEditor=true importPreview=true failureCache=true`)
  } finally {
    await browser.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
