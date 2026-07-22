#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'
const readerPath = '/books/1/read?chapter=0'
const fixtureText = Array.from({ length: 360 }, (_, index) => (
  `第${index + 1}段。春风过处，纸页微明。用于验证阅读模式的正文宽度、首屏位置和翻页位移。`
)).join('\n')

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

function book() {
  return {
    id: 1,
    title: '阅读模式契约测试',
    author: 'OpenReader',
    sourceId: 2,
    sourceName: '测试书源',
    url: 'https://source.example/book/modes',
    bookUrl: 'https://source.example/book/modes',
    chapterCount: 3,
    categoryIds: [],
    progress: null,
  }
}

async function installApiMocks(page, mode, animateDuration = 0) {
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    if (path === '/me') return route.fulfill(response({ id: 1, username: 'smoke', role: 'admin' }))
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(response({
        key: 'reader',
        updatedAt: '2026-07-16T00:00:00Z',
        value: {
          mode,
          pageMode: 'auto',
          theme: 'parchment',
          themeType: 'day',
          fontSize: 18,
          fontWeight: 400,
          lineHeight: 1.8,
          paragraphSpace: 0.2,
          columnWidth: 800,
          animateDuration,
          clickMethod: 'auto',
        },
      }))
    }
    if (path === '/settings/reader' && method === 'PUT') return route.fulfill(response({ key: 'reader', value: {} }))
    if (path === '/books/1') return route.fulfill(response(book()))
    if (path === '/books') return route.fulfill(response([book()]))
    if (path === '/books/1/chapters') return route.fulfill(response([
      { id: 11, index: 0, title: '第一章' },
      { id: 12, index: 1, title: '第二章' },
      { id: 13, index: 2, title: '第三章' },
    ]))
    if (/^\/books\/1\/chapters\/\d+\/content$/.test(path)) {
      const index = Number(path.match(/chapters\/(\d+)\/content/)?.[1] || 0)
      return route.fulfill(response({
        chapter: { id: 11 + index, index, title: `第${index + 1}章` },
        content: fixtureText,
      }))
    }
    if (path === '/progress/1') return route.fulfill(response({}))
    if (path === '/sources') return route.fulfill(response([{ id: 2, name: '测试书源', enabled: true }]))
    if (path === '/categories') return route.fulfill(response([]))
    return route.fulfill(response({}))
  })
}

async function readerGeometry(page) {
  return page.evaluate(() => {
    const rect = element => {
      const value = element?.getBoundingClientRect()
      return value ? { left: value.left, top: value.top, width: value.width, height: value.height, right: value.right, bottom: value.bottom } : null
    }
    const readerPage = document.querySelector('.reader-page')
    const content = document.querySelector('.reader-content')
    const body = document.querySelector('.reader-body')
    const heading = document.querySelector('.reader-body h3')
    const paragraph = document.querySelector('.reader-body p')
    return {
      page: rect(readerPage),
      content: rect(content),
      body: rect(body),
      heading: rect(heading),
      paragraph: rect(paragraph),
      transform: body ? getComputedStyle(body).transform : '',
      textAlign: paragraph ? getComputedStyle(paragraph).textAlign : '',
      contentScrollTop: content?.scrollTop ?? 0,
    }
  })
}

async function openReader(browser, viewport, mode, animateDuration = 0) {
  const context = await browser.newContext({
    viewport,
    hasTouch: viewport.width <= 750,
    isMobile: viewport.width <= 750,
  })
  await context.addInitScript(value => localStorage.setItem('openreader_token', value), token())
  const page = await context.newPage()
  await installApiMocks(page, mode, animateDuration)
  await page.goto(`${targetUrl.replace(/\/$/, '')}${readerPath}`, { waitUntil: 'networkidle' })
  await page.waitForSelector('.reader-body p', { timeout: 15_000 })
  return { context, page }
}

async function ensureMobileChromeVisible(page, viewport) {
  if (await page.locator('.reader-mobile-top.visible').count()) return
  await page.touchscreen.tap(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
  await page.waitForTimeout(80)
  assert(await page.locator('.reader-mobile-top.visible').count() === 1, 'mobile reader tools did not become visible')
}

async function setRuntimeAnimationDuration(page, viewport, duration) {
  await ensureMobileChromeVisible(page, viewport)
  await page.locator('.reader-mobile-top.visible .mobile-tool-button').filter({ hasText: '设置' }).click()
  await page.waitForSelector('.reader-mobile-primary-settings .settings-body')

  const row = page.locator('.reader-mobile-primary-settings .setting-row').filter({ hasText: '动画时长' }).first()
  const valueButton = row.locator('.reader-setting-stepper-value')
  await valueButton.scrollIntoViewIfNeeded()
  await valueButton.click()
  const input = row.locator('.reader-setting-stepper-input')
  await input.fill(String(duration))
  await input.press('Enter')
  assert((await valueButton.textContent())?.trim() === String(duration), `runtime duration did not update to ${duration}`)

  await page.locator('.reader-mobile-top.visible .mobile-tool-button').filter({ hasText: '设置' }).click()
  await page.waitForFunction(() => !document.querySelector('.reader-mobile-primary-settings'))
}

async function resetRuntimePage(page) {
  await page.locator('.reader-content').evaluate(element => { element.scrollTop = 0 })
  await page.waitForTimeout(40)
}

async function seekRuntimePageSliderToEnd(page, viewport) {
  await ensureMobileChromeVisible(page, viewport)
  return page.locator('.mobile-progress-slider').evaluate(element => {
    element.value = element.max
    element.dispatchEvent(new Event('input', { bubbles: true }))
    element.dispatchEvent(new Event('change', { bubbles: true }))
    const content = document.querySelector('.reader-content')
    return Math.max(0, (content?.scrollHeight || 0) - (content?.clientHeight || 0))
  })
}

async function assertDesktopPage(browser) {
  const viewport = { width: 1440, height: 900 }
  const { context, page } = await openReader(browser, viewport, 'page')
  const geometry = await readerGeometry(page)
  close(geometry.page.left, 319, 1, 'desktop page left')
  close(geometry.page.width, 802, 1, 'desktop page outer width')
  close(geometry.body.left, 385, 1, 'desktop text left')
  close(geometry.body.width, 670, 1, 'desktop text width')
  close(geometry.heading.top, 72, 1, 'desktop heading top')
  close(geometry.paragraph.top, 134, 1, 'desktop paragraph top')
  assert(geometry.textAlign === 'left', `desktop paragraph alignment ${geometry.textAlign}`)
  await context.close()
}

async function assertMobilePage(browser, viewport) {
  const { context, page } = await openReader(browser, viewport, 'page')
  const geometry = await readerGeometry(page)
  close(geometry.content.left, 16, 1, `${viewport.width}: page content left`)
  close(viewport.width - geometry.content.right, 16, 1, `${viewport.width}: page content right`)
  close(geometry.heading.top, 73, 1, `${viewport.width}: page heading top`)
  close(geometry.paragraph.top, 135, 1, `${viewport.width}: page paragraph top`)
  await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height * 0.8))
  await page.waitForTimeout(60)
  const after = await readerGeometry(page)
  close(after.contentScrollTop, viewport.height - 72, 2, `${viewport.width}: page lower-click step`)
  await context.close()
}

async function assertMobileFlip(browser, viewport) {
  const { context, page } = await openReader(browser, viewport, 'flip')
  const geometry = await readerGeometry(page)
  close(geometry.content.left, 0, 1, `${viewport.width}: flip outer content left`)
  close(geometry.content.width, viewport.width, 1, `${viewport.width}: flip outer content width`)
  close(geometry.content.top, 30, 1, `${viewport.width}: flip content top`)
  close(geometry.content.height, viewport.height - 54, 1, `${viewport.width}: flip content height`)
  close(geometry.body.left, 16, 1, `${viewport.width}: flip text left`)
  close(viewport.width - geometry.body.right, 16, 1, `${viewport.width}: flip text right`)
  close(geometry.heading.top, 58, 1, `${viewport.width}: flip heading top`)
  close(geometry.paragraph.top, 120, 1, `${viewport.width}: flip paragraph top`)
  await page.mouse.click(Math.round(viewport.width * 0.82), Math.round(viewport.height * 0.8))
  await page.waitForTimeout(60)
  const after = await readerGeometry(page)
  assert(after.transform.includes(`-${viewport.width - 16}`), `${viewport.width}: flip stride ${after.transform}`)
  await context.close()
}

async function assertConfiguredPageDuration(browser) {
  const viewport = { width: 390, height: 844 }
  const targetTop = viewport.height - 72

  const zero = await openReader(browser, viewport, 'page', 0)
  await zero.page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height * 0.8))
  close((await readerGeometry(zero.page)).contentScrollTop, targetTop, 2, '0ms page animation')
  await zero.context.close()

  const short = await openReader(browser, viewport, 'page', 100)
  await short.page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height * 0.8))
  await short.page.waitForTimeout(180)
  close((await readerGeometry(short.page)).contentScrollTop, targetTop, 2, '100ms page animation')
  await short.context.close()

  const long = await openReader(browser, viewport, 'page', 500)
  await long.page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height * 0.8))
  await long.page.waitForTimeout(180)
  const middle = await long.page.evaluate(() => {
    const content = document.querySelector('.reader-content')
    const body = document.querySelector('.reader-body')
    return {
      animationCount: body?.getAnimations?.().length || 0,
      scrollTop: content?.scrollTop || 0,
      transform: body ? getComputedStyle(body).transform : 'none',
    }
  })
  assert(middle.scrollTop > targetTop * 0.12 && middle.scrollTop < targetTop * 0.82, `500ms frame scroll must still be moving at 180ms: ${JSON.stringify(middle)}`)
  assert(middle.animationCount === 0 && middle.transform === 'none', `500ms frame scroll must not promote the full chapter: ${JSON.stringify(middle)}`)
  await long.page.waitForTimeout(420)
  close((await readerGeometry(long.page)).contentScrollTop, targetTop, 2, '500ms page animation')

  await long.page.locator('.reader-content').evaluate(element => { element.scrollTop = 0 })
  await long.page.locator('.reader-content').hover()
  await long.page.mouse.wheel(0, 137)
  await long.page.waitForTimeout(80)
  const wheelTop = (await readerGeometry(long.page)).contentScrollTop
  assert(wheelTop > 0 && wheelTop < 400, `wheel must remain native and continuous: ${wheelTop}`)
  await long.context.close()
}

async function assertRuntimeConfiguredPageDuration(browser) {
  const viewport = { width: 390, height: 844 }
  const targetTop = viewport.height - 72
  const { context, page } = await openReader(browser, viewport, 'page', 300)

  await setRuntimeAnimationDuration(page, viewport, 0)
  await resetRuntimePage(page)
  await page.touchscreen.tap(Math.round(viewport.width / 2), Math.round(viewport.height * 0.8))
  close((await readerGeometry(page)).contentScrollTop, targetTop, 2, 'runtime 0ms page animation')

  await setRuntimeAnimationDuration(page, viewport, 100)
  await resetRuntimePage(page)
  await page.touchscreen.tap(Math.round(viewport.width / 2), Math.round(viewport.height * 0.8))
  await page.waitForTimeout(180)
  close((await readerGeometry(page)).contentScrollTop, targetTop, 2, 'runtime 100ms page animation')

  await setRuntimeAnimationDuration(page, viewport, 500)
  await resetRuntimePage(page)
  await page.touchscreen.tap(Math.round(viewport.width / 2), Math.round(viewport.height * 0.8))
  await page.waitForTimeout(180)
  const middle = await page.evaluate(() => {
    const content = document.querySelector('.reader-content')
    const body = document.querySelector('.reader-body')
    return {
      animationCount: body?.getAnimations?.().length || 0,
      scrollTop: content?.scrollTop || 0,
      transform: body ? getComputedStyle(body).transform : 'none',
    }
  })
  assert(middle.scrollTop > targetTop * 0.12 && middle.scrollTop < targetTop * 0.82, `runtime 500ms frame scroll must still be moving at 180ms: ${JSON.stringify(middle)}`)
  assert(middle.animationCount === 0 && middle.transform === 'none', `runtime 500ms frame scroll must not promote the full chapter: ${JSON.stringify(middle)}`)
  await page.waitForTimeout(420)
  close((await readerGeometry(page)).contentScrollTop, targetTop, 2, 'runtime 500ms page animation')

  await setRuntimeAnimationDuration(page, viewport, 100)
  await resetRuntimePage(page)
  const shortSeekBottom = await seekRuntimePageSliderToEnd(page, viewport)
  await page.waitForTimeout(180)
  close((await readerGeometry(page)).contentScrollTop, shortSeekBottom, 2, 'runtime 100ms page-slider animation')

  await setRuntimeAnimationDuration(page, viewport, 500)
  await resetRuntimePage(page)
  const longSeekBottom = await seekRuntimePageSliderToEnd(page, viewport)
  await page.waitForTimeout(180)
  const seekMiddle = (await readerGeometry(page)).contentScrollTop
  assert(seekMiddle > 0 && seekMiddle < longSeekBottom - 80, `runtime 500ms page-slider animation must still be moving at 180ms: ${seekMiddle}/${longSeekBottom}`)
  await page.waitForTimeout(420)
  close((await readerGeometry(page)).contentScrollTop, longSeekBottom, 2, 'runtime 500ms page-slider animation')

  await context.close()
}

async function assertMobileVerticalAnimationCadence(browser, viewport, mode) {
  const { context, page } = await openReader(browser, viewport, mode, 300)
  const targetTop = viewport.height - 72
  await page.evaluate(() => {
    const content = document.querySelector('.reader-content')
    const body = document.querySelector('.reader-body')
    content.scrollTop = 0
    const samples = []
    let touchEndAt = null
    const originalGetSelection = window.getSelection.bind(window)
    window.__openReaderSelectionChecks = 0
    window.getSelection = (...args) => {
      window.__openReaderSelectionChecks += 1
      return originalGetSelection(...args)
    }
    window.__openReaderLongTasks = []
    window.__openReaderLayoutShifts = []
    window.__openReaderInputTimes = { touchStartAt: null, touchEndAt: null }
    if (globalThis.PerformanceObserver?.supportedEntryTypes?.includes('longtask')) {
      window.__openReaderLongTaskObserver = new PerformanceObserver(list => {
        window.__openReaderLongTasks.push(...list.getEntries().map(entry => ({
          duration: entry.duration,
          startTime: entry.startTime,
        })))
      })
      window.__openReaderLongTaskObserver.observe({ type: 'longtask', buffered: true })
    }
    if (globalThis.PerformanceObserver?.supportedEntryTypes?.includes('layout-shift')) {
      window.__openReaderLayoutShiftObserver = new PerformanceObserver(list => {
        window.__openReaderLayoutShifts.push(...list.getEntries().map(entry => ({
          value: entry.value,
          startTime: entry.startTime,
          hadRecentInput: entry.hadRecentInput,
        })))
      })
      window.__openReaderLayoutShiftObserver.observe({ type: 'layout-shift', buffered: true })
    }
    const measuredParagraph = body.querySelector('[data-reader-block]')
    window.__openReaderParagraphGeometry = measuredParagraph ? {
      height: measuredParagraph.getBoundingClientRect().height,
      width: measuredParagraph.getBoundingClientRect().width,
    } : null
    document.querySelector('.reader-page').addEventListener('touchstart', () => {
      window.__openReaderInputTimes.touchStartAt = performance.now()
    }, { once: true })
    document.querySelector('.reader-page').addEventListener('touchend', () => {
      touchEndAt = performance.now()
      window.__openReaderInputTimes.touchEndAt = touchEndAt
    }, { once: true })
    window.__openReaderMotionSamples = samples
    const startedAt = performance.now()
    const sample = () => {
      const now = performance.now()
      samples.push({
        at: now - startedAt,
        afterInput: touchEndAt === null ? null : now - touchEndAt,
        animationCount: body.getAnimations().length,
        scrollTop: content.scrollTop,
        transform: getComputedStyle(body).transform,
        willChange: body.style.willChange,
      })
      if (now - startedAt < 430) requestAnimationFrame(sample)
    }
    requestAnimationFrame(sample)
  })
  const tapX = Math.round(viewport.width / 2)
  const tapY = Math.round(viewport.height * 0.8)
  const cdp = await context.newCDPSession(page)
  await cdp.send('Input.dispatchTouchEvent', {
    type: 'touchStart',
    touchPoints: [{ x: tapX, y: tapY, radiusX: 1, radiusY: 1, force: 1, id: 0 }],
  })
  await page.waitForTimeout(48)
  const preparedLayer = await page.locator('.reader-body').evaluate(element => ({
    animationCount: element.getAnimations().length,
    height: element.getBoundingClientRect().height,
    transform: getComputedStyle(element).transform,
    willChange: element.style.willChange,
  }))
  assert(preparedLayer.height > viewport.height * 10, `${viewport.width}: long chapter fixture is too short (${preparedLayer.height})`)
  assert(preparedLayer.animationCount === 0 && preparedLayer.transform === 'none' && preparedLayer.willChange === '', `${viewport.width}: touchstart promoted the full chapter layer ${JSON.stringify(preparedLayer)}`)
  await cdp.send('Input.dispatchTouchEvent', {
    type: 'touchEnd',
    touchPoints: [],
  })
  await page.waitForTimeout(480)
  const samples = await page.evaluate(() => window.__openReaderMotionSamples || [])
  const moving = samples.filter(sample => (
    sample.scrollTop > targetTop * 0.03 && sample.scrollTop < targetTop * 0.97
  ))
  assert(moving.length >= 8, `${viewport.width}/${mode}: page animation exposed too few moving frames (${moving.length})`)
  assert(moving.every(sample => sample.animationCount === 0 && sample.transform === 'none' && sample.willChange === ''), `${viewport.width}/${mode}: page motion promoted the full chapter ${JSON.stringify(moving.find(sample => sample.animationCount || sample.transform !== 'none' || sample.willChange))}`)
  for (let index = 1; index < moving.length; index += 1) {
    assert(
      moving[index].scrollTop >= moving[index - 1].scrollTop,
      `${viewport.width}/${mode}: page animation moved backwards at sample ${index}`,
    )
  }
  const distinctPositions = new Set(moving.map(sample => Math.round(sample.scrollTop * 10))).size
  assert(
    distinctPositions >= Math.min(8, moving.length),
    `${viewport.width}/${mode}: page animation stalled inside its visible motion (${distinctPositions}/${moving.length})`,
  )
  const firstVisibleFrame = samples.find(sample => sample.afterInput !== null && sample.afterInput >= 0)
  assert(
    firstVisibleFrame?.scrollTop > 0,
    `${viewport.width}/${mode}: first observable frame remained at the origin ${JSON.stringify(firstVisibleFrame)}`,
  )
  const earlySamples = samples.filter(sample => (
    sample.afterInput !== null
    && sample.afterInput >= 0
    && sample.afterInput <= 40
  ))
  const earlyVisibleOffset = Math.max(0, ...earlySamples.map(sample => sample.scrollTop))
  assert(
    earlyVisibleOffset >= targetTop * 0.01,
    `${viewport.width}/${mode}: first 40ms remained in a perceptual dead zone (${earlyVisibleOffset}/${targetTop})`,
  )
  const firstCadenceSamples = samples.filter(sample => (
    sample.afterInput !== null
    && sample.afterInput >= 8
    && sample.afterInput <= 24
  ))
  const firstCadenceOffset = Math.max(0, ...firstCadenceSamples.map(sample => sample.scrollTop))
  assert(
    !firstCadenceSamples.length || firstCadenceOffset <= targetTop * 0.04,
    `${viewport.width}/${mode}: first refresh interval jumped too much text (${firstCadenceOffset}/${targetTop})`,
  )
  const movingGaps = moving.slice(1).map((sample, index) => sample.at - moving[index].at)
  assert(Math.max(...movingGaps) <= 50, `${viewport.width}/${mode}: page motion has a visible frame stall: ${Math.max(...movingGaps)}ms`)
  const runtimeWork = await page.evaluate(() => ({
    inputTimes: window.__openReaderInputTimes || {},
    layoutShifts: window.__openReaderLayoutShifts || [],
    longTasks: window.__openReaderLongTasks || [],
    paragraphGeometry: (() => {
      const paragraph = document.querySelector('.reader-body [data-reader-block]')
      const initial = window.__openReaderParagraphGeometry
      const rect = paragraph?.getBoundingClientRect()
      return {
        initial,
        current: rect ? { height: rect.height, width: rect.width } : null,
      }
    })(),
    selectionChecks: window.__openReaderSelectionChecks || 0,
    willChange: document.querySelector('.reader-body')?.style.willChange || '',
  }))
  assert(runtimeWork.selectionChecks <= 1, `${viewport.width}/${mode}: ordinary page tap polled text selection ${runtimeWork.selectionChecks} times`)
  const inputLongTasks = runtimeWork.longTasks.filter(task => (
    task.startTime >= Number(runtimeWork.inputTimes.touchStartAt || Infinity) - 1
    && task.startTime <= Number(runtimeWork.inputTimes.touchEndAt || 0) + 300
  ))
  assert(inputLongTasks.every(task => task.duration < 50), `${viewport.width}/${mode}: page tap exposed a long task ${JSON.stringify({ inputLongTasks, ...runtimeWork.inputTimes })}`)
  const inputLayoutShifts = runtimeWork.layoutShifts.filter(shift => (
    shift.startTime >= Number(runtimeWork.inputTimes.touchStartAt || Infinity) - 1
    && shift.startTime <= Number(runtimeWork.inputTimes.touchEndAt || 0) + 360
  ))
  assert(inputLayoutShifts.every(shift => shift.value === 0), `${viewport.width}/${mode}: page tap changed layout ${JSON.stringify(inputLayoutShifts)}`)
  assert(
    Math.abs(Number(runtimeWork.paragraphGeometry.current?.width || 0) - Number(runtimeWork.paragraphGeometry.initial?.width || 0)) <= 0.1
      && Math.abs(Number(runtimeWork.paragraphGeometry.current?.height || 0) - Number(runtimeWork.paragraphGeometry.initial?.height || 0)) <= 0.1,
    `${viewport.width}/${mode}: text geometry changed during paging ${JSON.stringify(runtimeWork.paragraphGeometry)}`,
  )
  assert(runtimeWork.willChange === '', `${viewport.width}/${mode}: settled animation leaked will-change (${runtimeWork.willChange})`)
  close((await readerGeometry(page)).contentScrollTop, targetTop, 2, `${viewport.width}/${mode}: sampled page animation`)

  await page.evaluate(() => {
    const content = document.querySelector('.reader-content')
    const startedAt = performance.now()
    let touchEndAt = null
    window.__openReaderReverseMotionSamples = []
    document.querySelector('.reader-page').addEventListener('touchend', () => {
      touchEndAt = performance.now()
    }, { once: true })
    const sample = () => {
      const now = performance.now()
      window.__openReaderReverseMotionSamples.push({
        at: now - startedAt,
        afterInput: touchEndAt === null ? null : now - touchEndAt,
        scrollTop: content?.scrollTop || 0,
      })
      if (now - startedAt < 430) requestAnimationFrame(sample)
    }
    requestAnimationFrame(sample)
  })
  const reverseTapX = Math.round(viewport.width / 2)
  const reverseTapY = Math.round(viewport.height * 0.2)
  await cdp.send('Input.dispatchTouchEvent', {
    type: 'touchStart',
    touchPoints: [{ x: reverseTapX, y: reverseTapY, radiusX: 1, radiusY: 1, force: 1, id: 0 }],
  })
  await page.waitForTimeout(48)
  await cdp.send('Input.dispatchTouchEvent', {
    type: 'touchEnd',
    touchPoints: [],
  })
  await page.waitForTimeout(480)
  const reverseSamples = await page.evaluate(() => window.__openReaderReverseMotionSamples || [])
  const reverseMoving = reverseSamples.filter(sample => (
    sample.scrollTop > targetTop * 0.03 && sample.scrollTop < targetTop * 0.97
  ))
  assert(reverseMoving.length >= 8, `${viewport.width}/${mode}: previous-page animation exposed too few moving frames (${reverseMoving.length})`)
  for (let index = 1; index < reverseMoving.length; index += 1) {
    assert(
      reverseMoving[index].scrollTop <= reverseMoving[index - 1].scrollTop,
      `${viewport.width}/${mode}: previous-page animation moved forward at sample ${index}`,
    )
  }
  const reverseGaps = reverseMoving.slice(1).map((sample, index) => sample.at - reverseMoving[index].at)
  assert(Math.max(...reverseGaps) <= 50, `${viewport.width}/${mode}: previous-page motion has a visible frame stall: ${Math.max(...reverseGaps)}ms`)
  const reverseFirstInterval = reverseSamples.filter(sample => (
    sample.afterInput !== null
    && sample.afterInput >= 8
    && sample.afterInput <= 24
  ))
  const reverseFirstOffset = Math.max(0, ...reverseFirstInterval.map(sample => targetTop - sample.scrollTop))
  assert(
    !reverseFirstInterval.length || reverseFirstOffset <= targetTop * 0.04,
    `${viewport.width}/${mode}: previous-page first refresh interval jumped too much text (${reverseFirstOffset}/${targetTop})`,
  )
  close((await readerGeometry(page)).contentScrollTop, 0, 2, `${viewport.width}/${mode}: sampled previous-page animation`)

  await resetRuntimePage(page)
  await page.evaluate(() => {
    const content = document.querySelector('.reader-content')
    const startedAt = performance.now()
    window.__openReaderBufferedMotionSamples = []
    const sample = () => {
      const now = performance.now()
      window.__openReaderBufferedMotionSamples.push({
        at: now - startedAt,
        scrollTop: content?.scrollTop || 0,
      })
      if (now - startedAt < 760) requestAnimationFrame(sample)
    }
    requestAnimationFrame(sample)
  })
  await page.touchscreen.tap(Math.round(viewport.width / 2), Math.round(viewport.height * 0.8))
  await page.waitForTimeout(60)
  await page.touchscreen.tap(Math.round(viewport.width / 2), Math.round(viewport.height * 0.8))
  await page.waitForTimeout(720)
  const bufferedSamples = await page.evaluate(() => window.__openReaderBufferedMotionSamples || [])
  const firstBoundaryIndex = bufferedSamples.findIndex(sample => sample.scrollTop >= targetTop - 1)
  assert(firstBoundaryIndex >= 0, `${viewport.width}/${mode}: buffered chain never reached the first page boundary`)
  const firstBoundaryFrames = bufferedSamples.slice(firstBoundaryIndex, firstBoundaryIndex + 3)
  assert(
    firstBoundaryFrames.some(sample => sample.scrollTop >= targetTop * 1.01),
    `${viewport.width}/${mode}: buffered chain painted a stationary first-page endpoint ${JSON.stringify(firstBoundaryFrames)}`,
  )
  for (let index = firstBoundaryIndex + 1; index < bufferedSamples.length; index += 1) {
    assert(
      bufferedSamples[index].scrollTop >= bufferedSamples[index - 1].scrollTop,
      `${viewport.width}/${mode}: buffered chain moved backwards at sample ${index}`,
    )
  }
  close((await readerGeometry(page)).contentScrollTop, targetTop * 2, 3, `${viewport.width}/${mode}: buffered repeated page tap`)
  await context.close()
}

async function assertMobileChapterEndPrompt(browser, viewport) {
  const { context, page } = await openReader(browser, viewport, 'page', 0)
  await page.touchscreen.tap(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
  await page.waitForFunction(() => !document.querySelector('.reader-mobile-top.visible'))

  const prompt = page.locator('.reader-chapter-end')
  await prompt.waitFor({ state: 'attached' })
  await page.locator('.reader-content').evaluate(element => {
    element.scrollTop = element.scrollHeight
  })
  await prompt.tap()
  await page.waitForURL(/chapter=1(?:&|$)/)
  await page.waitForFunction(() => document.querySelector('.reader-body h3')?.textContent?.includes('第2章'))

  await page.locator('.reader-content').evaluate(element => {
    element.scrollTop = element.scrollHeight
  })
  await prompt.tap()
  await page.waitForURL(/chapter=2(?:&|$)/)
  await page.waitForFunction(() => document.querySelector('.reader-body h3')?.textContent?.includes('第3章'))

  await page.locator('.reader-content').evaluate(element => {
    element.scrollTop = element.scrollHeight
  })
  await prompt.tap()
  await page.waitForFunction(() => document.querySelector('.reader-toast')?.textContent?.includes('本章是最后一章'))
  assert(new URL(page.url()).searchParams.get('chapter') === '2', `${viewport.width}: final prompt navigated out of range`)
  await context.close()
}

async function main() {
  const browser = await openSmokeBrowser()
  try {
    await assertDesktopPage(browser)
    for (const viewport of [{ width: 390, height: 844 }, { width: 360, height: 800 }]) {
      await assertMobilePage(browser, viewport)
      await assertMobileFlip(browser, viewport)
      for (const mode of ['page', 'scroll', 'scroll2']) {
        await assertMobileVerticalAnimationCadence(browser, viewport, mode)
      }
      await assertMobileChapterEndPrompt(browser, viewport)
    }
    await assertConfiguredPageDuration(browser)
    await assertRuntimeConfiguredPageDuration(browser)
    console.log('reader text-mode contract smoke passed')
  } finally {
    await browser.close()
  }
}

main().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
