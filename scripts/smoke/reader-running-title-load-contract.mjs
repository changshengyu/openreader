#!/usr/bin/env node

// Reader running title during loads and failures
// (docs/compat/reader-running-title-load-failure-fixed-baseline-second-audit-p0-contract.md section 8.2).
// An init script records every value `.reader-running-chapter-title` takes with a MutationObserver, including values
// that exist for one DOM update only. Every non-empty value must be `第 k 章 <catalog row k-1 title>` of the book being
// read, and each step checks the label while the target loads, after a failure and during a retry:
// - S1 下一章 to a chapter whose content is delayed; S2 a 500 chapter; S3 a slow chapter replaced by a cached one before
//   its response arrives late; S4 重新加载 on the failed chapter while its retry is delayed (all four modes);
// - T1/T2 the catalog panel to a slow chapter and to a failing chapter (page);
// - B1/B2 a Reader book switch to a book that is not on the shelf, whose catalog and target content are delayed (B2:
//   the target fails): only the old book's full label, an empty label or the new target may show;
// - F1 first entry without a chapter parameter, saved progress on row 3, catalog and content delayed: empty until the
//   catalog arrives, then the target, never a guessed `第 1 章`.
// Every case runs to the end and reports all of its failed checks; SMOKE_CASES=<regex> limits the cases.

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = (process.env.TARGET_URL || 'http://127.0.0.1:5173').replace(/\/$/, '')
const caseFilter = process.env.SMOKE_CASES ? new RegExp(process.env.SMOKE_CASES) : null

const VIEWPORTS = [
  { width: 1440, height: 900 },
  { width: 390, height: 844 },
  { width: 360, height: 800 },
]
const MODES = ['page', 'flip', 'scroll', 'scroll2']
const CATALOGS = {
  1: [
    '第一章 起点',
    '第二章 延迟',
    '第三章 缓存',
    '第四章 预载',
    '第五章 失败',
    '第六章 迟到',
    '第七章 取代',
    '第八章 终点',
  ],
  11: ['甲一 起点', '甲二 平稳', '甲三 当前', '甲四 预载', '甲五 终点'],
  12: ['乙一 起点', '乙二 平稳', '乙三 平稳', '乙四 目标', '乙五 终点'],
}
const BOOK_TITLES = { 1: '运行标题加载契约', 11: '运行标题书一', 12: '运行标题书二' }
const FAILURE = { error: 'failed to load chapter content', code: 'content_unavailable', stage: 'content' }

const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))

function json(data, status = 200) {
  return { status, contentType: 'application/json', body: JSON.stringify(data) }
}

function fakeToken() {
  const payload = Buffer.from(JSON.stringify({ userId: 1, sub: '1' })).toString('base64url')
  return `open.${payload}.reader`
}

function label(bookId, index) {
  return `第 ${index + 1} 章 ${CATALOGS[bookId][index]}`
}

function contentFor(bookId, index) {
  return Array.from({ length: 40 }, (_, paragraph) => (
    `${CATALOGS[bookId][index]}第${paragraph + 1}段。运行标题加载契约正文，用于验证加载中与失败时的章节名。`
  )).join('\n')
}

const isTouch = viewport => viewport.width <= 750

function newBehavior() {
  return {
    delay: new Map(),
    fail: new Map(),
    catalogDelay: new Map(),
    savedProgress: new Map(),
    requests: [],
    responses: [],
    routeErrors: [],
  }
}

const key = (bookId, index) => `${bookId}:${index}`

async function installMocks(page, behavior, mode) {
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    try {
      if (path === '/me') return await route.fulfill(json({ id: 1, username: 'smoke', role: 'admin' }))
      if (path === '/settings/reader' && method === 'GET') {
        return await route.fulfill(json({
          key: 'reader',
          updatedAt: '2026-11-12T00:00:00Z',
          value: {
            mode,
            pageMode: 'auto',
            autoTheme: false,
            fontSize: 18,
            lineHeight: 1.8,
            paragraphSpace: 0.2,
            columnWidth: 800,
            animateDuration: 0,
          },
        }))
      }
      if (path === '/settings/reader' && method === 'PUT') {
        return await route.fulfill(json({ key: 'reader', updatedAt: '2026-11-12T00:00:01Z', value: {} }))
      }
      // Nothing is on the shelf: a Reader book switch only learns the new book from its own request.
      if (path === '/books' && method === 'GET') return await route.fulfill(json([]))
      const bookMatch = path.match(/^\/books\/(\d+)$/)
      if (bookMatch && CATALOGS[bookMatch[1]]) {
        const bookId = Number(bookMatch[1])
        return await route.fulfill(json({
          id: bookId,
          title: BOOK_TITLES[bookId],
          author: 'OpenReader',
          sourceId: 0,
          chapterCount: CATALOGS[bookId].length,
          progress: null,
        }))
      }
      const catalogMatch = path.match(/^\/books\/(\d+)\/chapters$/)
      if (catalogMatch && CATALOGS[catalogMatch[1]]) {
        const bookId = Number(catalogMatch[1])
        const wait = behavior.catalogDelay.get(bookId) || 0
        if (wait) await sleep(wait)
        return await route.fulfill(json(CATALOGS[bookId].map((title, index) => ({ id: bookId * 100 + index, bookId, index, title }))))
      }
      const contentMatch = path.match(/^\/books\/(\d+)\/chapters\/(\d+)\/content$/)
      if (contentMatch && CATALOGS[contentMatch[1]]) {
        const bookId = Number(contentMatch[1])
        const index = Number(contentMatch[2])
        behavior.requests.push({ bookId, index, at: Date.now() })
        const wait = behavior.delay.get(key(bookId, index)) || 0
        if (wait) await sleep(wait)
        if (behavior.fail.has(key(bookId, index))) {
          behavior.responses.push({ bookId, index, status: 502, at: Date.now() })
          return await route.fulfill(json(FAILURE, 502))
        }
        behavior.responses.push({ bookId, index, status: 200, at: Date.now() })
        return await route.fulfill(json({
          chapter: { id: bookId * 100 + index, bookId, index, title: CATALOGS[bookId][index] },
          content: contentFor(bookId, index),
          format: 'text',
        }))
      }
      const progressMatch = path.match(/^\/progress\/(\d+)$/)
      if (progressMatch) return await route.fulfill(json(behavior.savedProgress.get(Number(progressMatch[1])) || {}))
      if (path === '/progress' && method === 'PUT') {
        const body = request.postDataJSON?.() || {}
        return await route.fulfill(json({ ...body, updatedAt: new Date().toISOString() }))
      }
      if (/^\/books\/\d+\/bookmarks$/.test(path)) return await route.fulfill(json([]))
      if (path === '/sources' || path === '/categories') return await route.fulfill(json([]))
      return await route.fulfill(json({}))
    } catch (error) {
      behavior.routeErrors.push(String(error?.message || error).split('\n')[0])
    }
  })
}

async function openReader(browser, viewport, mode, behavior, path, waitFor) {
  const context = await browser.newContext(isTouch(viewport)
    ? { viewport, isMobile: true, hasTouch: true }
    : { viewport })
  await context.addInitScript(({ readerMode, token }) => {
    localStorage.setItem('openreader_token', token)
    localStorage.setItem('reader', JSON.stringify({ mode: readerMode, settingsScope: 'user:1', progressScope: 'user:1' }))
    window.__runningTitleLog = []
    const read = () => {
      const node = document.querySelector('.reader-running-chapter-title')
      return node ? node.textContent.trim() : null
    }
    let last
    const record = () => {
      const value = read()
      if (value === null || value === last) return
      last = value
      window.__runningTitleLog.push({ value, at: Date.now() })
    }
    new MutationObserver(record).observe(document, { childList: true, subtree: true, characterData: true })
  }, { readerMode: mode, token: fakeToken() })
  const page = await context.newPage()
  const pageErrors = []
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    const text = message.text()
    if (text.includes('/ws/sync')) return
    if (text.includes('Failed to load resource: the server responded with a status of 502')) return
    pageErrors.push(text.split('\n')[0])
  })
  page.on('pageerror', error => pageErrors.push(error.message))
  await installMocks(page, behavior, mode)
  await page.goto(`${targetUrl}${path}`, { waitUntil: 'domcontentloaded' })
  if (waitFor) await page.waitForSelector(waitFor, { timeout: 15_000 })
  return { context, page, pageErrors }
}

async function routeTo(page, location) {
  await page.evaluate((target) => {
    const router = document.querySelector('#app').__vue_app__.config.globalProperties.$router
    return router.push(target)
  }, location)
}

const routeToChapter = (page, index) => page.evaluate((target) => {
  const router = document.querySelector('#app').__vue_app__.config.globalProperties.$router
  return router.push({ query: { ...router.currentRoute.value.query, chapter: String(target) } })
}, index)

async function runningTitle(page) {
  return page.evaluate(() => document.querySelector('.reader-running-chapter-title')?.textContent.trim() ?? null)
}

async function titleLog(page, since = 0) {
  return page.evaluate(mark => (window.__runningTitleLog || []).filter(row => row.at >= mark).map(row => row.value), since)
}

async function readerSnapshot(page) {
  return page.evaluate(() => ({
    loading: (document.querySelector('.reader-body')?.textContent || '').includes('正在加载章节'),
    heading: document.querySelector('.reader-body .chapter-content h3, .reader-body .chapter-load-error h3')?.textContent.trim() ?? null,
    error: Boolean(document.querySelector('.chapter-load-error, .chapter-inline-error')),
  }))
}

function check(failures, condition, message) {
  if (!condition) failures.push(message)
}

// Every non-empty value pairs an ordinal with that row's own title in the book's catalog.
function checkRows(failures, step, values, bookIds) {
  const valid = new Set(bookIds.flatMap(bookId => CATALOGS[bookId].map((_, index) => label(bookId, index))))
  const stray = values.filter(value => value && !valid.has(value))
  check(failures, stray.length === 0, `${step} ordinal and chapter name from different rows: ${JSON.stringify(stray)}`)
}

async function waitUntil(predicate, timeout) {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) {
    if (await predicate()) return true
    await sleep(50)
  }
  return predicate()
}

async function pressNextChapter(page, viewport) {
  if (isTouch(viewport)) {
    await page.locator('.reader-mobile-bottom .mobile-chapter-step', { hasText: '下一章' }).tap()
  } else {
    await page.locator('.reader-page-control button[aria-label="下一章"]').click()
  }
}

async function pressRetry(page, viewport, index) {
  const button = page.locator(`.chapter-load-error button, .chapter-content[data-index="${index}"] .chapter-inline-error button`).first()
  if (isTouch(viewport)) await button.tap()
  else await button.click()
}

async function finish(context, pageErrors, behavior, failures) {
  failures.push(...pageErrors.map(text => `page error: ${text}`))
  failures.push(...behavior.routeErrors.map(text => `route error: ${text}`))
  await context.close()
}

async function runStepsCase(browser, viewport, mode) {
  const behavior = newBehavior()
  const failures = []
  const notes = []
  behavior.delay.set(key(1, 1), 3000)
  const { context, page, pageErrors } = await openReader(
    browser, viewport, mode, behavior, '/books/1/read?chapter=0',
    '.chapter-content[data-index="0"] [data-reader-block]',
  )
  try {
    await page.waitForTimeout(500)
    check(failures, await runningTitle(page) === label(1, 0), `S0 ${await runningTitle(page)}`)

    // S1: 下一章 into a chapter whose content (and preload) is delayed.
    let mark = Date.now()
    await pressNextChapter(page, viewport)
    await page.waitForTimeout(700)
    const s1 = await readerSnapshot(page)
    const s1Title = await runningTitle(page)
    notes.push(`S1 loading=${s1.loading} ${JSON.stringify(s1Title)}`)
    check(failures, s1.loading, 'S1 the delayed chapter must still be loading at 700ms')
    check(failures, s1Title === label(1, 1), `S1 while loading ${JSON.stringify(s1Title)} (want ${label(1, 1)})`)
    await waitUntil(async () => (await readerSnapshot(page)).heading === CATALOGS[1][1], 6000)
    check(failures, await runningTitle(page) === label(1, 1), `S1 after loading ${await runningTitle(page)}`)
    checkRows(failures, 'S1', await titleLog(page, mark), [1])

    // S2: a failing chapter keeps its own label.
    behavior.fail.set(key(1, 4), true)
    behavior.delay.set(key(1, 5), 4000)
    mark = Date.now()
    await routeToChapter(page, 4)
    await waitUntil(async () => (await readerSnapshot(page)).error, 6000)
    const s2Now = await runningTitle(page)
    await page.waitForTimeout(1500)
    const s2Later = await runningTitle(page)
    notes.push(`S2 ${JSON.stringify(s2Now)} -> ${JSON.stringify(s2Later)}`)
    check(failures, s2Now === label(1, 4) && s2Later === label(1, 4), `S2 failed chapter ${JSON.stringify(s2Now)} / ${JSON.stringify(s2Later)} (want ${label(1, 4)})`)
    checkRows(failures, 'S2', await titleLog(page, mark), [1])

    // S3: a slow chapter, then a cached one before the slow response arrives.
    mark = Date.now()
    await routeToChapter(page, 5)
    await page.waitForTimeout(600)
    const s3Loading = await runningTitle(page)
    await routeToChapter(page, 2)
    await waitUntil(async () => (await readerSnapshot(page)).heading === CATALOGS[1][2], 4000)
    await waitUntil(() => behavior.responses.some(row => row.bookId === 1 && row.index === 5 && row.at >= mark), 6000)
    await page.waitForTimeout(500)
    const s3Final = await runningTitle(page)
    const s3Heading = (await readerSnapshot(page)).heading
    notes.push(`S3 ${JSON.stringify(s3Loading)} -> ${JSON.stringify(s3Final)} heading ${JSON.stringify(s3Heading)}`)
    check(failures, s3Loading === label(1, 5), `S3 while loading ${JSON.stringify(s3Loading)} (want ${label(1, 5)})`)
    check(failures, s3Final === label(1, 2), `S3 after the late response ${JSON.stringify(s3Final)} (want ${label(1, 2)})`)
    check(failures, s3Heading === CATALOGS[1][2], `S3 the late response replaced the content heading ${JSON.stringify(s3Heading)}`)
    checkRows(failures, 'S3', await titleLog(page, mark), [1])

    // S4: 重新加载 on the failed chapter while its retry is delayed.
    behavior.delay.delete(key(1, 5))
    mark = Date.now()
    await routeToChapter(page, 4)
    await waitUntil(async () => (await readerSnapshot(page)).error, 6000)
    await page.waitForTimeout(600)
    behavior.fail.delete(key(1, 4))
    behavior.delay.set(key(1, 4), 1500)
    await pressRetry(page, viewport, 4)
    await page.waitForTimeout(500)
    const s4Reloading = await runningTitle(page)
    await waitUntil(async () => (await readerSnapshot(page)).heading === CATALOGS[1][4] && !(await readerSnapshot(page)).error, 6000)
    const s4Final = await runningTitle(page)
    notes.push(`S4 ${JSON.stringify(s4Reloading)} -> ${JSON.stringify(s4Final)}`)
    check(failures, s4Reloading === label(1, 4), `S4 while reloading ${JSON.stringify(s4Reloading)} (want ${label(1, 4)})`)
    check(failures, s4Final === label(1, 4), `S4 after reloading ${JSON.stringify(s4Final)}`)
    checkRows(failures, 'S4', await titleLog(page, mark), [1])
  } catch (error) {
    failures.push(`harness: ${String(error?.message || error).split('\n')[0]}`)
  } finally {
    await finish(context, pageErrors, behavior, failures)
  }
  return { failures, notes }
}

async function openToc(page, viewport) {
  if (isTouch(viewport)) {
    if (!await page.locator('.reader-mobile-top.visible').count()) {
      await page.touchscreen.tap(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
      await page.waitForSelector('.reader-mobile-top.visible', { timeout: 3000 })
    }
    await page.locator('.reader-mobile-top .mobile-tool-button', { hasText: '目录' }).tap()
  } else {
    await page.locator('.reader-left-rail button[aria-label="目录"]').click()
  }
  await page.waitForSelector('.toc-item[data-chapter-index="0"]', { timeout: 5000 })
}

async function jumpFromToc(page, viewport, index) {
  const item = page.locator(`.toc-item[data-chapter-index="${index}"]`)
  await item.scrollIntoViewIfNeeded()
  if (isTouch(viewport)) await item.tap()
  else await item.click()
}

async function runTocCase(browser, viewport) {
  const behavior = newBehavior()
  const failures = []
  const notes = []
  behavior.delay.set(key(1, 5), 2500)
  behavior.fail.set(key(1, 4), true)
  const { context, page, pageErrors } = await openReader(
    browser, viewport, 'page', behavior, '/books/1/read?chapter=0',
    '.chapter-content[data-index="0"] [data-reader-block]',
  )
  try {
    await page.waitForTimeout(500)
    let mark = Date.now()
    await openToc(page, viewport)
    await jumpFromToc(page, viewport, 5)
    await page.waitForTimeout(700)
    const t1 = await runningTitle(page)
    const t1Loading = (await readerSnapshot(page)).loading
    notes.push(`T1 loading=${t1Loading} ${JSON.stringify(t1)}`)
    check(failures, t1Loading, 'T1 the delayed chapter must still be loading at 700ms')
    check(failures, t1 === label(1, 5), `T1 while loading ${JSON.stringify(t1)} (want ${label(1, 5)})`)
    await waitUntil(async () => (await readerSnapshot(page)).heading === CATALOGS[1][5], 5000)
    checkRows(failures, 'T1', await titleLog(page, mark), [1])

    mark = Date.now()
    await openToc(page, viewport)
    await jumpFromToc(page, viewport, 4)
    await waitUntil(async () => (await readerSnapshot(page)).error, 6000)
    await page.waitForTimeout(1500)
    const t2 = await runningTitle(page)
    notes.push(`T2 ${JSON.stringify(t2)}`)
    check(failures, t2 === label(1, 4), `T2 failed chapter ${JSON.stringify(t2)} (want ${label(1, 4)})`)
    checkRows(failures, 'T2', await titleLog(page, mark), [1])
  } catch (error) {
    failures.push(`harness: ${String(error?.message || error).split('\n')[0]}`)
  } finally {
    await finish(context, pageErrors, behavior, failures)
  }
  return { failures, notes }
}

async function runBookSwitchCase(browser, viewport, targetFails) {
  const behavior = newBehavior()
  const failures = []
  const notes = []
  const { context, page, pageErrors } = await openReader(
    browser, viewport, 'page', behavior, '/books/11/read?chapter=2',
    '.chapter-content[data-index="2"] [data-reader-block]',
  )
  try {
    await page.waitForTimeout(600)
    const before = await runningTitle(page)
    check(failures, before === label(11, 2), `B before the switch ${JSON.stringify(before)}`)
    behavior.catalogDelay.set(12, 1500)
    behavior.delay.set(key(12, 3), 1500)
    if (targetFails) behavior.fail.set(key(12, 3), true)
    const mark = Date.now()
    await routeTo(page, { name: 'reader', params: { id: 12 }, query: { resume: '1', chapter: '3' } })
    await waitUntil(async () => {
      const snapshot = await readerSnapshot(page)
      return targetFails ? snapshot.error : snapshot.heading === CATALOGS[12][3]
    }, 8000)
    await page.waitForTimeout(1500)
    const values = await titleLog(page, mark)
    const final = await runningTitle(page)
    notes.push(`log ${JSON.stringify(values)} final ${JSON.stringify(final)}`)
    const allowed = new Set([label(11, 2), '', label(12, 3)])
    const shown = values.filter(value => !allowed.has(value))
    check(failures, shown.length === 0, `B only the old book's full label, an empty label or the new target may show: ${JSON.stringify(shown)}`)
    check(failures, final === label(12, 3), `B final ${JSON.stringify(final)} (want ${label(12, 3)})`)
    const early = behavior.requests.filter(row => row.bookId === 12 && row.at < mark + 1400)
    check(failures, early.length === 0, `B the new book requested content before its catalog arrived: ${JSON.stringify(early.map(row => row.index))}`)
  } catch (error) {
    failures.push(`harness: ${String(error?.message || error).split('\n')[0]}`)
  } finally {
    await finish(context, pageErrors, behavior, failures)
  }
  return { failures, notes }
}

async function runFirstEntryCase(browser, viewport) {
  const behavior = newBehavior()
  const failures = []
  const notes = []
  behavior.catalogDelay.set(11, 1500)
  behavior.delay.set(key(11, 3), 1500)
  behavior.savedProgress.set(11, {
    bookId: 11,
    chapterIndex: 3,
    chapterId: 1103,
    chapterTitle: CATALOGS[11][3],
    offset: 0,
    chapterPercent: 0,
    percent: 0.6,
    updatedAt: '2026-11-12T00:00:00Z',
  })
  const { context, page, pageErrors } = await openReader(browser, viewport, 'page', behavior, '/books/11/read', null)
  try {
    await waitUntil(async () => (await readerSnapshot(page)).heading === CATALOGS[11][3], 9000)
    await page.waitForTimeout(800)
    const values = await titleLog(page)
    const final = await runningTitle(page)
    notes.push(`log ${JSON.stringify(values)} final ${JSON.stringify(final)}`)
    const guessed = values.filter(value => value && value !== label(11, 3))
    check(failures, guessed.length === 0, `F1 a guessed or partial label showed before the catalog: ${JSON.stringify(guessed)}`)
    check(failures, final === label(11, 3), `F1 final ${JSON.stringify(final)} (want ${label(11, 3)})`)
  } catch (error) {
    failures.push(`harness: ${String(error?.message || error).split('\n')[0]}`)
  } finally {
    await finish(context, pageErrors, behavior, failures)
  }
  return { failures, notes }
}

function plannedCases() {
  const cases = []
  for (const viewport of VIEWPORTS) {
    const size = `${viewport.width}x${viewport.height}`
    for (const mode of MODES) {
      cases.push({ label: `S ${size} ${mode}`, run: browser => runStepsCase(browser, viewport, mode) })
    }
    cases.push({ label: `T ${size} page`, run: browser => runTocCase(browser, viewport) })
    cases.push({ label: `B1 ${size} page`, run: browser => runBookSwitchCase(browser, viewport, false) })
    cases.push({ label: `B2 ${size} page`, run: browser => runBookSwitchCase(browser, viewport, true) })
    cases.push({ label: `F1 ${size} page`, run: browser => runFirstEntryCase(browser, viewport) })
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
      const status = result.failures.length ? 'FAIL' : 'PASS'
      console.log(`${status} ${row.label} | ${result.notes.join(' | ')}`)
      for (const failure of result.failures) console.log(`  - ${failure}`)
      if (result.failures.length) failed.push(row.label)
    }
  } finally {
    await browser.close()
  }
  if (failed.length) {
    throw new Error(`reader running title load contract failed ${failed.length}/${cases.length} cases:\n${failed.join('\n')}`)
  }
  console.log(`reader running title load contract smoke passed (${cases.length} cases)`)
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})
