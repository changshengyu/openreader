#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = (process.env.TARGET_URL || 'http://127.0.0.1:4173').replace(/\/$/, '')

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

async function installApiMocks(page) {
  const requests = new Map()
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname.replace(/^\/api/, '')
    const method = request.method()
    const key = `${method} ${path}`
    requests.set(key, Number(requests.get(key) || 0) + 1)

    if (path === '/me') {
      return route.fulfill(json({
        id: 1,
        username: 'index-action-smoke',
        role: 'admin',
        canAccessStore: true,
        canAccessWebdav: true,
      }))
    }
    if (path === '/health') return route.fulfill(json({ version: 'smoke', commit: 'index-action-surface' }))
    if (path === '/books') {
      if (requests.get(key) > 1) await new Promise(resolve => setTimeout(resolve, 80))
      return route.fulfill(json([{
        id: 1,
        title: 'Index 操作面契约',
        author: 'OpenReader',
        chapterCount: 1,
        lastCheckTime: '2026-08-02T00:00:00Z',
      }]))
    }
    if (path === '/categories' || path === '/book-groups' || path === '/sources') {
      return route.fulfill(json([]))
    }
    if (path === '/cache/stats') return route.fulfill(json({ files: 0, size: 0 }))
    if (['/settings/reader', '/settings/shelf', '/settings/search'].includes(path) && method === 'GET') {
      const settingKey = path.split('/').at(-1)
      const values = {
        reader: { theme: 'parchment', mode: 'page', pageMode: 'auto' },
        shelf: { view: 'grid', layoutVersion: 2, groupKey: 'builtin:all' },
        search: { searchType: 'all', concurrent: 24 },
      }
      return route.fulfill(json({
        key: settingKey,
        value: values[settingKey],
        updatedAt: '2026-08-02T00:00:00Z',
      }))
    }
    if (['/settings/reader', '/settings/shelf', '/settings/search'].includes(path) && method === 'PUT') {
      const settingKey = path.split('/').at(-1)
      return route.fulfill(json({ key: settingKey, value: request.postDataJSON()?.value || {}, updatedAt: '2026-08-02T00:00:01Z' }))
    }
    return route.fulfill(json({}))
  })
  return {
    count(method, path) {
      return Number(requests.get(`${method} ${path}`) || 0)
    },
    snapshot(paths) {
      return Object.fromEntries(paths.map(path => [path, Number(requests.get(`GET ${path}`) || 0)]))
    },
  }
}

async function openSidebar(page, viewport) {
  if (viewport.width > 900) return
  await page.locator('.mobile-menu-trigger').click()
  await page.waitForFunction(() => {
    const sidebar = document.querySelector('.app-sidebar')
    return sidebar && Math.abs(Number.parseFloat(getComputedStyle(sidebar).marginLeft)) < 0.5
  })
}

async function sidebarSnapshot(page) {
  return page.evaluate(() => {
    const rows = [...document.querySelectorAll('[data-sidebar-section]')]
    return rows.map(section => ({
      key: section.dataset.sidebarSection,
      title: section.querySelector('.app-nav-title span')?.textContent?.trim() || '',
      items: [...section.querySelectorAll('.app-nav-item')].map(item => item.textContent.trim()),
    }))
  })
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({
    viewport,
    isMobile: viewport.width <= 900,
    hasTouch: viewport.width <= 900,
  })
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() !== 'error') return
    const value = message.text()
    if (/WebSocket connection to .*\/ws\/sync/.test(value)) return
    failures.push(`console.error: ${value}`)
  })
  await page.addInitScript(token => localStorage.setItem('openreader_token', token), fakeToken())
  const api = await installApiMocks(page)

  await page.goto(targetUrl, { waitUntil: 'networkidle' })
  await page.waitForSelector('.book-row', { timeout: 10000 })
  await openSidebar(page, viewport)

  const sections = await sidebarSnapshot(page)
  assert(
    JSON.stringify(sections.map(section => section.key)) === JSON.stringify(['backend', 'sources', 'bookshelf', 'account', 'webdav', 'cache']),
    `${viewport.width}: unexpected sidebar sections ${JSON.stringify(sections.map(section => section.key))}`,
  )
  assert(
    JSON.stringify(sections.find(section => section.key === 'sources')?.items) === JSON.stringify(['书源管理', '探索书源', '导入书源', '远程书源', '失效书源', '调试书源']),
    `${viewport.width}: source action order diverged`,
  )
  assert(
    JSON.stringify(sections.find(section => section.key === 'bookshelf')?.items) === JSON.stringify(['书籍管理', '分组管理', '导入书籍', '浏览书仓', '刷新缓存']),
    `${viewport.width}: bookshelf action order diverged`,
  )
  assert(!sections.some(section => section.key === 'other'), `${viewport.width}: empty/duplicate Other section must not exist`)

  const titleActions = (await page.locator('.title-actions > button').allTextContents()).map(value => value.trim())
  assert(
    JSON.stringify(titleActions) === JSON.stringify(['编辑', '刷新', 'RSS', '书海']),
    `${viewport.width}: shelf title actions are ${JSON.stringify(titleActions)}`,
  )
  assert(await page.locator('.title-actions .view-switch').count() === 0, `${viewport.width}: unreviewed view switches remain in the upstream action row`)

  const beforeBackend = api.snapshot(['/health', '/books', '/categories', '/book-groups'])
  await page.locator('[data-sidebar-section="backend"] .app-nav-item').click()
  await page.waitForFunction(() => document.querySelector('.el-message--success'))
  assert(api.count('GET', '/health') > beforeBackend['/health'], `${viewport.width}: backend status did not recheck health`)
  for (const path of ['/books', '/categories', '/book-groups']) {
    assert(api.count('GET', path) === beforeBackend[path], `${viewport.width}: backend status unexpectedly requested ${path}`)
  }

  const refreshPaths = ['/books', '/categories', '/book-groups', '/sources', '/settings/shelf', '/settings/search', '/settings/reader', '/cache/stats']
  const beforeRefresh = api.snapshot(refreshPaths)
  await page.locator('[data-sidebar-section="bookshelf"] .app-nav-item').filter({ hasText: '刷新缓存' }).click()
  await page.waitForFunction(() => (
    [...document.querySelectorAll('[data-sidebar-section="bookshelf"] .app-nav-item')]
      .some(node => node.textContent.trim() === '刷新中...')
  ))
  await page.waitForFunction(() => (
    [...document.querySelectorAll('[data-sidebar-section="bookshelf"] .app-nav-item')]
      .some(node => node.textContent.trim() === '刷新缓存')
  ))
  for (const path of refreshPaths) {
    assert(api.count('GET', path) > beforeRefresh[path], `${viewport.width}: workspace refresh did not request ${path}`)
  }

  const content = await page.locator('body').innerText()
  assert(!content.includes('关注公众号'), `${viewport.width}: old MP promotion is visible`)
  assert(!content.includes('加入TG频道'), `${viewport.width}: old Telegram promotion is visible`)
  const bottomParent = await page.locator('.sidebar-bottom-icons').evaluate(node => node.parentElement?.className || '')
  assert(bottomParent.includes('app-sidebar'), `${viewport.width}: fixed bottom icons moved into the scroll owner`)
  const overflow = await page.evaluate(() => ({ scrollWidth: document.documentElement.scrollWidth, innerWidth }))
  assert(overflow.scrollWidth <= overflow.innerWidth + 1, `${viewport.width}: horizontal overflow ${overflow.scrollWidth} > ${overflow.innerWidth}`)
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
    console.log(`index-settings-action-surface: ok ${results.join(', ')} backendHealthOnly=true workspaceRefresh=true duplicateEntries=false`)
  } finally {
    await browser.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
