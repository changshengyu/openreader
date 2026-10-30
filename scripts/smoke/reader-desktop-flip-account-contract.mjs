#!/usr/bin/env node

// Reader desktop flip account browser contract
// (docs/compat/reader-desktop-flip-account-fixed-baseline-second-audit-p1-contract.md section 7.2).
// reader-dev derives isSlideRead from the interface and never rewrites readMethod: a 左右滑动 chosen on a phone stays
// 左右滑动 when a desktop opens the book, and the mini interface slides again after a round trip.
// The Reader route chunk is held until the account settings have been applied, so the Reader always sets up on flip:
// - A1 (1440x900 mouse; iPad 1366x1024 and 1024x1366 touch): opening at ?percent=0.5 renders .reader-shell.page with
//   加载下一章 mounted and the text scrolled to the middle, sends no reader settings PUT, and the progress saved after
//   加载下一章 records mode page;
// - A2 (iPad 1024x1366 and 1366x1024 touch, 手机模式): .reader-shell.flip; 页面模式 自适应 renders page; 手机模式 again
//   slides again; every reader settings PUT keeps mode flip;
// - A3 (390x844 phone, control): .reader-shell.flip and no reader settings PUT;
// - A4 (same viewports as A1): the desktop 翻页方式 row highlights nothing (左右滑动 is hidden, as in reader-dev), and
//   the PUT after 增大字号 keeps mode flip with the larger font.
// Each case uses a fresh browser context, runs to the end and reports all of its failed checks;
// SMOKE_CASES=<regex> limits the cases.

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = (process.env.TARGET_URL || 'http://127.0.0.1:5173').replace(/\/$/, '')
const caseFilter = process.env.SMOKE_CASES ? new RegExp(process.env.SMOKE_CASES) : null

const IPAD_UA = 'Mozilla/5.0 (iPad; CPU OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1'
const DESKTOP_VIEWPORTS = [
  { label: '1440x900 mouse', width: 1440, height: 900, input: 'mouse' },
  { label: '1366x1024 iPad touch', width: 1366, height: 1024, input: 'touch' },
  { label: '1024x1366 iPad touch', width: 1024, height: 1366, input: 'touch' },
]
const IPAD_VIEWPORTS = [
  { label: '1024x1366 iPad touch', width: 1024, height: 1366, input: 'touch' },
  { label: '1366x1024 iPad touch', width: 1366, height: 1024, input: 'touch' },
]
const PHONE_VIEWPORT = { label: '390x844 phone touch', width: 390, height: 844, input: 'phone' }
const BOOK_TITLE = '左右滑动账号契约'
const TITLES = ['第一章 起点', '第二章 后邻', '第三章 平稳']
const ACCOUNT_UPDATED_AT = '2026-10-30T00:00:00Z'
const CHAPTER_END = '.reader-chapter-end'
const FIRST_BLOCK = '.chapter-content[data-index="0"] [data-reader-block]'
// The Vite build names the route chunk Reader-<hash>.js; the dev server serves the component itself.
const READER_CHUNK = /\/(?:assets\/Reader-[A-Za-z0-9_-]+\.js|src\/views\/Reader\.vue)(?:\?.*)?$/
// A setting change is saved 700ms after it is made (stores/reader.js scheduleSettingsSync).
const SAVE_WINDOW = 1500

const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))

function json(data, status = 200) {
  return { status, contentType: 'application/json', body: JSON.stringify(data) }
}

function fakeToken() {
  const payload = Buffer.from(JSON.stringify({ userId: 1, sub: '1' })).toString('base64url')
  return `open.${payload}.reader`
}

function chapterContent(index) {
  return Array.from({ length: 80 }, (_, paragraph) => (
    `${TITLES[index]}第${paragraph + 1}段。左右滑动账号契约正文，用于验证桌面端打开后账号里的左右滑动保持不变。`
  )).join('\n')
}

function contextOptions(viewport) {
  const size = { width: viewport.width, height: viewport.height }
  if (viewport.input === 'phone') return { viewport: size, isMobile: true, hasTouch: true }
  if (viewport.input === 'touch') return { viewport: size, hasTouch: true, deviceScaleFactor: 2, userAgent: IPAD_UA }
  return { viewport: size }
}

function readerSettings(overrides = {}) {
  return {
    mode: 'flip',
    pageMode: 'auto',
    autoTheme: false,
    fontSize: 18,
    lineHeight: 1.8,
    paragraphSpace: 0.2,
    columnWidth: 800,
    animateDuration: 0,
    ...overrides,
  }
}

function newAccount(overrides) {
  return {
    settings: { value: readerSettings(overrides), updatedAt: ACCOUNT_UPDATED_AT },
    settingsAnswers: [],
    settingsWrites: [],
    progressWrites: [],
    routeErrors: [],
  }
}

// The store persists the reader settings on every change; the account timestamp shows the answer was applied.
async function waitForAppliedSettings(page, updatedAt) {
  return page.waitForFunction((expected) => {
    try {
      return JSON.parse(localStorage.getItem('reader') || '{}').settingsSyncBaseUpdatedAt === expected
    } catch {
      return false
    }
  }, updatedAt, { timeout: 10_000 }).then(() => true, () => false)
}

async function installMocks(page, account) {
  // The sync socket stays open and silent.
  await page.routeWebSocket(/\/ws\/sync/, () => {})
  // Hold the Reader until the account settings are applied: the Reader sets up on the account's flip every run.
  await page.route(READER_CHUNK, async (route) => {
    account.readerChunkHeldFor = await waitForAppliedSettings(page, ACCOUNT_UPDATED_AT)
    await route.continue()
  })
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    try {
      if (path === '/me') return await route.fulfill(json({ id: 1, username: 'smoke', role: 'admin' }))
      if (path === '/settings/reader' && method === 'GET') {
        const { value, updatedAt } = account.settings
        account.settingsAnswers.push({ at: Date.now(), mode: value.mode })
        return await route.fulfill(json({ key: 'reader', updatedAt, value }))
      }
      if (path === '/settings/reader' && method === 'PUT') {
        const value = request.postDataJSON?.()?.value || {}
        account.settingsWrites.push({ at: Date.now(), mode: value.mode, pageMode: value.pageMode, fontSize: value.fontSize })
        const updatedAt = `2026-10-30T00:10:${String(account.settingsWrites.length).padStart(2, '0')}Z`
        return await route.fulfill(json({ key: 'reader', updatedAt, value }))
      }
      const book = { id: 1, title: BOOK_TITLE, author: 'OpenReader', sourceId: 0, chapterCount: TITLES.length, progress: null }
      if (path === '/books/1') return await route.fulfill(json(book))
      if (path === '/books') return await route.fulfill(json([book]))
      if (path === '/books/1/chapters') {
        return await route.fulfill(json(TITLES.map((title, index) => ({ id: index + 11, bookId: 1, index, title }))))
      }
      const contentMatch = path.match(/^\/books\/1\/chapters\/(\d+)\/content$/)
      if (contentMatch) {
        const index = Number(contentMatch[1])
        return await route.fulfill(json({
          chapter: { id: index + 11, bookId: 1, index, title: TITLES[index] },
          content: chapterContent(index),
          format: 'text',
        }))
      }
      if (path === '/progress' && method === 'PUT') {
        const body = request.postDataJSON?.() || {}
        account.progressWrites.push({ at: Date.now(), chapterIndex: body.chapterIndex, mode: body.mode })
        return await route.fulfill(json({ ...body, bookId: 1 }))
      }
      if (path === '/progress/1') return await route.fulfill(json({}))
      if (path === '/books/1/bookmarks' || path === '/sources' || path === '/categories') {
        return await route.fulfill(json([]))
      }
      return await route.fulfill(json({}))
    } catch (error) {
      account.routeErrors.push(String(error?.message || error).split('\n')[0])
    }
  })
}

async function openReader(context, account, pageErrors, query) {
  await context.addInitScript(({ token }) => {
    localStorage.setItem('openreader_token', token)
    localStorage.setItem('reader', JSON.stringify({
      mode: 'page',
      autoTheme: false,
      settingsScope: 'user:1',
      progressScope: 'user:1',
    }))
  }, { token: fakeToken() })
  const page = await context.newPage()
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    const text = message.text()
    if (text.includes('/ws/sync')) return
    pageErrors.push(text.split('\n')[0])
  })
  page.on('pageerror', error => pageErrors.push(error.message))
  await installMocks(page, account)
  await page.goto(`${targetUrl}/books/1/read?${query}`, { waitUntil: 'domcontentloaded' })
  await page.waitForSelector(FIRST_BLOCK, { timeout: 15_000 })
  if (!account.readerChunkHeldFor) throw new Error('precondition: the account settings were not applied before the Reader set up')
  return page
}

async function readState(page) {
  return page.evaluate((chapterEnd) => {
    const shell = document.querySelector('.reader-shell')
    const documentScroll = Boolean(shell?.classList.contains('document-scroll'))
    const scroller = documentScroll
      ? (document.scrollingElement || document.documentElement)
      : document.querySelector('.reader-content')
    const range = Math.max(0, (scroller?.scrollHeight || 0) - (scroller?.clientHeight || 0))
    return {
      shell: shell ? [...shell.classList].filter(name => name !== 'reader-shell').join(' ') : null,
      flip: Boolean(shell?.classList.contains('flip')),
      page: Boolean(shell?.classList.contains('page')),
      mini: Boolean(shell?.classList.contains('mini-interface')),
      chapterEnd: Boolean(document.querySelector(chapterEnd)),
      scrollTop: Math.round(scroller?.scrollTop || 0),
      scrollRatio: range > 0 ? Number(((scroller?.scrollTop || 0) / range).toFixed(3)) : 0,
    }
  }, CHAPTER_END)
}

async function waitUntil(predicate, timeout) {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) {
    if (predicate()) return true
    await sleep(50)
  }
  return predicate()
}

async function ensureMobileSettings(page, viewport) {
  if (!await page.locator('.reader-mobile-top.visible').count()) {
    await page.touchscreen.tap(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
    await page.waitForSelector('.reader-mobile-top.visible', { timeout: 5000 })
  }
  if (!await page.locator('.reader-mobile-primary-settings').count()) {
    await page.locator('.reader-mobile-top.visible .mobile-tool-button').filter({ hasText: '设置' }).click()
    await page.waitForSelector('.reader-mobile-primary-settings .settings-body', { timeout: 5000 })
  }
  return page.locator('.reader-mobile-primary-settings')
}

async function ensureDesktopSettings(page) {
  if (!await page.locator('.reader-desktop-workspace .settings-body').count()) {
    await page.locator('.reader-left-rail button[title="设置"]').click()
    await page.waitForSelector('.reader-desktop-workspace .settings-body', { timeout: 5000 })
  }
  return page.locator('.reader-desktop-workspace')
}

function settingRow(panel, label) {
  return panel.locator('.setting-row').filter({ has: panel.page().locator('.setting-label', { hasText: label }) }).first()
}

async function clickSettingOption(panel, label, option) {
  const row = settingRow(panel, label)
  await row.scrollIntoViewIfNeeded()
  await row.locator('.selection-button').filter({ hasText: option }).first().click()
}

async function readMethodRow(panel) {
  const row = settingRow(panel, '翻页方式')
  await row.scrollIntoViewIfNeeded()
  return row.locator('.selection-button').evaluateAll(buttons => buttons.map(button => ({
    label: button.textContent.trim(),
    active: button.classList.contains('active'),
  })))
}

function check(failures, condition, message) {
  if (!condition) failures.push(message)
}

function describe(rows, fields) {
  return JSON.stringify(rows.map(row => fields.map(field => row[field]).join('/')))
}

async function runDesktopOpenCase(page, viewport, account, failures, notes) {
  const opened = Date.now()
  const chapterEnd = await page.waitForSelector(CHAPTER_END, { timeout: 5000 }).then(() => true, () => false)
  await sleep(Math.max(0, opened + SAVE_WINDOW - Date.now()))
  const state = await readState(page)
  const settingsWrites = [...account.settingsWrites]
  notes.push(`shell ${state.shell}; scroll ${state.scrollTop} (${state.scrollRatio}); 加载下一章 ${chapterEnd ? 'mounted' : 'missing'}; settings PUT ${describe(settingsWrites, ['mode'])}`)

  check(failures, account.settingsAnswers[0]?.mode === 'flip', `A1 setup: the account settings did not answer flip (${JSON.stringify(account.settingsAnswers)})`)
  check(failures, !state.mini, `A1 desktop: the ${viewport.label} Reader is in the mini interface (${state.shell})`)
  check(failures, state.page && !state.flip, `A1 layout: the account's flip must render as page on the desktop, shell is ${state.shell}`)
  check(failures, chapterEnd && state.chapterEnd, 'A1 chapter end: 加载下一章 is not mounted')
  check(failures, state.scrollRatio >= 0.35 && state.scrollRatio <= 0.65, `A1 position: ?percent=0.5 should open in the middle of the chapter, scroll ratio ${state.scrollRatio}`)
  check(failures, settingsWrites.length === 0, `A1 account: opening the desktop Reader wrote the reader settings ${describe(settingsWrites, ['mode'])}`)

  if (!chapterEnd) return
  const mark = Date.now()
  await page.locator(CHAPTER_END).click()
  const saved = await waitUntil(() => account.progressWrites.some(row => row.at >= mark && row.chapterIndex === 1), 5000)
  const progressWrites = [...account.progressWrites]
  notes.push(`progress PUT ${describe(progressWrites, ['chapterIndex', 'mode'])}`)
  check(failures, saved, `A1 progress: 加载下一章 saved no progress for chapter 1 ${describe(progressWrites, ['chapterIndex', 'mode'])}`)
  check(failures, progressWrites.every(row => row.mode === 'page'), `A1 progress: the desktop must record its page layout ${describe(progressWrites, ['chapterIndex', 'mode'])}`)
}

async function runIPadRoundTripCase(page, viewport, account, failures, notes) {
  const sliding = await page.waitForSelector('.reader-shell.flip.mini-interface', { timeout: 5000 }).then(() => true, () => false)
  if (!sliding) throw new Error(`precondition: 手机模式 with the account's flip should slide, shell is ${(await readState(page)).shell}`)

  const mobilePanel = await ensureMobileSettings(page, viewport)
  await clickSettingOption(mobilePanel, '页面模式', '自适应')
  const desktop = await page.waitForSelector('.reader-shell:not(.mini-interface)', { timeout: 5000 }).then(() => true, () => false)
  await sleep(SAVE_WINDOW)
  const adaptive = await readState(page)

  const desktopPanel = await ensureDesktopSettings(page)
  await clickSettingOption(desktopPanel, '页面模式', '手机模式')
  const mini = await page.waitForSelector('.reader-shell.mini-interface', { timeout: 5000 }).then(() => true, () => false)
  const slidesAgain = await page.waitForSelector('.reader-shell.flip', { timeout: 5000 }).then(() => true, () => false)
  await sleep(SAVE_WINDOW)
  const back = await readState(page)
  const writes = [...account.settingsWrites]
  notes.push(`自适应 ${adaptive.shell}; 手机模式 ${back.shell}; settings PUT ${describe(writes, ['pageMode', 'mode'])}`)

  check(failures, desktop && !adaptive.mini, `A2 自适应: the ${viewport.label} Reader did not leave the mini interface (${adaptive.shell})`)
  check(failures, adaptive.page && !adaptive.flip, `A2 自适应: the desktop must render the flip setting as page, shell is ${adaptive.shell}`)
  check(failures, mini && back.mini, `A2 手机模式: the Reader did not return to the mini interface (${back.shell})`)
  check(failures, slidesAgain && back.flip, `A2 手机模式: the round trip lost 左右滑动, shell is ${back.shell}`)
  check(failures, writes.some(row => row.pageMode === 'auto') && writes.at(-1)?.pageMode === 'mobile', `A2 account: expected the 页面模式 choices to be saved ${describe(writes, ['pageMode', 'mode'])}`)
  check(failures, writes.every(row => row.mode === 'flip'), `A2 account: every reader settings PUT must keep mode flip ${describe(writes, ['pageMode', 'mode'])}`)
}

async function runPhoneControlCase(page, viewport, account, failures, notes) {
  const opened = Date.now()
  const flip = await page.waitForSelector('.reader-shell.flip', { timeout: 5000 }).then(() => true, () => false)
  await sleep(Math.max(0, opened + SAVE_WINDOW - Date.now()))
  const state = await readState(page)
  const writes = [...account.settingsWrites]
  notes.push(`shell ${state.shell}; settings PUT ${describe(writes, ['mode'])}`)

  check(failures, state.mini, `A3 mini interface: the phone Reader is not in the mini interface (${state.shell})`)
  check(failures, flip && state.flip, `A3 layout: the phone must slide on the account's flip, shell is ${state.shell}`)
  check(failures, writes.length === 0, `A3 account: the phone wrote the reader settings ${describe(writes, ['mode'])}`)
}

async function runDesktopEditCase(page, viewport, account, failures, notes) {
  const ready = await page.waitForSelector(CHAPTER_END, { timeout: 5000 }).then(() => true, () => false)
  if (!ready) throw new Error('precondition: 加载下一章 is not mounted')
  const panel = await ensureDesktopSettings(page)
  const methods = await readMethodRow(panel)
  await sleep(SAVE_WINDOW)
  const mark = Date.now()
  const font = settingRow(panel, '字体大小')
  await font.scrollIntoViewIfNeeded()
  await font.locator('button[aria-label="增大字号"]').click()
  await waitUntil(() => account.settingsWrites.some(row => row.at >= mark), SAVE_WINDOW + 1000)
  await sleep(300)
  const state = await readState(page)
  const before = account.settingsWrites.filter(row => row.at < mark)
  const after = account.settingsWrites.filter(row => row.at >= mark)
  notes.push(`翻页方式 ${JSON.stringify(methods.map(row => `${row.label}${row.active ? '*' : ''}`))}; shell ${state.shell}; settings PUT before ${describe(before, ['mode', 'fontSize'])} after ${describe(after, ['mode', 'fontSize'])}`)

  check(failures, methods.every(row => row.label !== '左右滑动'), `A4 翻页方式: the desktop must hide 左右滑动 ${JSON.stringify(methods)}`)
  check(failures, methods.length === 3 && methods.every(row => !row.active), `A4 翻页方式: the account's 左右滑动 highlights nothing on the desktop ${JSON.stringify(methods)}`)
  check(failures, state.page && !state.flip && !state.mini, `A4 layout: the desktop must stay on page after the font change, shell is ${state.shell}`)
  check(failures, after.length === 1 && after[0].fontSize === 19, `A4 account: expected one PUT with font 19 ${describe(after, ['mode', 'fontSize'])}`)
  check(failures, [...before, ...after].every(row => row.mode === 'flip'), `A4 account: a desktop save must keep mode flip, before ${describe(before, ['mode', 'fontSize'])} after ${describe(after, ['mode', 'fontSize'])}`)
}

const SCENARIOS = [
  { id: 'A1', title: 'desktop open', viewports: DESKTOP_VIEWPORTS, account: () => newAccount(), query: 'chapter=0&percent=0.5', run: runDesktopOpenCase },
  { id: 'A2', title: 'iPad round trip', viewports: IPAD_VIEWPORTS, account: () => newAccount({ pageMode: 'mobile' }), query: 'chapter=0', run: runIPadRoundTripCase },
  { id: 'A3', title: 'phone control', viewports: [PHONE_VIEWPORT], account: () => newAccount(), query: 'chapter=0', run: runPhoneControlCase },
  { id: 'A4', title: 'desktop edit', viewports: DESKTOP_VIEWPORTS, account: () => newAccount(), query: 'chapter=0', run: runDesktopEditCase },
]

async function runCase(browser, scenario, viewport) {
  const account = scenario.account()
  const failures = []
  const notes = []
  const pageErrors = []
  const context = await browser.newContext(contextOptions(viewport))
  try {
    const page = await openReader(context, account, pageErrors, scenario.query)
    await scenario.run(page, viewport, account, failures, notes)
  } catch (error) {
    failures.push(`harness: ${String(error?.message || error).split('\n')[0]}`)
  } finally {
    failures.push(...pageErrors.map(text => `page error: ${text}`))
    failures.push(...account.routeErrors.map(text => `route error: ${text}`))
    await context.close()
  }
  return { failures, notes }
}

function plannedCases() {
  const cases = []
  for (const scenario of SCENARIOS) {
    for (const viewport of scenario.viewports) {
      cases.push({
        label: `${scenario.id} ${scenario.title} | ${viewport.label}`,
        run: browser => runCase(browser, scenario, viewport),
      })
    }
  }
  return cases.filter(row => !caseFilter || caseFilter.test(row.label))
}

async function main() {
  const cases = plannedCases()
  if (!cases.length) throw new Error(`no cases match SMOKE_CASES=${process.env.SMOKE_CASES}`)
  const browser = await openSmokeBrowser()
  const failed = []
  try {
    for (const row of cases) {
      const result = await row.run(browser)
      console.log(`${result.failures.length ? 'FAIL' : 'PASS'} ${row.label} | ${result.notes.join(' | ')}`)
      for (const failure of result.failures) console.log(`  - ${failure}`)
      if (result.failures.length) failed.push(row.label)
    }
  } finally {
    await browser.close()
  }
  if (failed.length) {
    throw new Error(`reader desktop flip account contract failed ${failed.length}/${cases.length} cases:\n${failed.join('\n')}`)
  }
  console.log(`reader desktop flip account contract smoke passed (${cases.length} cases)`)
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})
