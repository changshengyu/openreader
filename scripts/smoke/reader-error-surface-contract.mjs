#!/usr/bin/env node

// Reader chapter error surface browser contract
// (docs/compat/reader-chapter-error-surface-fixed-baseline-second-audit-p0-contract.md sections 8.2 and 13).
// - E1 (page, flip): the failed chapter shows reader-dev's error chapter page: <h3> with the target catalog title,
//   获取章节内容失败！ and the mapped Chinese reason; one 获取章节内容失败 <reason> toast; no 加载下一章; on phones the
//   page label is the error page's own 第 1/1 页;
// - E2: the error page's 重新加载 is the hit target at its center;
// - E3: after the source recovers, a real pointer press (desktop mouse, phone touch, narrow-window mouse)
//   reloads only the failed chapter: the route, the tool layer and the progress stay on that chapter;
// - E4: the toolbar 重新载入章节 still reloads the failed chapter in place;
// - E5: in scroll and scroll2 a failed target chapter stays in the continuous window as its own inline error
//   block (<h3>, 获取章节内容失败！, reason) and the following chapter loads (R6); the main-case E2/E3 checks press that
//   block's 重新加载 there (contract section 13 item 9). For an adjacent failure the inline 重新加载 is the hit target at
//   12%/50%/88% of the viewport (84% on phones, where reader-dev's 103px bottom bar covers the foot while the tool
//   layer shows) and a press only retries that chapter, without paging or toggling the tool layer; no toast;
// - E6: the D1 reasons for a network failure, a stale conflict that survives the automatic refetch, the two source
//   failure codes and a 404; no backend English on the page or in the toast;
// - E7: a failed catalog shows 获取章节目录失败！ with the mapped reason, no chapter title and a
//   获取书籍目录列表 <reason> toast; after recovery a real pointer press reloads the catalog and shows the routed chapter.
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
const NEXT_AFTER_FAIL = FAIL_INDEX + 1
const CATALOG_TARGET = 2
const CONTENT_LABEL = '获取章节内容失败！'
const CATALOG_LABEL = '获取章节目录失败！'
const SOURCE_REASON = '书源没有返回可用正文'
const MAIN_BUTTON = '.chapter-load-error button'
const TARGET_INLINE_BUTTON = `.chapter-content[data-index="${FAIL_INDEX}"] .chapter-inline-error button`
const INLINE_BUTTON = `.chapter-content[data-index="${ADJACENT_FAIL_INDEX}"] .chapter-inline-error button`
const BACKEND_ENGLISH = ['failed to load chapter content', 'failed to list chapters', 'chapter not found', 'chapter content changed']

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
  return {
    fail: new Map(),
    catalogFailure: null,
    catalogRequests: [],
    requests: [],
    responses: [],
    progressWrites: [],
    routeErrors: [],
  }
}

const isContinuous = mode => CONTINUOUS_MODES.includes(mode)

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
        behavior.catalogRequests.push({ at: Date.now() })
        if (behavior.catalogFailure) {
          return await route.fulfill(json(behavior.catalogFailure.body, behavior.catalogFailure.status))
        }
        return await route.fulfill(json(TITLES.map((title, index) => ({ id: index + 11, bookId: 1, index, title }))))
      }
      const contentMatch = path.match(/^\/books\/1\/chapters\/(\d+)\/content$/)
      if (contentMatch) {
        const index = Number(contentMatch[1])
        behavior.requests.push({ index, at: Date.now() })
        const failure = behavior.fail.get(index)
        if (failure?.abort) {
          behavior.responses.push({ index, status: 0, at: Date.now() })
          return await route.abort('failed')
        }
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

async function openReader(browser, viewport, input, mode, behavior, openOptions = {}) {
  const context = await browser.newContext(contextOptions(viewport, input))
  await context.addInitScript(({ readerMode, token }) => {
    localStorage.setItem('openreader_token', token)
    localStorage.setItem('reader', JSON.stringify({ mode: readerMode, settingsScope: 'user:1', progressScope: 'user:1' }))
    // Every Element Plus message the page shows, in order (R4 toasts).
    window.__toastLog = []
    const record = (node) => {
      if (!(node instanceof Element)) return
      const messages = node.matches('.el-message') ? [node] : [...node.querySelectorAll('.el-message')]
      for (const message of messages) {
        window.__toastLog.push({
          error: message.classList.contains('el-message--error'),
          text: (message.querySelector('.el-message__content')?.textContent || message.textContent || '').trim(),
          at: Date.now(),
        })
      }
    }
    new MutationObserver(records => records.forEach(row => row.addedNodes.forEach(record)))
      .observe(document, { childList: true, subtree: true })
  }, { readerMode: mode, token: fakeToken() })
  const page = await context.newPage()
  const pageErrors = []
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    const text = message.text()
    if (text.includes('/ws/sync')) return
    if (/Failed to load resource: the server responded with a status of (404|409|500|502)/.test(text)) return
    if (text.includes('Failed to load resource: net::ERR_FAILED')) return
    pageErrors.push(text.split('\n')[0])
  })
  page.on('pageerror', error => pageErrors.push(error.message))
  await installMocks(page, behavior, mode)
  await page.goto(`${targetUrl}/books/1/read?chapter=${openOptions.chapter ?? 0}`, { waitUntil: 'domcontentloaded' })
  await page.waitForSelector(openOptions.waitFor || '.chapter-content[data-index="0"] [data-reader-block]', { timeout: 15_000 })
  return { context, page, pageErrors }
}

async function toastsSince(page, mark) {
  return page.evaluate(since => (window.__toastLog || []).filter(row => row.at >= since), mark)
}

// The page clock and the Node clock are the same machine clock; marks are taken in Node.
function describeToasts(rows) {
  return JSON.stringify(rows.map(row => `${row.error ? 'error' : 'info'}:${row.text}`))
}

async function readErrorSurface(page) {
  return page.evaluate(({ failIndex, nextIndex }) => {
    const text = node => (node ? node.textContent.trim() : null)
    const main = document.querySelector('.chapter-load-error')
    const target = document.querySelector(`.chapter-content[data-index="${failIndex}"] .chapter-inline-error`)
    const next = document.querySelector(`.chapter-content[data-index="${nextIndex}"]`)
    return {
      main: main
        ? {
            title: text(main.querySelector('h3')),
            titlePos: main.querySelector('h3')?.getAttribute('data-pos') ?? null,
            lines: [...main.querySelectorAll('p')].map(text),
            firstChild: main.firstElementChild?.tagName.toLowerCase() || null,
          }
        : null,
      target: target
        ? { title: text(target.querySelector('h3')), lines: [...target.querySelectorAll('p')].map(text) }
        : null,
      nextParagraph: text(next?.querySelector('[data-reader-block]:not(.chapter-inline-error *)')),
      chapterEnd: Boolean(document.querySelector('.reader-chapter-end')),
      pageLabel: text(document.querySelector('.reader-mobile-bottom .progress-tip')),
      bodyText: document.querySelector('.reader-body')?.textContent || '',
    }
  }, { failIndex: FAIL_INDEX, nextIndex: NEXT_AFTER_FAIL })
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

function checkNoBackendEnglish(failures, label, ...texts) {
  for (const text of texts) {
    for (const english of BACKEND_ENGLISH) {
      check(failures, !String(text || '').includes(english), `${label} shows backend English ${JSON.stringify(english)}`)
    }
  }
}

async function runMainCase(browser, viewport, input, mode) {
  const behavior = newBehavior()
  const failures = []
  const notes = []
  const continuous = isContinuous(mode)
  const button = continuous ? TARGET_INLINE_BUTTON : MAIN_BUTTON
  const { context, page, pageErrors } = await openReader(browser, viewport, input, mode, behavior)
  try {
    behavior.fail.set(FAIL_INDEX, { status: 502, body: BACKEND_CONTENT_FAILURE })
    const failMark = Date.now()
    await routeToChapter(page, FAIL_INDEX)
    await page.waitForSelector(`${MAIN_BUTTON}, ${TARGET_INLINE_BUTTON}`, { timeout: 10_000 })
    if (continuous) {
      await page.waitForFunction(index => document.querySelector(
        `.chapter-content[data-index="${index}"] [data-reader-block]:not(.chapter-inline-error *)`,
      ), NEXT_AFTER_FAIL, { timeout: 4000 }).catch(() => {})
    }
    await page.waitForTimeout(1200)

    const surface = await readErrorSurface(page)
    const toasts = (await toastsSince(page, failMark)).filter(row => row.error)
    if (continuous) {
      notes.push(`target ${JSON.stringify(surface.target)} next ${JSON.stringify(surface.nextParagraph)} main ${Boolean(surface.main)}`)
      check(failures, !surface.main, `E5 ${mode}: a failed target chapter must stay in the window, got the main error page ${JSON.stringify(surface.main)}`)
      check(failures, surface.target?.title === TITLES[FAIL_INDEX], `E5 target <h3> ${JSON.stringify(surface.target?.title)} (want ${TITLES[FAIL_INDEX]})`)
      check(
        failures,
        JSON.stringify(surface.target?.lines) === JSON.stringify([CONTENT_LABEL, SOURCE_REASON]),
        `E5 target lines ${JSON.stringify(surface.target?.lines)} (want ${JSON.stringify([CONTENT_LABEL, SOURCE_REASON])})`,
      )
      check(
        failures,
        String(surface.nextParagraph || '').startsWith(TITLES[NEXT_AFTER_FAIL]),
        `E5 chapter ${NEXT_AFTER_FAIL} after the failed target shows ${JSON.stringify(surface.nextParagraph)}`,
      )
    } else {
      notes.push(`page ${JSON.stringify(surface.main)} label ${JSON.stringify(surface.pageLabel)}`)
      check(failures, surface.main?.firstChild === 'h3' && surface.main?.titlePos === '0', `E1 error page must start with <h3 data-pos="0">, got ${JSON.stringify(surface.main)}`)
      check(failures, surface.main?.title === TITLES[FAIL_INDEX], `E1 error page <h3> ${JSON.stringify(surface.main?.title)} (want ${TITLES[FAIL_INDEX]})`)
      check(
        failures,
        JSON.stringify(surface.main?.lines) === JSON.stringify([CONTENT_LABEL, SOURCE_REASON]),
        `E1 error page lines ${JSON.stringify(surface.main?.lines)} (want ${JSON.stringify([CONTENT_LABEL, SOURCE_REASON])})`,
      )
      check(failures, !surface.chapterEnd, 'E1 加载下一章 must stay hidden on the error page')
      if (viewport.width <= 750) {
        check(failures, surface.pageLabel === '第 1/1 页', `E1 mobile page label ${JSON.stringify(surface.pageLabel)} (want the error page's 第 1/1 页)`)
      }
    }
    notes.push(`toasts ${describeToasts(toasts)}`)
    check(
      failures,
      toasts.length === 1 && toasts[0].text === `获取章节内容失败 ${SOURCE_REASON}`,
      `E1 toasts ${describeToasts(toasts)} (want one 获取章节内容失败 ${SOURCE_REASON})`,
    )
    checkNoBackendEnglish(failures, 'E1 page', surface.bodyText)
    checkNoBackendEnglish(failures, 'E1 toast', ...toasts.map(row => row.text))

    const hit = await hitTest(page, button)
    notes.push(`button (${hit?.x},${hit?.y}) hits ${hit?.hit}`)
    check(failures, hit?.hitIsTarget, `E2 重新加载 center (${hit?.x},${hit?.y}) hits ${hit?.hit}`)

    const before = await readState(page)
    behavior.fail.delete(FAIL_INDEX)
    const mark = Date.now()
    if (hit) await press(page, input, hit)
    await page.waitForFunction((index) => (
      !document.querySelector('.chapter-load-error')
      && !document.querySelector(`.chapter-content[data-index="${index}"] .chapter-inline-error`)
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
      !after.mainError && !after.inlineErrors.includes(FAIL_INDEX) && String(shown || '').startsWith(TITLES[FAIL_INDEX]),
      `E3 after press shows blocks ${JSON.stringify(after.blocks)} (chapter ${FAIL_INDEX}: ${JSON.stringify(shown)}, error block ${after.mainError}, inline ${JSON.stringify(after.inlineErrors)})`,
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
  const openMark = Date.now()
  const { context, page, pageErrors } = await openReader(browser, viewport, input, mode, behavior)
  try {
    await page.waitForSelector(INLINE_BUTTON, { timeout: 10_000 })
    await page.waitForTimeout(1200)

    const inline = await page.evaluate((index) => {
      const block = document.querySelector(`.chapter-content[data-index="${index}"] .chapter-inline-error`)
      return block
        ? { title: block.querySelector('h3')?.textContent.trim() ?? null, lines: [...block.querySelectorAll('p')].map(node => node.textContent.trim()) }
        : null
    }, ADJACENT_FAIL_INDEX)
    const adjacentToasts = (await toastsSince(page, openMark)).filter(row => row.error)
    notes.push(`inline ${JSON.stringify(inline)} toasts ${describeToasts(adjacentToasts)}`)
    check(failures, inline?.title === TITLES[ADJACENT_FAIL_INDEX], `E5 inline <h3> ${JSON.stringify(inline?.title)}`)
    check(
      failures,
      JSON.stringify(inline?.lines) === JSON.stringify([CONTENT_LABEL, SOURCE_REASON]),
      `E5 inline lines ${JSON.stringify(inline?.lines)} (want ${JSON.stringify([CONTENT_LABEL, SOURCE_REASON])})`,
    )
    check(failures, adjacentToasts.length === 0, `E5 an adjacent chapter failure must not toast, got ${describeToasts(adjacentToasts)}`)

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

const TOOLBAR_RELOAD = viewport => (viewport.width <= 750
  ? '.reader-mobile-float-right.visible button[aria-label="重新载入章节"]'
  : '.reader-right-rail .round-tool[aria-label="重新载入章节"]')

// E4: the toolbar retry keeps its current behavior on the failed chapter (regression control).
async function runToolbarCase(browser, viewport, input, mode) {
  const behavior = newBehavior()
  const failures = []
  const notes = []
  const { context, page, pageErrors } = await openReader(browser, viewport, input, mode, behavior)
  try {
    behavior.fail.set(FAIL_INDEX, { status: 502, body: BACKEND_CONTENT_FAILURE })
    await routeToChapter(page, FAIL_INDEX)
    await page.waitForSelector(`${MAIN_BUTTON}, ${TARGET_INLINE_BUTTON}`, { timeout: 10_000 })
    await page.waitForTimeout(1200)
    const hit = await hitTest(page, TOOLBAR_RELOAD(viewport))
    check(failures, hit?.hitIsTarget, `E4 重新载入章节 (${hit?.x},${hit?.y}) hits ${hit?.hit}`)
    const before = await readState(page)
    behavior.fail.delete(FAIL_INDEX)
    const mark = Date.now()
    if (hit) await press(page, input, hit)
    await page.waitForFunction((index) => (
      !document.querySelector('.chapter-load-error')
      && !document.querySelector(`.chapter-content[data-index="${index}"] .chapter-inline-error`)
      && document.querySelector(`.chapter-content[data-index="${index}"] [data-reader-block]`)
    ), FAIL_INDEX, { timeout: 4000 }).catch(() => {})
    const readerToast = await page.waitForSelector('.reader-toast', { timeout: 2500 })
      .then(node => node.textContent()).catch(() => null)
    await page.waitForTimeout(800)
    const after = await readState(page)
    const requests = since(behavior.requests, mark).map(row => row.index)
    notes.push(`requests ${JSON.stringify(requests)} route ${after.query} toast ${JSON.stringify(readerToast)}`)
    check(
      failures,
      requests[0] === FAIL_INDEX && requests.filter(index => index === FAIL_INDEX).length === 1,
      `E4 toolbar reload must request chapter ${FAIL_INDEX} first and once, got ${JSON.stringify(requests)}`,
    )
    check(failures, after.query === before.query && after.query === `?chapter=${FAIL_INDEX}`, `E4 route ${before.query} -> ${after.query}`)
    check(
      failures,
      !after.mainError && !after.inlineErrors.includes(FAIL_INDEX)
        && String(after.paragraphs[String(FAIL_INDEX)] || '').startsWith(TITLES[FAIL_INDEX]),
      `E4 after the toolbar reload shows blocks ${JSON.stringify(after.blocks)} (inline ${JSON.stringify(after.inlineErrors)})`,
    )
    check(failures, String(readerToast || '').trim() === '章节已重新载入', `E4 reload toast ${JSON.stringify(readerToast)}`)
  } catch (error) {
    failures.push(`harness: ${String(error?.message || error).split('\n')[0]}`)
  } finally {
    failures.push(...pageErrors.map(text => `page error: ${text}`))
    failures.push(...behavior.routeErrors.map(text => `route error: ${text}`))
    await context.close()
  }
  return { failures, notes }
}

const COPY_VARIANTS = [
  { label: 'network failure', failure: { abort: true }, reason: '章节加载失败，请检查书源或网络后重试' },
  { label: 'stale conflict after the refetch', failure: { status: 409, body: { error: 'chapter content changed; retry' } }, reason: '章节状态已更新，请重试' },
  { label: '502 content_unavailable', failure: { status: 502, body: BACKEND_CONTENT_FAILURE }, reason: SOURCE_REASON },
  {
    label: '502 source_request_failed',
    failure: { status: 502, body: { error: 'failed to load chapter content', code: 'source_request_failed', stage: 'content' } },
    reason: '请求书源失败，请检查网络或稍后重试',
  },
  { label: '404', failure: { status: 404, body: { error: 'chapter not found' } }, reason: '章节或书籍不存在，请刷新目录后重试' },
]

// E6: D1 reasons on the page and in the toast (page mode).
async function runCopyCase(browser, viewport, input) {
  const behavior = newBehavior()
  const failures = []
  const notes = []
  const { context, page, pageErrors } = await openReader(browser, viewport, input, 'page', behavior)
  try {
    for (const variant of COPY_VARIANTS) {
      behavior.fail.set(FAIL_INDEX, variant.failure)
      const mark = Date.now()
      await routeToChapter(page, FAIL_INDEX)
      await page.waitForSelector(MAIN_BUTTON, { timeout: 10_000 })
      await page.waitForTimeout(900)
      const surface = await readErrorSurface(page)
      const toasts = (await toastsSince(page, mark)).filter(row => row.error)
      notes.push(`${variant.label}: ${JSON.stringify(surface.main?.lines)} ${describeToasts(toasts)}`)
      check(
        failures,
        JSON.stringify(surface.main?.lines) === JSON.stringify([CONTENT_LABEL, variant.reason]),
        `E6 ${variant.label}: lines ${JSON.stringify(surface.main?.lines)} (want ${JSON.stringify([CONTENT_LABEL, variant.reason])})`,
      )
      check(
        failures,
        toasts.length === 1 && toasts[0].text === `获取章节内容失败 ${variant.reason}`,
        `E6 ${variant.label}: toasts ${describeToasts(toasts)}`,
      )
      checkNoBackendEnglish(failures, `E6 ${variant.label} page`, surface.bodyText)
      checkNoBackendEnglish(failures, `E6 ${variant.label} toast`, ...toasts.map(row => row.text))
      behavior.fail.delete(FAIL_INDEX)
      await routeToChapter(page, 0)
      await page.waitForSelector('.chapter-content[data-index="0"] [data-reader-block]', { timeout: 10_000 })
      await page.waitForTimeout(400)
    }
  } catch (error) {
    failures.push(`harness: ${String(error?.message || error).split('\n')[0]}`)
  } finally {
    failures.push(...pageErrors.map(text => `page error: ${text}`))
    failures.push(...behavior.routeErrors.map(text => `route error: ${text}`))
    await context.close()
  }
  return { failures, notes }
}

// E7: a failed catalog is an error page without a chapter title; its 重新加载 reloads the whole book.
async function runCatalogCase(browser, viewport, input) {
  const behavior = newBehavior()
  const failures = []
  const notes = []
  behavior.catalogFailure = { status: 500, body: { error: 'failed to list chapters' } }
  const openMark = Date.now()
  const { context, page, pageErrors } = await openReader(browser, viewport, input, 'page', behavior, {
    chapter: CATALOG_TARGET,
    waitFor: MAIN_BUTTON,
  })
  try {
    await page.waitForTimeout(1200)
    const surface = await readErrorSurface(page)
    const toasts = (await toastsSince(page, openMark)).filter(row => row.error)
    const reason = '服务器处理失败，请稍后重试'
    notes.push(`page ${JSON.stringify(surface.main)} toasts ${describeToasts(toasts)}`)
    check(failures, !surface.main?.title, `E7 catalog failure must show no chapter title, got ${JSON.stringify(surface.main?.title)}`)
    check(
      failures,
      JSON.stringify(surface.main?.lines) === JSON.stringify([CATALOG_LABEL, reason]),
      `E7 lines ${JSON.stringify(surface.main?.lines)} (want ${JSON.stringify([CATALOG_LABEL, reason])})`,
    )
    check(
      failures,
      toasts.length === 1 && toasts[0].text === `获取书籍目录列表 ${reason}`,
      `E7 toasts ${describeToasts(toasts)} (want one 获取书籍目录列表 ${reason})`,
    )
    checkNoBackendEnglish(failures, 'E7 page', surface.bodyText)
    check(failures, behavior.requests.length === 0, `E7 no chapter content may load without a catalog, got ${JSON.stringify(behavior.requests.map(row => row.index))}`)

    behavior.catalogFailure = null
    const hit = await hitTest(page, MAIN_BUTTON)
    check(failures, hit?.hitIsTarget, `E7 重新加载 (${hit?.x},${hit?.y}) hits ${hit?.hit}`)
    const mark = Date.now()
    if (hit) await press(page, input, hit)
    await page.waitForFunction((index) => (
      !document.querySelector('.chapter-load-error')
      && document.querySelector(`.chapter-content[data-index="${index}"] [data-reader-block]`)
    ), CATALOG_TARGET, { timeout: 5000 }).catch(() => {})
    await page.waitForTimeout(800)
    const after = await readState(page)
    const catalogRequests = since(behavior.catalogRequests, mark).length
    const requests = since(behavior.requests, mark).map(row => row.index)
    notes.push(`catalog requests ${catalogRequests} content ${JSON.stringify(requests)} route ${after.query} blocks ${JSON.stringify(after.paragraphs)}`)
    check(failures, catalogRequests >= 1, 'E7 重新加载 must request the catalog again')
    check(failures, after.query === `?chapter=${CATALOG_TARGET}`, `E7 route ${after.query}`)
    check(
      failures,
      !after.mainError && String(after.paragraphs[String(CATALOG_TARGET)] || '').startsWith(TITLES[CATALOG_TARGET]),
      `E7 after recovery shows ${JSON.stringify(after.paragraphs)} (want ${TITLES[CATALOG_TARGET]} under ?chapter=${CATALOG_TARGET})`,
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

const primaryInput = viewport => (viewport.width <= 750 ? 'touch' : 'mouse')

function plannedCases() {
  const cases = []
  for (const viewport of VIEWPORTS) {
    const size = `${viewport.width}x${viewport.height}`
    for (const input of inputsFor(viewport)) {
      for (const mode of MODES) {
        cases.push({ label: `main ${size} ${input} ${mode}`, run: browser => runMainCase(browser, viewport, input, mode) })
      }
      for (const mode of CONTINUOUS_MODES) {
        cases.push({ label: `inline ${size} ${input} ${mode}`, run: browser => runInlineCase(browser, viewport, input, mode) })
      }
    }
    const input = primaryInput(viewport)
    for (const mode of MODES) {
      cases.push({ label: `toolbar ${size} ${input} ${mode}`, run: browser => runToolbarCase(browser, viewport, input, mode) })
    }
    cases.push({ label: `copy ${size} ${input} page`, run: browser => runCopyCase(browser, viewport, input) })
    cases.push({ label: `catalog ${size} ${input} page`, run: browser => runCatalogCase(browser, viewport, input) })
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
