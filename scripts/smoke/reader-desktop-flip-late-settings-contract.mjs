#!/usr/bin/env node

// Reader desktop late flip settings browser contract
// (docs/compat/reader-desktop-flip-late-settings-fixed-baseline-second-audit-p0-contract.md section 8.2).
// reader-dev only slides in its mini interface (isSlideRead): a 左右滑动 setting that reaches a desktop Reader after it
// has set up still renders as page, and the desktop does not write its page back to the account:
// - L1 (1440x900 mouse; iPad 1366x1024 and 1024x1366 touch): the account settings answer flip only once the Reader
//   shell is mounted: the Reader stays .reader-shell.page with 加载下一章 mounted and sends no reader settings PUT;
// - L2 (same viewports): the Reader is open on page at a scrolled position when another device switches the account
//   to flip (settings_update on the sync socket): still page at the same scroll position, 加载下一章 still mounted,
//   no reader settings PUT;
// - L3 (390x844 phone, control): with the same late answer the mini interface enters .reader-shell.flip.
// This device starts on page with automatic day/night off, so only the account settings bring flip.
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
const PHONE_VIEWPORT = { label: '390x844 phone touch', width: 390, height: 844, input: 'phone' }
const BOOK_TITLE = '晚到设置契约'
const TITLES = ['第一章 起点', '第二章 后邻', '第三章 平稳']
const ACCOUNT_UPDATED_AT = '2026-10-30T00:00:00Z'
const OTHER_DEVICE_UPDATED_AT = '2026-10-30T00:05:00Z'
const CHAPTER_END = '.reader-chapter-end'
const FIRST_BLOCK = '.chapter-content[data-index="0"] [data-reader-block]'
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
    `${TITLES[index]}第${paragraph + 1}段。晚到设置契约正文，用于验证桌面端收到左右滑动设置后的版式。`
  )).join('\n')
}

function contextOptions(viewport) {
  const size = { width: viewport.width, height: viewport.height }
  if (viewport.input === 'phone') return { viewport: size, isMobile: true, hasTouch: true }
  if (viewport.input === 'touch') return { viewport: size, hasTouch: true, deviceScaleFactor: 2, userAgent: IPAD_UA }
  return { viewport: size }
}

function readerSettings(mode) {
  return {
    mode,
    pageMode: 'auto',
    autoTheme: false,
    fontSize: 18,
    lineHeight: 1.8,
    paragraphSpace: 0.2,
    columnWidth: 800,
    animateDuration: 0,
  }
}

function newAccount(mode, { holdSettings = false } = {}) {
  return {
    settings: { value: readerSettings(mode), updatedAt: ACCOUNT_UPDATED_AT },
    holdSettings,
    settingsAnswers: [],
    settingsWrites: [],
    socket: null,
    routeErrors: [],
  }
}

async function installMocks(page, account) {
  await page.routeWebSocket(/\/ws\/sync/, (socket) => {
    account.socket = socket
  })
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    try {
      if (path === '/me') return await route.fulfill(json({ id: 1, username: 'smoke', role: 'admin' }))
      if (path === '/settings/reader' && method === 'GET') {
        // Hold the answer until the Reader has set up, so the account settings arrive late on every run.
        if (account.holdSettings) await page.waitForSelector('.reader-shell', { state: 'attached', timeout: 10_000 }).catch(() => {})
        const { value, updatedAt } = account.settings
        account.settingsAnswers.push({ at: Date.now(), mode: value.mode })
        return await route.fulfill(json({ key: 'reader', updatedAt, value }))
      }
      if (path === '/settings/reader' && method === 'PUT') {
        const body = request.postDataJSON?.() || {}
        account.settingsWrites.push({ at: Date.now(), mode: body.value?.mode })
        return await route.fulfill(json({ key: 'reader', updatedAt: '2026-10-30T00:10:00Z', value: body.value || {} }))
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

async function openReader(context, account, pageErrors) {
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
  await page.goto(`${targetUrl}/books/1/read?chapter=0`, { waitUntil: 'domcontentloaded' })
  await page.waitForSelector(FIRST_BLOCK, { timeout: 15_000 })
  return page
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

async function readState(page) {
  return page.evaluate((chapterEnd) => {
    const shell = document.querySelector('.reader-shell')
    const documentScroll = Boolean(shell?.classList.contains('document-scroll'))
    const scroller = documentScroll
      ? (document.scrollingElement || document.documentElement)
      : document.querySelector('.reader-content')
    return {
      shell: shell ? [...shell.classList].filter(name => name !== 'reader-shell').join(' ') : null,
      flip: Boolean(shell?.classList.contains('flip')),
      page: Boolean(shell?.classList.contains('page')),
      mini: Boolean(shell?.classList.contains('mini-interface')),
      chapterEnd: Boolean(document.querySelector(chapterEnd)),
      scrollTop: Math.round(scroller?.scrollTop || 0),
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

function check(failures, condition, message) {
  if (!condition) failures.push(message)
}

function describeWrites(rows) {
  return JSON.stringify(rows.map(row => row.mode))
}

async function runLateAccountCase(page, viewport, account, failures, notes) {
  const applied = await waitForAppliedSettings(page, ACCOUNT_UPDATED_AT)
  const answer = account.settingsAnswers[0]
  check(failures, applied && answer?.mode === 'flip', `L1 setup: the late flip account settings were not applied (answers ${JSON.stringify(account.settingsAnswers)})`)
  const chapterEnd = await page.waitForSelector(CHAPTER_END, { timeout: 5000 }).then(() => true, () => false)
  await sleep(Math.max(0, (answer?.at || Date.now()) + SAVE_WINDOW - Date.now()))
  const state = await readState(page)
  const writes = account.settingsWrites.filter(row => row.at >= (answer?.at || 0))
  notes.push(`shell ${state.shell}; 加载下一章 ${chapterEnd ? 'mounted' : 'missing'}; settings PUT ${describeWrites(writes)}`)

  check(failures, !state.mini, `L1 desktop: the ${viewport.label} Reader is in the mini interface (${state.shell})`)
  check(failures, state.page && !state.flip, `L1 layout: a late flip setting must render as page on the desktop, shell is ${state.shell}`)
  check(failures, chapterEnd && state.chapterEnd, 'L1 chapter end: 加载下一章 is not mounted after the late flip setting')
  check(failures, writes.length === 0, `L1 account: the desktop wrote the reader settings back ${describeWrites(writes)}`)
}

async function runOtherDeviceCase(page, viewport, account, failures, notes) {
  const applied = await waitForAppliedSettings(page, ACCOUNT_UPDATED_AT)
  const ready = await page.waitForSelector(CHAPTER_END, { timeout: 5000 }).then(() => true, () => false)
  const connected = await waitUntil(() => account.socket, 10_000)
  if (!applied || !ready || !connected) {
    throw new Error(`precondition: account applied ${applied}, 加载下一章 ${ready}, sync socket ${Boolean(connected)}`)
  }

  await page.evaluate(() => {
    const shell = document.querySelector('.reader-shell')
    const scroller = shell?.classList.contains('document-scroll')
      ? (document.scrollingElement || document.documentElement)
      : document.querySelector('.reader-content')
    scroller.scrollTop = Math.round((scroller.scrollHeight - scroller.clientHeight) / 3)
    ;(scroller === document.scrollingElement || scroller === document.documentElement ? window : scroller)
      .dispatchEvent(new Event('scroll'))
  })
  // The position counts once it reads the same on two checks in a row.
  let before = await readState(page)
  for (let attempt = 0; attempt < 10; attempt += 1) {
    await sleep(450)
    const again = await readState(page)
    if (again.scrollTop === before.scrollTop) break
    before = again
  }

  account.settings = { value: readerSettings('flip'), updatedAt: OTHER_DEVICE_UPDATED_AT }
  const mark = Date.now()
  account.socket.send(JSON.stringify({ type: 'settings_update', payload: { key: 'reader', updatedAt: OTHER_DEVICE_UPDATED_AT } }))
  const reapplied = await waitForAppliedSettings(page, OTHER_DEVICE_UPDATED_AT)
  await sleep(Math.max(0, mark + SAVE_WINDOW - Date.now()))
  const after = await readState(page)
  const writes = account.settingsWrites.filter(row => row.at >= mark)
  notes.push(`before ${before.shell} scroll ${before.scrollTop}; after ${after.shell} scroll ${after.scrollTop}; 加载下一章 ${after.chapterEnd ? 'mounted' : 'missing'}; settings PUT ${describeWrites(writes)}`)

  check(failures, reapplied, 'L2 setup: the other device\'s flip settings were not applied')
  check(failures, !after.mini, `L2 desktop: the ${viewport.label} Reader is in the mini interface (${after.shell})`)
  check(failures, after.page && !after.flip, `L2 layout: another device's flip setting must render as page on the desktop, shell is ${after.shell}`)
  check(failures, Math.abs(after.scrollTop - before.scrollTop) <= 1, `L2 position: scroll ${before.scrollTop} -> ${after.scrollTop}`)
  check(failures, after.chapterEnd, 'L2 chapter end: 加载下一章 is not mounted after the other device\'s flip setting')
  check(failures, writes.length === 0, `L2 account: the desktop wrote the reader settings back ${describeWrites(writes)}`)
}

async function runPhoneControlCase(page, viewport, account, failures, notes) {
  const applied = await waitForAppliedSettings(page, ACCOUNT_UPDATED_AT)
  const flip = await page.waitForSelector('.reader-shell.flip', { timeout: 5000 }).then(() => true, () => false)
  const answer = account.settingsAnswers[0]
  await sleep(Math.max(0, (answer?.at || Date.now()) + SAVE_WINDOW - Date.now()))
  const state = await readState(page)
  const writes = account.settingsWrites.filter(row => row.at >= (answer?.at || 0))
  notes.push(`shell ${state.shell}; settings PUT ${describeWrites(writes)}`)

  check(failures, applied && answer?.mode === 'flip', `L3 setup: the late flip account settings were not applied (answers ${JSON.stringify(account.settingsAnswers)})`)
  check(failures, state.mini, `L3 mini interface: the phone Reader is not in the mini interface (${state.shell})`)
  check(failures, flip && state.flip, `L3 layout: the phone must enter flip, shell is ${state.shell}`)
  check(failures, writes.length === 0, `L3 account: the phone wrote the reader settings back ${describeWrites(writes)}`)
}

const SCENARIOS = [
  { id: 'L1', title: 'late account flip', viewports: DESKTOP_VIEWPORTS, account: () => newAccount('flip', { holdSettings: true }), run: runLateAccountCase },
  { id: 'L2', title: 'other device flip', viewports: DESKTOP_VIEWPORTS, account: () => newAccount('page'), run: runOtherDeviceCase },
  { id: 'L3', title: 'phone control', viewports: [PHONE_VIEWPORT], account: () => newAccount('flip', { holdSettings: true }), run: runPhoneControlCase },
]

async function runCase(browser, scenario, viewport) {
  const account = scenario.account()
  const failures = []
  const notes = []
  const pageErrors = []
  const context = await browser.newContext(contextOptions(viewport))
  try {
    const page = await openReader(context, account, pageErrors)
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
        scenario: scenario.id,
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
    throw new Error(`reader desktop late flip settings contract failed ${failed.length}/${cases.length} cases:\n${failed.join('\n')}`)
  }
  console.log(`reader desktop late flip settings contract smoke passed (${cases.length} cases)`)
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})
