#!/usr/bin/env node

import { pathToFileURL } from 'node:url'
import { openSmokeBrowser } from './playwright-runtime.mjs'

// Contract: docs/compat/reader-shelf-switch-stale-catalog-fixed-baseline-second-audit-p1-contract.md 8.2(1), R1-R5.
// Every switch goes through the Reader shelf panel while the target book's catalog response is held, so the window
// between the route change and the new catalog is observable. Failures are collected per run so one run records
// every failing point for a viewport and mode.

const targetUrl = (process.env.TARGET_URL || 'http://127.0.0.1:5173').replace(/\/$/, '')
const catalogHoldMs = Number(process.env.SMOKE_CATALOG_HOLD_MS || 1500)
const savedAt = '2026-10-01T00:00:00Z'

const books = {
  1: { id: 1, title: '换书契约一', author: '换书作者', sourceId: 0, chapterCount: 3, categoryIds: [], progress: null },
  3: {
    id: 3,
    title: '换书契约三',
    author: '换书作者',
    sourceId: 0,
    chapterCount: 12,
    categoryIds: [],
    progress: { bookId: 3, chapterId: 309, chapterIndex: 9, offset: 300, chapterPercent: 0.5, percent: 9.5 / 12, updatedAt: savedAt },
  },
  4: { id: 4, title: '换书契约四', author: '换书作者', sourceId: 0, chapterCount: 2, categoryIds: [], progress: null },
  5: {
    id: 5,
    title: '换书契约五',
    author: '换书作者',
    sourceId: 0,
    chapterCount: 6,
    categoryIds: [],
    progress: { bookId: 5, chapterId: 501, chapterIndex: 1, offset: 0, chapterPercent: 0, percent: 1 / 6, updatedAt: savedAt },
  },
  // Book 6: the shelf row has no chapterCount and its old/WebDAV progress has only a book-level percent. Borrowing the
  // previous book's 6 chapters fabricates 0.75 * 6 - 4 = 0.5; its own 10 chapters give 3.5, which is not a fraction.
  6: {
    id: 6,
    title: '换书契约六',
    author: '换书作者',
    sourceId: 0,
    categoryIds: [],
    progress: { bookId: 6, chapterId: 604, chapterIndex: 4, offset: 0, percent: 0.75, updatedAt: savedAt },
  },
}
const catalogs = {
  1: ['甲一 起点', '甲二 中途', '甲三 当前'],
  3: Array.from({ length: 12 }, (_, index) => `乙${index + 1}`),
  4: ['丁一', '丁二'],
  5: ['戊一', '戊二 首次失败', '戊三', '戊四', '戊五', '戊六'],
  6: Array.from({ length: 10 }, (_, index) => `己${index + 1}`),
}
// The first request for book 5 chapter 2 fails, whichever load sends it.
const failOnce = { bookId: 5, index: 1, status: 502 }
const readerSettings = {
  pageMode: 'auto',
  theme: 'parchment',
  themeType: 'day',
  autoTheme: false,
  fontSize: 18,
  fontWeight: 400,
  lineHeight: 1.8,
  paragraphSpace: 0.2,
  columnWidth: 800,
  animateDuration: 0,
}

function json(data, status = 200) {
  return { status, contentType: 'application/json', body: JSON.stringify(data) }
}

function tokenFor(nonce) {
  const payload = Buffer.from(JSON.stringify({ userId: 1, sub: '1' })).toString('base64url')
  return `open.${payload}.${nonce}`
}

function listFromEnv(name, fallback) {
  return String(process.env[name] || fallback).split(',').map(value => value.trim()).filter(Boolean)
}

function smokeViewports() {
  return listFromEnv('SMOKE_VIEWPORTS', '1440x900,390x844,360x800').map((value) => {
    const [width, height] = value.toLowerCase().split('x').map(Number)
    if (!Number.isInteger(width) || !Number.isInteger(height) || width < 1 || height < 1) {
      throw new Error(`Invalid SMOKE_VIEWPORTS entry: ${value}`)
    }
    return { width, height }
  })
}

function smokeModes() {
  const modes = listFromEnv('SMOKE_SHELF_MODES', 'page,scroll2')
  for (const mode of modes) {
    if (!['page', 'flip', 'scroll', 'scroll2'].includes(mode)) throw new Error(`Invalid SMOKE_SHELF_MODES entry: ${mode}`)
  }
  return modes
}

function catalogRows(bookId) {
  // Real /books/:id/chapters rows carry bookId (models.Chapter.BookID).
  return catalogs[bookId].map((title, index) => ({ id: bookId * 100 + index, bookId, index, title }))
}

function chapterContent(bookId, index) {
  // Each paragraph carries a book marker so the content area shows whose text is visible.
  return Array.from({ length: 14 }, (_, paragraph) => (
    `【书${bookId}】${books[bookId].title}第${index + 1}章第${paragraph + 1}段。换书契约正文，用来确认换书窗口里正文区显示的是谁的内容。`
  )).join('\n')
}

function createGate() {
  let release
  let markArrived
  const opened = new Promise(resolve => { release = resolve })
  const arrived = new Promise(resolve => { markArrived = resolve })
  return { arrived, markArrived, opened, release }
}

function withTimeout(promise, ms, message) {
  let timer
  return Promise.race([
    promise.finally(() => clearTimeout(timer)),
    new Promise((_, reject) => {
      timer = setTimeout(() => reject(new Error(message)), ms)
    }),
  ])
}

function installReaderObservers() {
  // Content-area states, deduplicated, with page timestamps.
  const states = []
  window.__shelfSwitchStates = states
  let lastKey = ''
  window.__shelfSwitchMessages = []
  const seenMessages = new WeakSet()
  const snapshot = () => {
    const body = document.querySelector('.reader-body')
    const text = body ? body.innerText : ''
    const progress = (
      document.querySelector('.reader-page-control .progress-box')?.textContent
      ?? document.querySelector('.mobile-chapter-progress')?.textContent
      ?? ''
    ).replace(/^\s*阅读进度:\s*/, '').trim()
    return {
      path: location.pathname,
      loading: text.includes('正在加载章节'),
      error: Boolean(document.querySelector('.chapter-load-error, .chapter-inline-error')),
      markers: [...new Set([...text.matchAll(/【书(\d+)】/g)].map(match => Number(match[1])))],
      progress,
    }
  }
  window.__shelfSwitchSnapshot = snapshot
  const record = () => {
    for (const message of document.querySelectorAll('.el-message')) {
      if (seenMessages.has(message) || message.querySelector('.el-message__content')?.textContent !== '换书成功') continue
      seenMessages.add(message)
      window.__shelfSwitchMessages.push({ path: location.pathname, info: message.classList.contains('el-message--info') })
    }
    const state = snapshot()
    const key = JSON.stringify(state)
    if (key === lastKey) return
    lastKey = key
    states.push({ t: performance.now(), ...state })
  }
  new MutationObserver(record).observe(document, { childList: true, subtree: true, characterData: true })

  // Page-side request start times: the catalog request starts after the switch render (IndexedDB miss first).
  const requests = []
  window.__shelfSwitchRequests = requests
  const open = XMLHttpRequest.prototype.open
  XMLHttpRequest.prototype.open = function patchedOpen(method, url, ...rest) {
    requests.push({ t: performance.now(), method: String(method).toUpperCase(), url: String(url) })
    return open.call(this, method, url, ...rest)
  }
}

async function installApiMocks(page, state) {
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()

    if (path === '/me') return route.fulfill(json(state.user))
    if (path === '/health') return route.fulfill(json({ version: 'shelf-switch-smoke', commit: 'shelf-switch-smoke' }))
    if (path.startsWith('/settings/') && method === 'GET') {
      const key = path.slice('/settings/'.length)
      return route.fulfill(json({
        key,
        updatedAt: '2026-10-17T00:00:00Z',
        value: key === 'reader' ? { ...readerSettings, mode: state.mode } : {},
      }))
    }
    if (path.startsWith('/settings/') && method === 'PUT') {
      return route.fulfill(json({
        key: path.slice('/settings/'.length),
        updatedAt: '2026-10-17T00:00:01Z',
        value: request.postDataJSON?.()?.value || {},
      }))
    }
    if (path === '/books' && method === 'GET') return route.fulfill(json(Object.values(books)))
    const bookMatch = path.match(/^\/books\/(\d+)$/)
    if (bookMatch && method === 'GET') {
      const book = books[bookMatch[1]]
      return route.fulfill(book ? json(book) : json({ error: 'book not found' }, 404))
    }
    const catalogMatch = path.match(/^\/books\/(\d+)\/chapters$/)
    if (catalogMatch && method === 'GET') {
      const bookId = Number(catalogMatch[1])
      if (!catalogs[bookId]) return route.fulfill(json({ error: 'book not found' }, 404))
      const gate = state.catalogGates.get(bookId)
      state.events.push({ t: Date.now(), type: 'catalog', bookId, held: Boolean(gate) })
      if (gate) {
        gate.markArrived()
        await gate.opened
      }
      return route.fulfill(json(catalogRows(bookId)))
    }
    const contentMatch = path.match(/^\/books\/(\d+)\/chapters\/(\d+)\/content$/)
    if (contentMatch) {
      const bookId = Number(contentMatch[1])
      const index = Number(contentMatch[2])
      const event = { t: Date.now(), type: 'content', bookId, index, status: 200 }
      state.events.push(event)
      const title = catalogs[bookId]?.[index]
      if (title === undefined) {
        event.status = 404
        return route.fulfill(json({ error: 'chapter not found' }, 404))
      }
      if (bookId === failOnce.bookId && index === failOnce.index && !state.failedOnce) {
        state.failedOnce = true
        event.status = failOnce.status
        return route.fulfill(json({ error: 'fixture first-attempt chapter failure' }, failOnce.status))
      }
      return route.fulfill(json({
        chapter: { id: bookId * 100 + index, bookId, index, title },
        content: chapterContent(bookId, index),
        format: 'text',
      }))
    }
    if (/^\/books\/\d+\/bookmarks$/.test(path)) return route.fulfill(json([]))
    const progressMatch = path.match(/^\/progress\/(\d+)$/)
    if (progressMatch && method === 'GET') {
      return route.fulfill(json(books[progressMatch[1]]?.progress || {}))
    }
    if (path === '/progress' && method === 'PUT') {
      const body = request.postDataJSON?.() || {}
      state.events.push({ t: Date.now(), type: 'put', bookId: Number(body.bookId), body })
      return route.fulfill(json({ ...body, updatedAt: new Date().toISOString() }))
    }
    if (path === '/categories' || path === '/book-groups' || path === '/sources') return route.fulfill(json([]))
    return route.fulfill(json({}))
  })
}

async function openContext(browser, viewport, mode) {
  const state = {
    catalogGates: new Map(),
    events: [],
    failedOnce: false,
    mode,
    user: { id: 1, username: 'shelf-switch-smoke', role: 'admin' },
  }
  const context = await browser.newContext({ viewport })
  await context.addInitScript(({ readerMode, token }) => {
    localStorage.setItem('openreader_token', token)
    localStorage.setItem('reader', JSON.stringify({ mode: readerMode, settingsScope: 'user:1', progressScope: 'user:1' }))
  }, { readerMode: mode, token: tokenFor('shelf-switch') })
  await context.addInitScript(installReaderObservers)
  const page = await context.newPage()
  const consoleErrors = []
  page.on('pageerror', error => consoleErrors.push(`pageerror: ${error.stack || error.message}`))
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    const text = message.text()
    if (/WebSocket connection to .*\/ws\/sync/.test(text)) return
    consoleErrors.push(`console.error: ${text} ${message.location()?.url || ''}`.trim())
  })
  await installApiMocks(page, state)
  return { consoleErrors, context, page, state }
}

function isMobile(viewport) {
  return viewport.width <= 750
}

async function ensureMobileChrome(page, viewport) {
  if (!isMobile(viewport)) return
  if (await page.locator('.reader-mobile-top.visible').count()) return
  await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
  await page.locator('.reader-mobile-top.visible').waitFor({ timeout: 5000 })
}

async function openShelf(page, viewport) {
  if (!isMobile(viewport)) {
    await page.locator('.reader-left-rail button[aria-label="书架"]').click()
    return
  }
  await ensureMobileChrome(page, viewport)
  await page.locator('.reader-mobile-top.visible .mobile-tool-button').filter({ hasText: '书架' }).click()
}

function contentEvents(state, bookId, filter = () => true) {
  return state.events.filter(event => event.type === 'content' && event.bookId === bookId && filter(event))
}

function describe(events) {
  return JSON.stringify(events.map(({ bookId, index, status, body }) => (
    body ? { bookId, chapterIndex: body.chapterIndex, chapterId: body.chapterId, percent: body.percent } : { bookId, index, status }
  )))
}

async function switchFromShelf(run, target, settle) {
  const { failures, label, page, state, viewport } = run
  const step = `${label} -> book ${target.id}`
  const previousBookId = Number(new URL(page.url()).pathname.match(/^\/books\/(\d+)\/read$/)?.[1])
  const gate = createGate()
  state.catalogGates.set(target.id, gate)
  const stateStart = await page.evaluate(() => window.__shelfSwitchStates.length)
  const requestStart = await page.evaluate(() => window.__shelfSwitchRequests.length)
  const messageStart = await page.evaluate(() => window.__shelfSwitchMessages.length)
  let releasedAt = 0
  let releasedAtPage = 0
  try {
    await openShelf(page, viewport)
    const card = page.locator(`.reader-shelf-card[data-book-id="${target.id}"]:visible`)
    await card.waitFor({ timeout: 10_000 })
    await card.click()
    await withTimeout(gate.arrived, 10_000, `${step}: catalog request for book ${target.id} never arrived`)
    await page.waitForURL(url => new URL(url).pathname === `/books/${target.id}/read`, { timeout: 10_000 })
    const switchedUrl = new URL(page.url())
    await page.waitForTimeout(catalogHoldMs)
    const messages = await page.evaluate(start => window.__shelfSwitchMessages.slice(start), messageStart)
    if (messages.length !== 1 || !messages[0].info || messages[0].path !== `/books/${target.id}/read`) {
      failures.push(`${step}: expected one info 换书成功 after navigation while catalog held, got ${JSON.stringify(messages)}`)
    }
    const held = await page.evaluate(() => window.__shelfSwitchSnapshot())
    releasedAtPage = await page.evaluate(() => performance.now())
    releasedAt = Date.now()
    state.catalogGates.delete(target.id)
    gate.release()

    const pageLog = await page.evaluate(({ stateStart: states, requestStart: requests }) => ({
      states: window.__shelfSwitchStates.slice(states),
      requests: window.__shelfSwitchRequests.slice(requests),
    }), { stateStart, requestStart })
    const catalogRequest = pageLog.requests.find(request => (
      request.method === 'GET' && new RegExp(`/api/books/${target.id}/chapters$`).test(new URL(request.url, targetUrl).pathname)
    ))
    if (!catalogRequest) throw new Error(`${step}: the page never requested book ${target.id}'s catalog`)
    const windowStates = [
      ...pageLog.states.filter(entry => entry.t >= catalogRequest.t && entry.t <= releasedAtPage),
      { ...held, t: releasedAtPage },
    ]

    // R1/R2: nothing is requested or saved for the target before its catalog is applied.
    const heldContent = contentEvents(state, target.id, event => event.t < releasedAt)
    if (heldContent.length) failures.push(`${step}: chapter content requested while the catalog was held: ${describe(heldContent)}`)
    const heldPuts = state.events.filter(event => event.type === 'put' && event.bookId === target.id && event.t < releasedAt)
    if (heldPuts.length) failures.push(`${step}: PUT /progress for book ${target.id} while its catalog was held: ${describe(heldPuts)}`)

    // R4: the content area is in the existing loading state, shows nobody's text and no stale error.
    const shownText = windowStates.filter(entry => entry.markers.length)
    if (shownText.length) {
      failures.push(`${step}: content area showed book text ${JSON.stringify([...new Set(shownText.flatMap(entry => entry.markers))])} while book ${target.id}'s catalog was held (previous book ${previousBookId})`)
    }
    const errors = windowStates.filter(entry => entry.error)
    if (errors.length) failures.push(`${step}: .chapter-load-error shown while book ${target.id}'s catalog was held`)
    if (!held.loading) failures.push(`${step}: "正在加载章节..." not shown while book ${target.id}'s catalog was held (state ${JSON.stringify(held)})`)
    // R2: N% stays empty until the target's own catalog is known.
    const progressTexts = [...new Set(windowStates.map(entry => entry.progress).filter(Boolean))]
    if (progressTexts.length) failures.push(`${step}: N% showed ${JSON.stringify(progressTexts)} while book ${target.id}'s catalog was held`)

    await settle({ releasedAt, switchedUrl, step })
    const messageCount = await page.evaluate(start => window.__shelfSwitchMessages.length - start, messageStart)
    if (messageCount !== 1) failures.push(`${step}: expected exactly one switch toast through chapter completion, got ${messageCount}`)
    await openShelf(page, viewport)
    await page.locator(`.reader-shelf-card[data-book-id="${target.id}"]:visible`).click()
    await page.waitForTimeout(100)
    if (await page.evaluate(start => window.__shelfSwitchMessages.length - start, messageStart) !== messageCount) {
      failures.push(`${step}: selecting current book emitted a switch toast`)
    }
  } finally {
    if (state.catalogGates.get(target.id) === gate) state.catalogGates.delete(target.id)
    gate.release()
  }
}

async function waitForBookText(page, bookId, step) {
  await page.waitForFunction(id => window.__shelfSwitchSnapshot().markers.includes(id), bookId, { timeout: 10_000 })
    .catch(async (error) => {
      throw new Error(`${step}: book ${bookId} text never appeared (${JSON.stringify(await page.evaluate(() => window.__shelfSwitchSnapshot()))}; ${error.message.split('\n')[0]})`)
    })
}

async function runShelfSwitches(browser, viewport, mode) {
  const label = `${viewport.width}x${viewport.height} ${mode}`
  const { consoleErrors, context, page, state } = await openContext(browser, viewport, mode)
  const failures = []
  const run = { failures, label, page, state, viewport }
  try {
    await page.goto(`${targetUrl}/books/1/read?chapter=2`, { waitUntil: 'networkidle' })
    await waitForBookText(page, 1, `${label} open book 1`)
    // Let book 1's own preloads finish so they are not mistaken for switch traffic.
    await page.waitForTimeout(500)

    // Book 3: 12 chapters, saved chapter 10 (index 9) beyond book 1's 3-row catalog.
    await switchFromShelf(run, books[3], async ({ step }) => {
      await waitForBookText(page, 3, step)
      await page.waitForTimeout(1600)
      const requests = contentEvents(state, 3)
      const chapterNine = requests.filter(event => event.index === 9)
      if (chapterNine.length !== 1) failures.push(`${step}: chapter index 9 requested ${chapterNine.length} times: ${describe(requests)}`)
      const outside = requests.filter(event => event.index < 7 || event.index > 11)
      if (outside.length) failures.push(`${step}: requests outside 7-11 around saved chapter 9: ${describe(outside)}`)
      const puts = state.events.filter(event => event.type === 'put' && event.bookId === 3)
      const wrongPuts = puts.filter(({ body }) => (
        body.chapterIndex !== 9
        || body.chapterId !== 309
        || Math.abs(Number(body.percent) - (9 + Number(body.chapterPercent || 0)) / 12) > 1e-6
      ))
      if (wrongPuts.length) failures.push(`${step}: progress for book 3 not on chapter 9 of 12: ${describe(wrongPuts)}`)
      const shown = await page.evaluate(() => document.querySelector('.reader-body')?.innerText || '')
      if (!shown.includes('换书契约三第10章')) failures.push(`${step}: book 3 did not open at chapter 10 (index 9)`)
    })

    // Book 4: 2 chapters, no progress; book 3's 12-row catalog must not drive preloads.
    await switchFromShelf(run, books[4], async ({ step }) => {
      await waitForBookText(page, 4, step)
      await page.waitForTimeout(800)
      const beyond = contentEvents(state, 4, event => event.index >= 2)
      if (beyond.length) failures.push(`${step}: requests beyond book 4's 2 chapters: ${describe(beyond)}`)
    })

    // Book 5: saved chapter 2 (index 1) whose first request fails; only book 5's own load may send it.
    await switchFromShelf(run, books[5], async ({ releasedAt, step }) => {
      await page.waitForFunction(() => {
        const snapshot = window.__shelfSwitchSnapshot()
        return snapshot.error || snapshot.markers.includes(5)
      }, null, { timeout: 10_000 })
      await page.waitForTimeout(800)
      const chapterOne = contentEvents(state, 5, event => event.index === 1)
      if (chapterOne.length !== 1) failures.push(`${step}: book 5 chapter index 1 requested ${chapterOne.length} times: ${describe(chapterOne)}`)
      const failedBeforeCatalog = chapterOne.filter(event => event.status === failOnce.status && event.t < releasedAt)
      if (failedBeforeCatalog.length) failures.push(`${step}: the first-attempt failure was consumed by a request sent before book 5's catalog`)
      // R4: book 5's own first attempt fails after its catalog arrived and uses the existing error surface.
      if (!(await page.locator('.chapter-load-error:visible, .chapter-inline-error:visible').count())) {
        failures.push(`${step}: book 5's own chapter failure is not shown with the existing error surface`)
      }
    })

    // Book 6: no chapterCount on the shelf row and only a book-level percent in its progress.
    await switchFromShelf(run, books[6], async ({ step, switchedUrl }) => {
      if (switchedUrl.searchParams.has('percent')) {
        failures.push(`${step}: switch route carries percent=${switchedUrl.searchParams.get('percent')} derived from the previous book's catalog (${switchedUrl.search})`)
      }
      await waitForBookText(page, 6, step)
      await page.waitForTimeout(600)
    })

    // R3: every progress write names a chapter row of the book it is written for, including the forced save that
    // leaves a book whose own first chapter failed (book 5 -> book 6).
    const crossBookPuts = state.events.filter(({ type, bookId, body }) => type === 'put' && (
      !catalogs[bookId]?.[body.chapterIndex] || body.chapterId !== bookId * 100 + body.chapterIndex
    ))
    if (crossBookPuts.length) failures.push(`${label}: progress written with another book's chapter: ${describe(crossBookPuts)}`)
    const notFound = state.events.filter(event => event.type === 'content' && event.status === 404)
    if (notFound.length) failures.push(`${label}: 404 chapter requests ${describe(notFound)}`)
    const unexpectedConsole = consoleErrors.filter(text => !(
      /Failed to load resource: the server responded with a status of 502/.test(text)
      && text.includes(`/api/books/${failOnce.bookId}/chapters/${failOnce.index}/content`)
    ))
    if (unexpectedConsole.length) failures.push(`${label}: console errors\n  ${unexpectedConsole.join('\n  ')}`)
  } catch (error) {
    failures.push(`${label}: ${error.stack || error.message}`)
  } finally {
    await context.close()
  }
  return failures
}

async function main() {
  const browser = await openSmokeBrowser()
  const failures = []
  const checked = []
  try {
    for (const viewport of smokeViewports()) {
      for (const mode of smokeModes()) {
        const runFailures = await runShelfSwitches(browser, viewport, mode)
        checked.push(`${viewport.width}x${viewport.height} ${mode}: ${runFailures.length ? `${runFailures.length} failing` : 'ok'}`)
        failures.push(...runFailures)
      }
    }
  } finally {
    await browser.close()
  }
  if (failures.length) {
    console.error(`reader shelf switch contract smoke failed (${checked.join('; ')}):\n- ${failures.join('\n- ')}`)
    process.exit(1)
  }
  console.log(`reader shelf switch contract smoke passed: ${checked.join('; ')} hold=${catalogHoldMs}ms`)
}

export { openContext, openShelf, waitForBookText, ensureMobileChrome }

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch((error) => {
    console.error(error.stack || error.message)
    process.exit(1)
  })
}
