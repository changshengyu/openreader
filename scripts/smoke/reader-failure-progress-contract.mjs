#!/usr/bin/env node

// Reader failure-state progress payload browser contract
// (docs/compat/reader-failure-progress-payload-fixed-baseline-second-audit-p0-contract.md section 9.2).
// It needs a real OpenReader server: PUT /api/progress must be answered by the backend, whose chapter identity check
// (ErrChapterIdentity -> 400) mocked contracts cannot see. page.route only fails one chapter's content, fails one book's
// catalog, holds one chapter's content or fails PUT /api/progress (E5b).
// - E1 (P1, P2, P6, P7): read chapter 4, open the failing chapter 5. The failure saves row 5 at its start, hidden and
//   leave writes name row 5, a catalog press retries the chapter (and shows it once the source recovers); no
//   PUT /api/progress is refused and no page error appears.
// - E2 (P2): a first load that fails saves row 5 when the reader is left.
// - E3 (P3): a save while the target chapter is still loading names row 5 and is accepted.
// - E4 (P4): switching to a book whose catalog fails writes nothing for it, its local snapshot and shelf card never take
//   the previous book's chapter, and after recovery it opens at its own start.
// - E5 (P5): a reader shelf switch during a failure navigates; E5b also when PUT /api/progress fails with 500.
// The planned E7 (scroll into a failed adjacent chapter) is not built: a failed adjacent block is always the last, short
// block of its window, so no scroll position makes it the top visible block (contract section 13).
// Every case runs to the end and reports all of its failed checks; SMOKE_CASES=<regex> limits the cases.
//   TARGET_URL=http://127.0.0.1:8080 node scripts/smoke/reader-failure-progress-contract.mjs

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = (process.env.TARGET_URL || 'http://127.0.0.1:8080').replace(/\/$/, '')
const caseFilter = process.env.SMOKE_CASES ? new RegExp(process.env.SMOKE_CASES) : null

const VIEWPORTS = [
  { label: '1440x900 mouse', width: 1440, height: 900, touch: false },
  { label: '390x844 touch', width: 390, height: 844, touch: true },
  { label: '360x800 touch', width: 360, height: 800, touch: true },
]
const MODES = ['page', 'flip', 'scroll', 'scroll2']
const CONTINUOUS = ['scroll', 'scroll2']
const TITLES = ['第一章 起点', '第二章 后邻', '第三章 平稳', '第四章 预载', '第五章 前邻', '第六章 失败', '第七章 后续', '第八章 终点']
const FAIL = 5
const PREVIOUS = 4
// Body the Go backend writes when a source fails to deliver chapter content (books.go chapterContent).
const CONTENT_FAILURE = { error: 'failed to load chapter content', code: 'content_unavailable', stage: 'content' }
const CATALOG_FAILURE = { error: 'failed to list chapters' }

const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))

async function call(path, { token, method = 'GET', body } = {}) {
  const response = await fetch(`${targetUrl}/api${path}`, {
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
  return { status: response.status, data }
}

async function importBook(token, title, chapterCount = TITLES.length) {
  const text = TITLES.slice(0, chapterCount).map(heading => [
    heading,
    ...Array.from({ length: 40 }, (_, index) => `${heading}第${index + 1}段。失败态进度合同正文，用于验证错误页与加载中的进度写入。`),
  ].join('\n')).join('\n')
  const form = new FormData()
  form.append('file', new Blob([text], { type: 'text/plain' }), `${title}.txt`)
  form.append('title', title)
  const response = await fetch(`${targetUrl}/api/imports/books`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: form,
  })
  const body = await response.text()
  if (!response.ok) throw new Error(`import ${title}: ${response.status} ${body}`)
  const book = JSON.parse(body)
  const chapters = (await call(`/books/${book.id}/chapters`, { token })).data
  if (!Array.isArray(chapters) || chapters.length !== chapterCount) {
    throw new Error(`import ${title}: expected ${chapterCount} chapters, got ${JSON.stringify(chapters).slice(0, 200)}`)
  }
  return { ...book, chapters }
}

async function setMode(token, mode) {
  const result = await call('/settings/reader', {
    token,
    method: 'PUT',
    body: {
      value: { mode, pageMode: 'auto', animateDuration: 0, fontSize: 18, lineHeight: 1.8, paragraphSpace: 0.2, autoTheme: false },
      force: true,
    },
  })
  if (result.status !== 200) throw new Error(`reader settings ${mode}: ${result.status} ${JSON.stringify(result.data)}`)
}

async function serverProgress(token, bookId) {
  const result = await call(`/progress/${bookId}`, { token })
  return result.data?.bookId ? result.data : null
}

function describe(progress) {
  if (!progress?.bookId) return 'none'
  return `${progress.chapterIndex}:${progress.chapterId}:${progress.chapterTitle}@${progress.offset}`
}

function describeWrite(row) {
  return `${row.body.bookId}/${row.body.chapterIndex}:${row.body.chapterId}:${row.body.chapterTitle}@${row.body.offset}=>${row.status}`
}

async function openContext(browser, viewport, token, userId, mode) {
  const context = await browser.newContext({
    viewport: { width: viewport.width, height: viewport.height },
    ...(viewport.touch ? { isMobile: true, hasTouch: true } : {}),
  })
  await context.addInitScript(({ tokenValue, readerMode, scope }) => {
    localStorage.setItem('openreader_token', tokenValue)
    if (!localStorage.getItem('reader')) {
      localStorage.setItem('reader', JSON.stringify({ mode: readerMode, settingsScope: scope, progressScope: scope }))
    }
  }, { tokenValue: token, readerMode: mode, scope: `user:${userId}` })
  const page = await context.newPage()
  const log = { writes: [], pageErrors: [], consoleErrors: [], navigations: [] }
  page.on('framenavigated', (frame) => {
    if (frame === page.mainFrame()) log.navigations.push(new URL(frame.url()).pathname)
  })
  page.on('request', (request) => {
    if (request.method() !== 'PUT' || new URL(request.url()).pathname !== '/api/progress') return
    let body = {}
    try {
      body = request.postDataJSON() || {}
    } catch {
      body = {}
    }
    const row = { body, status: null }
    log.writes.push(row)
    request.response()
      .then((response) => {
        row.status = response ? response.status() : 'none'
      })
      .catch(() => {
        row.status = 'failed'
      })
  })
  page.on('pageerror', error => log.pageErrors.push(error.message.split('\n')[0]))
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    const text = message.text().split('\n')[0]
    // The injected content (502) and catalog (500) failures, and E5b's injected 500, are expected resource errors.
    if (/Failed to load resource: the server responded with a status of (500|502)/.test(text)) return
    log.consoleErrors.push(text)
  })
  return { context, page, log }
}

async function routeTo(page, query) {
  await page.evaluate((next) => {
    const router = document.querySelector('#app').__vue_app__.config.globalProperties.$router
    return router.push({ query: { ...router.currentRoute.value.query, ...next } })
  }, query)
}

async function leaveReader(page) {
  try {
    await page.evaluate(() => document.querySelector('#app').__vue_app__.config.globalProperties.$router.push({ name: 'home' }))
  } catch (error) {
    // The red baseline once lost the page context here on 390x844 (a document navigation, logged in `navigations`).
    if (!/Execution context was destroyed/.test(String(error?.message))) throw error
  }
  await page.waitForURL(url => !url.pathname.includes('/read'), { timeout: 10_000 })
}

async function hide(page) {
  await page.evaluate(() => {
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => true })
    Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'hidden' })
    document.dispatchEvent(new Event('visibilitychange'))
  })
  await sleep(1500)
  await page.evaluate(() => {
    delete document.hidden
    delete document.visibilityState
  })
}

async function localProgress(page, bookId) {
  return page.evaluate((id) => {
    const key = Object.keys(localStorage).find(name => name.startsWith('openreader_chapter_progress@') && name.endsWith(`@${id}`))
    if (!key) return null
    try {
      return JSON.parse(localStorage.getItem(key))
    } catch {
      return null
    }
  }, bookId)
}

async function shelfProgress(page, bookId) {
  return page.evaluate((id) => {
    const pinia = document.querySelector('#app').__vue_app__.config.globalProperties.$pinia
    const item = pinia._s.get('bookshelf')?.books.find(row => Number(row.id) === Number(id))
    return item?.progress || null
  }, bookId)
}

async function waitFailure(page, mode) {
  const selector = CONTINUOUS.includes(mode)
    ? `.chapter-content[data-index="${FAIL}"] .chapter-inline-error`
    : '.chapter-load-error'
  await page.waitForSelector(selector, { timeout: 15_000 })
}

async function settleBaseline(page, mode, viewport) {
  if (mode === 'flip' && viewport.touch) {
    await page.keyboard.press('ArrowRight')
    await sleep(500)
    await page.keyboard.press('ArrowRight')
  } else {
    await page.evaluate(() => {
      const scroller = document.scrollingElement
      scroller.scrollTop = Math.min(scroller.scrollHeight, scroller.scrollTop + 500)
      window.dispatchEvent(new Event('scroll'))
    })
  }
  await sleep(2600)
}

async function pressTool(page, viewport, label) {
  if (!viewport.touch) {
    await page.locator(`.reader-left-rail .rail-item[aria-label="${label}"]`).click()
    return
  }
  // The mobile tool layer: DOM click, the checks are about the writes, not the hit testing.
  const pressed = await page.evaluate((text) => {
    const button = [...document.querySelectorAll('.mobile-tool-button')].find(node => node.textContent.includes(text))
    button?.click()
    return Boolean(button)
  }, label)
  if (!pressed) throw new Error(`mobile tool ${label} is missing`)
}

function writeChecks(log, failures, label, { expectedRows = {} } = {}) {
  const refused = log.writes.filter(row => typeof row.status !== 'number' || row.status < 200 || row.status >= 300)
  if (refused.length) failures.push(`${label}: refused progress writes ${JSON.stringify(refused.map(describeWrite))}`)
  for (const row of log.writes) {
    const expected = expectedRows[`${row.body.bookId}:${row.body.chapterIndex}`]
    if (expected && Number(row.body.chapterId) !== Number(expected.id)) {
      failures.push(`${label}: write ${describeWrite(row)} names chapter id ${row.body.chapterId}, row ${row.body.chapterIndex} is ${expected.id}`)
    }
  }
  if (log.pageErrors.length) failures.push(`${label}: page errors ${JSON.stringify(log.pageErrors)}`)
  if (log.consoleErrors.length) failures.push(`${label}: console errors ${JSON.stringify(log.consoleErrors)}`)
}

async function runInBook({ browser, token, userId, viewport, mode, results }) {
  const label = `E1 ${viewport.label} ${mode}`
  if (caseFilter && !caseFilter.test(label)) return
  const failures = []
  await setMode(token, mode)
  const book = await importBook(token, `失败态进度 ${viewport.width} ${mode}`)
  const row = book.chapters[FAIL]
  const { context, page, log } = await openContext(browser, viewport, token, userId, mode)
  let failing = true
  let failRequests = 0
  await page.route(new RegExp(`/api/books/${book.id}/chapters/${FAIL}/content$`), (route) => {
    failRequests += 1
    return failing
      ? route.fulfill({ status: 502, contentType: 'application/json', body: JSON.stringify(CONTENT_FAILURE) })
      : route.continue()
  })
  try {
    await page.goto(`${targetUrl}/books/${book.id}/read?chapter=${PREVIOUS}`, { waitUntil: 'domcontentloaded' })
    await page.waitForSelector(`.chapter-content[data-index="${PREVIOUS}"] [data-reader-block]`, { timeout: 15_000 })
    await sleep(800)
    await settleBaseline(page, mode, viewport)
    const baseline = await serverProgress(token, book.id)
    await routeTo(page, { chapter: String(FAIL) })
    await waitFailure(page, mode)
    await sleep(2500)
    const afterFailure = await serverProgress(token, book.id)
    if (Number(afterFailure?.chapterIndex) !== FAIL || Number(afterFailure?.chapterId) !== Number(row.id)) {
      failures.push(`P2: after the failure the server holds ${describe(afterFailure)} (baseline ${describe(baseline)}), expected row ${FAIL} (${row.id})`)
    }
    await hide(page)
    // P6: a catalog press on the failed current chapter retries it.
    const before = failRequests
    await pressTool(page, viewport, '目录')
    await page.locator('.toc-item.active').first().waitFor({ timeout: 8_000 })
    await page.locator('.toc-item.active').first().click()
    await sleep(2600)
    if (failRequests - before !== 1) failures.push(`P6: a catalog press on the failed chapter made ${failRequests - before} content requests, expected 1`)
    failing = false
    await pressTool(page, viewport, '目录')
    await page.locator('.toc-item.active').first().waitFor({ timeout: 8_000 })
    await page.locator('.toc-item.active').first().click()
    const recovered = await page.waitForSelector(`.chapter-content[data-index="${FAIL}"] [data-reader-block]:not(.chapter-inline-error *)`, { timeout: 8_000 })
      .then(() => true)
      .catch(() => false)
    if (!recovered) failures.push('P6: after the source recovered a catalog press did not show chapter 5')
    await sleep(1500)
    await leaveReader(page)
    await sleep(2500)
    const afterLeave = await serverProgress(token, book.id)
    if (Number(afterLeave?.chapterIndex) !== FAIL || Number(afterLeave?.chapterId) !== Number(row.id)) {
      failures.push(`after leaving the server holds ${describe(afterLeave)}, expected row ${FAIL} (${row.id})`)
    }
    writeChecks(log, failures, 'P1/P7', { expectedRows: { [`${book.id}:${FAIL}`]: row } })
  } catch (error) {
    failures.push(`harness: ${String(error?.message || error).split('\n')[0]}`)
  } finally {
    results.push({ label, failures, writes: log.writes.map(describeWrite), navigations: log.navigations })
    await context.close()
  }
}

async function runFirstLoad({ browser, token, userId, viewport, mode, results }) {
  const label = `E2 ${viewport.label} ${mode}`
  if (caseFilter && !caseFilter.test(label)) return
  const failures = []
  await setMode(token, mode)
  const book = await importBook(token, `首载失败 ${viewport.width} ${mode}`)
  const row = book.chapters[FAIL]
  const { context, page, log } = await openContext(browser, viewport, token, userId, mode)
  await page.route(new RegExp(`/api/books/${book.id}/chapters/${FAIL}/content$`), route => (
    route.fulfill({ status: 502, contentType: 'application/json', body: JSON.stringify(CONTENT_FAILURE) })
  ))
  try {
    await page.goto(`${targetUrl}/books/${book.id}/read?chapter=${FAIL}`, { waitUntil: 'domcontentloaded' })
    await waitFailure(page, mode)
    await sleep(1500)
    await leaveReader(page)
    await sleep(2500)
    const afterLeave = await serverProgress(token, book.id)
    if (Number(afterLeave?.chapterIndex) !== FAIL || Number(afterLeave?.chapterId) !== Number(row.id) || Number(afterLeave?.offset) !== 0) {
      failures.push(`P2: after leaving a failed first load the server holds ${describe(afterLeave)}, expected ${FAIL}:${row.id}@0`)
    }
    writeChecks(log, failures, 'P1/P7', { expectedRows: { [`${book.id}:${FAIL}`]: row } })
  } catch (error) {
    failures.push(`harness: ${String(error?.message || error).split('\n')[0]}`)
  } finally {
    results.push({ label, failures, writes: log.writes.map(describeWrite), navigations: log.navigations })
    await context.close()
  }
}

async function runLoading({ browser, token, userId, viewport, mode, results }) {
  const label = `E3 ${viewport.label} ${mode}`
  if (caseFilter && !caseFilter.test(label)) return
  const failures = []
  await setMode(token, mode)
  const book = await importBook(token, `加载中 ${viewport.width} ${mode}`)
  const row = book.chapters[FAIL]
  const { context, page, log } = await openContext(browser, viewport, token, userId, mode)
  // Every request for chapter 5 (the neighbour preload too) is held, so the routed load is still pending when hidden.
  await page.route(new RegExp(`/api/books/${book.id}/chapters/${FAIL}/content$`), async (route) => {
    await sleep(10_000)
    await route.continue().catch(() => {})
  })
  try {
    await page.goto(`${targetUrl}/books/${book.id}/read?chapter=${PREVIOUS}`, { waitUntil: 'domcontentloaded' })
    await page.waitForSelector(`.chapter-content[data-index="${PREVIOUS}"] [data-reader-block]`, { timeout: 15_000 })
    await sleep(800)
    await settleBaseline(page, mode, viewport)
    await routeTo(page, { chapter: String(FAIL) })
    await sleep(900)
    const loading = await page.evaluate(() => document.body.innerText.includes('正在加载章节'))
    if (!loading) failures.push('harness: chapter 5 was not loading when the page was hidden')
    const mark = log.writes.length
    await hide(page)
    const hiddenWrites = log.writes.slice(mark)
    if (!hiddenWrites.length) failures.push('P3: hiding the page while loading wrote no progress')
    for (const write of hiddenWrites) {
      if (Number(write.body.chapterIndex) !== FAIL || Number(write.body.chapterId) !== Number(row.id) || Number(write.body.offset) !== 0) {
        failures.push(`P3: the loading-state write ${describeWrite(write)} must be ${FAIL}:${row.id}@0`)
      }
    }
    await sleep(11_000)
    writeChecks(log, failures, 'P1/P7', { expectedRows: { [`${book.id}:${FAIL}`]: row } })
  } catch (error) {
    failures.push(`harness: ${String(error?.message || error).split('\n')[0]}`)
  } finally {
    results.push({ label, failures, writes: log.writes.map(describeWrite), navigations: log.navigations })
    await context.close()
  }
}

async function openShelfAndSelect(page, viewport, bookId) {
  await pressTool(page, viewport, '书架')
  await page.locator(`.reader-shelf-card[data-book-id="${bookId}"]`).waitFor({ timeout: 10_000 })
  await page.locator(`.reader-shelf-card[data-book-id="${bookId}"]`).click()
}

async function runCatalogSwitch({ browser, token, userId, viewport, results }) {
  const mode = 'page'
  const label = `E4 ${viewport.label} ${mode}`
  if (caseFilter && !caseFilter.test(label)) return
  const failures = []
  await setMode(token, mode)
  const book = await importBook(token, `目录切换 ${viewport.width}`)
  const other = await importBook(token, `目录切换目标 ${viewport.width}`, 3)
  const { context, page, log } = await openContext(browser, viewport, token, userId, mode)
  let catalogFailing = true
  await page.route(new RegExp(`/api/books/${other.id}/chapters$`), route => (
    catalogFailing
      ? route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify(CATALOG_FAILURE) })
      : route.continue()
  ))
  try {
    await page.goto(`${targetUrl}/books/${book.id}/read?chapter=${PREVIOUS}`, { waitUntil: 'domcontentloaded' })
    await page.waitForSelector(`.chapter-content[data-index="${PREVIOUS}"] [data-reader-block]`, { timeout: 15_000 })
    await sleep(800)
    await settleBaseline(page, mode, viewport)
    await openShelfAndSelect(page, viewport, other.id)
    await page.waitForSelector('.chapter-load-error', { timeout: 15_000 })
    await sleep(2500)
    await hide(page)
    const otherWrites = log.writes.filter(write => Number(write.body.bookId) === Number(other.id))
    if (otherWrites.length) failures.push(`P4: wrote progress for the book whose catalog failed: ${JSON.stringify(otherWrites.map(describeWrite))}`)
    const previousTitles = new Set(book.chapters.map(chapter => chapter.title))
    const local = await localProgress(page, other.id)
    if (local && previousTitles.has(local.chapterTitle)) failures.push(`P4: the new book's local snapshot took the previous book's chapter ${JSON.stringify(local)}`)
    const shelf = await shelfProgress(page, other.id)
    if (shelf && previousTitles.has(shelf.chapterTitle)) failures.push(`P4: the new book's shelf card took the previous book's chapter ${JSON.stringify(shelf)}`)
    catalogFailing = false
    await page.reload({ waitUntil: 'domcontentloaded' })
    await page.waitForSelector('.reader-content .chapter-content [data-reader-block]', { timeout: 15_000 })
    await sleep(1500)
    const opened = await page.evaluate(() => {
      const chapter = document.querySelector('.reader-content .chapter-content')
      return { index: chapter?.dataset.index, scrollTop: Math.round(document.scrollingElement.scrollTop) }
    })
    if (opened.index !== '0' || opened.scrollTop > 40) {
      failures.push(`P4: after recovery the book opened at chapter ${opened.index} scrollTop ${opened.scrollTop}, expected its own start`)
    }
    writeChecks(log, failures, 'P1/P7')
  } catch (error) {
    failures.push(`harness: ${String(error?.message || error).split('\n')[0]}`)
  } finally {
    results.push({ label, failures, writes: log.writes.map(describeWrite), navigations: log.navigations })
    await context.close()
  }
}

async function runShelfSwitch({ browser, token, userId, viewport, results, failProgress = false }) {
  const mode = 'page'
  const label = `${failProgress ? 'E5b' : 'E5'} ${viewport.label} ${mode}`
  if (caseFilter && !caseFilter.test(label)) return
  const failures = []
  await setMode(token, mode)
  const book = await importBook(token, `书架切换 ${viewport.width} ${failProgress ? 'b' : 'a'}`)
  const other = await importBook(token, `书架切换目标 ${viewport.width} ${failProgress ? 'b' : 'a'}`, 3)
  const { context, page, log } = await openContext(browser, viewport, token, userId, mode)
  await page.route(new RegExp(`/api/books/${book.id}/chapters/${FAIL}/content$`), route => (
    route.fulfill({ status: 502, contentType: 'application/json', body: JSON.stringify(CONTENT_FAILURE) })
  ))
  try {
    await page.goto(`${targetUrl}/books/${book.id}/read?chapter=${PREVIOUS}`, { waitUntil: 'domcontentloaded' })
    await page.waitForSelector(`.chapter-content[data-index="${PREVIOUS}"] [data-reader-block]`, { timeout: 15_000 })
    await sleep(800)
    await routeTo(page, { chapter: String(FAIL) })
    await waitFailure(page, mode)
    await sleep(1500)
    if (failProgress) {
      await page.route(/\/api\/progress$/, route => (
        route.request().method() === 'PUT'
          ? route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'failed to save progress' }) })
          : route.continue()
      ))
    }
    await openShelfAndSelect(page, viewport, other.id)
    const switched = await page.waitForURL(url => url.pathname === `/books/${other.id}/read`, { timeout: 8_000 })
      .then(() => true)
      .catch(() => false)
    if (!switched) failures.push(`P5: the shelf switch stayed on ${await page.evaluate(() => location.pathname + location.search)}`)
    await sleep(1500)
    if (failProgress) {
      log.writes = log.writes.filter(write => write.status !== 500)
      log.consoleErrors = log.consoleErrors.filter(text => !/status of 500/.test(text))
    }
    writeChecks(log, failures, 'P1/P7', { expectedRows: { [`${book.id}:${FAIL}`]: book.chapters[FAIL] } })
  } catch (error) {
    failures.push(`harness: ${String(error?.message || error).split('\n')[0]}`)
  } finally {
    results.push({ label, failures, writes: log.writes.map(describeWrite), navigations: log.navigations })
    await context.close()
  }
}

const registered = await call('/auth/register', {
  method: 'POST',
  body: { username: `failprogress${Date.now() % 1_000_000}`, password: 'failure-progress-contract' },
})
if (!registered.data?.token) throw new Error(`register: ${registered.status} ${JSON.stringify(registered.data)}`)
const token = registered.data.token
const userId = (await call('/me', { token })).data?.id
const browser = await openSmokeBrowser()
const results = []
try {
  for (const viewport of VIEWPORTS) {
    for (const mode of MODES) await runInBook({ browser, token, userId, viewport, mode, results })
    for (const mode of ['page', 'scroll']) await runFirstLoad({ browser, token, userId, viewport, mode, results })
    for (const mode of ['page', 'scroll']) await runLoading({ browser, token, userId, viewport, mode, results })
    await runCatalogSwitch({ browser, token, userId, viewport, results })
    await runShelfSwitch({ browser, token, userId, viewport, results })
    await runShelfSwitch({ browser, token, userId, viewport, results, failProgress: true })
  }
} finally {
  await browser.close()
}

let failed = 0
for (const result of results) {
  if (result.failures.length) {
    failed += 1
    console.log(`FAIL ${result.label}`)
    for (const failure of result.failures) console.log(`  - ${failure}`)
    console.log(`  writes ${JSON.stringify(result.writes)}`)
    console.log(`  navigations ${JSON.stringify(result.navigations)}`)
  } else {
    console.log(`PASS ${result.label}`)
  }
}
console.log(`${results.length - failed}/${results.length} cases passed`)
if (failed) process.exit(1)
