#!/usr/bin/env node

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'
const chromePath = process.env.CHROME_PATH || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
const readerPath = '/books/1/read?chapter=0'
const fixtureText = Array.from({ length: 44 }, (_, index) => (
  `第${index + 1}段。春风过处，纸页微明。用于验证阅读模式的正文宽度、首屏位置和翻页位移。`
)).join('\n')

async function loadPlaywright() {
  try {
    const module = await import('playwright')
    return module.chromium ? module : module.default
  } catch {
    const bundled = '/Users/yuchangsheng/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright/index.js'
    const module = await import(bundled)
    return module.chromium ? module : module.default
  }
}

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

async function installApiMocks(page, mode) {
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
          animateDuration: 0,
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

async function openReader(browser, viewport, mode) {
  const context = await browser.newContext({ viewport })
  await context.addInitScript(value => localStorage.setItem('openreader_token', value), token())
  const page = await context.newPage()
  await installApiMocks(page, mode)
  await page.goto(`${targetUrl.replace(/\/$/, '')}${readerPath}`, { waitUntil: 'networkidle' })
  await page.waitForSelector('.reader-body p', { timeout: 15_000 })
  return { context, page }
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

async function main() {
  const { chromium } = await loadPlaywright()
  const browser = await chromium.launch({ headless: true, executablePath: chromePath })
  try {
    await assertDesktopPage(browser)
    for (const viewport of [{ width: 390, height: 844 }, { width: 360, height: 800 }]) {
      await assertMobilePage(browser, viewport)
      await assertMobileFlip(browser, viewport)
    }
    console.log('reader text-mode contract smoke passed')
  } finally {
    await browser.close()
  }
}

main().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
