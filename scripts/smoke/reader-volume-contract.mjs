#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'
const readerUrl = `${targetUrl.replace(/\/$/, '')}/books/1/read?chapter=0`

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

async function installMocks(page) {
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const path = new URL(request.url()).pathname.replace(/^\/api/, '')
    const method = request.method()
    if (path === '/me') return route.fulfill(json({ id: 1, username: 'smoke', role: 'admin' }))
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(json({
        key: 'reader',
        updatedAt: '2026-07-16T00:00:00Z',
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
          animateDuration: 0,
          clickMethod: 'auto',
        },
      }))
    }
    if (path === '/settings/reader' && method === 'PUT') return route.fulfill(json({ key: 'reader', updatedAt: '2026-07-16T00:00:01Z', value: {} }))
    if (path === '/books/1') return route.fulfill(json({ id: 1, title: '卷章节契约测试', author: 'OpenReader', chapterCount: 1, progress: null }))
    if (path === '/books') return route.fulfill(json([{ id: 1, title: '卷章节契约测试', author: 'OpenReader', chapterCount: 1, progress: null }]))
    if (path === '/books/1/chapters') return route.fulfill(json([{ id: 11, index: 0, title: '卷一', isVolume: true }]))
    if (path === '/books/1/chapters/0/content') {
      return route.fulfill(json({
        chapter: { id: 11, index: 0, title: '卷一', isVolume: true },
        content: '卷首语\n敬请期待',
        format: 'text',
      }))
    }
    if (path === '/books/1/bookmarks' || path === '/progress/1' || path === '/sources' || path === '/categories') return route.fulfill(json([]))
    if (path === '/progress' && method === 'PUT') return route.fulfill(json({ bookId: 1, chapterId: 11, chapterIndex: 0, offset: 0, percent: 0, chapterPercent: 0 }))
    return route.fulfill(json({}))
  })
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({ viewport })
  await context.addInitScript(token => {
    localStorage.setItem('openreader_token', token)
    localStorage.setItem('reader', JSON.stringify({ mode: 'page', settingsScope: 'user:1', progressScope: 'user:1' }))
  }, fakeToken())
  const page = await context.newPage()
  const failures = []
  page.on('console', message => {
    if (message.type() === 'error' && !message.text().includes('/ws/sync')) failures.push(message.text())
  })
  page.on('pageerror', error => failures.push(error.stack || error.message))
  await installMocks(page)
  try {
    await page.goto(readerUrl, { waitUntil: 'networkidle' })
    await page.waitForSelector('.volume-chapter .volume-content', { timeout: 10_000 })
    const state = await page.evaluate(() => {
      const chapter = document.querySelector('.volume-chapter')
      const content = chapter?.querySelector('.volume-content')
      const title = content?.querySelector('h3')
      const tag = content?.querySelector('.volume-tag')
      const chapterRect = chapter?.getBoundingClientRect()
      const contentRect = content?.getBoundingClientRect()
      const titleRect = title?.getBoundingClientRect()
      const tagStyle = tag ? getComputedStyle(tag) : null
      const chapterStyle = chapter ? getComputedStyle(chapter) : null
      return {
        contentTopDelta: contentRect.top - chapterRect.top,
        titleCenterDelta: Math.abs((titleRect.left + titleRect.width / 2) - (chapterRect.left + chapterRect.width / 2)),
        minHeight: chapterStyle?.minHeight,
        display: chapterStyle?.display,
        flexDirection: chapterStyle?.flexDirection,
        justifyContent: chapterStyle?.justifyContent,
        alignItems: chapterStyle?.alignItems,
        tagTextAlign: tagStyle?.textAlign,
        tagTextIndent: tagStyle?.textIndent,
        tagWhiteSpace: tagStyle?.whiteSpace,
      }
    })
    assert(Math.abs(state.contentTopDelta) <= 1, `${viewport.width}: volume content must start at chapter top, got ${state.contentTopDelta}px`)
    assert(state.titleCenterDelta <= 1, `${viewport.width}: volume title is not centered (${state.titleCenterDelta}px)`)
    assert(state.minHeight === `${viewport.height}px`, `${viewport.width}: volume min-height ${state.minHeight}`)
    assert(state.display === 'flex' && state.flexDirection === 'column' && state.alignItems === 'center', `${viewport.width}: volume flex layout ${JSON.stringify(state)}`)
    assert(state.justifyContent !== 'center', `${viewport.width}: volume must not vertically center content`)
    assert(state.tagTextAlign === 'right', `${viewport.width}: volume tag alignment ${state.tagTextAlign}`)
    assert(state.tagTextIndent === '36px', `${viewport.width}: volume tag indent ${state.tagTextIndent}`)
    assert(state.tagWhiteSpace === 'normal', `${viewport.width}: volume tag whitespace ${state.tagWhiteSpace}`)
    assert(failures.length === 0, `${viewport.width}: browser errors\n${failures.join('\n')}`)
  } finally {
    await context.close()
  }
}

async function main() {
  const browser = await openSmokeBrowser()
  try {
    for (const viewport of [
      { width: 1440, height: 900 },
      { width: 390, height: 844 },
      { width: 360, height: 800 },
    ]) {
      await runViewport(browser, viewport)
    }
    console.log('reader volume contract smoke passed')
  } finally {
    await browser.close()
  }
}

main().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
