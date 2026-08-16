#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetURL = (process.env.TARGET_URL || 'http://127.0.0.1:8080').replace(/\/$/, '')
const runID = Date.now()

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function multipartValue(body, field) {
  const match = body.toString('utf8').match(new RegExp(`name="${field}"\\r\\n\\r\\n([^\\r]*)`))
  return match?.[1] || ''
}

function multipartValues(body, field) {
  return [...body.toString('utf8').matchAll(new RegExp(`name="${field}"\\r\\n\\r\\n([^\\r]*)`, 'g'))]
    .map(match => match[1])
}

function multipartFilename(body) {
  return body.toString('utf8').match(/name="file"; filename="([^"]+)"/)?.[1] || ''
}

async function register(page, viewport) {
  const username = `direct${viewport.width}${runID}`
  const registration = await page.request.post(`${targetURL}/api/auth/register`, {
    data: { username, password: 'direct-import-password' },
  })
  assert(registration.ok(), `registration failed: ${registration.status()} ${await registration.text()}`)
  const { token } = await registration.json()
  assert(token, 'registration did not return a token')
  await page.evaluate(value => localStorage.setItem('openreader_token', value), token)

  const category = await page.request.post(`${targetURL}/api/categories`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { name: '导入组' },
  })
  assert(category.ok(), `category creation failed: ${category.status()} ${await category.text()}`)
  await page.reload({ waitUntil: 'networkidle' })
}

async function openImport(page) {
  const action = page.getByText('导入书籍', { exact: true })
  const mobileTrigger = page.getByLabel('打开侧边栏', { exact: true })
  const inViewport = async () => {
    const box = await action.boundingBox()
    const viewport = page.viewportSize()
    return Boolean(box && viewport && box.x < viewport.width && box.x + box.width > 0)
  }
  if (!await inViewport() && await mobileTrigger.isVisible()) {
    await mobileTrigger.click()
    await action.waitFor({ state: 'visible' })
    const actionElement = await action.elementHandle()
    await page.waitForFunction(element => {
      const rect = element?.getBoundingClientRect()
      return Boolean(rect && rect.left < innerWidth && rect.right > 0)
    }, actionElement)
    await action.scrollIntoViewIfNeeded()
  }
  assert(await inViewport(), 'import action is outside the viewport')
  await action.click()
  await page.locator('.direct-import-picker-dialog').waitFor()
}

async function setDirectFiles(page, files) {
  await page.locator('.direct-import-picker-dialog input[type="file"]').setInputFiles(files)
  await page.locator('.direct-import-picker-dialog').waitFor({ state: 'hidden' })
}

async function selectCategory(page, dialogSelector) {
  const dialog = page.locator(dialogSelector)
  await dialog.locator('.storage-import-single-form > .el-select').first().click()
  await page.getByText('导入组', { exact: true }).last().click()
  await page.keyboard.press('Escape')
}

async function assertFullscreen(page, selector, label) {
  const geometry = await page.locator(selector).evaluate(node => {
    const rect = node.getBoundingClientRect()
    return {
      left: rect.left,
      top: rect.top,
      width: rect.width,
      height: rect.height,
      viewportWidth: innerWidth,
      viewportHeight: innerHeight,
    }
  })
  assert(Math.abs(geometry.left) <= 1 && Math.abs(geometry.top) <= 1, `${label}: dialog is not viewport anchored: ${JSON.stringify(geometry)}`)
  assert(
    Math.abs(geometry.width - geometry.viewportWidth) <= 1 && Math.abs(geometry.height - geometry.viewportHeight) <= 1,
    `${label}: dialog is not fullscreen: ${JSON.stringify(geometry)}`,
  )
}

async function assertNoHorizontalOverflow(page, label) {
  const geometry = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    viewportWidth: innerWidth,
  }))
  assert(geometry.scrollWidth <= geometry.viewportWidth + 1, `${label}: horizontal overflow ${geometry.scrollWidth} > ${geometry.viewportWidth}`)
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 750,
    hasTouch: viewport.width <= 750,
  })
  const page = await context.newPage()
  const failures = []
  const previews = []
  const imports = []
  const activePreviews = new Map()
  const maxActivePreviews = new Map()
  let phase = ''
  let firstRaceSeenResolve
  let releaseFirstRaceResolve
  const firstRaceSeen = new Promise(resolve => { firstRaceSeenResolve = resolve })
  const releaseFirstRace = new Promise(resolve => { releaseFirstRaceResolve = resolve })

  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error' && !/WebSocket connection to .*\/ws\/sync/.test(message.text())) {
      failures.push(`console.error: ${message.text()}`)
    }
  })
  page.on('response', response => {
    if (response.status() >= 500 && response.url().includes('/api/')) {
      failures.push(`api ${response.status()}: ${response.url()}`)
    }
  })

  await page.route('**/api/imports/books/preview', async route => {
    const requestPhase = phase
    const body = route.request().postDataBuffer() || Buffer.alloc(0)
    const filename = multipartFilename(body)
    const active = Number(activePreviews.get(requestPhase) || 0) + 1
    activePreviews.set(requestPhase, active)
    maxActivePreviews.set(requestPhase, Math.max(active, Number(maxActivePreviews.get(requestPhase) || 0)))
    if (requestPhase === 'race-first') {
      firstRaceSeenResolve()
      await releaseFirstRace
      activePreviews.set(requestPhase, active - 1)
      await route.abort().catch(() => {})
      return
    }
    try {
      const response = await route.fetch()
      const data = await response.json().catch(() => ({}))
      previews.push({ phase: requestPhase, filename, token: String(data?.importToken || '') })
      await route.fulfill({ response })
    } catch {
      await route.abort().catch(() => {})
    } finally {
      activePreviews.set(requestPhase, active - 1)
    }
  })

  await page.route('**/api/imports/books', async route => {
    const body = route.request().postDataBuffer() || Buffer.alloc(0)
    imports.push({
      phase,
      token: multipartValue(body, 'importToken'),
      categoryIds: multipartValues(body, 'categoryIds'),
      hasFile: /name="file"; filename=/.test(body.toString('utf8')),
    })
    const response = await route.fetch()
    await route.fulfill({ response })
  })

  try {
    await page.goto(targetURL, { waitUntil: 'networkidle' })
    await register(page, viewport)

    await openImport(page)
    phase = 'race-first'
    await setDirectFiles(page, [{
      name: 'first-race.txt',
      mimeType: 'text/plain',
      buffer: Buffer.from('第一章 旧请求\n旧正文', 'utf8'),
    }])
    await firstRaceSeen
    await openImport(page)
    phase = 'race-second'
    await setDirectFiles(page, [{
      name: 'second-race.txt',
      mimeType: 'text/plain',
      buffer: Buffer.from('第二章 新请求\n新正文', 'utf8'),
    }])
    const raceDialog = page.locator('.storage-import-single-dialog')
    await raceDialog.waitFor()
    assert(await raceDialog.getByPlaceholder('书名').inputValue() === 'second-race', `${viewport.width}: replacement preview did not win`)
    releaseFirstRaceResolve()
    await page.waitForTimeout(150)
    assert(await raceDialog.getByPlaceholder('书名').inputValue() === 'second-race', `${viewport.width}: cancelled preview overwrote the new batch`)
    await raceDialog.getByRole('button', { name: '取消', exact: true }).click()
    await raceDialog.waitFor({ state: 'hidden' })

    await openImport(page)
    phase = 'single'
    await setDirectFiles(page, [{
      name: 'single-numeric.txt',
      mimeType: 'text/plain',
      buffer: Buffer.from('1\n第一段正文。\n2\n第二段正文。', 'utf8'),
    }])
    const singleDialog = page.locator('.storage-import-single-dialog')
    await singleDialog.waitFor()
    if (viewport.width <= 750) await assertFullscreen(page, '.storage-import-single-dialog', `${viewport.width} single`)
    await singleDialog.locator('.storage-import-rule-row .el-select').click()
    await page.getByText('数字(纯数字标题)', { exact: true }).last().click()
    await singleDialog.getByRole('button', { name: '刷新目录', exact: true }).click()
    await singleDialog.getByText('章节列表（2）', { exact: true }).waitFor()
    const singleTitle = `单本导入-${viewport.width}`
    await singleDialog.getByPlaceholder('书名').fill(singleTitle)
    await selectCategory(page, '.storage-import-single-dialog')
    await singleDialog.getByRole('button', { name: '确定导入', exact: true }).click()
    await singleDialog.waitFor({ state: 'hidden' })
    await page.getByText(singleTitle, { exact: true }).first().waitFor()

    await openImport(page)
    phase = 'batch'
    await setDirectFiles(page, [
      { name: 'same.txt', mimeType: 'text/plain', buffer: Buffer.from('第一章\n批量一', 'utf8') },
      { name: 'same.txt', mimeType: 'text/plain', buffer: Buffer.from('第二章\n批量二', 'utf8') },
    ])
    const modeDialog = page.locator('.storage-import-mode-dialog')
    await modeDialog.waitFor()
    if (viewport.width <= 750) await assertFullscreen(page, '.storage-import-mode-dialog', `${viewport.width} mode`)
    await page.keyboard.press('Escape')
    assert(await modeDialog.isVisible(), `${viewport.width}: Escape bypassed the required import mode choice`)
    await modeDialog.getByRole('button', { name: '批量导入', exact: true }).click()
    const groupsDialog = page.locator('.storage-import-groups-dialog')
    await groupsDialog.locator('.el-select').click()
    await page.getByText('导入组', { exact: true }).last().click()
    await page.keyboard.press('Escape')
    await groupsDialog.getByRole('button', { name: '确定', exact: true }).click()
    await groupsDialog.waitFor({ state: 'hidden' })

    const batchPreviews = previews.filter(item => item.phase === 'batch')
    const batchImports = imports.filter(item => item.phase === 'batch')
    assert(batchPreviews.length === 2, `${viewport.width}: batch preview count ${batchPreviews.length}`)
    assert(batchPreviews.every(item => item.filename === 'same.txt'), `${viewport.width}: duplicate filenames changed order`)
    assert(batchPreviews[0].token && batchPreviews[1].token && batchPreviews[0].token !== batchPreviews[1].token, `${viewport.width}: duplicate filenames did not retain distinct tokens`)
    assert(batchImports.length === 2, `${viewport.width}: batch import count ${batchImports.length}`)
    assert(batchImports.map(item => item.token).join(',') === batchPreviews.map(item => item.token).join(','), `${viewport.width}: batch token order changed`)
    assert(batchImports.every(item => item.categoryIds.length === 1), `${viewport.width}: batch group was not applied to every book`)

    await openImport(page)
    phase = 'sequential'
    await setDirectFiles(page, [
      { name: 'sequence-one.txt', mimeType: 'text/plain', buffer: Buffer.from('第一章\n逐本一', 'utf8') },
      { name: 'sequence-two.txt', mimeType: 'text/plain', buffer: Buffer.from('第二章\n逐本二', 'utf8') },
    ])
    await modeDialog.waitFor()
    await modeDialog.getByRole('button', { name: '逐一确认导入', exact: true }).click()
    await singleDialog.getByText('导入本地书籍（1/2）', { exact: true }).waitFor()
    await singleDialog.getByRole('button', { name: '取消', exact: true }).click()
    await singleDialog.getByText('导入本地书籍（2/2）', { exact: true }).waitFor()
    const sequentialTitle = `逐本第二本-${viewport.width}`
    await singleDialog.getByPlaceholder('书名').fill(sequentialTitle)
    await singleDialog.getByRole('button', { name: '确定导入', exact: true }).click()
    await singleDialog.waitFor({ state: 'hidden' })
    await page.getByText(sequentialTitle, { exact: true }).first().waitFor()

    const sequentialPreviews = previews.filter(item => item.phase === 'sequential')
    const sequentialImports = imports.filter(item => item.phase === 'sequential')
    assert(sequentialPreviews.length === 2, `${viewport.width}: sequential preview count ${sequentialPreviews.length}`)
    assert(sequentialImports.length === 1, `${viewport.width}: skipped sequential item was imported`)
    assert(sequentialImports[0].token === sequentialPreviews[1].token, `${viewport.width}: sequential confirmation imported the wrong token`)
    assert(imports.every(item => item.token && !item.hasFile), `${viewport.width}: final confirmation retransmitted a browser File`)
    assert(previews.filter(item => item.phase === 'race-first').length === 0, `${viewport.width}: cancelled preview reached the backend response`)
    assert(Number(maxActivePreviews.get('batch') || 0) === 1, `${viewport.width}: batch preview uploads overlapped`)
    assert(Number(maxActivePreviews.get('sequential') || 0) === 1, `${viewport.width}: sequential preview uploads overlapped`)

    await assertNoHorizontalOverflow(page, `${viewport.width} direct import`)
    assert(failures.length === 0, failures.join('\n'))
    console.log(`${viewport.width}x${viewport.height}: cancellation + single + batch + sequential direct import ok`)
  } finally {
    releaseFirstRaceResolve?.()
    await context.close()
  }
}

const browser = await openSmokeBrowser()
try {
  await runViewport(browser, { width: 1440, height: 900 })
  await runViewport(browser, { width: 390, height: 844 })
  await runViewport(browser, { width: 360, height: 800 })
} finally {
  await browser.close()
}
