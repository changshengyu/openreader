#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'
const readerUrl = process.env.SMOKE_READER_URL || `${targetUrl.replace(/\/$/, '')}/books/1/read?chapter=0`

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

function chapterContent(index) {
  return Array.from({ length: 110 }, (_, paragraph) => (
    `第${index + 1}章第${paragraph + 1}段。连续滚动契约正文用于验证原生滚动、跨章窗口、锚点保持与左右留白。`
  )).join('\n')
}

async function installMocks(page, requestCounts, options = {}) {
  const attempts = new Map()
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    if (path === '/me') {
      return route.fulfill(json({ id: 1, username: 'smoke', role: 'admin' }))
    }
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(json({
        key: 'reader',
        updatedAt: '2026-07-06T00:00:00Z',
        value: {
          mode: options.mode || 'scroll2',
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
        title: '连续跨章契约测试',
        author: 'OpenReader',
        sourceId: 0,
        chapterCount: 6,
        progress: null,
      }))
    }
    if (path === '/books/1/chapters') {
      return route.fulfill(json(Array.from({ length: 6 }, (_, index) => ({
        id: index + 11,
        index,
        title: `第 ${index + 1} 章`,
      }))))
    }
    const contentMatch = path.match(/^\/books\/1\/chapters\/(\d+)\/content$/)
    if (contentMatch) {
      const index = Number(contentMatch[1])
      requestCounts.set(index, (requestCounts.get(index) || 0) + 1)
      attempts.set(index, (attempts.get(index) || 0) + 1)
      if (options.failOnceAt === index && attempts.get(index) === 1) {
        return route.fulfill(json({ error: 'fixture adjacent chapter failure' }, 502))
      }
      return route.fulfill(json({
        chapter: { id: index + 11, index, title: `第 ${index + 1} 章` },
        content: chapterContent(index),
        format: 'text',
      }))
    }
    if (path === '/books/1/bookmarks') return route.fulfill(json([]))
    if (path === '/progress/1') return route.fulfill(json({}))
    if (path === '/progress' && method === 'PUT') {
      const body = request.postDataJSON?.() || {}
      return route.fulfill(json({ ...body, bookId: 1 }))
    }
    if (path === '/sources' || path === '/categories') return route.fulfill(json([]))
    return route.fulfill(json({}))
  })
}

function collectFailures(page) {
  const failures = []
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    const text = message.text()
    if (text.includes('/ws/sync') && text.includes('WebSocket connection')) return
    failures.push(text)
  })
  page.on('pageerror', error => failures.push(error.stack || error.message))
  return failures
}

async function openReader(browser, viewport, options = {}) {
  const context = await browser.newContext({ viewport })
  await context.addInitScript(({ mode, token }) => {
    localStorage.setItem('openreader_token', token)
    localStorage.setItem('reader', JSON.stringify({
      mode,
      settingsScope: 'user:1',
      progressScope: 'user:1',
    }))
  }, { mode: options.mode || 'scroll2', token: fakeToken() })
  const page = await context.newPage()
  const failures = collectFailures(page)
  const requestCounts = new Map()
  await installMocks(page, requestCounts, options)
  await page.goto(readerUrl, { waitUntil: 'networkidle' })
  await page.waitForSelector('.chapter-content[data-index="0"] [data-reader-block]', { timeout: 10_000 })
  return { context, failures, page, requestCounts }
}

async function runContinuousViewport(browser, viewport, mode) {
  const fixture = await openReader(browser, viewport, { mode })
  const { context, failures, page, requestCounts } = fixture
  try {
    await page.waitForFunction(() => (
      [...document.querySelectorAll('.chapter-content')].map(element => Number(element.dataset.index)).join(',') === '0,1'
    ))

    const initial = await page.evaluate(() => {
      const pageEl = document.querySelector('.reader-page').getBoundingClientRect()
      const chapter = document.querySelector('.chapter-content[data-index="0"]').getBoundingClientRect()
      const content = document.querySelector('.reader-content')
      return {
        indexes: [...document.querySelectorAll('.chapter-content')].map(element => Number(element.dataset.index)),
        leftGap: chapter.left - pageEl.left,
        rightGap: pageEl.right - chapter.right,
        scrollTop: content.scrollTop,
      }
    })
    assert(initial.indexes.join(',') === '0,1', `${viewport.width}: initial blocks ${initial.indexes}`)
    assert(Math.abs(initial.leftGap - initial.rightGap) <= 1, `${viewport.width}: asymmetric gaps ${initial.leftGap}/${initial.rightGap}`)

    await page.locator('.reader-content').hover()
    await page.mouse.wheel(0, 137)
    await page.waitForTimeout(120)
    const wheelTop = await page.locator('.reader-content').evaluate(element => element.scrollTop)
    assert(wheelTop > initial.scrollTop, `${viewport.width}: native wheel did not move`)
    assert(wheelTop - initial.scrollTop < 500, `${viewport.width}: wheel became paged movement ${wheelTop - initial.scrollTop}`)

    await page.keyboard.press('ArrowDown')
    await page.waitForTimeout(120)
    const keyboardTop = await page.locator('.reader-content').evaluate(element => element.scrollTop)
    assert(keyboardTop > wheelTop, `${viewport.width}/${mode}: ArrowDown did not page the vertical reader`)

    await page.locator('.reader-content').evaluate(element => { element.scrollTop = 0 })
    await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height * 0.72))
    await page.waitForTimeout(120)
    const clickTop = await page.locator('.reader-content').evaluate(element => element.scrollTop)
    assert(clickTop > 0, `${viewport.width}/${mode}: lower-region click did not page the vertical reader`)

    await page.locator('.reader-content').evaluate(element => { element.scrollTop = 0 })
    const preExtension = await page.evaluate(() => {
      const content = document.querySelector('.reader-content')
      const chapter = document.querySelector('.chapter-content[data-index="1"]')
      const threshold = content.scrollHeight - content.clientHeight * 4
      const target = Math.min(
        Math.max(chapter.offsetTop + 240, 0),
        Math.max(0, threshold - 40),
      )
      if (target <= chapter.offsetTop) {
        throw new Error(`fixture cannot enter chapter 2 before extension threshold (${target}/${chapter.offsetTop}/${threshold})`)
      }
      content.scrollTop = target
      content.dispatchEvent(new Event('scroll'))
      return { target, threshold }
    })
    await page.waitForTimeout(180)
    const beforeExtension = await page.evaluate(() => ({
      indexes: [...document.querySelectorAll('.chapter-content')].map(element => Number(element.dataset.index)),
      chapterLabel: document.querySelector('.reader-page-head')?.lastElementChild?.textContent || '',
    }))
    assert(beforeExtension.indexes.join(',') === '0,1', `${viewport.width}/${mode}: pre-extension blocks ${beforeExtension.indexes}`)
    assert(beforeExtension.chapterLabel.startsWith('2 /'), `${viewport.width}/${mode}: visible chapter did not advance before extension (${beforeExtension.chapterLabel})`)

    const anchor = await page.evaluate(() => {
      const content = document.querySelector('.reader-content')
      const chapter = document.querySelector('.chapter-content[data-index="1"]')
      content.scrollTop = Math.min(
        content.scrollHeight - content.clientHeight,
        content.scrollHeight - content.clientHeight * 4 + 20,
      )
      const viewport = content.getBoundingClientRect()
      const anchorY = viewport.top + Math.min(viewport.height * 0.32, 180)
      const paragraph = [...chapter.querySelectorAll('[data-reader-block]')]
        .find(element => {
          const rect = element.getBoundingClientRect()
          return rect.top <= anchorY && rect.bottom >= anchorY
        })
        || chapter.querySelector('[data-reader-block]')
      const state = {
        chapterIndex: 1,
        pos: paragraph.dataset.pos,
        top: paragraph.getBoundingClientRect().top,
        scrollTop: content.scrollTop,
        scrollHeight: content.scrollHeight,
        clientHeight: content.clientHeight,
      }
      content.dispatchEvent(new Event('scroll'))
      return state
    })

    try {
      await page.waitForFunction(() => {
        const indexes = [...document.querySelectorAll('.chapter-content')]
          .map(element => Number(element.dataset.index))
        return indexes.includes(2)
      }, null, { timeout: 10_000 })
    } catch (error) {
      const state = await page.evaluate(() => {
        const content = document.querySelector('.reader-content')
        return {
          indexes: [...document.querySelectorAll('.chapter-content')].map(element => Number(element.dataset.index)),
          scrollTop: content.scrollTop,
          scrollHeight: content.scrollHeight,
          clientHeight: content.clientHeight,
        }
      })
      throw new Error(`${error.message}; before=${JSON.stringify(anchor)} after=${JSON.stringify(state)} requests=${JSON.stringify([...requestCounts])}`)
    }
    const after = await page.evaluate(({ chapterIndex, pos }) => {
      const paragraph = document.querySelector(
        `.chapter-content[data-index="${chapterIndex}"] [data-reader-block][data-pos="${pos}"]`,
      )
      return {
        top: paragraph?.getBoundingClientRect().top,
        indexes: [...document.querySelectorAll('.chapter-content')].map(element => Number(element.dataset.index)),
      }
    }, anchor)
    const expectedIndexes = mode === 'scroll' ? '0,1,2' : '1,2'
    assert(after.indexes.join(',') === expectedIndexes, `${viewport.width}/${mode}: blocks ${after.indexes}`)
    assert(Number.isFinite(after.top), `${viewport.width}: anchored paragraph disappeared`)
    assert(Math.abs(after.top - anchor.top) <= 2, `${viewport.width}: anchor jumped ${after.top - anchor.top}px`)
    assert((requestCounts.get(1) || 0) === 1, `${viewport.width}: duplicate chapter 1 requests ${requestCounts.get(1)}`)
    assert((requestCounts.get(2) || 0) === 1, `${viewport.width}: duplicate chapter 2 requests ${requestCounts.get(2)}`)
    assert(failures.length === 0, failures.join('\n'))
  } finally {
    await context.close()
  }
}

async function runFailureRetry(browser) {
  const fixture = await openReader(browser, { width: 390, height: 844 }, { failOnceAt: 1 })
  const { context, failures, page, requestCounts } = fixture
  try {
    await page.waitForSelector('.chapter-inline-error', { timeout: 10_000 })
    assert(await page.locator('.chapter-content[data-index="0"] [data-reader-block]').count() > 0, 'current chapter disappeared after adjacent failure')
    assert(await page.locator('.chapter-inline-error').innerText().then(text => text.includes('fixture adjacent chapter failure')), 'adjacent error is not visible')
    await page.locator('.chapter-inline-error button').click()
    await page.waitForFunction(() => (
      !document.querySelector('.chapter-content[data-index="1"] .chapter-inline-error')
      && document.querySelectorAll('.chapter-content[data-index="1"] [data-reader-block]').length > 1
    ), null, { timeout: 10_000 })
    assert(await page.locator('.chapter-inline-error').count() === 0, 'retry did not replace adjacent error block')
    assert((requestCounts.get(1) || 0) === 2, `retry request count ${requestCounts.get(1)}`)
    const unexpectedFailures = failures.filter(text => (
      !text.includes('Failed to load resource: the server responded with a status of 502')
    ))
    assert(unexpectedFailures.length === 0, unexpectedFailures.join('\n'))
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
      await runContinuousViewport(browser, viewport, 'scroll')
      await runContinuousViewport(browser, viewport, 'scroll2')
    }
    await runFailureRetry(browser)
    console.log('reader continuous contract smoke passed')
  } finally {
    await browser.close()
  }
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})
