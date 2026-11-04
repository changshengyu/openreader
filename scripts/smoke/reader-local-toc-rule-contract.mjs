#!/usr/bin/env node

// docs/compat/txt-toc-rule-selection-fixed-baseline-second-audit-p1-contract.md B1, against a real OpenReader server:
// a TXT whose first 回 shares its 章 line splits at every 回 (rule -19), and the Reader catalogue "修改规则" opens
// reader-dev's PopCatalog dialog (built-in rule select + shared textarea), switches to a built-in rule, and refuses
// an unchanged rule with "未修改规则" without refreshing.

import { openSmokeBrowser } from './playwright-runtime.mjs'

import { access, mkdtemp, rm } from 'node:fs/promises'
import { createServer } from 'node:http'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'
import { execFile, spawn } from 'node:child_process'

const execFileAsync = promisify(execFile)
const rootDir = join(dirname(fileURLToPath(import.meta.url)), '..', '..')
const backendDir = join(rootDir, 'backend')
const publicDir = join(rootDir, 'frontend', 'dist')

const HEADINGS = [
  '第一章 风起青萍 第一回 少年初入江湖路',
  '第二回 古寺夜话',
  '第三回 剑出霜寒',
  '第四回 别过故人',
  '第二章 云涌四海 第五回 长街初遇',
  '第六回 雨夜追踪',
  '第七回 渡口斗智',
  '第八回 灯下论剑',
]
const ZHANG_HEADINGS = [HEADINGS[0], HEADINGS[4]]

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function bookText() {
  return HEADINGS.map((heading, index) => {
    const paragraphs = Array.from({ length: 30 }, (_, paragraph) =>
      `　　甲${index + 1}第${paragraph + 1}段：山风穿过松林，远处的钟声一下一下地敲着，少年把剑背在身后，沿着石阶慢慢往上走。`)
    return `${heading}\n${paragraphs.join('\n')}\n`
  }).join('')
}

async function reserveLocalPort() {
  const server = createServer()
  await new Promise((resolve, reject) => {
    server.once('error', reject)
    server.listen(0, '127.0.0.1', resolve)
  })
  const address = server.address()
  assert(address && typeof address === 'object', 'unable to reserve a local OpenReader test port')
  await new Promise(resolve => server.close(resolve))
  return address.port
}

async function stopProcess(child) {
  if (!child || child.exitCode !== null) return
  const exited = new Promise(resolve => child.once('exit', resolve))
  child.kill('SIGTERM')
  await Promise.race([exited, new Promise(resolve => setTimeout(resolve, 5_000))])
  if (child.exitCode === null) {
    child.kill('SIGKILL')
    await Promise.race([exited, new Promise(resolve => setTimeout(resolve, 2_000))])
  }
}

async function waitForHealth(root, output) {
  const deadline = Date.now() + 60_000
  let lastError = null
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`${root}/api/health`)
      if (response.ok) return
      lastError = new Error(`health returned ${response.status}`)
    } catch (error) {
      lastError = error
    }
    await new Promise(resolve => setTimeout(resolve, 300))
  }
  throw new Error(`OpenReader TOC rule test server did not start: ${lastError?.message || 'unknown error'}\n${output()}`)
}

async function startOpenReader() {
  await access(join(publicDir, 'index.html')).catch(() => {
    throw new Error('frontend/dist is missing; run `cd frontend && npm run build` before this smoke')
  })
  const tempRoot = await mkdtemp(join(tmpdir(), 'openreader-toc-rule-'))
  const binary = join(tempRoot, 'openreader')
  const port = await reserveLocalPort()
  await execFileAsync('go', ['build', '-o', binary, '.'], {
    cwd: backendDir,
    env: process.env,
    maxBuffer: 4 * 1024 * 1024,
  })
  let output = ''
  const child = spawn(binary, [], {
    cwd: backendDir,
    env: {
      ...process.env,
      OPENREADER_ADDR: `127.0.0.1:${port}`,
      OPENREADER_DATA_DIR: join(tempRoot, 'data'),
      OPENREADER_CACHE_DIR: join(tempRoot, 'cache'),
      OPENREADER_LIBRARY_DIR: join(tempRoot, 'library'),
      OPENREADER_LOCAL_STORE_DIR: join(tempRoot, 'library', 'localStore'),
      OPENREADER_DB: join(tempRoot, 'data', 'openreader.db'),
      OPENREADER_PUBLIC_DIR: publicDir,
      OPENREADER_JWT_SECRET: 'reader-toc-rule-contract-secret',
      OPENREADER_CORS_ORIGIN: `http://127.0.0.1:${port}`,
      OPENREADER_CHECK_INTERVAL: '24h',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  child.stdout.on('data', chunk => { output += chunk.toString() })
  child.stderr.on('data', chunk => { output += chunk.toString() })
  const root = `http://127.0.0.1:${port}`
  try {
    await waitForHealth(root, () => output)
  } catch (error) {
    await stopProcess(child)
    await rm(tempRoot, { recursive: true, force: true })
    throw error
  }
  return {
    root,
    close: async () => {
      await stopProcess(child)
      await rm(tempRoot, { recursive: true, force: true })
    },
  }
}

async function api(root, path, { token = '', method = 'GET', body } = {}) {
  const response = await fetch(`${root}/api${path}`, {
    method,
    headers: {
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const text = await response.text()
  let data = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    data = text
  }
  if (!response.ok) throw new Error(`${method} ${path} failed with ${response.status}: ${text}`)
  return data
}

async function seed(root, viewport) {
  const suffix = `${viewport.width}x${viewport.height}`
  const registered = await api(root, '/auth/register', {
    method: 'POST',
    body: { username: `tocrule${viewport.width}${viewport.height}`, password: 'toc-rule-contract' },
  })
  const token = registered?.token
  assert(token, `${suffix}: registration did not return a token`)
  const rules = await api(root, '/txt-toc-rules', { token })
  const byID = id => rules.find(rule => Number(rule.id) === id)
  const zhangHui = byID(-19)
  const directory = byID(-2)
  assert(zhangHui?.rule && zhangHui.name === '目录(章回、话)', `${suffix}: /api/txt-toc-rules must list rule -19 目录(章回、话)`)
  assert(directory?.rule && directory.name === '目录', `${suffix}: /api/txt-toc-rules must list rule -2 目录`)

  const form = new FormData()
  form.append('file', new Blob([bookText()], { type: 'text/plain' }), `章回测试 ${suffix}.txt`)
  form.append('title', `章回测试 ${suffix}`)
  const response = await fetch(`${root}/api/imports/books`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: form,
  })
  const text = await response.text()
  assert(response.ok, `${suffix}: TXT import failed with ${response.status}: ${text}`)
  const imported = JSON.parse(text)
  const book = await api(root, `/books/${imported.id}`, { token })
  assert(Number(book.chapterCount) === HEADINGS.length,
    `${suffix}: automatic detection must split every 回 (${HEADINGS.length}), got ${book.chapterCount}`)
  assert(book.tocRule === zhangHui.rule, `${suffix}: the imported book must record rule -19, got ${JSON.stringify(book.tocRule)}`)
  return { token, book, zhangHui, directory, suffix }
}

// Waits for the finite enter/leave animations of Element overlays and poppers, so a tap never lands on a
// message box that is still fading in or on a dropdown that is still moving.
async function settle(page) {
  await page.evaluate(() => Promise.all(document.getAnimations()
    .filter(animation => {
      const target = animation.effect?.target
      const timing = animation.effect?.getComputedTiming?.()
      return target?.closest?.('.el-overlay, .el-popper') && Number.isFinite(timing?.endTime)
    })
    .map(animation => animation.finished.catch(() => null))))
}

async function tap(page, viewport, locator) {
  // An option can sit outside the dropdown's scroll viewport (the list opens scrolled to the current rule);
  // a coordinate tap there would hit the message box overlay instead.
  await locator.scrollIntoViewIfNeeded()
  await settle(page)
  const box = await locator.boundingBox()
  assert(box, `missing target for tap: ${locator}`)
  if (viewport.mobile) await page.touchscreen.tap(box.x + box.width / 2, box.y + box.height / 2)
  else await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2)
}

async function openCatalogue(page, viewport) {
  const tool = viewport.mobile
    ? page.locator('.reader-mobile-top .mobile-tool-button').filter({ hasText: '目录' }).first()
    : page.locator('.reader-left-rail .rail-item').filter({ hasText: '目录' }).first()
  await tool.waitFor({ state: 'visible', timeout: 15_000 })
  await tap(page, viewport, tool)
  const panel = page.locator('.reader-toc-popup').first()
  await panel.waitFor({ state: 'visible', timeout: 10_000 })
  return panel
}

async function catalogueTitles(panel) {
  return panel.locator('.toc-item .log-text').allInnerTexts()
}

async function waitFor(check, description, timeout = 15_000) {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) {
    const value = await check()
    if (value) return value
    await new Promise(resolve => setTimeout(resolve, 120))
  }
  throw new Error(`timed out: ${description}`)
}

async function openRuleDialog(page, viewport, panel) {
  await tap(page, viewport, panel.getByRole('button', { name: '修改规则', exact: true }))
  const dialog = page.locator('.el-message-box.reader-toc-rule-dialog')
  await dialog.waitFor({ state: 'visible', timeout: 10_000 })
  await settle(page)
  return dialog
}

async function runViewport(browser, root, viewport) {
  const seeded = await seed(root, viewport)
  const { suffix } = seeded
  const context = await browser.newContext({
    viewport: { width: viewport.width, height: viewport.height },
    ...(viewport.mobile ? { isMobile: true, hasTouch: true } : {}),
  })
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error' && !/WebSocket connection to .*\/ws\/sync/.test(message.text())) {
      failures.push(`console.error: ${message.text()}`)
    }
  })
  const refreshes = []
  page.on('request', request => {
    if (request.method() === 'POST' && new URL(request.url()).pathname === `/api/books/${seeded.book.id}/refresh-local`) {
      refreshes.push(request.postData() || '')
    }
  })

  try {
    await page.addInitScript(token => localStorage.setItem('openreader_token', token), seeded.token)
    await page.goto(`${root}/books/${seeded.book.id}/read`, { waitUntil: 'domcontentloaded' })
    await page.locator('.reader-content .chapter-content').first().waitFor({ state: 'visible', timeout: 20_000 })
    await page.waitForFunction(() => !document.body.innerText.includes('正在加载章节'), null, { timeout: 15_000 })

    let panel = await openCatalogue(page, viewport)
    const automatic = await catalogueTitles(panel)
    assert(JSON.stringify(automatic) === JSON.stringify(HEADINGS),
      `${suffix}: the catalogue must list every 回, got ${JSON.stringify(automatic)}`)

    // reader-dev PopCatalog renderComp: "请选择规则：" + select + textarea sharing the book's current rule.
    let dialog = await openRuleDialog(page, viewport, panel)
    assert((await dialog.locator('.el-message-box__title').innerText()).trim() === '修改目录规则', `${suffix}: dialog title`)
    assert((await dialog.innerText()).includes('请选择规则：'), `${suffix}: dialog must say 请选择规则：`)
    const select = dialog.locator('.el-select')
    const textarea = dialog.locator('textarea')
    assert(await select.count() === 1 && await textarea.count() === 1, `${suffix}: dialog must offer one rule select and one textarea`)
    assert((await select.innerText()).includes('目录(章回、话)'), `${suffix}: the select must show the detected rule, got ${await select.innerText()}`)
    assert(await textarea.inputValue() === seeded.zhangHui.rule, `${suffix}: the textarea must hold the detected rule text`)
    const dialogBox = await dialog.boundingBox()
    assert(dialogBox && dialogBox.x >= -1 && dialogBox.x + dialogBox.width <= viewport.width + 1,
      `${suffix}: the rule dialog must fit the viewport, got ${JSON.stringify(dialogBox)}`)

    await tap(page, viewport, select)
    const option = page.locator('.el-select-dropdown__item:visible').filter({ hasText: /^目录$/ })
    await option.waitFor({ state: 'visible', timeout: 10_000 })
    const optionNames = await page.locator('.el-select-dropdown__item:visible').allInnerTexts()
    for (const name of ['目录(去空白)', '目录', '目录(古典、轻小说备用)', '双标题(后向)', '目录(章回、话)']) {
      assert(optionNames.includes(name), `${suffix}: the select must list built-in rule ${name}, got ${JSON.stringify(optionNames)}`)
    }
    await tap(page, viewport, option)
    await waitFor(async () => (await textarea.inputValue()) === seeded.directory.rule, `${suffix}: textarea follows the chosen rule`)
    assert(await dialog.isVisible(), `${suffix}: choosing a rule must keep the dialog open`)

    const refreshResponse = page.waitForResponse(
      response => response.request().method() === 'POST' && new URL(response.url()).pathname === `/api/books/${seeded.book.id}/refresh-local`,
      { timeout: 20_000 },
    )
    await tap(page, viewport, dialog.getByRole('button', { name: '确定', exact: true }))
    assert((await refreshResponse).status() === 200, `${suffix}: refresh-local must succeed`)
    assert(refreshes.length === 1 && JSON.parse(refreshes[0]).tocRule === seeded.directory.rule,
      `${suffix}: refresh-local must send the chosen built-in rule, got ${JSON.stringify(refreshes)}`)
    await dialog.waitFor({ state: 'hidden', timeout: 10_000 })
    const switched = await waitFor(async () => {
      const book = await api(root, `/books/${seeded.book.id}`, { token: seeded.token })
      return Number(book.chapterCount) === ZHANG_HEADINGS.length && book.tocRule === seeded.directory.rule ? book : null
    }, `${suffix}: the book switches to rule -2`)
    assert(switched, `${suffix}: rule switch did not persist`)

    if (!(await page.locator('.reader-toc-popup').first().isVisible())) panel = await openCatalogue(page, viewport)
    else panel = page.locator('.reader-toc-popup').first()
    await waitFor(async () => JSON.stringify(await catalogueTitles(panel)) === JSON.stringify(ZHANG_HEADINGS),
      `${suffix}: the catalogue shows the 章 rule result`)

    dialog = await openRuleDialog(page, viewport, panel)
    assert((await dialog.locator('.el-select').innerText()).includes('目录'), `${suffix}: the select shows the saved rule`)
    assert(await dialog.locator('textarea').inputValue() === seeded.directory.rule, `${suffix}: the textarea shows the saved rule`)
    await tap(page, viewport, dialog.getByRole('button', { name: '确定', exact: true }))
    await page.locator('.el-message--error').filter({ hasText: '未修改规则' }).first().waitFor({ state: 'visible', timeout: 10_000 })
    await page.waitForTimeout(1_500)
    assert(refreshes.length === 1, `${suffix}: an unchanged rule must not refresh, got ${refreshes.length} refresh requests`)

    if (await dialog.isVisible()) {
      await tap(page, viewport, dialog.getByRole('button', { name: '取消', exact: true }))
      await dialog.waitFor({ state: 'hidden', timeout: 10_000 })
    }
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)
    assert(overflow <= 1, `${suffix}: Reader has ${overflow}px horizontal overflow`)
    assert(failures.length === 0, `${suffix}: ${failures.join('\n')}`)
    return suffix
  } finally {
    await context.close()
  }
}

async function run() {
  const app = await startOpenReader()
  try {
    const browser = await openSmokeBrowser()
    try {
      const completed = []
      for (const viewport of [
        { width: 1440, height: 900 },
        { width: 390, height: 844, mobile: true },
        { width: 360, height: 800, mobile: true },
      ]) {
        completed.push(await runViewport(browser, app.root, viewport))
      }
      console.log(`reader-local-toc-rule: ok ${completed.join(', ')} realApi=true zhangHuiDetection=true builtInRuleSelect=true sharedTextarea=true unchangedRefused=true`)
    } finally {
      await browser.close()
    }
  } finally {
    await app.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
