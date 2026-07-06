#!/usr/bin/env node

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'
const readerUrl = process.env.SMOKE_READER_URL || `${targetUrl.replace(/\/$/, '')}/books/1/read?chapter=0`
const defaultChromePath = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'

async function loadPlaywright() {
  try {
    const module = await import('playwright')
    return module.chromium ? module : module.default
  } catch (error) {
    const bundled = '/Users/yuchangsheng/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright/index.js'
    try {
      const module = await import(bundled)
      return module.chromium ? module : module.default
    } catch {
      console.error('Playwright is required for reader image contract smoke.')
      console.error(`Original import error: ${error.message}`)
      process.exit(2)
    }
  }
}

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function json(data, status = 200) {
  return {
    status,
    contentType: 'application/json',
    body: JSON.stringify(data),
  }
}

function fakeToken() {
  const payload = Buffer.from(JSON.stringify({ userId: 1, sub: '1' })).toString('base64url')
  return `open.${payload}.reader`
}

function tallSVG() {
  return `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="2500" viewBox="0 0 1200 2500">
    <rect width="1200" height="2500" fill="#365f5f"/>
    <text x="80" y="180" font-size="96" fill="#fff">OpenReader Comic Fixture</text>
  </svg>`
}

async function installMocks(page, apiRequests, mode) {
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/comic\/tall\.svg.*$/, async (route) => {
    await new Promise(resolve => setTimeout(resolve, 600))
    return route.fulfill({
      status: 200,
      contentType: 'image/svg+xml',
      body: tallSVG(),
    })
  })
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    apiRequests.push(`${method} ${path}`)
    if (path === '/me') {
      return route.fulfill(json({ id: 1, username: 'smoke', role: 'admin' }))
    }
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(json({
        key: 'reader',
        updatedAt: '2026-07-06T00:00:00Z',
        value: {
          mode,
          pageMode: 'normal',
          fontSize: 18,
          lineHeight: 1.8,
          paragraphSpace: 0.2,
          columnWidth: 800,
          animateDuration: 0,
        },
      }))
    }
    if (path === '/settings/reader' && method === 'PUT') {
      return route.fulfill(json({ key: 'reader', updatedAt: '2026-07-06T00:00:01Z', value: {} }))
    }
    if (path === '/books/1') {
      return route.fulfill(json({
        id: 1,
        title: 'CBZ 图片契约测试',
        author: 'OpenReader',
        sourceId: 0,
        url: '/library/demo.CBZ?cache=1#page',
        originalFile: 'demo.CBZ',
        libraryPath: 'imports/demo.CBZ',
        chapterCount: 1,
        progress: null,
      }))
    }
    if (path === '/books/1/chapters') {
      return route.fulfill(json([{ id: 11, index: 0, title: '图片章' }]))
    }
    if (path === '/books/1/chapters/0/content') {
      return route.fulfill(json({
        chapter: { id: 11, index: 0, title: '图片章' },
        content: [
          '<img data-src="__API_ROOT__/comic/tall.svg" alt="漫画页">',
          '页尾文字一用于确认图片加载后的分页位置。',
          '页尾文字二用于确认图片加载后的分页位置。',
          '页尾文字三用于确认图片加载后的分页位置。',
          '页尾文字四用于确认图片加载后的分页位置。',
          '页尾文字五用于确认图片加载后的分页位置。',
        ].join('\n'),
        format: 'text',
      }))
    }
    if (path === '/books/1/bookmarks') {
      return route.fulfill(json([]))
    }
    if (path === '/progress/1') {
      return route.fulfill(json({}))
    }
    if (path === '/progress' && method === 'PUT') {
      return route.fulfill(json({ bookId: 1, chapterId: 11, chapterIndex: 0, offset: 0, percent: 0, chapterPercent: 0 }))
    }
    if (path === '/sources') {
      return route.fulfill(json([]))
    }
    if (path === '/categories') {
      return route.fulfill(json([]))
    }
    return route.fulfill(json({}))
  })
}

async function runViewport(browser, viewport, requestedMode) {
  const context = await browser.newContext({ viewport })
  const mode = requestedMode || (viewport.width > 750 ? 'page' : 'scroll')
  await context.addInitScript(({ token, mode: initialMode }) => {
    window.localStorage.setItem('openreader_token', token)
    window.localStorage.setItem('reader', JSON.stringify({
      mode: initialMode,
      pageMode: 'auto',
      settingsScope: 'user:1',
      progressScope: 'user:1',
    }))
  }, { token: fakeToken(), mode })
  const page = await context.newPage()
  const failures = []
  const apiRequests = []
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    const text = message.text()
    if (text.includes('/ws/sync') && text.includes('WebSocket connection')) return
    failures.push(text)
  })
  page.on('pageerror', error => failures.push(error.message))
  await installMocks(page, apiRequests, mode)
  await page.goto(readerUrl, { waitUntil: 'networkidle' })
  await page.waitForSelector('.reader-content-image img', { timeout: 10_000 })
  await page.waitForFunction(() => {
    const image = document.querySelector('.reader-content-image img')
    return image?.complete && image.naturalWidth > 0
  }, null, { timeout: 10_000 })

  const state = await page.evaluate(() => {
    const pageEl = document.querySelector('.reader-page')
    const body = document.querySelector('.reader-body')
    const content = document.querySelector('.reader-content')
    const chapter = document.querySelector('.chapter-content')
    const imageBox = document.querySelector('.reader-content-image')
    const image = document.querySelector('.reader-content-image img')
    const titleCount = document.querySelectorAll('.reader-body h1').length
    const imageRect = image.getBoundingClientRect()
    const boxRect = imageBox.getBoundingClientRect()
    const contentRect = content.getBoundingClientRect()
    const chapterRect = chapter.getBoundingClientRect()
    return {
      titleCount,
      pagePaddingLeft: getComputedStyle(pageEl).paddingLeft,
      pagePaddingRight: getComputedStyle(pageEl).paddingRight,
      imageWidth: Math.round(imageRect.width),
      boxWidth: Math.round(boxRect.width),
      contentWidth: Math.round(contentRect.width),
      chapterWidth: Math.round(chapterRect.width),
      transform: getComputedStyle(body).transform,
      bodyColumnWidth: getComputedStyle(body).columnWidth,
      bodyColumnCount: getComputedStyle(body).columnCount,
      shellClass: document.querySelector('.reader-shell')?.className || '',
      bodyScrollWidth: body.scrollWidth,
      bodyClientWidth: body.clientWidth,
      bodyScrollHeight: body.scrollHeight,
      chapterHeight: Math.round(chapterRect.height),
      scrollHeight: content.scrollHeight,
      clientHeight: content.clientHeight,
      topVisible: Boolean(document.querySelector('.reader-mobile-top.visible')),
      persistedReader: window.localStorage.getItem('reader'),
    }
  })
  assert(state.titleCount === 0, `${viewport.width}: CBZ chapter title should be hidden`)
  assert(Math.abs(state.imageWidth - state.boxWidth) <= 1, `${viewport.width}: comic image width ${state.imageWidth}, box ${state.boxWidth}`)
  const readableWidth = mode === 'flip' ? state.bodyClientWidth : state.chapterWidth
  assert(Math.abs(state.boxWidth - readableWidth) <= 1, `${viewport.width}: comic box width ${state.boxWidth}, readable column ${readableWidth}`)

  if (viewport.width <= 750) {
    assert(state.topVisible, `${viewport.width}: mobile toolbar should be visible by default`)
    assert(state.pagePaddingLeft === '16px', `${viewport.width}: left padding ${state.pagePaddingLeft}`)
    assert(state.pagePaddingRight === '16px', `${viewport.width}: right padding ${state.pagePaddingRight}`)
    await page.locator('.reader-content-image img').click()
    await page.waitForSelector('.el-image-viewer__wrapper', { timeout: 5000 })
    assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: image preview should not hide toolbar`)
    await page.locator('.el-image-viewer__close').click()
    await page.waitForSelector('.el-image-viewer__wrapper', { state: 'detached' })
  }

  if (mode === 'flip') {
    assert(state.shellClass.includes('flip'), `${viewport.width}: expected flip mode, class=${state.shellClass}`)
    assert(state.bodyScrollWidth > state.bodyClientWidth, `${viewport.width}: delayed image should create another flip column`)
    await page.keyboard.press('ArrowRight')
    await page.waitForTimeout(180)
    const moved = await page.evaluate(() => getComputedStyle(document.querySelector('.reader-body')).transform)
    assert(
      moved !== 'none' && !moved.endsWith(', 0, 0)'),
      `${viewport.width}: image load did not enable flip pagination, transform=${moved}, state=${JSON.stringify(state)}, requests=${JSON.stringify(apiRequests)}`,
    )
  } else if (viewport.width > 750) {
    assert(state.shellClass.includes('page'), `desktop: expected page mode, class=${state.shellClass}`)
    assert(state.scrollHeight > state.clientHeight, 'desktop: delayed image should extend the paged viewport')
    await page.keyboard.press('PageDown')
    await page.waitForTimeout(180)
    const scrollTop = await page.evaluate(() => document.querySelector('.reader-content').scrollTop)
    assert(scrollTop > 0, `desktop: paged image content did not advance, scrollTop=${scrollTop}`)
  } else {
    assert(state.shellClass.includes('scroll'), `${viewport.width}: expected scroll mode, class=${state.shellClass}`)
    assert(state.scrollHeight > state.clientHeight, `${viewport.width}: delayed image should extend continuous scroll content`)
  }

  assert(failures.length === 0, failures.join('\n'))
  await context.close()
}

async function main() {
  const { chromium } = await loadPlaywright()
  const browser = await chromium.launch({
    headless: true,
    executablePath: process.env.CHROME_PATH || defaultChromePath,
  })
  try {
    for (const viewport of [
      { width: 1440, height: 900 },
      { width: 390, height: 844 },
      { width: 360, height: 800 },
    ]) {
      await runViewport(browser, viewport)
    }
    await runViewport(browser, { width: 390, height: 844 }, 'flip')
    console.log('reader image contract smoke passed')
  } finally {
    await browser.close()
  }
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})
