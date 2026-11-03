#!/usr/bin/env node

// Reader chapter error surface browser contract
// (docs/compat/reader-chapter-error-surface-fixed-baseline-second-audit-p0-contract.md section 8.2).
// This script currently covers R7 only:
// - E2: the in-content 重新加载 of the main error block is the hit target at its center;
// - E3: after the source recovers, a real pointer press (desktop mouse, phone touch, narrow-window mouse)
//   reloads only the failed chapter: the route, the tool layer and the progress stay on that chapter;
// - E5 (adjacent-chapter part): the continuous inline 重新加载 is the hit target at 12%/50%/88% of the
//   viewport (84% on phones, where reader-dev's 103px bottom bar covers the foot while the tool layer shows)
//   and a press only retries that chapter, without paging or toggling the tool layer.
// E1, E4, E6, E7 and the target-chapter part of E5 belong to the R1-R6/R8 slices.
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
const CONTINUOUS_MODES = ['scroll', 'scroll2']
const BOOK_TITLE = '正文错误面契约'
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
const FAIL_INDEX = 5
const ADJACENT_FAIL_INDEX = 1
// Body the Go backend writes when a source fails to deliver chapter content (books.go chapterContent).
const BACKEND_CONTENT_FAILURE = { error: 'failed to load chapter content', code: 'content_unavailable', stage: 'content' }
const MAIN_BUTTON = '.chapter-load-error button'
const INLINE_BUTTON = `.chapter-content[data-index="${ADJACENT_FAIL_INDEX}"] .chapter-inline-error button`

const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))

function json(data, status = 200) {
  return { status, contentType: 'application/json', body: JSON.stringify(data) }
}

function fakeToken() {
  const payload = Buffer.from(JSON.stringify({ userId: 1, sub: '1' })).toString('base64url')
  return `open.${payload}.reader`
}

function chapterContent(index) {
  return Array.from({ length: 60 }, (_, paragraph) => (
    `${TITLES[index]}第${paragraph + 1}段。正文错误面契约正文，用于验证重新加载按钮的命中与触摸穿透隔离。`
  )).join('\n')
}

function inputsFor(viewport) {
  return viewport.width <= 750 ? ['touch', 'mouse'] : ['mouse']
}

function contextOptions(viewport, input) {
  return input === 'touch' ? { viewport, isMobile: true, hasTouch: true } : { viewport }
}

function newBehavior() {
  return { fail: new Map(), requests: [], responses: [], progressWrites: [], routeErrors: [] }
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
          updatedAt: '2026-10-17T00:00:00Z',
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
        return await route.fulfill(json({ key: 'reader', updatedAt: '2026-10-17T00:00:01Z', value: {} }))
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
        if (failure) {
          behavior.responses.push({ index, status: failure.status, at: Date.now() })
          return await route.fulfill(json(failure.body, failure.status))
        }
        behavior.responses.push({ index, status: 200, at: Date.now() })
        return await route.fulfill(json({
          chapter: { id: index + 11, bookId: 1, index, title: TITLES[index] },
          content: chapterContent(index),
          format: 'text',
        }))
      }
      if (path === '/books/1/bookmarks') return await route.fulfill(json([]))
      if (path === '/progress/1') return await route.fulfill(json({}))
      if (path === '/progress' && method === 'PUT') {
        const body = request.postDataJSON?.() || {}
        behavior.progressWrites.push({ chapterIndex: body.chapterIndex, chapterTitle: body.chapterTitle, at: Date.now() })
        return await route.fulfill(json({ ...body, bookId: 1 }))
      }
      if (path === '/sources' || path === '/categories') return await route.fulfill(json([]))
      return await route.fulfill(json({}))
    } catch (error) {
      behavior.routeErrors.push(String(error?.message || error).split('\n')[0])
    }
  })
}

async function openReader(browser, viewport, input, mode, behavior) {
  const context = await browser.newContext(contextOptions(viewport, input))
  await context.addInitScript(({ readerMode, token }) => {
    localStorage.setItem('openreader_token', token)
    localStorage.setItem('reader', JSON.stringify({ mode: readerMode, settingsScope: 'user:1', progressScope: 'user:1' }))
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
  await page.goto(`${targetUrl}/books/1/read?chapter=0`, { waitUntil: 'domcontentloaded' })
  await page.waitForSelector('.chapter-content[data-index="0"] [data-reader-block]', { timeout: 15_000 })
  return { context, page, pageErrors }
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
    return {
      query: location.search,
      chromeVisible: Boolean(document.querySelector('.reader-mobile-top.visible')),
      scrollTop: Math.round(scroller?.scrollTop || 0),
      mainError: Boolean(document.querySelector('.chapter-load-error')),
      inlineErrors: [...document.querySelectorAll('.chapter-inline-error')]
        .map(node => Number(node.closest('.chapter-content')?.dataset.index)),
      blocks: [...document.querySelectorAll('.reader-body .chapter-content')].map(node => Number(node.dataset.index)),
      paragraphs: Object.fromEntries([...document.querySelectorAll('.reader-body .chapter-content')].map(node => [
        node.dataset.index,
        node.querySelector('[data-reader-block]:not(.chapter-inline-error *)')?.textContent.trim().slice(0, 12) ?? null,
      ])),
    }
  })
}

async function hitTest(page, selector) {
  return page.evaluate((target) => {
    const element = document.querySelector(target)
    if (!element) return null
    const describe = node => (node
      ? node.tagName.toLowerCase() + [...node.classList].map(name => `.${name}`).join('')
      : null)
    const rect = element.getBoundingClientRect()
    const x = rect.left + rect.width / 2
    const y = rect.top + rect.height / 2
    const hit = document.elementFromPoint(x, y)
    return {
      x: Math.round(x),
      y: Math.round(y),
      hitIsTarget: Boolean(hit && (hit === element || element.contains(hit))),
      hit: describe(hit),
    }
  }, selector)
}

async function press(page, input, point) {
  if (input === 'touch') await page.touchscreen.tap(point.x, point.y)
  else await page.mouse.click(point.x, point.y)
}

async function placeButton(page, selector, band) {
  return page.evaluate(({ target, targetBand }) => {
    const button = document.querySelector(target)
    if (!button) return null
    const documentScroll = Boolean(document.querySelector('.reader-shell.document-scroll'))
    const scroller = documentScroll
      ? (document.scrollingElement || document.documentElement)
      : document.querySelector('.reader-content')
    const rect = button.getBoundingClientRect()
    const viewportTop = documentScroll ? 0 : scroller.getBoundingClientRect().top
    const viewportHeight = documentScroll ? window.innerHeight : scroller.clientHeight
    const wanted = viewportTop + viewportHeight * targetBand
    scroller.scrollTop = Math.max(0, scroller.scrollTop + rect.top + rect.height / 2 - wanted)
    ;(documentScroll ? window : scroller).dispatchEvent(new Event('scroll'))
    return Math.round(wanted)
  }, { target: selector, targetBand: band })
}

async function waitUntil(predicate, timeout) {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) {
    if (predicate()) return true
    await sleep(50)
  }
  return predicate()
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

async function runMainCase(browser, viewport, input, mode) {
  const behavior = newBehavior()
  const failures = []
  const notes = []
  const { context, page, pageErrors } = await openReader(browser, viewport, input, mode, behavior)
  try {
    behavior.fail.set(FAIL_INDEX, { status: 502, body: BACKEND_CONTENT_FAILURE })
    await routeToChapter(page, FAIL_INDEX)
    await page.waitForSelector(MAIN_BUTTON, { timeout: 10_000 })
    await page.waitForTimeout(1200)

    const hit = await hitTest(page, MAIN_BUTTON)
    notes.push(`button (${hit.x},${hit.y}) hits ${hit.hit}`)
    check(failures, hit.hitIsTarget, `E2 重新加载 center (${hit.x},${hit.y}) hits ${hit.hit}`)

    const before = await readState(page)
    behavior.fail.delete(FAIL_INDEX)
    const mark = Date.now()
    await press(page, input, hit)
    await page.waitForFunction((index) => (
      !document.querySelector('.chapter-load-error')
      && document.querySelector(`.chapter-content[data-index="${index}"] [data-reader-block]`)
    ), FAIL_INDEX, { timeout: 4000 }).catch(() => {})
    await page.waitForTimeout(1500)
    const after = await readState(page)
    const requests = since(behavior.requests, mark).map(row => row.index)
    const writes = since(behavior.progressWrites, mark)
    notes.push(`requests ${JSON.stringify(requests)} route ${after.query} tools ${before.chromeVisible}->${after.chromeVisible} PUT ${describeWrites(writes)}`)

    check(
      failures,
      requests[0] === FAIL_INDEX && requests.filter(index => index === FAIL_INDEX).length === 1,
      `E3 press must request chapter ${FAIL_INDEX} first and once, got ${JSON.stringify(requests)}`,
    )
    check(
      failures,
      before.query === `?chapter=${FAIL_INDEX}` && after.query === before.query,
      `E3 route ${before.query} -> ${after.query}`,
    )
    check(failures, after.chromeVisible === before.chromeVisible, `E3 tool layer ${before.chromeVisible} -> ${after.chromeVisible}`)
    const strayWrites = writes.filter(row => row.chapterIndex !== FAIL_INDEX || row.chapterTitle !== TITLES[FAIL_INDEX])
    check(failures, strayWrites.length === 0, `E3 progress PUT for another chapter or title ${describeWrites(strayWrites)}`)
    const shown = after.paragraphs[String(FAIL_INDEX)]
    check(
      failures,
      !after.mainError && String(shown || '').startsWith(TITLES[FAIL_INDEX]),
      `E3 after press shows blocks ${JSON.stringify(after.blocks)} (chapter ${FAIL_INDEX}: ${JSON.stringify(shown)}, error block ${after.mainError})`,
    )
  } catch (error) {
    failures.push(`harness: ${String(error?.message || error).split('\n')[0]}`)
  } finally {
    failures.push(...pageErrors.map(text => `page error: ${text}`))
    failures.push(...behavior.routeErrors.map(text => `route error: ${text}`))
    await context.close()
  }
  return { failures, notes }
}

async function pressInline(page, behavior, input, band) {
  const wanted = await placeButton(page, INLINE_BUTTON, band)
  await page.waitForTimeout(350)
  const before = await readState(page)
  const hit = await hitTest(page, INLINE_BUTTON)
  const mark = Date.now()
  await press(page, input, hit)
  return { before, hit, mark, wanted }
}

// reader-dev's mini bottom bar is 103px tall, so on phones the lowest pressable band sits above it.
const lowBand = viewport => (viewport.width <= 750 ? 0.84 : 0.88)

async function runInlineCase(browser, viewport, input, mode) {
  const behavior = newBehavior()
  const failures = []
  const notes = []
  behavior.fail.set(ADJACENT_FAIL_INDEX, { status: 502, body: BACKEND_CONTENT_FAILURE })
  const { context, page, pageErrors } = await openReader(browser, viewport, input, mode, behavior)
  try {
    await page.waitForSelector(INLINE_BUTTON, { timeout: 10_000 })
    await page.waitForTimeout(1200)

    for (const band of [0.12, 0.5, lowBand(viewport)]) {
      const wanted = await placeButton(page, INLINE_BUTTON, band)
      await page.waitForTimeout(350)
      const hit = await hitTest(page, INLINE_BUTTON)
      notes.push(`${band * 100}% wanted y=${wanted} at y=${hit.y} hits ${hit.hit}`)
      check(failures, hit.hitIsTarget, `E5 inline 重新加载 at ${band * 100}% (y=${hit.y}) hits ${hit.hit}`)
    }

    // The source still fails: a press must retry once and change nothing else.
    for (const band of [0.12, lowBand(viewport)]) {
      const { before, hit, mark } = await pressInline(page, behavior, input, band)
      await waitUntil(() => since(behavior.responses, mark).some(row => row.index === ADJACENT_FAIL_INDEX), 2500)
      await page.waitForTimeout(700)
      const after = await readState(page)
      const requests = since(behavior.requests, mark).map(row => row.index)
      notes.push(`${band * 100}% failing press y=${hit.y}: requests ${JSON.stringify(requests)} scroll ${before.scrollTop}->${after.scrollTop} tools ${before.chromeVisible}->${after.chromeVisible}`)
      check(failures, JSON.stringify(requests) === JSON.stringify([ADJACENT_FAIL_INDEX]), `E5 ${band * 100}% press (source failing) requests ${JSON.stringify(requests)}`)
      check(failures, Math.abs(after.scrollTop - before.scrollTop) <= 1, `E5 ${band * 100}% press scroll ${before.scrollTop} -> ${after.scrollTop}`)
      check(failures, after.chromeVisible === before.chromeVisible, `E5 ${band * 100}% press tool layer ${before.chromeVisible} -> ${after.chromeVisible}`)
      check(failures, after.query === before.query, `E5 ${band * 100}% press route ${before.query} -> ${after.query}`)
      check(failures, after.inlineErrors.includes(ADJACENT_FAIL_INDEX), `E5 ${band * 100}% press lost the inline error block ${JSON.stringify(after.blocks)}`)
    }

    // The source recovers: a press in the middle band replaces the block in place.
    behavior.fail.delete(ADJACENT_FAIL_INDEX)
    const { before, hit, mark } = await pressInline(page, behavior, input, 0.5)
    await page.waitForFunction((index) => (
      !document.querySelector(`.chapter-content[data-index="${index}"] .chapter-inline-error`)
      && document.querySelectorAll(`.chapter-content[data-index="${index}"] [data-reader-block]`).length > 1
    ), ADJACENT_FAIL_INDEX, { timeout: 4000 }).catch(() => {})
    await page.waitForTimeout(500)
    const after = await readState(page)
    const requests = since(behavior.requests, mark).map(row => row.index)
    notes.push(`50% recovered press y=${hit.y}: requests ${JSON.stringify(requests)} scroll ${before.scrollTop}->${after.scrollTop} tools ${before.chromeVisible}->${after.chromeVisible}`)
    check(
      failures,
      requests[0] === ADJACENT_FAIL_INDEX && requests.filter(index => index === ADJACENT_FAIL_INDEX).length === 1,
      `E5 50% press (source recovered) requests ${JSON.stringify(requests)}`,
    )
    check(
      failures,
      !after.inlineErrors.includes(ADJACENT_FAIL_INDEX)
        && String(after.paragraphs[String(ADJACENT_FAIL_INDEX)] || '').startsWith(TITLES[ADJACENT_FAIL_INDEX]),
      `E5 50% press did not replace the inline error block (${JSON.stringify(after.inlineErrors)})`,
    )
    check(failures, after.chromeVisible === before.chromeVisible, `E5 50% press tool layer ${before.chromeVisible} -> ${after.chromeVisible}`)
    check(failures, after.query === before.query, `E5 50% press route ${before.query} -> ${after.query}`)
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
    for (const input of inputsFor(viewport)) {
      for (const mode of MODES) {
        cases.push({ label: `main ${viewport.width}x${viewport.height} ${input} ${mode}`, run: browser => runMainCase(browser, viewport, input, mode) })
      }
      for (const mode of CONTINUOUS_MODES) {
        cases.push({ label: `inline ${viewport.width}x${viewport.height} ${input} ${mode}`, run: browser => runInlineCase(browser, viewport, input, mode) })
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
    throw new Error(`reader error surface contract failed ${failed.length}/${cases.length} cases:\n${failed.join('\n')}`)
  }
  console.log(`reader error surface contract smoke passed (${cases.length} cases)`)
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})
