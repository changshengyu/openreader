#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'
import { readShellDocumentTitle, waitForDocumentTitle } from './document-title.mjs'

// Contract: docs/compat/browser-document-title-fixed-baseline-second-audit-p0-contract.md 8.2(1), Reader steps
// 2-10 and 12. Steps 1 and 11 (home overlays, login and source-debug titles) wait for the product-name decision D1.

const targetUrl = (process.env.TARGET_URL || 'http://127.0.0.1:5173').replace(/\/$/, '')
const shellTitle = readShellDocumentTitle()

const books = {
  1: { id: 1, title: '标题契约测试', author: '标题作者', sourceId: 0, chapterCount: 6, categoryIds: [], progress: null },
  2: { id: 2, title: '标题契约测试二', author: '标题作者', sourceId: 0, chapterCount: 3, categoryIds: [], progress: null },
  3: { id: 3, title: '标题契约测试三', author: '标题作者', sourceId: 0, chapterCount: 3, categoryIds: [], progress: null },
}
const catalogs = {
  1: ['第一章 起点', '第二章 中途', '', '第四章 失败', '第五章', '   '],
  2: ['序章 新书', '第一章 起点', '第二章 余波'],
  3: ['第一章 起点', '第二章 归途', '第三章 终点'],
}
const failingChapter = { bookId: 1, index: 3 }
const remoteSessionId = 'document-title-session'
const remoteCatalog = ['第一章', '第二章']
const readerSettings = {
  mode: 'page',
  pageMode: 'auto',
  theme: 'parchment',
  themeType: 'day',
  autoTheme: false,
  fontSize: 18,
  fontWeight: 400,
  lineHeight: 1.8,
  paragraphSpace: 0.2,
  columnWidth: 800,
  animateDuration: 0,
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

function tokenFor(nonce) {
  const payload = Buffer.from(JSON.stringify({ userId: 1, sub: '1' })).toString('base64url')
  return `open.${payload}.${nonce}`
}

function smokeViewports() {
  const requested = String(process.env.SMOKE_VIEWPORTS || '1440x900,390x844,360x800')
    .split(',')
    .map(value => value.trim())
    .filter(Boolean)
  return requested.map((value) => {
    const [width, height] = value.toLowerCase().split('x').map(Number)
    if (!Number.isInteger(width) || !Number.isInteger(height) || width < 1 || height < 1) {
      throw new Error(`Invalid SMOKE_VIEWPORTS entry: ${value}`)
    }
    return { width, height }
  })
}

function catalogRows(bookId) {
  // Real /books/:id/chapters rows carry bookId (models.Chapter.BookID).
  return catalogs[bookId].map((title, index) => ({ id: bookId * 100 + index, bookId, index, title }))
}

function chapterContent(label, index) {
  return Array.from({ length: 12 }, (_, paragraph) => (
    `${label}第${index + 1}章第${paragraph + 1}段。标签页标题契约正文。`
  )).join('\n')
}

function remoteSession() {
  // Temporary sessions are not persisted: the API serializes book.id and row.bookId as 0.
  return {
    id: remoteSessionId,
    expiresAt: '2099-01-01T00:00:00Z',
    book: {
      id: 0,
      title: '远程临时书',
      author: '远程作者',
      sourceId: 7,
      url: 'https://source.example/document-title/book',
      chapterCount: remoteCatalog.length,
    },
    chapters: remoteCatalog.map((title, index) => ({
      id: 0,
      bookId: 0,
      index,
      title,
      url: `https://source.example/document-title/chapter/${index + 1}`,
    })),
  }
}

function createGate() {
  let release
  let markArrived
  const opened = new Promise(resolve => { release = resolve })
  const arrived = new Promise(resolve => { markArrived = resolve })
  return { arrived, markArrived, opened, release }
}

function withTimeout(promise, ms, message) {
  let timer
  return Promise.race([
    promise.finally(() => clearTimeout(timer)),
    new Promise((_, reject) => {
      timer = setTimeout(() => reject(new Error(message)), ms)
    }),
  ])
}

function installTitleLog() {
  const log = []
  window.__titleLog = log
  const record = () => {
    const title = document.title
    if (log[log.length - 1] !== title) log.push(title)
  }
  // Wrap the setter so writes that are replaced within one task are still recorded.
  const descriptor = Object.getOwnPropertyDescriptor(Document.prototype, 'title')
  Object.defineProperty(Document.prototype, 'title', {
    configurable: true,
    enumerable: descriptor.enumerable,
    get() {
      return descriptor.get.call(this)
    },
    set(value) {
      descriptor.set.call(this, value)
      if (this === window.document) record()
    },
  })
  new MutationObserver(record).observe(document, { childList: true, subtree: true, characterData: true })
}

async function installApiMocks(page, state) {
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    const authorization = request.headers().authorization || ''
    const token = authorization.startsWith('Bearer ') ? authorization.slice(7) : ''

    if (path === '/auth/login' && method === 'POST') {
      return route.fulfill(json({ token: state.renewedToken, user: state.user }))
    }
    if (path === '/me') return route.fulfill(json(state.user))
    if (path === '/health') return route.fulfill(json({ version: 'document-title-smoke', commit: 'document-title-smoke' }))
    if (path.startsWith('/settings/') && method === 'GET') {
      const key = path.slice('/settings/'.length)
      return route.fulfill(json({
        key,
        updatedAt: '2026-10-16T00:00:00Z',
        value: key === 'reader' ? readerSettings : {},
      }))
    }
    if (path.startsWith('/settings/') && method === 'PUT') {
      return route.fulfill(json({
        key: path.slice('/settings/'.length),
        updatedAt: '2026-10-16T00:00:01Z',
        value: request.postDataJSON?.()?.value || {},
      }))
    }
    if (path === '/books' && method === 'GET') return route.fulfill(json(Object.values(books)))
    const bookMatch = path.match(/^\/books\/(\d+)$/)
    if (bookMatch && method === 'GET') {
      const book = books[bookMatch[1]]
      return route.fulfill(book ? json(book) : json({ error: 'book not found' }, 404))
    }
    const catalogMatch = path.match(/^\/books\/(\d+)\/chapters$/)
    if (catalogMatch && method === 'GET') {
      const bookId = Number(catalogMatch[1])
      if (!catalogs[bookId]) return route.fulfill(json({ error: 'book not found' }, 404))
      const gate = state.catalogGates.get(bookId)
      if (gate) {
        gate.markArrived()
        await gate.opened
      }
      return route.fulfill(json(catalogRows(bookId)))
    }
    const contentMatch = path.match(/^\/books\/(\d+)\/chapters\/(\d+)\/content$/)
    if (contentMatch) {
      const bookId = Number(contentMatch[1])
      const index = Number(contentMatch[2])
      const reject = state.rejectContent
      if (reject && reject.bookId === bookId && reject.index === index && token === state.initialToken) {
        state.rejectContent = null
        return route.fulfill(json({ error: 'token expired' }, 401))
      }
      if (bookId === failingChapter.bookId && index === failingChapter.index) {
        return route.fulfill(json({ error: 'fixture chapter failure' }, 500))
      }
      const title = catalogs[bookId]?.[index]
      if (title === undefined) return route.fulfill(json({ error: 'chapter not found' }, 404))
      return route.fulfill(json({
        chapter: { id: bookId * 100 + index, bookId, index, title },
        content: chapterContent(books[bookId].title, index),
        format: 'text',
      }))
    }
    if (/^\/books\/\d+\/bookmarks$/.test(path)) return route.fulfill(json([]))
    if (/^\/progress\/\d+$/.test(path) && method === 'GET') return route.fulfill(json({}))
    if (path === '/progress' && method === 'PUT') {
      const body = request.postDataJSON?.() || {}
      return route.fulfill(json({ ...body, updatedAt: '2026-10-16T00:00:02Z' }))
    }
    if (path === `/reader/remote-sessions/${remoteSessionId}` && method === 'GET') {
      return route.fulfill(json(remoteSession()))
    }
    const remoteContentMatch = path.match(/^\/reader\/remote-sessions\/([^/]+)\/chapters\/(\d+)\/content$/)
    if (remoteContentMatch && remoteContentMatch[1] === remoteSessionId) {
      const index = Number(remoteContentMatch[2])
      const title = remoteCatalog[index]
      if (title === undefined) return route.fulfill(json({ error: 'chapter not found' }, 404))
      return route.fulfill(json({
        chapter: { id: 0, bookId: 0, index, title },
        content: chapterContent('远程临时书', index),
        format: 'text',
      }))
    }
    if (path === '/categories' || path === '/book-groups' || path === '/sources') {
      return route.fulfill(json([]))
    }
    return route.fulfill(json({}))
  })
}

async function openContext(browser, viewport) {
  const state = {
    initialToken: tokenFor('initial'),
    renewedToken: tokenFor('renewed'),
    user: { id: 1, username: 'title-smoke', role: 'admin' },
    catalogGates: new Map(),
    rejectContent: null,
  }
  const context = await browser.newContext({ viewport })
  await context.addInitScript(token => localStorage.setItem('openreader_token', token), state.initialToken)
  await context.addInitScript(installTitleLog)
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.stack || error.message}`))
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    const text = message.text()
    if (/WebSocket connection to .*\/ws\/sync/.test(text)) return
    if (/Failed to load resource: the server responded with a status of (401|500)/.test(text)) return
    failures.push(`console.error: ${text} ${message.location()?.url || ''}`.trim())
  })
  await installApiMocks(page, state)
  return { context, failures, page, state }
}

async function assertTitleLogClean(page, label) {
  const log = await page.evaluate(() => window.__titleLog || [])
  const malformed = log.filter(title => (
    !String(title || '').trim()
    || /^(?:undefined|null)(?: - |$)/.test(title)
    || / - (?:undefined|null)$/.test(title)
    || /^\s*-/.test(title)
  ))
  assert(malformed.length === 0, `${label}: malformed tab titles ${JSON.stringify(malformed)} in ${JSON.stringify(log)}`)
}

function isMobile(viewport) {
  return viewport.width <= 750
}

async function ensureMobileChrome(page, viewport) {
  if (!isMobile(viewport)) return
  if (await page.locator('.reader-mobile-top.visible').count()) return
  await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
  await page.locator('.reader-mobile-top.visible').waitFor({ timeout: 5000 })
}

async function clickReaderTool(page, viewport, label) {
  if (!isMobile(viewport)) {
    const title = label === '首页' ? '返回首页' : label
    await page.locator(`.reader-left-rail button[aria-label="${title}"]`).click()
    return
  }
  await ensureMobileChrome(page, viewport)
  await page.locator('.reader-mobile-top.visible .mobile-tool-button').filter({ hasText: label }).click()
}

// Desktop rail tools sit beside an open panel and close it; on phones reader-dev's panel covers the
// toolbar, so a tap below the panel closes it.
async function closeReaderPanel(page, viewport, label) {
  if (!isMobile(viewport)) {
    await clickReaderTool(page, viewport, label)
    return
  }
  const bottom = await page.locator('.reader-mobile-workspace').evaluate(element => element.getBoundingClientRect().bottom)
  await page.mouse.click(Math.round(viewport.width / 2), Math.min(viewport.height - 130, Math.ceil(bottom + 24)))
  await page.locator('.reader-mobile-workspace').waitFor({ state: 'detached', timeout: 10_000 })
}

async function nextChapter(page, viewport) {
  if (!isMobile(viewport)) {
    await page.locator('.reader-page-control button[aria-label="下一章"]').click()
    return
  }
  await ensureMobileChrome(page, viewport)
  await page.locator('.reader-mobile-bottom.visible .mobile-chapter-step').filter({ hasText: '下一章' }).click()
}

async function jumpFromToc(page, viewport, index) {
  await clickReaderTool(page, viewport, '目录')
  const item = page.locator(`.toc-item[data-chapter-index="${index}"]:visible`)
  await item.waitFor({ timeout: 10_000 })
  await item.click()
  await page.waitForURL(url => new URL(url).searchParams.get('chapter') === String(index), { timeout: 10_000 })
}

async function assertPanelsKeepTitle(page, viewport, label) {
  const expected = await page.evaluate(() => document.title)
  const before = await page.evaluate(() => window.__titleLog.length)
  await clickReaderTool(page, viewport, '目录')
  await page.locator('.toc-item:visible').first().waitFor({ timeout: 10_000 })
  await closeReaderPanel(page, viewport, '目录')
  await page.locator('.toc-item:visible').first().waitFor({ state: 'hidden', timeout: 10_000 })
  await clickReaderTool(page, viewport, '设置')
  await page.waitForTimeout(250)
  await closeReaderPanel(page, viewport, '设置')
  await page.waitForTimeout(250)
  const after = await page.evaluate(start => ({ title: document.title, writes: window.__titleLog.slice(start) }), before)
  assert(after.title === expected, `${label}: opening and closing panels changed the tab title to ${JSON.stringify(after.title)}`)
  assert(after.writes.length === 0, `${label}: panels wrote tab titles ${JSON.stringify(after.writes)}`)
}

async function switchBookFromShelf(page, viewport, state, target, label) {
  const gate = createGate()
  state.catalogGates.set(target.id, gate)
  const titleBefore = await page.evaluate(() => document.title)
  const logStart = await page.evaluate(() => window.__titleLog.length)
  try {
    await clickReaderTool(page, viewport, '书架')
    const card = page.locator(`.reader-shelf-card[data-book-id="${target.id}"]:visible`)
    await card.waitFor({ timeout: 10_000 })
    await card.click()
    await withTimeout(gate.arrived, 10_000, `${label}: catalog request for book ${target.id} never arrived`)
    // The Reader has switched books; route sync and book load run against the previous catalog meanwhile.
    await page.waitForTimeout(400)
    const held = await page.evaluate(start => ({
      title: document.title,
      writes: window.__titleLog.slice(start),
    }), logStart)
    const borrowed = held.writes.filter(title => title.startsWith(`${target.title} - `))
    assert(
      borrowed.length === 0,
      `${label}: ${target.title} was paired with a previous catalog title while its own catalog was held: ${JSON.stringify(held.writes)}`,
    )
    // R3 allows keeping the previous full title or only the new book name; a remounted Reader restores the shell (R5).
    assert(
      [titleBefore, target.title, shellTitle].includes(held.title),
      `${label}: while the catalog was held the tab must keep ${JSON.stringify(titleBefore)} or show only ${JSON.stringify(target.title)}, got ${JSON.stringify(held.title)}`,
    )
  } finally {
    state.catalogGates.delete(target.id)
    gate.release()
  }
}

async function runReaderNavigation(browser, viewport) {
  const label = `${viewport.width}x${viewport.height}`
  const { context, failures, page, state } = await openContext(browser, viewport)
  try {
    await page.goto(`${targetUrl}/books/1/read?chapter=0`, { waitUntil: 'networkidle' })
    await waitForDocumentTitle(page, '标题契约测试 - 第一章 起点', `${label} step 2 enter reader`)
    await assertTitleLogClean(page, `${label} step 2`)

    await nextChapter(page, viewport)
    await waitForDocumentTitle(page, '标题契约测试 - 第二章 中途', `${label} step 3 next chapter`)
    await assertPanelsKeepTitle(page, viewport, `${label} panels`)

    await jumpFromToc(page, viewport, 4)
    await waitForDocumentTitle(page, '标题契约测试 - 第五章', `${label} step 4 catalog jump`)

    await jumpFromToc(page, viewport, 2)
    await waitForDocumentTitle(page, '标题契约测试', `${label} step 5 empty chapter title`)

    await jumpFromToc(page, viewport, 3)
    await page.locator('.chapter-load-error:visible').waitFor({ timeout: 10_000 })
    await waitForDocumentTitle(page, '标题契约测试 - 第四章 失败', `${label} step 6 failed chapter keeps the target`)

    await jumpFromToc(page, viewport, 5)
    await waitForDocumentTitle(page, '标题契约测试', `${label} step 5 blank chapter title`)
    await assertTitleLogClean(page, `${label} steps 3-6`)

    await switchBookFromShelf(page, viewport, state, books[2], `${label} step 7 switch to book 2`)
    await waitForDocumentTitle(page, '标题契约测试二 - 序章 新书', `${label} step 7 book 2 loaded`)
    await nextChapter(page, viewport)
    await waitForDocumentTitle(page, '标题契约测试二 - 第一章 起点', `${label} step 7 book 2 next chapter`)
    await switchBookFromShelf(page, viewport, state, books[3], `${label} step 7 same chapter title switch`)
    await waitForDocumentTitle(page, '标题契约测试三 - 第一章 起点', `${label} step 7 book 3 loaded`)
    await assertTitleLogClean(page, `${label} step 7`)

    await clickReaderTool(page, viewport, '首页')
    await page.waitForURL(url => new URL(url).pathname === '/', { timeout: 10_000 })
    const homeTitle = await page.evaluate(() => document.title)
    assert(homeTitle === shellTitle, `${label} step 8 home control: tab title ${JSON.stringify(homeTitle)} should be the shell ${JSON.stringify(shellTitle)}`)

    const bookOneTitles = catalogs[1].map(title => (title.trim() ? `${books[1].title} - ${title.trim()}` : books[1].title))
    const shelfRow = page.locator('.book-row:visible strong.name').filter({ hasText: /^标题契约测试$/ })
    await shelfRow.waitFor({ timeout: 10_000 })
    await shelfRow.click()
    await page.waitForURL(url => new URL(url).pathname === '/books/1/read', { timeout: 10_000 })
    await page.waitForFunction(titles => titles.includes(document.title), bookOneTitles, { timeout: 10_000 })
      .catch(async (error) => {
        throw new Error(`${label} step 9 re-enter reader: tab title ${JSON.stringify(await page.evaluate(() => document.title))} is not a book 1 title (${error.message.split('\n')[0]})`)
      })
    await page.goBack()
    await page.waitForURL(url => new URL(url).pathname === '/', { timeout: 10_000 })
    const backTitle = await page.evaluate(() => document.title)
    assert(backTitle === shellTitle, `${label} step 9 browser back: tab title ${JSON.stringify(backTitle)} should be the shell ${JSON.stringify(shellTitle)}`)
    await assertTitleLogClean(page, `${label} steps 8-9`)
    assert(failures.length === 0, `${label}: ${failures.join('\n')}`)
  } finally {
    await context.close()
  }
}

async function runAuthenticationBlock(browser, viewport) {
  const label = `${viewport.width}x${viewport.height}`
  const { context, failures, page, state } = await openContext(browser, viewport)
  try {
    await page.goto(`${targetUrl}/books/1/read?chapter=0`, { waitUntil: 'networkidle' })
    await waitForDocumentTitle(page, '标题契约测试 - 第一章 起点', `${label} step 10 reader before expiry`)
    state.rejectContent = { bookId: 1, index: 4 }
    await jumpFromToc(page, viewport, 4)
    await page.waitForFunction(() => (
      !document.querySelector('.reader-shell')
      && Boolean(document.querySelector('.reader-auth-blocked'))
      && Boolean(document.querySelector('.auth-dialog'))
    ), null, { timeout: 10_000 })
    const blockedTitle = await page.evaluate(() => document.title)
    assert(!blockedTitle.includes(books[1].title), `${label} step 10: blocked tab title still exposes the book name ${JSON.stringify(blockedTitle)}`)
    assert(blockedTitle === shellTitle, `${label} step 10: blocked tab title ${JSON.stringify(blockedTitle)} should be the shell ${JSON.stringify(shellTitle)}`)

    await page.getByPlaceholder('请输入用户名').fill(state.user.username)
    await page.getByPlaceholder('请输入密码').fill('password')
    await page.locator('.auth-dialog button[type="submit"]').click()
    await page.locator('.auth-dialog').waitFor({ state: 'hidden', timeout: 10_000 })
    await waitForDocumentTitle(page, '标题契约测试 - 第五章', `${label} step 10 same-account relogin`)
    await assertTitleLogClean(page, `${label} step 10`)
    assert(failures.length === 0, `${label}: ${failures.join('\n')}`)
  } finally {
    await context.close()
  }
}

async function runRemoteReader(browser, viewport) {
  const label = `${viewport.width}x${viewport.height}`
  const { context, failures, page, state } = await openContext(browser, viewport)
  try {
    await page.goto(`${targetUrl}/reader/remote/${remoteSessionId}`, { waitUntil: 'networkidle' })
    await waitForDocumentTitle(page, '远程临时书 - 第一章', `${label} step 12 remote reader`)
    await nextChapter(page, viewport)
    await waitForDocumentTitle(page, '远程临时书 - 第二章', `${label} step 12 remote next chapter`)
    await switchBookFromShelf(page, viewport, state, books[2], `${label} step 12 remote to shelf book`)
    await waitForDocumentTitle(page, '标题契约测试二 - 序章 新书', `${label} step 12 shelf book loaded`)
    await assertTitleLogClean(page, `${label} step 12`)
    assert(failures.length === 0, `${label}: ${failures.join('\n')}`)
  } finally {
    await context.close()
  }
}

const flows = {
  navigation: runReaderNavigation,
  auth: runAuthenticationBlock,
  remote: runRemoteReader,
}

function selectedFlows() {
  const names = String(process.env.SMOKE_TITLE_FLOWS || Object.keys(flows).join(','))
    .split(',')
    .map(value => value.trim())
    .filter(Boolean)
  for (const name of names) {
    if (!flows[name]) throw new Error(`Invalid SMOKE_TITLE_FLOWS entry: ${name}`)
  }
  return names
}

async function main() {
  const names = selectedFlows()
  const browser = await openSmokeBrowser()
  try {
    const checked = []
    for (const viewport of smokeViewports()) {
      for (const name of names) await flows[name](browser, viewport)
      checked.push(`${viewport.width}x${viewport.height}`)
    }
    console.log(`document title contract smoke passed: ${checked.join(', ')} flows=${names.join(',')} shell=${JSON.stringify(shellTitle)}`)
  } finally {
    await browser.close()
  }
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})
