#!/usr/bin/env node

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'
const readerUrl = process.env.SMOKE_READER_URL || `${targetUrl.replace(/\/$/, '')}/books/1/read?chapter=0&offset=37`
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
      console.error('Playwright is required for reader audio contract smoke.')
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

async function installMocks(page, requests) {
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/media\/audio\.mp3.*$/, route => route.fulfill({
    status: 200,
    contentType: 'audio/mpeg',
    body: Buffer.from([0x49, 0x44, 0x33, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]),
  }))
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    requests.push(`${method} ${path}`)
    if (path === '/me') return route.fulfill(json({ id: 1, username: 'smoke', role: 'admin' }))
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(json({
        key: 'reader',
        updatedAt: '2026-07-07T00:00:00Z',
        value: {
          mode: 'scroll2',
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
      return route.fulfill(json({ key: 'reader', updatedAt: '2026-07-07T00:00:01Z', value: {} }))
    }
    if (path === '/books/1') {
      return route.fulfill(json({
        id: 1,
        title: '音频契约测试',
        author: 'OpenReader',
        sourceId: 2,
        type: 1,
        chapterCount: 2,
        progress: null,
      }))
    }
    if (path === '/books/1/chapters') {
      return route.fulfill(json([
        { id: 11, index: 0, title: '第一集' },
        { id: 12, index: 1, title: '第二集' },
      ]))
    }
    if (path === '/books/1/chapters/0/content') {
      return route.fulfill(json({
        chapter: { id: 11, index: 0, title: '第一集' },
        content: `${targetUrl.replace(/\/$/, '')}/media/audio.mp3`,
        format: 'audio',
        resourceUrl: `${targetUrl.replace(/\/$/, '')}/media/audio.mp3`,
        resourceExpiresAt: '2026-07-07T12:00:00Z',
      }))
    }
    if (path === '/books/1/chapters/1/content') {
      return route.fulfill(json({
        chapter: { id: 12, index: 1, title: '第二集' },
        content: `${targetUrl.replace(/\/$/, '')}/media/audio.mp3`,
        format: 'audio',
        resourceUrl: `${targetUrl.replace(/\/$/, '')}/media/audio.mp3`,
        resourceExpiresAt: '2026-07-07T12:00:00Z',
      }))
    }
    if (path === '/books/1/bookmarks') return route.fulfill(json([]))
    if (path === '/progress/1') return route.fulfill(json({}))
    if (path === '/progress' && method === 'PUT') {
      const body = request.postDataJSON?.() || {}
      return route.fulfill(json({ ...body, bookId: 1 }))
    }
    if (path === '/sources') return route.fulfill(json([{ id: 2, name: '音频书源', enabled: true }]))
    if (path === '/categories') return route.fulfill(json([]))
    return route.fulfill(json({}))
  })
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({ viewport })
  await context.addInitScript((token) => {
    window.localStorage.setItem('openreader_token', token)
    window.localStorage.setItem('reader', JSON.stringify({
      mode: 'scroll2',
      pageMode: 'normal',
      settingsScope: 'user:1',
      progressScope: 'user:1',
    }))
  }, fakeToken())
  const page = await context.newPage()
  const failures = []
  const requests = []
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    const text = message.text()
    if (text.includes('/ws/sync') && text.includes('WebSocket connection')) return
    failures.push(text)
  })
  page.on('pageerror', error => failures.push(error.message))
  await installMocks(page, requests)
  await page.goto(readerUrl, { waitUntil: 'networkidle' })
  await page.waitForSelector('.reader-audio-content audio', { timeout: 10_000 })

  const initial = await page.evaluate(() => ({
    shellClass: document.querySelector('.reader-shell')?.className || '',
    audioTitle: document.querySelector('.reader-audio-content h1')?.textContent || '',
    hasTextBlocks: Boolean(document.querySelector('.reader-body [data-reader-block]')),
    hasChapterContent: Boolean(document.querySelector('.chapter-content')),
    hasAutoReading: Boolean(document.querySelector('[title="自动阅读"]')),
    hasTTS: Boolean(document.querySelector('[title="朗读"]')),
    mobileTopVisible: Boolean(document.querySelector('.reader-mobile-top.visible')),
  }))
  assert(initial.shellClass.includes('page'), `${viewport.width}: audio reader should force page mode, class=${initial.shellClass}`)
  assert(initial.audioTitle.includes('第一集'), `${viewport.width}: audio title missing: ${initial.audioTitle}`)
  assert(!initial.hasTextBlocks, `${viewport.width}: audio should not render text reader blocks`)
  assert(!initial.hasChapterContent, `${viewport.width}: audio should not render ordinary chapter-content sections`)
  assert(!initial.hasAutoReading, `${viewport.width}: audio should hide auto-reading control`)
  assert(!initial.hasTTS, `${viewport.width}: audio should hide TTS control`)

  const chapterOneRequestsBeforeKey = requests.filter(item => item === 'GET /books/1/chapters/1/content').length
  await page.keyboard.press('ArrowRight')
  await page.waitForTimeout(160)
  assert(
    requests.filter(item => item === 'GET /books/1/chapters/1/content').length === chapterOneRequestsBeforeKey,
    `${viewport.width}: ArrowRight must not page or jump audio chapters`,
  )

  if (viewport.width <= 750) {
    assert(initial.mobileTopVisible, `${viewport.width}: mobile toolbar should be visible by default`)
    await page.mouse.click(viewport.width - 20, Math.round(viewport.height / 2))
    await page.waitForTimeout(160)
    assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: side tap should not hide toolbar for audio`)
    const contentTapY = Math.round(viewport.height / 2) - 110
    await page.mouse.click(Math.round(viewport.width / 2), contentTapY)
    await page.waitForTimeout(160)
    assert(await page.locator('.reader-mobile-top.visible').count() === 0, `${viewport.width}: center tap should hide toolbar for audio`)
    await page.mouse.click(Math.round(viewport.width / 2), contentTapY)
    await page.waitForTimeout(160)
    assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: second center tap should show toolbar for audio`)
  }

  assert(failures.length === 0, `${viewport.width}: browser errors:\n${failures.join('\n')}`)
  await context.close()
}

async function main() {
  const playwright = await loadPlaywright()
  const browser = await playwright.chromium.launch({
    headless: true,
    executablePath: process.env.CHROME_PATH || defaultChromePath,
  })
  try {
    await runViewport(browser, { width: 390, height: 844 })
    await runViewport(browser, { width: 1440, height: 900 })
    console.log('reader audio contract smoke passed')
  } finally {
    await browser.close()
  }
}

main().catch(error => {
  console.error(error)
  process.exit(1)
})
