#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = (process.env.TARGET_URL || 'http://127.0.0.1:4173').replace(/\/$/, '')
const readerUrl = `${targetUrl}/books/1/read?chapter=0`

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function json(data, status = 200) {
  return { status, contentType: 'application/json', body: JSON.stringify(data) }
}

function fakeToken() {
  const payload = Buffer.from(JSON.stringify({ userId: 1, sub: '1' })).toString('base64url')
  return `open.${payload}.reader`
}

function initialBook() {
  return {
    id: 1,
    title: '换源浏览器契约书',
    author: 'OpenReader',
    sourceId: 2,
    sourceName: '当前来源',
    url: 'https://current.example/book',
    bookUrl: 'https://current.example/book',
    chapterCount: 2,
    lastChapter: '第二章',
    categoryIds: [],
    progress: null,
  }
}

function currentCandidate(latestChapterTitle = '第二章') {
  return {
    sourceId: 2,
    sourceName: '当前来源',
    group: '默认',
    title: '换源浏览器契约书',
    author: 'OpenReader',
    bookUrl: 'https://current.example/book',
    latestChapterTitle,
    time: 18,
    current: true,
    type: 0,
  }
}

function cachedCandidate() {
  return {
    sourceId: 3,
    sourceName: '缓存来源',
    group: '默认',
    title: '换源浏览器契约书',
    author: 'OpenReader',
    bookUrl: 'https://cached.example/book',
    latestChapterTitle: '第三章',
    time: 34,
    current: false,
    type: 0,
  }
}

function searchedCandidate() {
  return {
    sourceId: 4,
    sourceName: '加载来源',
    group: '备用',
    title: '换源浏览器契约书',
    author: 'OpenReader',
    bookUrl: 'https://searched.example/book',
    latestChapterTitle: '第四章',
    time: 51,
    current: false,
    type: 0,
  }
}

function chapterContent() {
  return [
    '第一章 开始',
    ...Array.from({ length: 70 }, (_, index) => `换源位置契约段落 ${index + 1}：面板操作与来源切换不应把正文重置到章节开头。`),
  ].join('\n')
}

async function installMocks(page) {
  const state = {
    book: initialBook(),
    candidateRequests: [],
    changeRequests: [],
    emptyAvailable: false,
  }
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    if (path === '/me') return route.fulfill(json({ id: 1, username: 'source-smoke', role: 'admin' }))
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(json({
        key: 'reader',
        value: {
          mode: 'scroll', pageMode: 'auto', autoTheme: false, theme: 'parchment', themeType: 'day',
          fontSize: 18, lineHeight: 1.8, paragraphSpace: 0.2, columnWidth: 800,
        },
      }))
    }
    if (path === '/settings/reader' && method === 'PUT') return route.fulfill(json({ key: 'reader', value: {} }))
    if (path === '/books/1' && method === 'GET') return route.fulfill(json(state.book))
    if (path === '/books' && method === 'GET') return route.fulfill(json([state.book]))
    if (path === '/books/1/chapters' && method === 'GET') {
      return route.fulfill(json([
        { id: 11, index: 0, title: '第一章' },
        { id: 12, index: 1, title: '第二章' },
      ]))
    }
    if (path === '/books/1/chapters/0/content' && method === 'GET') {
      return route.fulfill(json({ chapter: { id: 11, index: 0, title: '第一章' }, content: chapterContent() }))
    }
    if (path === '/progress/1' && method === 'GET') return route.fulfill(json({}))
    if (path === '/progress' && method === 'PUT') return route.fulfill(json(request.postDataJSON()))
    if (path === '/sources' && method === 'GET') {
      return route.fulfill(json([
        { id: 2, name: '当前来源', group: '默认', enabled: true },
        { id: 3, name: '缓存来源', group: '默认', enabled: true },
        { id: 4, name: '加载来源', group: '备用', enabled: true },
      ]))
    }
    if (path === '/categories' || path === '/book-groups') return route.fulfill(json([]))
    if (path === '/cache/stats') return route.fulfill(json({ total: 0, size: 0 }))
    if (path === '/books/1/source-candidates' && method === 'GET') {
      const params = Object.fromEntries(url.searchParams.entries())
      state.candidateRequests.push(params)
      if ((params.mode || 'available') === 'available') {
        await new Promise(resolve => setTimeout(resolve, 180))
        return route.fulfill(json(state.emptyAvailable ? [] : [currentCandidate(), cachedCandidate()]))
      }
      if (params.mode === 'refresh') {
        await new Promise(resolve => setTimeout(resolve, 160))
        return route.fulfill(json([currentCandidate('刷新后的第二章')]))
      }
      if (params.mode === 'search') {
        await new Promise(resolve => setTimeout(resolve, 160))
        return route.fulfill(json({
          list: [searchedCandidate()],
          offset: Number(params.offset || 0),
          nextOffset: 3,
          hasMore: false,
          total: 3,
          searched: 3,
          matched: 1,
          failed: 1,
          empty: 1,
        }))
      }
      return route.fulfill(json({ error: 'invalid source candidate mode' }, 400))
    }
    if (path === '/books/1/change-source' && method === 'POST') {
      const payload = request.postDataJSON()
      state.changeRequests.push(payload)
      state.book = {
        ...state.book,
        sourceId: payload.sourceId,
        sourceName: '加载来源',
        url: payload.bookUrl,
        bookUrl: payload.bookUrl,
      }
      return route.fulfill(json(state.book))
    }
    if (path === '/books/1/bookmarks') return route.fulfill(json([]))
    return route.fulfill(json({}))
  })
  return state
}

async function openSourcePanel(page, viewport) {
  const button = viewport.width <= 750
    ? page.locator('.reader-mobile-top.visible .mobile-tool-button').filter({ hasText: '书源' })
    : page.locator('.reader-left-rail button[title="书源"]')
  await button.click()
  await page.locator('.source-switch-list').waitFor({ state: 'visible', timeout: 10_000 })
}

async function readerPosition(page) {
  return page.evaluate(() => {
    const content = document.querySelector('.reader-content')
    if (!content) return null
    const contentRect = content.getBoundingClientRect()
    const documentScroll = Math.abs(Number(window.scrollY) || 0) > 1
      && Math.abs(Number(content.scrollTop) || 0) <= 1
    const viewportTop = documentScroll ? 36 : contentRect.top + 36
    const blocks = Array.from(content.querySelectorAll('[data-reader-block]'))
    const visible = blocks.find((block) => block.getBoundingClientRect().bottom > viewportTop)
    return {
      scrollTop: Math.round(documentScroll ? window.scrollY : content.scrollTop),
      text: String(visible?.textContent || '').trim(),
    }
  })
}

async function waitForStablePosition(page, expectedText) {
  await page.waitForFunction((text) => {
    const content = document.querySelector('.reader-content')
    if (!content) return false
    const contentRect = content.getBoundingClientRect()
    const documentScroll = Math.abs(Number(window.scrollY) || 0) > 1
      && Math.abs(Number(content.scrollTop) || 0) <= 1
    const viewportTop = documentScroll ? 24 : contentRect.top + 24
    const viewportBottom = documentScroll ? window.innerHeight : contentRect.bottom
    return Array.from(content.querySelectorAll('[data-reader-block]')).some((block) => (
      String(block.textContent || '').trim() === text
      && block.getBoundingClientRect().bottom > viewportTop
      && block.getBoundingClientRect().top < viewportBottom
    ))
  }, expectedText, { timeout: 10_000 })
}

async function assertPanelGeometry(page, viewport) {
  const geometry = await page.evaluate(() => {
    const list = document.querySelector('.source-switch-list')
    const actions = document.querySelector('.title-actions')
    const listRect = list?.getBoundingClientRect()
    const actionRects = Array.from(actions?.querySelectorAll('button') || []).map(button => {
      const rect = button.getBoundingClientRect()
      return { left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom, text: button.innerText }
    })
    return {
      list: listRect ? { left: listRect.left, right: listRect.right, top: listRect.top, bottom: listRect.bottom } : null,
      actions: actionRects,
    }
  })
  assert(geometry.list, `${viewport.width}: source list geometry missing`)
  assert(geometry.list.left >= 0 && geometry.list.right <= viewport.width, `${viewport.width}: source list overflows horizontally`)
  for (const action of geometry.actions) {
    assert(action.left >= 0 && action.right <= viewport.width, `${viewport.width}: source action ${action.text} overflows`)
  }
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({ viewport })
  await context.addInitScript(token => window.localStorage.setItem('openreader_token', token), fakeToken())
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() !== 'error') return
    const text = message.text()
    if (text.includes('/ws/sync') && text.includes('WebSocket connection')) return
    failures.push(`console.error: ${text}`)
  })
  const state = await installMocks(page)
  await page.goto(readerUrl, { waitUntil: 'networkidle' })
  await page.locator('.reader-body p').nth(24).evaluate(element => {
    element.scrollIntoView({ block: 'start' })
  })
  await page.waitForTimeout(120)
  const beforeOpen = await readerPosition(page)
  const initialGeometry = await page.evaluate(() => {
    const content = document.querySelector('.reader-content')
    const target = document.querySelectorAll('.reader-body p')[24]
    return {
      windowY: window.scrollY,
      documentScrollTop: document.scrollingElement?.scrollTop,
      documentHeight: document.scrollingElement?.scrollHeight,
      documentClientHeight: document.scrollingElement?.clientHeight,
      contentScrollTop: content?.scrollTop,
      contentHeight: content?.scrollHeight,
      contentClientHeight: content?.clientHeight,
      contentRect: content?.getBoundingClientRect().toJSON(),
      targetRect: target?.getBoundingClientRect().toJSON(),
      shellClass: document.querySelector('.reader-shell')?.className,
    }
  })
  assert(beforeOpen?.text.includes('换源位置契约段落'), `${viewport.width}: reader did not reach a stable paragraph position=${JSON.stringify(beforeOpen)} geometry=${JSON.stringify(initialGeometry)}`)

  const availableResponse = page.waitForResponse(response => {
    const url = new URL(response.url())
    return url.pathname === '/api/books/1/source-candidates' && (url.searchParams.get('mode') || 'available') === 'available'
  })
  await openSourcePanel(page, viewport)
  assert(await page.locator('.title-actions button').filter({ hasText: '刷新' }).isDisabled(), `${viewport.width}: refresh must wait for available`)
  assert(await page.locator('.title-actions button').filter({ hasText: '加载更多' }).isDisabled(), `${viewport.width}: load-more must wait for available`)
  if (viewport.width <= 750) {
    assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: source panel hid the mobile tools`)
  }
  await availableResponse
  await page.getByText('来源(2)', { exact: true }).waitFor({ state: 'visible' })
  await assertPanelGeometry(page, viewport)
  assert(await page.locator('.source-item.selected').count() === 1, `${viewport.width}: current source projection missing`)
  assert((await readerPosition(page)).text === beforeOpen.text, `${viewport.width}: opening source panel moved the reader body`)
  assert(state.candidateRequests.length === 1 && state.candidateRequests[0].mode === 'available', `${viewport.width}: opening mode = ${JSON.stringify(state.candidateRequests)}`)

  const refreshButton = page.locator('.title-actions button').filter({ hasText: /^刷新/ })
  const loadMoreButton = page.locator('.title-actions button').filter({ hasText: /加载更多|没有更多|加载中/ })
  await refreshButton.click()
  await page.getByRole('button', { name: '刷新中...' }).waitFor({ state: 'visible' })
  assert(await loadMoreButton.isDisabled(), `${viewport.width}: load-more remained actionable during refresh`)
  assert((await loadMoreButton.innerText()) === '加载更多', `${viewport.width}: refresh mislabeled load-more as busy`)
  await page.getByText('来源(1)', { exact: true }).waitFor({ state: 'visible' })
  assert(await page.getByText('刷新后的第二章', { exact: true }).count() === 1, `${viewport.width}: partial refresh lost the retained current source`)

  await loadMoreButton.click()
  await page.getByRole('button', { name: '加载中...' }).waitFor({ state: 'visible' })
  assert((await refreshButton.innerText()) === '刷新', `${viewport.width}: load-more mislabeled refresh as busy`)
  await page.getByText('来源(2)', { exact: true }).waitFor({ state: 'visible' })
  await page.getByText('加载来源', { exact: true }).waitFor({ state: 'visible' })
  assert((await loadMoreButton.innerText()) === '没有更多', `${viewport.width}: source cursor did not reach its tail`)

  const beforeChange = await readerPosition(page)
  const requestCountBeforeChange = state.candidateRequests.length
  const changeResponse = page.waitForResponse(response => (
    new URL(response.url()).pathname === '/api/books/1/change-source'
  ))
  await page.locator('.source-item').filter({ hasText: '加载来源' }).click()
  await changeResponse
  await page.waitForFunction(() => (
    !document.querySelector('.source-switch-list')
    || Boolean(document.querySelector('.el-message--error'))
  ), null, { timeout: 10_000 })
  const immediateMessages = await page.locator('.el-message--error').allInnerTexts()
  assert(immediateMessages.length === 0, `${viewport.width}: source change failed: ${immediateMessages.join(' | ')} failures=${JSON.stringify(failures)}`)
  try {
    await page.locator('.source-switch-list').waitFor({ state: 'hidden', timeout: 10_000 })
  } catch (error) {
    const status = await page.locator('.source-status').allInnerTexts()
    const messages = await page.locator('.el-message').allInnerTexts()
    throw new Error(`${viewport.width}: source panel stayed open requests=${JSON.stringify(state.changeRequests)} status=${JSON.stringify(status)} messages=${JSON.stringify(messages)} failures=${JSON.stringify(failures)} cause=${error.message}`)
  }
  await waitForStablePosition(page, beforeChange.text)
  assert(state.changeRequests.length === 1, `${viewport.width}: source change request count ${state.changeRequests.length}`)
  assert(state.changeRequests[0].sourceId === 4 && state.changeRequests[0].bookUrl === 'https://searched.example/book', `${viewport.width}: source change payload ${JSON.stringify(state.changeRequests[0])}`)
  await page.waitForTimeout(250)
  assert(state.candidateRequests.length === requestCountBeforeChange, `${viewport.width}: source change triggered an extra candidate request`)
  const afterChange = await readerPosition(page)
  assert(afterChange.text === beforeChange.text, `${viewport.width}: source change moved from ${beforeChange.text} to ${afterChange.text}`)
  assert(failures.length === 0, `${viewport.width}: ${failures.join('\n')}`)

  state.emptyAvailable = true
  await page.reload({ waitUntil: 'networkidle' })
  await openSourcePanel(page, viewport)
  await page.getByText('没有找到可用来源', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 })
  assert(await page.getByText('来源(0)', { exact: true }).count() === 1, `${viewport.width}: empty candidate state title missing`)
  await context.close()
  console.log(`${viewport.width}x${viewport.height}: source switch ok available/refresh/search/change/empty position=${afterChange.text}`)
}

async function main() {
  const browser = await openSmokeBrowser()
  try {
    for (const viewport of [
      { width: 1440, height: 900 },
      { width: 390, height: 844 },
      { width: 360, height: 800 },
      { width: 1024, height: 1366 },
    ]) {
      await runViewport(browser, viewport)
    }
  } finally {
    await browser.close()
  }
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})
