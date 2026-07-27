#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'
const fixtureText = Array.from({ length: 2400 }, (_, index) => (
  `第${index + 1}段。春风过处，纸页微明。用于验证深章节点击翻页的绘制与空闲结算。`
)).join('\n')
const customBackground = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Wl2nJ8AAAAASUVORK5CYII='

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function close(actual, expected, tolerance, message) {
  if (Math.abs(actual - expected) > tolerance) {
    throw new Error(`${message}: expected ${expected}±${tolerance}, got ${actual}`)
  }
}

function token() {
  const payload = Buffer.from(JSON.stringify({ userId: 1, sub: '1' })).toString('base64url')
  return `open.${payload}.reader`
}

function response(data, status = 200) {
  return { status, contentType: 'application/json', body: JSON.stringify(data) }
}

async function installApiMocks(page, appearance) {
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    if (path === '/me') return route.fulfill(response({ id: 1, username: 'deep-page', role: 'admin' }))
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(response({
        key: 'reader',
        updatedAt: '2026-07-27T00:00:00Z',
        value: {
          mode: 'page',
          pageMode: 'auto',
          theme: 'parchment',
          themeType: 'day',
          fontSize: 18,
          fontWeight: 400,
          lineHeight: 1.8,
          paragraphSpace: 0.2,
          columnWidth: 800,
          animateDuration: 300,
          clickMethod: 'auto',
          autoTheme: false,
          brightness: appearance.brightness,
          customBgImage: appearance.custom ? customBackground : '',
          customBgImageList: appearance.custom ? [customBackground] : [],
        },
      }))
    }
    if (path === '/settings/reader' && method === 'PUT') {
      return route.fulfill(response({ key: 'reader', value: {} }))
    }
    const book = {
      id: 1,
      title: '深章节翻页合同',
      author: 'OpenReader',
      sourceId: 2,
      sourceName: '测试书源',
      url: 'https://source.example/book/deep-page',
      bookUrl: 'https://source.example/book/deep-page',
      chapterCount: 2,
      categoryIds: [],
      progress: null,
    }
    if (path === '/books/1') return route.fulfill(response(book))
    if (path === '/books') return route.fulfill(response([book]))
    if (path === '/books/1/chapters') {
      return route.fulfill(response([
        { id: 11, index: 0, title: '第一章' },
        { id: 12, index: 1, title: '第二章' },
      ]))
    }
    if (/^\/books\/1\/chapters\/\d+\/content$/.test(path)) {
      return route.fulfill(response({
        chapter: { id: 11, index: 0, title: '第一章' },
        content: fixtureText,
      }))
    }
    if (path === '/progress/1') return route.fulfill(response({}))
    if (path === '/progress' && method === 'PUT') {
      const payload = request.postDataJSON()
      return route.fulfill(response({ ...payload, updatedAt: new Date().toISOString() }))
    }
    if (path === '/sources') return route.fulfill(response([]))
    if (path === '/categories') return route.fulfill(response([]))
    return route.fulfill(response({}))
  })
}

async function stopTrace(cdp) {
  const completed = new Promise(resolve => cdp.once('Tracing.tracingComplete', resolve))
  await cdp.send('Tracing.end')
  const { stream } = await completed
  let raw = ''
  while (true) {
    const chunk = await cdp.send('IO.read', { handle: stream })
    raw += chunk.data || ''
    if (chunk.eof) break
  }
  await cdp.send('IO.close', { handle: stream })
  return JSON.parse(raw).traceEvents || []
}

async function verifyDeepPage(browser, viewport, appearance) {
  const context = await browser.newContext({
    viewport,
    hasTouch: true,
    isMobile: true,
  })
  await context.addInitScript(value => localStorage.setItem('openreader_token', value), token())
  const page = await context.newPage()
  await installApiMocks(page, appearance)
  await page.goto(`${targetUrl.replace(/\/$/, '')}/books/1/read?chapter=0`, { waitUntil: 'networkidle' })
  await page.waitForSelector('.reader-body [data-reader-block]', { timeout: 15_000 })
  if (await page.locator('.reader-mobile-top.visible').count()) {
    await page.touchscreen.tap(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
    await page.waitForFunction(() => !document.querySelector('.reader-mobile-top.visible'))
  }

  const initial = await page.evaluate(() => {
    const root = document.scrollingElement || document.documentElement
    root.scrollTop = Math.round((root.scrollHeight - root.clientHeight) * 0.72)
    return {
      blockCount: document.querySelectorAll('.reader-body h3[data-pos], .reader-body [data-reader-block]').length,
      scrollHeight: root.scrollHeight,
      startTop: root.scrollTop,
    }
  })
  assert(initial.blockCount >= 2000, `${viewport.width}: deep fixture rendered only ${initial.blockCount} blocks`)
  await page.waitForTimeout(850)

  const renderSurface = await page.locator('.reader-page').evaluate(element => {
    const style = getComputedStyle(element)
    const overlay = getComputedStyle(element, '::after')
    return {
      attachment: style.backgroundAttachment,
      backgroundImage: style.backgroundImage,
      backgroundSize: style.backgroundSize,
      boxShadow: style.boxShadow,
      overlayColor: overlay.backgroundColor,
      overlayPosition: overlay.position,
    }
  })
  assert(renderSurface.boxShadow === 'none', `${viewport.width}: mobile chapter-height shadow remained ${JSON.stringify(renderSurface)}`)
  assert(renderSurface.overlayPosition === 'fixed', `${viewport.width}: brightness overlay is not viewport-sized ${JSON.stringify(renderSurface)}`)
  if (appearance.custom) {
    assert(renderSurface.attachment === 'fixed' && renderSurface.backgroundSize === 'cover', `${viewport.width}: custom background is not viewport-limited ${JSON.stringify(renderSurface)}`)
    assert(renderSurface.overlayColor !== 'rgba(0, 0, 0, 0)', `${viewport.width}: dimmed appearance lost its brightness overlay`)
  } else {
    assert(renderSurface.backgroundImage.includes('data:image/png;base64'), `${viewport.width}: default upstream texture is not a small embedded image ${JSON.stringify(renderSurface)}`)
    assert(renderSurface.backgroundSize === 'auto', `${viewport.width}: default texture is still chapter-height cover ${JSON.stringify(renderSurface)}`)
  }

  await page.evaluate(() => {
    const root = document.scrollingElement || document.documentElement
    const blocks = [...document.querySelectorAll('.reader-body h3[data-pos], .reader-body [data-reader-block]')]
    window.__deepPageProbe = {
      frames: [],
      inputAt: null,
      longTasks: [],
      reads: [],
      startTop: root.scrollTop,
    }
    for (const block of blocks) {
      const original = block.getBoundingClientRect.bind(block)
      block.getBoundingClientRect = () => {
        window.__deepPageProbe.reads.push(performance.now())
        return original()
      }
    }
    if (PerformanceObserver.supportedEntryTypes.includes('longtask')) {
      const observer = new PerformanceObserver(list => {
        window.__deepPageProbe.longTasks.push(...list.getEntries().map(entry => ({
          duration: entry.duration,
          startTime: entry.startTime,
        })))
      })
      observer.observe({ type: 'longtask', buffered: true })
      window.__deepPageProbe.observer = observer
    }
    document.querySelector('.reader-page').addEventListener('touchend', () => {
      window.__deepPageProbe.inputAt = performance.now()
    }, { once: true })
    const startedAt = performance.now()
    const sample = at => {
      window.__deepPageProbe.frames.push(at)
      if (at - startedAt < 1400) requestAnimationFrame(sample)
    }
    requestAnimationFrame(sample)
  })

  const cdp = await context.newCDPSession(page)
  await cdp.send('Emulation.setCPUThrottlingRate', { rate: 6 })
  await cdp.send('Tracing.start', {
    categories: 'devtools.timeline,disabled-by-default-devtools.timeline.frame,cc,blink',
    transferMode: 'ReturnAsStream',
  })
  await page.touchscreen.tap(
    Math.round(viewport.width / 2),
    Math.round(viewport.height * 0.8),
  )
  await page.waitForTimeout(1300)
  const trace = await stopTrace(cdp)

  const runtime = await page.evaluate(() => {
    const probe = window.__deepPageProbe
    const inputAt = probe.inputAt
    const inWindow = (values, start, end) => values.filter(value => (
      value >= inputAt + start && value <= inputAt + end
    ))
    const frameGaps = probe.frames.slice(1).map((at, index) => ({
      at,
      gap: at - probe.frames[index],
    }))
    return {
      finalTop: (document.scrollingElement || document.documentElement).scrollTop,
      inputAt,
      readsAtSettlement: inWindow(probe.reads, 280, 560).length,
      readsBeforeIdleSave: inWindow(probe.reads, 560, 750).length,
      totalReads: probe.reads.length,
      longTasks: probe.longTasks.filter(task => (
        task.startTime >= inputAt - 5 && task.startTime <= inputAt + 750
      )),
      maxFrameGap: Math.max(
        0,
        ...frameGaps
          .filter(frame => frame.at >= inputAt && frame.at <= inputAt + 600)
          .map(frame => frame.gap),
      ),
      startTop: probe.startTop,
    }
  })
  const rasterTime = trace
    .filter(event => event.ph === 'X' && event.name === 'RasterTask')
    .reduce((total, event) => total + Number(event.dur || 0) / 1000, 0)
  const expectedStep = viewport.height - 72
  close(runtime.finalTop - runtime.startTop, expectedStep, 3, `${viewport.width}: deep page click step`)
  assert(runtime.readsAtSettlement < 80, `${viewport.width}: settlement scanned too many deep blocks ${JSON.stringify(runtime)}`)
  assert(runtime.readsBeforeIdleSave < 80, `${viewport.width}: visual window performed unexpected idle-save geometry work ${JSON.stringify(runtime)}`)
  assert(runtime.totalReads < 220, `${viewport.width}: one deep page click performed unbounded geometry work ${JSON.stringify(runtime)}`)
  assert(runtime.longTasks.length === 0, `${viewport.width}: deep page visual window exposed a Long Task ${JSON.stringify(runtime)}`)
  assert(runtime.maxFrameGap <= 50, `${viewport.width}: deep page click stalled visible motion for ${runtime.maxFrameGap}ms`)
  assert(rasterTime < 60, `${viewport.width}: mobile render surface rastered ${rasterTime.toFixed(1)}ms of work`)
  console.log(JSON.stringify({
    viewport: `${viewport.width}x${viewport.height}`,
    appearance: appearance.custom ? 'custom-dimmed' : 'default',
    rasterTimeMs: Number(rasterTime.toFixed(2)),
    settlementGeometryReads: runtime.readsAtSettlement,
    totalGeometryReads: runtime.totalReads,
    maxVisualFrameGapMs: Number(runtime.maxFrameGap.toFixed(2)),
    visualLongTasks: runtime.longTasks.length,
  }))
  await context.close()
}

async function main() {
  const browser = await openSmokeBrowser()
  try {
    for (const viewport of [{ width: 390, height: 844 }, { width: 360, height: 800 }]) {
      await verifyDeepPage(browser, viewport, { brightness: 100, custom: false })
    }
    await verifyDeepPage(browser, { width: 390, height: 844 }, { brightness: 70, custom: true })
    console.log('reader deep-page contract smoke passed')
  } finally {
    await browser.close()
  }
}

main().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
