#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:4173'

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

function fakeToken(userId = 1) {
  const payload = Buffer.from(JSON.stringify({ userId, sub: String(userId) })).toString('base64url')
  return `open.${payload}.reader`
}

function seededCache() {
  return {
    'localCache@bookSourceList@user:1': [{ id: 1, name: '当前账号书源', enabled: true }],
    'localCache@rssSources@user:1': [{ id: 1, name: '当前账号 RSS', url: 'https://rss.example/a' }],
    'localCache@reader@user:1@chapters:7': [{ id: 1, title: '第一章' }],
    'localCache@reader@user:1@book:7': { id: 7, title: '当前账号书籍' },
    'localCache@user:1@Scoped_Author@scoped-url@chapterContent-0': {
      chapter: { index: 0 },
      content: '当前账号正文',
    },
    'localCache@bookshelf@getBookshelf:fixture:user:1': [{ id: 7, title: '当前账号书籍' }],
    'localCache@bookSourceList@user:2': [{
      id: 2,
      name: `其它账号书源-${'B'.repeat(20000)}`,
      enabled: true,
    }],
    'localCache@rssSources@user:2': [{
      id: 2,
      name: `其它账号 RSS-${'C'.repeat(20000)}`,
      url: 'https://rss.example/b',
    }],
    'localCache@user:2@Other_Author@other-url@chapterContent-0': {
      chapter: { index: 0 },
      content: `其它账号正文-${'D'.repeat(20000)}`,
    },
    'localCache@Legacy_Author@legacy-url@chapterContent-0': {
      chapter: { index: 0 },
      content: `无归属旧正文-${'E'.repeat(20000)}`,
    },
    'localCache@random-bookSourceList-note': {
      note: `名称碰撞-${'F'.repeat(20000)}`,
    },
  }
}

function parseDisplayedBytes(text) {
  const match = String(text || '').match(/本地缓存(?:\s+([\d.]+)\s*(B|KB|MB|GB))?/)
  if (!match?.[1]) return 0
  const unit = { B: 1, KB: 1024, MB: 1024 ** 2, GB: 1024 ** 3 }[match[2]]
  return Number(match[1]) * unit
}

async function installApiMocks(page, delayedStats) {
  let cacheStatsRequest = 0
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()

    if (path === '/cache/stats' && method === 'GET') {
      cacheStatsRequest += 1
      if (cacheStatsRequest === 2) {
        await delayedStats.promise
        return route.fulfill(json({ files: 50, size: 50000 }))
      }
      if (cacheStatsRequest >= 3) {
        delayedStats.newestResolve()
        return route.fulfill(json({ files: 2, size: 200 }))
      }
      return route.fulfill(json({ files: 0, size: 0 }))
    }
    if (path === '/cache' && method === 'DELETE') {
      return route.fulfill(json({ clearedFiles: 0, clearedSize: 0 }))
    }
    if (path === '/me') return route.fulfill(json({ id: 1, username: 'cache-user', role: 'admin' }))
    if (path === '/health') return route.fulfill(json({ version: 'smoke', commit: 'cache-scope' }))
    if (path === '/books') return route.fulfill(json([]))
    if (path === '/categories') return route.fulfill(json([]))
    if (path === '/sources') return route.fulfill(json([]))
    if (path.startsWith('/settings/')) {
      const key = path.slice('/settings/'.length)
      if (method === 'PUT') return route.fulfill(json({ key, value: {}, updatedAt: '2026-07-27T00:00:01Z' }))
      return route.fulfill(json({ key, value: {}, updatedAt: '2026-07-27T00:00:00Z' }))
    }
    return route.fulfill(json({}))
  })
}

async function openSidebar(page, viewport) {
  if (viewport.width > 900) return
  await page.locator('.mobile-menu-trigger').click()
  await page.waitForFunction(() => {
    const node = document.querySelector('.app-sidebar')
    return node && Math.abs(Number.parseFloat(getComputedStyle(node).marginLeft)) < 0.5
  })
}

async function cacheSectionText(page) {
  return page.locator('[data-sidebar-section="cache"] .app-nav-title').innerText()
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 900,
    hasTouch: viewport.width <= 900,
  })
  const page = await context.newPage()
  const failures = []
  const delayedStats = {}
  delayedStats.promise = new Promise(resolve => {
    delayedStats.resolve = resolve
  })
  delayedStats.newestPromise = new Promise(resolve => {
    delayedStats.newestResolve = resolve
  })
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() !== 'error') return
    const value = message.text()
    if (/WebSocket connection to .*\/ws\/sync/.test(value)) return
    failures.push(`console.error: ${value}`)
  })

  await page.addInitScript(({ token, entries }) => {
    localStorage.setItem('openreader_token', token)
    Object.entries(entries).forEach(([key, value]) => {
      localStorage.setItem(key, JSON.stringify(value))
    })
  }, { token: fakeToken(1), entries: seededCache() })
  await installApiMocks(page, delayedStats)

  await page.goto(targetUrl, { waitUntil: 'domcontentloaded' })
  await page.waitForSelector('[data-sidebar-section="cache"]', { timeout: 10000 })
  await openSidebar(page, viewport)
  await page.waitForFunction(() => (
    [...document.querySelectorAll('[data-sidebar-section="cache"] .app-nav-item')]
      .some(node => node.textContent.includes('清空书源缓存'))
  ))

  const initialTitle = await cacheSectionText(page)
  const initialBytes = parseDisplayedBytes(initialTitle)
  assert(
    initialBytes > 0 && initialBytes < 10 * 1024,
    `${viewport.width}: total must exclude other-user/legacy/unknown cache, got ${initialTitle}`,
  )
  const sourceAction = page.locator('[data-sidebar-section="cache"] .app-nav-item')
    .filter({ hasText: '清空书源缓存' })
  const sourceLabel = await sourceAction.innerText()
  assert(
    parseDisplayedBytes(sourceLabel.replace('清空书源缓存', '本地缓存')) < 1024,
    `${viewport.width}: source group must exclude user:2, got ${sourceLabel}`,
  )

  const refresh = page.locator('[data-sidebar-section="cache"] .app-nav-item')
    .filter({ hasText: '刷新缓存统计' })
  await refresh.click()
  await refresh.click()
  await delayedStats.newestPromise
  await page.waitForTimeout(50)
  const newestTitle = await cacheSectionText(page)
  delayedStats.resolve()
  await page.waitForTimeout(100)
  assert(
    await cacheSectionText(page) === newestTitle,
    `${viewport.width}: delayed old stats overwrote the newest generation`,
  )

  await sourceAction.click()
  const confirm = page.locator('.el-message-box')
  await confirm.waitFor({ state: 'visible' })
  await confirm.getByRole('button', { name: '确定', exact: true }).click()
  await page.waitForFunction(() => (
    [...document.querySelectorAll('[data-sidebar-section="cache"] .app-nav-item')]
      .some(node => node.textContent.trim() === '清空书源缓存')
  ))

  const remaining = await page.evaluate(() => ({
    current: localStorage.getItem('localCache@bookSourceList@user:1'),
    other: localStorage.getItem('localCache@bookSourceList@user:2'),
    legacy: localStorage.getItem('localCache@Legacy_Author@legacy-url@chapterContent-0'),
    collision: localStorage.getItem('localCache@random-bookSourceList-note'),
    scrollWidth: document.documentElement.scrollWidth,
    innerWidth: window.innerWidth,
  }))
  assert(remaining.current === null, `${viewport.width}: current source cache was not removed`)
  assert(remaining.other !== null, `${viewport.width}: another user's source cache was removed`)
  assert(remaining.legacy !== null, `${viewport.width}: unowned legacy chapter cache was removed`)
  assert(remaining.collision !== null, `${viewport.width}: substring-collision key was removed`)
  assert(
    remaining.scrollWidth <= remaining.innerWidth + 1,
    `${viewport.width}: cache sidebar overflowed horizontally`,
  )
  assert(failures.length === 0, failures.join('\n'))

  await context.close()
  return `${viewport.width}x${viewport.height}`
}

async function run() {
  const browser = await openSmokeBrowser()
  try {
    const results = []
    for (const viewport of [
      { width: 1440, height: 900 },
      { width: 390, height: 844 },
      { width: 360, height: 800 },
    ]) {
      results.push(await runViewport(browser, viewport))
    }
    console.log(
      `index-cache-scope: ok ${results.join(', ')} current-only=true stale-generation=true scoped-clear=true`,
    )
  } finally {
    await browser.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
