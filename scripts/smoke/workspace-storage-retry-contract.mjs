#!/usr/bin/env node

const targetUrl = (process.env.TARGET_URL || 'http://127.0.0.1:4173').replace(/\/$/, '')
const defaultChromePath = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
const localToken = 'a'.repeat(48)
const webdavToken = 'b'.repeat(48)

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

function json(data, status = 200) {
  return { status, contentType: 'application/json', body: JSON.stringify(data) }
}

function fakeToken() {
  const payload = Buffer.from(JSON.stringify({ userId: 1, sub: '1' })).toString('base64url')
  return `open.${payload}.reader`
}

function requestBody(request) {
  try {
    return request.postDataJSON() || {}
  } catch {
    return {}
  }
}

function webdavListing(name) {
  return `<multistatus><response><propstat><prop><displayname></displayname><iscollection>true</iscollection><getcontentlength>0</getcontentlength><lastmodified></lastmodified></prop></propstat></response><response><propstat><prop><displayname>${name}</displayname><iscollection>false</iscollection><getcontentlength>128</getcontentlength><lastmodified>Wed, 01 Jan 2025 00:00:00 GMT</lastmodified></prop></propstat></response></multistatus>`
}

async function installApiMocks(page) {
  const retries = { local: 0, webdav: 0 }
  const imports = { local: 0, webdav: 0 }

  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/webdav\/.*$/, async route => {
    const authorization = route.request().headers().authorization || ''
    assert(authorization.startsWith('Bearer '), 'WebDAV request must keep the authenticated client')
    if (route.request().method() === 'GET') {
      return route.fulfill({ status: 207, contentType: 'application/xml', body: webdavListing('retry-rule.txt') })
    }
    return route.fulfill({ status: 204, body: '' })
  })
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname.replace(/^\/api/, '')
    const method = request.method()

    if (path === '/me') return route.fulfill(json({ id: 1, username: 'storage-retry-smoke', role: 'admin' }))
    if (path === '/health') return route.fulfill(json({ version: 'smoke', commit: 'storage-retry' }))
    if (path === '/settings/reader' && method === 'GET') return route.fulfill(json({ key: 'reader', value: { theme: 'parchment', mode: 'page', pageMode: 'auto' } }))
    if (path === '/settings/reader' && method === 'PUT') return route.fulfill(json({ key: 'reader', value: {} }))
    if (path === '/settings/preferences') return route.fulfill(json({ key: 'preferences', value: {} }))
    if (path === '/books') return route.fulfill(json([]))
    if (path === '/categories') return route.fulfill(json([]))
    if (path === '/sources') return route.fulfill(json([]))
    if (path === '/cache/stats') return route.fulfill(json({ files: 0, size: 0, cachedChapters: 0 }))
    if (path === '/local-store' && method === 'GET') {
      return route.fulfill(json({ path: '', items: [{ name: 'retry-rule.txt', path: 'retry-rule.txt', extension: '.txt', size: 128, isDir: false, importable: true }] }))
    }
    if (path === '/local-store/import-preview') {
      const body = requestBody(request)
      if (Array.isArray(body.items)) {
        retries.local += 1
        assert(body.items.length === 1, 'LocalStore reparse must only send the edited row')
        assert(body.items[0].path === 'retry-rule.txt', 'LocalStore reparse path changed')
        assert(body.items[0].importToken === localToken, 'LocalStore reparse must retain its staged token')
        assert(body.items[0].tocRule === '^== .+ ==$', 'LocalStore reparse must submit the edited rule')
        return route.fulfill(json({ items: [{ path: 'retry-rule.txt', importToken: localToken, book: { title: '本地重试', author: '', chapterCount: 1, chapters: [{ index: 0, title: '== 第一章 ==' }] } }] }))
      }
      assert(Array.isArray(body.paths), 'LocalStore initial preview must be path based')
      return route.fulfill(json({ items: [{ path: 'retry-rule.txt', importToken: localToken, error: 'failed to parse local book: no readable chapters' }] }))
    }
    if (path === '/local-store/import') {
      const body = requestBody(request)
      imports.local += 1
      assert(body.items?.[0]?.importToken === localToken, 'LocalStore confirmation must import the staged snapshot')
      return route.fulfill(json({ imported: [{ path: 'retry-rule.txt', book: { id: 901, title: '本地重试', chapterCount: 1 } }] }))
    }
    if (path === '/webdav/import-preview') {
      const body = requestBody(request)
      if (Array.isArray(body.items)) {
        retries.webdav += 1
        assert(body.items.length === 1, 'WebDAV reparse must only send the edited row')
        assert(body.items[0].path === 'retry-rule.txt', 'WebDAV reparse path changed')
        assert(body.items[0].importToken === webdavToken, 'WebDAV reparse must retain its staged token')
        assert(body.items[0].tocRule === '^== .+ ==$', 'WebDAV reparse must submit the edited rule')
        return route.fulfill(json({ items: [{ path: 'retry-rule.txt', importToken: webdavToken, book: { title: 'WebDAV 重试', author: '', chapterCount: 1, chapters: [{ index: 0, title: '== 第一章 ==' }] } }] }))
      }
      assert(Array.isArray(body.paths), 'WebDAV initial preview must be path based')
      return route.fulfill(json({ items: [{ path: 'retry-rule.txt', importToken: webdavToken, error: 'failed to parse local book: no readable chapters' }] }))
    }
    if (path === '/webdav/import') {
      const body = requestBody(request)
      imports.webdav += 1
      assert(body.items?.[0]?.importToken === webdavToken, 'WebDAV confirmation must import the staged snapshot')
      return route.fulfill(json({ imported: [{ path: 'retry-rule.txt', book: { id: 902, title: 'WebDAV 重试', chapterCount: 1 } }] }))
    }
    if (path === '/backup/list') return route.fulfill(json([]))
    return route.fulfill(json({}))
  })

  return { retries, imports }
}

async function assertNoHorizontalOverflow(page, label) {
  const geometry = await page.evaluate(() => ({ scrollWidth: document.documentElement.scrollWidth, width: innerWidth }))
  assert(geometry.scrollWidth <= geometry.width + 1, `${label}: horizontal overflow ${geometry.scrollWidth} > ${geometry.width}`)
}

async function openAndRetry(page, root, viewport, source) {
  const isLocal = source === 'local'
  const path = isLocal ? '/local-store?storageRetry=1' : '/settings?panel=webdav&storageRetry=1'
  const dialog = isLocal ? '.global-local-store-dialog' : '.global-webdav-dialog'
  const startLabel = isLocal ? '导入' : '加入书架'

  await page.goto(`${root}${path}`, { waitUntil: 'networkidle' })
  await page.waitForSelector(dialog, { timeout: 10000 })
  await page.getByRole('button', { name: startLabel, exact: true }).first().click()
  await page.locator('.storage-import-preflight-dialog').getByText('待修复', { exact: true }).waitFor()
  const rule = page.locator('.storage-import-preflight-dialog').getByPlaceholder('TXT 目录规则（可选）')
  await rule.fill('^== .+ ==$')
  await page.locator('.storage-import-preflight-dialog').getByRole('button', { name: '重新解析', exact: true }).click()
  await page.locator('.storage-import-preflight-dialog').getByText('已解析 1 章', { exact: true }).waitFor()
  await page.locator('.storage-import-preflight-dialog').getByRole('button', { name: '继续导入 1 本', exact: true }).click()
  await page.locator('.storage-import-single-dialog').getByRole('button', { name: '确定导入', exact: true }).click()
  await page.locator('.storage-import-single-dialog').waitFor({ state: 'hidden' })
  await assertNoHorizontalOverflow(page, `${viewport.width} ${source} staged retry`)

  if (viewport.width <= 750) {
    const geometry = await page.locator(dialog).evaluate(node => {
      const rect = node.getBoundingClientRect()
      return { left: rect.left, top: rect.top, width: rect.width, height: rect.height, viewportWidth: innerWidth, viewportHeight: innerHeight }
    })
    assert(Math.abs(geometry.left) <= 1 && Math.abs(geometry.top) <= 1, `${viewport.width}: ${source} root dialog must remain fullscreen`)
    assert(Math.abs(geometry.width - geometry.viewportWidth) <= 1 && Math.abs(geometry.height - geometry.viewportHeight) <= 1, `${viewport.width}: ${source} root dialog must fill the viewport`)
  }
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({ viewport, isMobile: viewport.width <= 750, hasTouch: viewport.width <= 750 })
  const page = await context.newPage()
  const failures = []
  page.on('pageerror', error => failures.push(`pageerror: ${error.message}`))
  page.on('console', message => {
    if (message.type() === 'error' && !/WebSocket connection to .*\/ws\/sync/.test(message.text())) failures.push(`console.error: ${message.text()}`)
  })
  await page.addInitScript(token => localStorage.setItem('openreader_token', token), fakeToken())
  const calls = await installApiMocks(page)

  try {
    await openAndRetry(page, targetUrl, viewport, 'local')
    await openAndRetry(page, targetUrl, viewport, 'webdav')
    assert(calls.retries.local === 1 && calls.retries.webdav === 1, `${viewport.width}: both retry operations must make one tokenized preview request`)
    assert(calls.imports.local === 1 && calls.imports.webdav === 1, `${viewport.width}: both confirmation operations must keep the staged token`)
    assert(failures.length === 0, failures.join('\n'))
    return `${viewport.width}x${viewport.height}`
  } finally {
    await context.close()
  }
}

async function run() {
  const { chromium } = await loadPlaywright()
  const browser = await chromium.launch({ headless: true, executablePath: process.env.CHROME_PATH || defaultChromePath })
  try {
    const checks = []
    checks.push(await runViewport(browser, { width: 1440, height: 900 }))
    checks.push(await runViewport(browser, { width: 390, height: 844 }))
    checks.push(await runViewport(browser, { width: 360, height: 800 }))
    console.log(`workspace-storage-retry: ok ${checks.join(', ')} localStore=true webdav=true tokenReparse=true`)
  } finally {
    await browser.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
