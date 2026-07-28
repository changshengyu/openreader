#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function assertClose(actual, expected, tolerance, message) {
  if (Math.abs(actual - expected) > tolerance) {
    throw new Error(`${message}: expected ${expected}±${tolerance}, got ${actual}`)
  }
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

async function installApiMocks(page) {
  let nextRuleId = 2
  let updatePayloads = []
  let importedRows = []
  let rules = [{
    id: 1,
    name: '  空白规则  ',
    group: '隐藏分组',
    pattern: ' ad ',
    replacement: '旧替换',
    scope: '*',
    isRegex: false,
    isEnabled: true,
    enabled: true,
    order: 7,
  }]

  await page.exposeFunction('__replaceRuleSmokeState', () => ({
    rules: rules.map(rule => ({ ...rule })),
    updatePayloads: updatePayloads.map(row => ({ ...row })),
    importedRows: importedRows.map(row => ({ ...row })),
  }))
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname.replace(/^\/api/, '')
    const method = request.method()

    if (path === '/me') return route.fulfill(json({ id: 1, username: 'replace-smoke', role: 'admin' }))
    if (path === '/health') return route.fulfill(json({ version: 'smoke', commit: 'replace-rule-fixed-baseline' }))
    if (path === '/books') return route.fulfill(json([]))
    if (path === '/categories') return route.fulfill(json([]))
    if (path === '/sources') return route.fulfill(json([]))
    if (path === '/cache/stats') return route.fulfill(json({ files: 0, size: 0, cachedChapters: 0 }))
    if (path === '/rss/sources') return route.fulfill(json([]))
    if (path === '/admin/users') return route.fulfill(json([]))
    if (path === '/replace-rules' && method === 'GET') {
      return route.fulfill(json(rules))
    }
    if (/^\/replace-rules\/\d+$/.test(path) && method === 'PUT') {
      const id = Number(path.split('/').at(-1))
      const payload = request.postDataJSON()
      updatePayloads.push({ id, ...payload })
      rules = rules.map(rule => (
        rule.id === id
          ? {
              ...rule,
              ...payload,
              isEnabled: payload.enabled !== false,
              enabled: payload.enabled !== false,
            }
          : rule
      ))
      return route.fulfill(json(rules.find(rule => rule.id === id)))
    }
    if (path === '/replace-rules/batch-delete' && method === 'POST') {
      const ids = request.postDataJSON()?.ids || []
      const deletedIds = ids.filter(id => rules.some(rule => rule.id === id))
      rules = rules.filter(rule => !deletedIds.includes(rule.id))
      return route.fulfill(json({ deletedIds }))
    }
    if (path === '/replace-rules/batch' && method === 'POST') {
      importedRows = request.postDataJSON()
      const accepted = importedRows.filter(row => row.name !== '' && row.pattern !== '')
      rules = accepted.map(row => ({
        id: nextRuleId++,
        ...row,
        isEnabled: row.enabled !== false,
        enabled: row.enabled !== false,
      }))
      return route.fulfill(json({
        rules,
        created: accepted.length,
        updated: 0,
        skipped: importedRows.length - accepted.length,
      }))
    }
    if (path.startsWith('/settings/') && method === 'GET') {
      const key = path.split('/').at(-1)
      return route.fulfill(json({ key, value: {}, updatedAt: '2026-07-28T00:00:00Z' }))
    }
    return route.fulfill(json({}))
  })
}

async function waitForRuleState(page, expected) {
  await page.waitForFunction(async target => {
    const state = await window.__replaceRuleSmokeState()
    return Object.entries(target).every(([key, length]) => (
      Array.isArray(state[key]) && state[key].length === length
    ))
  }, expected)
}

async function assertManagerGeometry(page, viewport) {
  const state = await page.locator('.global-replace-dialog').evaluate(node => {
    const dialog = node.getBoundingClientRect()
    const table = node.querySelector('.el-table')?.getBoundingClientRect()
    return {
      dialog: {
        left: dialog.left,
        top: dialog.top,
        width: dialog.width,
        height: dialog.height,
      },
      tableHeight: table?.height || 0,
      viewport: { width: innerWidth, height: innerHeight },
      scrollWidth: document.documentElement.scrollWidth,
    }
  })

  assert(state.scrollWidth <= viewport.width + 1, `${viewport.width}: manager caused horizontal overflow`)
  if (viewport.width <= 750) {
    assertClose(state.dialog.left, 0, 1, `${viewport.width}: fullscreen manager left`)
    assertClose(state.dialog.top, 0, 1, `${viewport.width}: fullscreen manager top`)
    assertClose(state.dialog.width, viewport.width, 1, `${viewport.width}: fullscreen manager width`)
    assertClose(state.dialog.height, viewport.height, 1, `${viewport.width}: fullscreen manager height`)
    assertClose(state.tableHeight, viewport.height - 184, 2, `${viewport.width}: upstream mobile table height`)
    return
  }

  const expectedWidth = Math.min(1000, Math.max(750, viewport.width * 0.7))
  const expectedTop = Math.max(viewport.height * 0.15, (viewport.height - 584) / 2)
  const expectedTableHeight = Math.min(400, viewport.height * 0.7 - 184)
  assertClose(state.dialog.width, expectedWidth, 1, `${viewport.width}: shared upstream dialog width`)
  assertClose(state.dialog.left, (viewport.width - expectedWidth) / 2, 1, `${viewport.width}: centered manager`)
  assertClose(state.dialog.top, expectedTop, 1, `${viewport.width}: shared upstream dialog top`)
  assertClose(state.tableHeight, expectedTableHeight, 2, `${viewport.width}: upstream desktop table height`)
}

async function verifyViewport(browser, viewport) {
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
  page.on('response', response => {
    if (response.status() >= 500 && /\/api\//.test(response.url())) {
      failures.push(`api ${response.status()}: ${response.url()}`)
    }
  })

  await page.addInitScript(token => localStorage.setItem('openreader_token', token), fakeToken())
  await installApiMocks(page)
  const root = targetUrl.replace(/\/$/, '')
  await page.goto(`${root}/settings?panel=replace&keep=fixed-baseline`, { waitUntil: 'networkidle' })

  const manager = page.locator('.global-replace-dialog')
  await manager.waitFor({ state: 'visible', timeout: 10000 })
  await manager.getByText('替换规则管理', { exact: true }).first().waitFor({ state: 'visible' })
  const headers = await manager.locator('.el-table__header th').allTextContents()
  assert(
    JSON.stringify(headers.map(value => value.trim()).filter(Boolean)) ===
      JSON.stringify(['规则名称', '替换范围', '是否启用', '操作']),
    `${viewport.width}: unexpected manager columns ${JSON.stringify(headers)}`,
  )
  for (const forbidden of ['新增规则', '刷新', '测试规则']) {
    assert(await manager.getByText(forbidden, { exact: true }).count() === 0, `${viewport.width}: manager exposed ${forbidden}`)
  }
  assert(await manager.getByRole('button', { name: '删除', exact: true }).count() === 0, `${viewport.width}: manager exposed row delete`)
  await assertManagerGeometry(page, viewport)

  await manager.getByRole('button', { name: '编辑', exact: true }).click()
  const editor = page.locator('.replace-rule-editor-dialog')
  await editor.waitFor({ state: 'visible', timeout: 10000 })
  await page.waitForTimeout(350)
  assert(await manager.isVisible(), `${viewport.width}: manager must remain under its sibling editor`)
  await editor.getByText('替换规则', { exact: true }).first().waitFor({ state: 'visible' })
  const inputs = editor.locator('.el-form-item input')
  assert(await inputs.count() === 4, `${viewport.width}: editor must expose exactly four upstream fields`)
  assert(await inputs.nth(0).inputValue() === '  空白规则  ', `${viewport.width}: editor trimmed rule name`)
  assert(await inputs.nth(1).inputValue() === ' ad ', `${viewport.width}: editor trimmed pattern`)
  assert(await inputs.nth(2).inputValue() === '旧替换', `${viewport.width}: editor replacement mismatch`)
  assert(await inputs.nth(3).inputValue() === '*', `${viewport.width}: editor scope mismatch`)
  assert(await editor.getByText('使用正则表达式', { exact: true }).count() === 1, `${viewport.width}: regex checkbox missing`)
  assert(await editor.getByText('是否启用', { exact: true }).count() === 1, `${viewport.width}: enabled checkbox missing`)
  assert(await editor.getByText('测试规则', { exact: true }).count() === 0, `${viewport.width}: editor exposed retired test UI`)

  const editorRect = await editor.evaluate(node => {
    const rect = node.getBoundingClientRect()
    return { top: rect.top, width: rect.width, height: rect.height }
  })
  if (viewport.width <= 750) {
    assertClose(editorRect.top, 0, 1, `${viewport.width}: editor fullscreen top`)
    assertClose(editorRect.width, viewport.width, 1, `${viewport.width}: editor fullscreen width`)
    assertClose(editorRect.height, viewport.height, 1, `${viewport.width}: editor fullscreen height`)
  } else {
    const expectedWidth = Math.min(1000, Math.max(750, viewport.width * 0.7))
    assertClose(editorRect.width, expectedWidth, 1, `${viewport.width}: editor shared width`)
  }

  await inputs.nth(2).fill('[$&]')
  await editor.getByRole('button', { name: '确 定', exact: true }).click()
  await editor.waitFor({ state: 'hidden', timeout: 10000 })
  assert(await manager.isVisible(), `${viewport.width}: saving the editor closed its manager`)
  await waitForRuleState(page, { updatePayloads: 1 })
  let state = await page.evaluate(() => window.__replaceRuleSmokeState())
  assert(state.updatePayloads[0].name === '  空白规则  ', `${viewport.width}: save trimmed name`)
  assert(state.updatePayloads[0].pattern === ' ad ', `${viewport.width}: save trimmed pattern`)
  assert(state.updatePayloads[0].group === '隐藏分组', `${viewport.width}: save dropped hidden group`)
  assert(state.updatePayloads[0].order === 7, `${viewport.width}: save dropped hidden order`)

  await manager.locator('.el-switch').click()
  await waitForRuleState(page, { updatePayloads: 2 })
  state = await page.evaluate(() => window.__replaceRuleSmokeState())
  assert(state.updatePayloads[1].enabled === false, `${viewport.width}: toggle did not persist disabled state`)
  assert(state.updatePayloads[1].group === '隐藏分组' && state.updatePayloads[1].order === 7, `${viewport.width}: toggle dropped hidden fields`)

  await manager.locator('.el-table__body .el-checkbox').first().click()
  await manager.getByText('已选择 1 个', { exact: true }).waitFor({ state: 'visible' })
  await manager.getByRole('button', { name: '批量删除', exact: true }).click()
  await page.getByText('确认要删除所选择的替换规则吗?', { exact: true }).waitFor({ state: 'visible' })
  await page.locator('.el-message-box').last().getByRole('button', { name: '确定', exact: true }).click()
  await waitForRuleState(page, { rules: 0 })
  await manager.getByText('暂无数据', { exact: true }).waitFor({ state: 'visible' })

  const imported = [
    { name: '导入规则', pattern: ' keep spaces ', replacement: '', scope: '*', isRegex: false, enabled: true },
    { name: '', pattern: 'invalid-but-counted', replacement: '', scope: '*', isRegex: false, enabled: true },
  ]
  await manager.locator('input[type="file"]').setInputFiles({
    name: 'replace-rules.json',
    mimeType: 'application/json',
    buffer: Buffer.from(JSON.stringify(imported)),
  })
  await page.getByText('确认要导入文件中的2条替换规则吗?', { exact: true }).waitFor({ state: 'visible' })
  await page.locator('.el-message-box').last().getByRole('button', { name: '确定', exact: true }).click()
  await waitForRuleState(page, { importedRows: 2, rules: 1 })
  await manager.getByText('导入规则', { exact: true }).waitFor({ state: 'visible' })
  state = await page.evaluate(() => window.__replaceRuleSmokeState())
  assert(state.importedRows[0].pattern === ' keep spaces ', `${viewport.width}: import trimmed pattern`)
  assert(state.importedRows[1].name === '', `${viewport.width}: import fabricated a missing name`)

  await manager.getByRole('button', { name: '编辑', exact: true }).click()
  await editor.waitFor({ state: 'visible', timeout: 10000 })
  assert(await editor.locator('.el-form-item input').nth(1).inputValue() === ' keep spaces ', `${viewport.width}: imported pattern changed before edit`)
  await editor.getByRole('button', { name: '取 消', exact: true }).click()
  await editor.waitFor({ state: 'hidden', timeout: 10000 })
  await manager.getByRole('button', { name: '取消', exact: true }).click()
  await manager.waitFor({ state: 'hidden', timeout: 10000 })

  assert(failures.length === 0, failures.join('\n'))
  await context.close()
  return `${viewport.width}x${viewport.height}`
}

async function run() {
  const browser = await openSmokeBrowser()
  try {
    const viewports = [
      { width: 1440, height: 900 },
      { width: 1024, height: 1366 },
      { width: 390, height: 844 },
      { width: 360, height: 800 },
    ]
    const results = []
    for (const viewport of viewports) {
      results.push(await verifyViewport(browser, viewport))
    }
    console.log(`replace-rule-fixed-baseline: ok ${results.join(', ')} manager=true editor=true import=true toggle=true batch=true`)
  } finally {
    await browser.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
