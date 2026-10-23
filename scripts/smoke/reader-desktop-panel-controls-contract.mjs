#!/usr/bin/env node

// Reader desktop workspace panel content controls browser contract
// (docs/compat/reader-desktop-panel-content-controls-fixed-baseline-second-audit-p0-contract.md section 8.2).
// Upstream's desktop panels (书架、书源、目录、设置) are click popovers without a mask: a press on a control outside
// the panel runs the control and the panel closes. OpenReader puts a transparent dismiss layer under the panel;
// content controls drawn above that layer and exposed below the panel must behave like upstream (P2-P6):
// - C1 加载下一章 (page, flip setting): the button is the hit target at 88% of the viewport; a press closes the
//   panel, routes to ?chapter=1, shows chapter 1 and writes one progress row for it; neither the tap zones, the
//   reader page nor the document receive the press;
// - C2 continuous inline 重新加载 (scroll, scroll2; chapter 1 fails until the panel is open): the button is the
//   hit target at 88%; a press closes the panel, requests chapter 1 once and replaces its block in place, keeps
//   the scroll position and writes no progress;
// - C3 main error block 重新加载 (page, scroll): with the panel open the button center hits the panel; after the
//   panel's close button is pressed the button is the hit target again;
// - C4 controls (page): a press on the page below the panel only closes it (route, scroll and progress unchanged,
//   the tap zones and the reader page see nothing); the right rail 重新载入章节 closes the panel and reloads the
//   chapter; another panel's rail entry switches panels.
// Viewports: 1440x900 mouse; iPad 1366x1024 and 1024x1366 touch (hasTouch, iPad UA, as reader-mobile-contract).
// Each case uses a fresh browser context, runs to the end and reports all of its failed checks;
// SMOKE_CASES=<regex> limits the cases.

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = (process.env.TARGET_URL || 'http://127.0.0.1:5173').replace(/\/$/, '')
const caseFilter = process.env.SMOKE_CASES ? new RegExp(process.env.SMOKE_CASES) : null

const IPAD_UA = 'Mozilla/5.0 (iPad; CPU OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1'
const VIEWPORTS = [
  { label: '1440x900 mouse', width: 1440, height: 900, input: 'mouse' },
  { label: '1366x1024 iPad touch', width: 1366, height: 1024, input: 'touch' },
  { label: '1024x1366 iPad touch', width: 1024, height: 1366, input: 'touch' },
]
const PANELS = [
  { label: '书架', id: 'shelf' },
  { label: '书源', id: 'source' },
  { label: '目录', id: 'toc' },
  { label: '设置', id: 'settings' },
]
const BOOK_TITLE = '面板正文控件契约'
const TITLES = [
  '第一章 起点',
  '第二章 后邻',
  '第三章 平稳',
  '第四章 预载',
  '第五章 前邻',
  '第六章 失败',
  '第七章 后续',
  '第八章 终点',
]
const NEXT_INDEX = 1
const INLINE_FAIL_INDEX = 1
const MAIN_FAIL_INDEX = 5
// Body the Go backend writes when a source fails to deliver chapter content (books.go chapterContent).
const BACKEND_CONTENT_FAILURE = { error: 'failed to load chapter content', code: 'content_unavailable', stage: 'content' }
const CHAPTER_END = '.reader-chapter-end'
const INLINE_BUTTON = `.chapter-content[data-index="${INLINE_FAIL_INDEX}"] .chapter-inline-error button`
const MAIN_BUTTON = '.chapter-load-error button'
const RAIL_RELOAD = '.reader-right-rail .round-tool[title="重新载入章节"]'
const PANEL_CLOSE = '.reader-desktop-workspace-close'
const BAND = 0.88

const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))
const railEntry = label => `.reader-left-rail .rail-item[title="${label}"]`

function json(data, status = 200) {
  return { status, contentType: 'application/json', body: JSON.stringify(data) }
}

function fakeToken() {
  const payload = Buffer.from(JSON.stringify({ userId: 1, sub: '1' })).toString('base64url')
  return `open.${payload}.reader`
}

function chapterContent(index) {
  return Array.from({ length: 60 }, (_, paragraph) => (
    `${TITLES[index]}第${paragraph + 1}段。面板正文控件契约正文，用于验证面板打开时露出的正文控件。`
  )).join('\n')
}

function contextOptions(viewport) {
  const size = { width: viewport.width, height: viewport.height }
  return viewport.input === 'touch'
    ? { viewport: size, hasTouch: true, deviceScaleFactor: 2, userAgent: IPAD_UA }
    : { viewport: size }
}

function newBehavior() {
  return { fail: new Map(), requests: [], progressWrites: [], routeErrors: [] }
}

// Runs before the app: records, for every press, the event target seen in the capture phase (and whether it is a
// tap zone) and whether the event bubbled up to the document.
function instrumentPresses() {
  const describe = (node) => {
    if (!node || !node.tagName) return String(node && node.nodeName)
    const own = node.tagName.toLowerCase() + [...node.classList].map(name => `.${name}`).join('')
    const flags = [
      ['.reader-tap-zones', 'tap-zone'],
      ['.reader-workspace-dismiss', 'dismiss'],
      ['.reader-desktop-workspace', 'panel'],
      ['.reader-page', 'reader-page'],
    ].filter(([selector]) => node.closest?.(selector)).map(([, flag]) => flag)
    return flags.length ? `${own} [${flags.join(',')}]` : own
  }
  window.__panelControls = { events: [], describe }
  for (const type of ['touchstart', 'touchend', 'click']) {
    document.addEventListener(type, (event) => {
      window.__panelControls.events.push({
        type,
        phase: 'target',
        target: describe(event.target),
        zone: Boolean(event.target?.closest?.('.reader-tap-zones')),
      })
    }, true)
    document.addEventListener(type, () => window.__panelControls.events.push({ type, phase: 'document' }), false)
  }
}

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
          updatedAt: '2026-10-23T00:00:00Z',
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
        return await route.fulfill(json({ key: 'reader', updatedAt: '2026-10-23T00:00:01Z', value: {} }))
      }
      if (path === '/books/1') {
        return await route.fulfill(json({
          id: 1,
          title: BOOK_TITLE,
          author: 'OpenReader',
          sourceId: 0,
          chapterCount: TITLES.length,
          progress: null,
        }))
      }
      if (path === '/books/1/chapters') {
        return await route.fulfill(json(TITLES.map((title, index) => ({ id: index + 11, bookId: 1, index, title }))))
      }
      const contentMatch = path.match(/^\/books\/1\/chapters\/(\d+)\/content$/)
      if (contentMatch) {
        const index = Number(contentMatch[1])
        behavior.requests.push({ index, at: Date.now() })
        const failure = behavior.fail.get(index)
        if (failure) return await route.fulfill(json(failure.body, failure.status))
        return await route.fulfill(json({
          chapter: { id: index + 11, bookId: 1, index, title: TITLES[index] },
          content: chapterContent(index),
          format: 'text',
        }))
      }
      if (path === '/progress' && method === 'PUT') {
        const body = request.postDataJSON?.() || {}
        behavior.progressWrites.push({ chapterIndex: body.chapterIndex, chapterTitle: body.chapterTitle, at: Date.now() })
        return await route.fulfill(json({ ...body, bookId: 1 }))
      }
      if (path === '/progress/1') return await route.fulfill(json({}))
      if (path === '/books' || path === '/books/1/bookmarks' || path === '/sources' || path === '/categories') {
        return await route.fulfill(json([]))
      }
      return await route.fulfill(json({}))
    } catch (error) {
      behavior.routeErrors.push(String(error?.message || error).split('\n')[0])
    }
  })
}

async function openReader(context, mode, behavior, pageErrors) {
  await context.addInitScript(({ readerMode, token }) => {
    localStorage.setItem('openreader_token', token)
    localStorage.setItem('reader', JSON.stringify({ mode: readerMode, settingsScope: 'user:1', progressScope: 'user:1' }))
  }, { readerMode: mode, token: fakeToken() })
  await context.addInitScript(instrumentPresses)
  const page = await context.newPage()
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    const text = message.text()
    if (text.includes('/ws/sync')) return
    if (text.includes('Failed to load resource: the server responded with a status of 502')) return
    pageErrors.push(text.split('\n')[0])
  })
  page.on('pageerror', error => pageErrors.push(error.message))
  await installMocks(page, behavior, mode)
  await page.goto(`${targetUrl}/books/1/read?chapter=0`, { waitUntil: 'domcontentloaded' })
  await page.waitForSelector('.chapter-content[data-index="0"] [data-reader-block]', { timeout: 15_000 })
  return page
}

async function routeToChapter(page, index) {
  await page.evaluate((target) => {
    const router = document.querySelector('#app').__vue_app__.config.globalProperties.$router
    return router.push({ query: { ...router.currentRoute.value.query, chapter: String(target) } })
  }, index)
}

async function readState(page) {
  return page.evaluate(() => {
    const documentScroll = Boolean(document.querySelector('.reader-shell.document-scroll'))
    const scroller = documentScroll
      ? (document.scrollingElement || document.documentElement)
      : document.querySelector('.reader-content')
    const workspaces = [...document.querySelectorAll('.reader-desktop-workspace')]
    const zones = document.querySelector('.reader-tap-zones')
    return {
      query: location.search,
      scrollTop: Math.round(scroller?.scrollTop || 0),
      panels: workspaces.map(node => (
        [...node.classList].find(name => name.startsWith('workspace-panel-'))?.slice('workspace-panel-'.length) || '?'
      )),
      panelBottom: workspaces[0] ? Math.round(workspaces[0].getBoundingClientRect().bottom) : null,
      dismissLayers: document.querySelectorAll('.reader-workspace-dismiss').length,
      mainError: Boolean(document.querySelector('.chapter-load-error')),
      inlineErrors: [...document.querySelectorAll('.chapter-inline-error')]
        .map(node => Number(node.closest('.chapter-content')?.dataset.index)),
      blocks: [...document.querySelectorAll('.reader-body .chapter-content')].map(node => Number(node.dataset.index)),
      paragraphs: Object.fromEntries([...document.querySelectorAll('.reader-body .chapter-content')].map(node => [
        node.dataset.index,
        node.querySelector('[data-reader-block]:not(.chapter-inline-error *)')?.textContent.trim().slice(0, 12) ?? null,
      ])),
      tapZonesShown: zones ? getComputedStyle(zones).display !== 'none' : false,
    }
  })
}

// Hit test at the rounded center of `selector` (or at `point`), which is also where press() lands.
async function hitTest(page, selector, point = null) {
  return page.evaluate(({ target, at }) => {
    const element = target ? document.querySelector(target) : null
    if (target && !element) return null
    let x = at?.x
    let y = at?.y
    if (!at) {
      const rect = element.getBoundingClientRect()
      x = Math.round(rect.left + rect.width / 2)
      y = Math.round(rect.top + rect.height / 2)
    }
    const hit = document.elementFromPoint(x, y)
    return {
      x,
      y,
      hitIsTarget: Boolean(element && hit && (hit === element || element.contains(hit))),
      hit: window.__panelControls.describe(hit),
      inPanel: Boolean(hit?.closest('.reader-desktop-workspace')),
      dismiss: Boolean(hit?.closest('.reader-workspace-dismiss')),
    }
  }, { target: selector, at: point })
}

async function press(page, viewport, point) {
  if (viewport.input === 'touch') await page.touchscreen.tap(point.x, point.y)
  else await page.mouse.click(point.x, point.y)
}

// Clears the press log and listens on .reader-page itself: an event that reaches it would run the page's own
// click/touch handlers (paging, tool toggling).
async function armPressLog(page) {
  await page.evaluate(() => {
    window.__panelControls.events.length = 0
    const readerPage = document.querySelector('.reader-page')
    if (readerPage && !readerPage.__panelControlsArmed) {
      for (const type of ['touchstart', 'touchend', 'click']) {
        readerPage.addEventListener(type, () => window.__panelControls.events.push({ type, phase: 'reader-page' }), false)
      }
      readerPage.__panelControlsArmed = true
    }
  })
}

async function pressLog(page) {
  const events = await page.evaluate(() => [...window.__panelControls.events])
  return {
    targets: [...new Set(events.filter(row => row.phase === 'target').map(row => `${row.type}:${row.target}`))],
    zone: events.filter(row => row.phase === 'target' && row.zone).map(row => `${row.type}:${row.target}`),
    readerPage: events.filter(row => row.phase === 'reader-page').map(row => row.type),
    document: events.filter(row => row.phase === 'document').map(row => row.type),
  }
}

function scrollIntoBand({ target, band }) {
  const element = document.querySelector(target)
  if (!element) return false
  const documentScroll = Boolean(document.querySelector('.reader-shell.document-scroll'))
  const scroller = documentScroll
    ? (document.scrollingElement || document.documentElement)
    : document.querySelector('.reader-content')
  const rect = element.getBoundingClientRect()
  scroller.scrollTop = Math.max(0, scroller.scrollTop + rect.top + rect.height / 2 - window.innerHeight * band)
  ;(documentScroll ? window : scroller).dispatchEvent(new Event('scroll'))
  return true
}

async function bandOffset(page, selector, band) {
  return page.evaluate(({ target, wantedBand }) => {
    const element = document.querySelector(target)
    if (!element) return null
    const rect = element.getBoundingClientRect()
    const y = rect.top + rect.height / 2
    const wanted = window.innerHeight * wantedBand
    return { y: Math.round(y), wanted: Math.round(wanted), ok: Math.abs(y - wanted) <= 4 }
  }, { target: selector, wantedBand: band })
}

// The continuous window can re-anchor to the reading position shortly after it settles, so a placement only
// counts once the center is still at the band on two checks in a row.
async function holdsBand(page, selector, band) {
  let last = null
  for (const wait of [450, 450]) {
    await page.waitForTimeout(wait)
    last = await bandOffset(page, selector, band)
    if (!last?.ok) return { ok: false, last }
  }
  return { ok: true, last }
}

async function placeInBand(page, selector, band) {
  let last = null
  for (let attempt = 1; attempt <= 6; attempt += 1) {
    if (!await page.evaluate(scrollIntoBand, { target: selector, band })) throw new Error(`${selector} is missing`)
    const held = await holdsBand(page, selector, band)
    last = held.last
    if (held.ok) return { attempts: attempt, y: last.y }
  }
  throw new Error(`could not keep ${selector} at ${band * 100}% (last y ${last?.y}, wanted ${last?.wanted})`)
}

// After the panel opens, confirm the placement again; place again if the window moved meanwhile.
async function confirmInBand(page, selector, band) {
  const held = await holdsBand(page, selector, band)
  if (held.ok) return `held at y=${held.last.y}`
  const placed = await placeInBand(page, selector, band)
  return `moved to y=${held.last?.y}, placed again at y=${placed.y}`
}

async function openPanel(page, viewport, panel) {
  const entry = await hitTest(page, railEntry(panel.label))
  if (!entry?.hitIsTarget) throw new Error(`rail entry ${panel.label} at (${entry?.x},${entry?.y}) hits ${entry?.hit}`)
  await press(page, viewport, entry)
  await page.waitForSelector(`.reader-desktop-workspace.workspace-panel-${panel.id}`, { timeout: 10_000 })
  await page.waitForTimeout(600)
}

async function waitUntil(predicate, timeout) {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) {
    if (predicate()) return true
    await sleep(50)
  }
  return predicate()
}

// Waits until no chapter request or progress write arrived for `quiet` ms, so that a debounced write caused by
// the placement scroll cannot be attributed to the press.
async function waitForQuiet(behavior, quiet = 900, timeout = 6000) {
  const lastActivity = () => Math.max(0, ...behavior.requests.map(row => row.at), ...behavior.progressWrites.map(row => row.at))
  return waitUntil(() => Date.now() - lastActivity() >= quiet, timeout)
}

function since(rows, mark) {
  return rows.filter(row => row.at >= mark)
}

function check(failures, condition, message) {
  if (!condition) failures.push(message)
}

function describeWrites(rows) {
  return JSON.stringify(rows.map(row => `${row.chapterIndex}:${row.chapterTitle}`))
}

function panelClosedCheck(failures, id, after) {
  check(
    failures,
    after.panels.length === 0 && after.dismissLayers === 0,
    `${id} panel closed: panel ${JSON.stringify(after.panels)} and ${after.dismissLayers} dismiss layer(s) still mounted after the press`,
  )
}

function isolationCheck(failures, id, log, { documentTouch = true } = {}) {
  check(failures, log.zone.length === 0, `${id} tap zones: the press reached a tap zone ${JSON.stringify(log.zone)}`)
  check(failures, log.readerPage.length === 0, `${id} reader page: ${JSON.stringify(log.readerPage)} reached .reader-page`)
  const reached = log.document.filter(type => type === 'click' || (documentTouch && type.startsWith('touch')))
  check(failures, reached.length === 0, `${id} document: ${JSON.stringify(reached)} reached the document`)
}

async function runChapterEndCase(page, viewport, panel, behavior, failures, notes) {
  await page.waitForSelector(CHAPTER_END, { timeout: 10_000 })
  await page.waitForTimeout(1200)
  const placed = await placeInBand(page, CHAPTER_END, BAND)
  await openPanel(page, viewport, panel)
  notes.push(`placed y=${placed.y} (${placed.attempts} try), after open ${await confirmInBand(page, CHAPTER_END, BAND)}`)
  await waitForQuiet(behavior)

  const hit = await hitTest(page, CHAPTER_END)
  const before = await readState(page)
  notes.push(`panel ${before.panels} bottom ${before.panelBottom}, tap zones ${before.tapZonesShown ? 'shown' : 'hidden'}; button (${hit.x},${hit.y}) hits ${hit.hit}`)
  check(failures, hit.hitIsTarget, `C1 hit: 加载下一章 center (${hit.x},${hit.y}) hits ${hit.hit}`)

  await armPressLog(page)
  const mark = Date.now()
  await press(page, viewport, hit)
  await page.waitForFunction((index) => (
    new URLSearchParams(location.search).get('chapter') === String(index)
    && document.querySelector(`.chapter-content[data-index="${index}"] [data-reader-block]`)
  ), NEXT_INDEX, { timeout: 5000 }).catch(() => {})
  await waitUntil(() => since(behavior.progressWrites, mark).length > 0, 3000)
  await page.waitForTimeout(1000)
  const after = await readState(page)
  const writes = since(behavior.progressWrites, mark)
  const log = await pressLog(page)
  notes.push(`panel ${JSON.stringify(after.panels)} route ${before.query}->${after.query} scroll ${before.scrollTop}->${after.scrollTop} blocks ${JSON.stringify(after.blocks)} PUT ${describeWrites(writes)} events ${JSON.stringify(log.targets)}`)

  panelClosedCheck(failures, 'C1', after)
  check(failures, before.query === '?chapter=0' && after.query === `?chapter=${NEXT_INDEX}`, `C1 route: ${before.query} -> ${after.query}`)
  check(
    failures,
    JSON.stringify(after.blocks) === JSON.stringify([NEXT_INDEX])
      && String(after.paragraphs[String(NEXT_INDEX)] || '').startsWith(TITLES[NEXT_INDEX]),
    `C1 shows chapter ${NEXT_INDEX}: blocks ${JSON.stringify(after.blocks)} (${JSON.stringify(after.paragraphs)})`,
  )
  check(
    failures,
    writes.length === 1 && writes[0].chapterIndex === NEXT_INDEX && writes[0].chapterTitle === TITLES[NEXT_INDEX],
    `C1 progress: expected one PUT for ${NEXT_INDEX}:${TITLES[NEXT_INDEX]}, got ${describeWrites(writes)}`,
  )
  isolationCheck(failures, 'C1', log)
}

async function runInlineRetryCase(page, viewport, panel, behavior, failures, notes) {
  await page.waitForSelector(INLINE_BUTTON, { timeout: 10_000 })
  await page.waitForTimeout(1200)
  const placed = await placeInBand(page, INLINE_BUTTON, BAND)
  await openPanel(page, viewport, panel)
  notes.push(`placed y=${placed.y} (${placed.attempts} try), after open ${await confirmInBand(page, INLINE_BUTTON, BAND)}`)
  behavior.fail.delete(INLINE_FAIL_INDEX)
  await waitForQuiet(behavior)

  const hit = await hitTest(page, INLINE_BUTTON)
  const before = await readState(page)
  notes.push(`panel ${before.panels} bottom ${before.panelBottom}, tap zones ${before.tapZonesShown ? 'shown' : 'hidden'}; button (${hit.x},${hit.y}) hits ${hit.hit}`)
  check(failures, hit.hitIsTarget, `C2 hit: inline 重新加载 center (${hit.x},${hit.y}) hits ${hit.hit}`)

  await armPressLog(page)
  const mark = Date.now()
  await press(page, viewport, hit)
  await page.waitForFunction((index) => (
    !document.querySelector(`.chapter-content[data-index="${index}"] .chapter-inline-error`)
    && document.querySelectorAll(`.chapter-content[data-index="${index}"] [data-reader-block]`).length > 1
  ), INLINE_FAIL_INDEX, { timeout: 4000 }).catch(() => {})
  await page.waitForTimeout(1000)
  const after = await readState(page)
  const requests = since(behavior.requests, mark).map(row => row.index)
  const writes = since(behavior.progressWrites, mark)
  const log = await pressLog(page)
  notes.push(`panel ${JSON.stringify(after.panels)} requests ${JSON.stringify(requests)} blocks ${JSON.stringify(before.blocks)}->${JSON.stringify(after.blocks)} scroll ${before.scrollTop}->${after.scrollTop} PUT ${describeWrites(writes)} events ${JSON.stringify(log.targets)}`)

  panelClosedCheck(failures, 'C2', after)
  check(
    failures,
    requests[0] === INLINE_FAIL_INDEX && requests.filter(index => index === INLINE_FAIL_INDEX).length === 1,
    `C2 retry: expected chapter ${INLINE_FAIL_INDEX} to be requested first and once, got ${JSON.stringify(requests)}`,
  )
  check(
    failures,
    !after.inlineErrors.includes(INLINE_FAIL_INDEX)
      && after.blocks.indexOf(INLINE_FAIL_INDEX) === before.blocks.indexOf(INLINE_FAIL_INDEX)
      && String(after.paragraphs[String(INLINE_FAIL_INDEX)] || '').startsWith(TITLES[INLINE_FAIL_INDEX]),
    `C2 in place: blocks ${JSON.stringify(before.blocks)} -> ${JSON.stringify(after.blocks)}, inline errors ${JSON.stringify(after.inlineErrors)}, chapter ${INLINE_FAIL_INDEX} shows ${JSON.stringify(after.paragraphs[String(INLINE_FAIL_INDEX)] ?? null)}`,
  )
  check(failures, Math.abs(after.scrollTop - before.scrollTop) <= 1, `C2 scroll: ${before.scrollTop} -> ${after.scrollTop}`)
  check(failures, writes.length === 0, `C2 progress: expected no PUT, got ${describeWrites(writes)}`)
  check(failures, after.query === before.query, `C2 route: ${before.query} -> ${after.query}`)
  isolationCheck(failures, 'C2', log)
}

async function runMainErrorCase(page, viewport, panel, behavior, failures, notes) {
  behavior.fail.set(MAIN_FAIL_INDEX, { status: 502, body: BACKEND_CONTENT_FAILURE })
  await routeToChapter(page, MAIN_FAIL_INDEX)
  await page.waitForSelector(MAIN_BUTTON, { timeout: 10_000 })
  await page.waitForTimeout(1200)
  await openPanel(page, viewport, panel)

  const covered = await hitTest(page, MAIN_BUTTON)
  const open = await readState(page)
  notes.push(`panel ${open.panels} bottom ${open.panelBottom}; button (${covered.x},${covered.y}) hits ${covered.hit}`)
  check(failures, covered.inPanel && !covered.hitIsTarget, `C3 covered: with the panel open 重新加载 center (${covered.x},${covered.y}) hits ${covered.hit}`)

  const close = await hitTest(page, PANEL_CLOSE)
  if (!close?.hitIsTarget) throw new Error(`panel close button at (${close?.x},${close?.y}) hits ${close?.hit}`)
  await press(page, viewport, close)
  await page.waitForFunction(() => !document.querySelector('.reader-desktop-workspace'), null, { timeout: 5000 }).catch(() => {})
  await page.waitForTimeout(400)
  const closed = await readState(page)
  const reachable = await hitTest(page, MAIN_BUTTON)
  notes.push(`after close panel ${JSON.stringify(closed.panels)}; button (${reachable?.x},${reachable?.y}) hits ${reachable?.hit}`)
  check(failures, closed.panels.length === 0, `C3 close: panel ${JSON.stringify(closed.panels)} still open after its close button`)
  check(failures, Boolean(reachable?.hitIsTarget), `C3 reachable: after closing 重新加载 center (${reachable?.x},${reachable?.y}) hits ${reachable?.hit}`)
  check(failures, closed.query === `?chapter=${MAIN_FAIL_INDEX}` && closed.mainError, `C3 state: route ${closed.query}, error block ${closed.mainError}`)
}

async function runBlankCase(page, viewport, panel, behavior, failures, notes) {
  await page.waitForTimeout(1200)
  await openPanel(page, viewport, panel)
  await waitForQuiet(behavior)
  const point = { x: Math.round(viewport.width / 2), y: Math.round(viewport.height * BAND) }
  const hit = await hitTest(page, null, point)
  const before = await readState(page)
  notes.push(`panel ${before.panels} bottom ${before.panelBottom}, tap zones ${before.tapZonesShown ? 'shown' : 'hidden'}; blank (${point.x},${point.y}) hits ${hit.hit}`)
  check(failures, hit.dismiss, `C4-blank hit: (${point.x},${point.y}) below the panel hits ${hit.hit}, not the dismiss layer`)

  await armPressLog(page)
  const mark = Date.now()
  await press(page, viewport, point)
  await page.waitForFunction(() => !document.querySelector('.reader-desktop-workspace'), null, { timeout: 3000 }).catch(() => {})
  await page.waitForTimeout(800)
  const after = await readState(page)
  const requests = since(behavior.requests, mark).map(row => row.index)
  const writes = since(behavior.progressWrites, mark)
  const log = await pressLog(page)
  notes.push(`panel ${JSON.stringify(after.panels)} route ${before.query}->${after.query} scroll ${before.scrollTop}->${after.scrollTop} requests ${JSON.stringify(requests)} PUT ${describeWrites(writes)} events ${JSON.stringify(log.targets)}`)

  panelClosedCheck(failures, 'C4-blank', after)
  check(failures, after.query === before.query, `C4-blank route: ${before.query} -> ${after.query}`)
  check(failures, Math.abs(after.scrollTop - before.scrollTop) <= 1, `C4-blank scroll: ${before.scrollTop} -> ${after.scrollTop}`)
  check(failures, requests.length === 0, `C4-blank requests: ${JSON.stringify(requests)}`)
  check(failures, writes.length === 0, `C4-blank progress: expected no PUT, got ${describeWrites(writes)}`)
  // A touch on the dismiss layer bubbles its touchend to the document; the layer sits outside .reader-page, so the
  // page's touch handling never sees it.
  isolationCheck(failures, 'C4-blank', log, { documentTouch: false })
}

async function runRailReloadCase(page, viewport, panel, behavior, failures, notes) {
  await page.waitForTimeout(1200)
  await openPanel(page, viewport, panel)
  await waitForQuiet(behavior)
  const hit = await hitTest(page, RAIL_RELOAD)
  const before = await readState(page)
  notes.push(`panel ${before.panels}; 重新载入章节 (${hit.x},${hit.y}) hits ${hit.hit}`)
  check(failures, hit.hitIsTarget, `C4-reload hit: 重新载入章节 (${hit.x},${hit.y}) hits ${hit.hit}`)

  const mark = Date.now()
  await press(page, viewport, hit)
  const toast = await page.waitForFunction(
    () => document.querySelector('.reader-toast')?.textContent.includes('章节已重新载入'),
    null,
    { timeout: 4000 },
  ).then(() => true, () => false)
  await page.waitForTimeout(800)
  const after = await readState(page)
  const requests = since(behavior.requests, mark).map(row => row.index)
  notes.push(`panel ${JSON.stringify(after.panels)} requests ${JSON.stringify(requests)} toast ${toast} route ${after.query} scroll ${before.scrollTop}->${after.scrollTop}`)

  panelClosedCheck(failures, 'C4-reload', after)
  check(failures, requests[0] === 0 && requests.filter(index => index === 0).length === 1, `C4-reload request: expected chapter 0 once, got ${JSON.stringify(requests)}`)
  check(failures, toast, 'C4-reload toast: 章节已重新载入 was not shown')
  check(failures, after.query === before.query, `C4-reload route: ${before.query} -> ${after.query}`)
}

async function runPanelSwitchCase(page, viewport, panel, behavior, failures, notes) {
  await page.waitForTimeout(1200)
  await openPanel(page, viewport, panel)
  await waitForQuiet(behavior)
  const other = PANELS.find(row => row.id === (panel.id === 'toc' ? 'shelf' : 'toc'))
  const hit = await hitTest(page, railEntry(other.label))
  const before = await readState(page)
  check(failures, hit.hitIsTarget, `C4-switch hit: ${other.label} entry (${hit.x},${hit.y}) hits ${hit.hit}`)

  const mark = Date.now()
  await press(page, viewport, hit)
  await page.waitForSelector(`.reader-desktop-workspace.workspace-panel-${other.id}`, { timeout: 3000 }).catch(() => {})
  await page.waitForTimeout(600)
  const after = await readState(page)
  const requests = since(behavior.requests, mark).map(row => row.index)
  const writes = since(behavior.progressWrites, mark)
  notes.push(`${panel.label} -> ${other.label}: panels ${JSON.stringify(before.panels)} -> ${JSON.stringify(after.panels)} requests ${JSON.stringify(requests)} PUT ${describeWrites(writes)}`)

  check(failures, JSON.stringify(after.panels) === JSON.stringify([other.id]), `C4-switch panel: ${JSON.stringify(before.panels)} -> ${JSON.stringify(after.panels)}, expected [${JSON.stringify(other.id)}]`)
  check(failures, after.query === before.query, `C4-switch route: ${before.query} -> ${after.query}`)
  check(failures, requests.length === 0 && writes.length === 0, `C4-switch side effects: requests ${JSON.stringify(requests)} PUT ${describeWrites(writes)}`)
}

const SCENARIOS = [
  { id: 'C1', modes: ['page', 'flip'], run: runChapterEndCase },
  { id: 'C2', modes: ['scroll', 'scroll2'], failFirst: INLINE_FAIL_INDEX, run: runInlineRetryCase },
  { id: 'C3', modes: ['page', 'scroll'], run: runMainErrorCase },
  { id: 'C4-blank', modes: ['page'], run: runBlankCase },
  { id: 'C4-reload', modes: ['page'], run: runRailReloadCase },
  { id: 'C4-switch', modes: ['page'], run: runPanelSwitchCase },
]

async function runCase(browser, scenario, viewport, panel, mode) {
  const behavior = newBehavior()
  const failures = []
  const notes = []
  const pageErrors = []
  if (scenario.failFirst !== undefined) behavior.fail.set(scenario.failFirst, { status: 502, body: BACKEND_CONTENT_FAILURE })
  const context = await browser.newContext(contextOptions(viewport))
  try {
    const page = await openReader(context, mode, behavior, pageErrors)
    await scenario.run(page, viewport, panel, behavior, failures, notes)
  } catch (error) {
    failures.push(`harness: ${String(error?.message || error).split('\n')[0]}`)
  } finally {
    failures.push(...pageErrors.map(text => `page error: ${text}`))
    failures.push(...behavior.routeErrors.map(text => `route error: ${text}`))
    await context.close()
  }
  return { failures, notes }
}

function plannedCases() {
  const cases = []
  for (const viewport of VIEWPORTS) {
    for (const scenario of SCENARIOS) {
      for (const mode of scenario.modes) {
        for (const panel of PANELS) {
          cases.push({
            scenario: scenario.id,
            label: `${scenario.id} ${mode} | ${viewport.label} | ${panel.label}`,
            run: browser => runCase(browser, scenario, viewport, panel, mode),
          })
        }
      }
    }
  }
  return cases.filter(row => !caseFilter || caseFilter.test(row.label))
}

async function main() {
  const cases = plannedCases()
  if (!cases.length) throw new Error(`no cases match SMOKE_CASES=${process.env.SMOKE_CASES}`)
  const browser = await openSmokeBrowser()
  const failed = []
  const tally = new Map()
  try {
    for (const row of cases) {
      const result = await row.run(browser)
      const status = result.failures.length ? 'FAIL' : 'PASS'
      console.log(`${status} ${row.label} | ${result.notes.join(' | ')}`)
      for (const failure of result.failures) console.log(`  - ${failure}`)
      const group = tally.get(row.scenario) || { pass: 0, fail: 0, checks: new Map() }
      group[result.failures.length ? 'fail' : 'pass'] += 1
      for (const failure of result.failures) {
        const name = failure.split(':')[0]
        group.checks.set(name, (group.checks.get(name) || 0) + 1)
      }
      tally.set(row.scenario, group)
      if (result.failures.length) failed.push(row.label)
    }
  } finally {
    await browser.close()
  }
  for (const [scenario, group] of tally) {
    const checks = [...group.checks].map(([name, count]) => `${name} x${count}`).join(', ')
    console.log(`summary ${scenario}: ${group.pass} pass, ${group.fail} fail${checks ? ` (${checks})` : ''}`)
  }
  if (failed.length) {
    throw new Error(`reader desktop panel controls contract failed ${failed.length}/${cases.length} cases:\n${failed.join('\n')}`)
  }
  console.log(`reader desktop panel controls contract smoke passed (${cases.length} cases)`)
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})
