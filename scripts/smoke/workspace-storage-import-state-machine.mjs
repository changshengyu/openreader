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

function requestBody(request) {
  try {
    return request.postDataJSON() || {}
  } catch {
    return {}
  }
}

function previewItem(path) {
  const token = path === 'one.txt' ? 'a'.repeat(48) : 'b'.repeat(48)
  return {
    path,
    importToken: token,
    book: {
      title: path.replace(/\.txt$/, ''),
      author: '',
      chapterCount: 1,
      chapters: [{ index: 0, title: '第一章' }],
    },
  }
}

function webdavListing() {
  return `<multistatus><response><propstat><prop><displayname></displayname><iscollection>true</iscollection><getcontentlength>0</getcontentlength><lastmodified></lastmodified></prop></propstat></response>${['one.txt', 'two.txt'].map((name, index) => `<response><propstat><prop><displayname>${name}</displayname><iscollection>false</iscollection><getcontentlength>${128 * (index + 1)}</getcontentlength><lastmodified>Wed, 01 Jan 2025 00:00:00 GMT</lastmodified></prop></propstat></response>`).join('')}</multistatus>`
}

async function installApiMocks(page) {
  const imports = []
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/webdav\/.*$/, async route => {
    assert(route.request().headers().authorization === `Bearer ${fakeToken()}`, 'every WebDAV file-manager request must retain bearer auth')
    if (route.request().method() === 'GET') {
      return route.fulfill({ status: 207, contentType: 'application/xml', body: webdavListing() })
    }
    return route.fulfill({ status: 204, body: '' })
  })
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const path = new URL(request.url()).pathname.replace(/^\/api/, '')
    const method = request.method()

    if (path === '/me') return route.fulfill(json({ id: 1, username: 'storage-state-smoke', role: 'admin' }))
    if (path === '/health') return route.fulfill(json({ version: 'smoke', commit: 'storage-state' }))
    if (path === '/settings/reader' && method === 'GET') return route.fulfill(json({ key: 'reader', value: { theme: 'parchment', mode: 'page', pageMode: 'auto' } }))
    if (path === '/settings/reader' && method === 'PUT') return route.fulfill(json({ key: 'reader', value: {} }))
    if (path === '/settings/preferences') return route.fulfill(json({ key: 'preferences', value: {} }))
    if (path === '/books') return route.fulfill(json([]))
    if (path === '/categories') return route.fulfill(json([{ id: 7, name: '导入组' }]))
    if (path === '/sources') return route.fulfill(json([]))
    if (path === '/cache/stats') return route.fulfill(json({ files: 0, size: 0, cachedChapters: 0 }))
    if (path === '/backup/list') return route.fulfill(json([]))
    if (path === '/local-store' && method === 'GET') {
      return route.fulfill(json({
        path: '',
        items: [
          { name: 'one.txt', path: 'one.txt', extension: '.txt', size: 128, isDir: false, importable: true },
          { name: 'two.txt', path: 'two.txt', extension: '.txt', size: 256, isDir: false, importable: true },
        ],
      }))
    }
    if (path === '/local-store/import-preview') {
      const body = requestBody(request)
      const paths = Array.isArray(body.paths) ? body.paths : body.items?.map(item => item.path)
      assert(Array.isArray(paths) && paths.length > 0, 'preview must receive a local staged path request')
      return route.fulfill(json({ items: paths.map(previewItem) }))
    }
    if (path === '/local-store/import') {
      const body = requestBody(request)
      assert(body.items?.length === 1, 'batch import must preserve upstream per-item import calls')
      const item = body.items[0]
      imports.push({ source: 'local', path: item.path, token: item.importToken, categoryIds: body.categoryIds || [] })
      return route.fulfill(json({ imported: [{ path: item.path, book: { id: 100 + imports.length, title: item.title, chapterCount: 1 } }] }))
    }
    if (path === '/webdav/import-preview') {
      const body = requestBody(request)
      const paths = Array.isArray(body.paths) ? body.paths : body.items?.map(item => item.path)
      assert(Array.isArray(paths) && paths.length > 0, 'preview must receive a WebDAV staged path request')
      return route.fulfill(json({ items: paths.map(previewItem) }))
    }
    if (path === '/webdav/import') {
      const body = requestBody(request)
      assert(body.items?.length === 1, 'WebDAV batch import must preserve upstream per-item import calls')
      const item = body.items[0]
      imports.push({ source: 'webdav', path: item.path, token: item.importToken, categoryIds: body.categoryIds || [] })
      return route.fulfill(json({ imported: [{ path: item.path, book: { id: 100 + imports.length, title: item.title, chapterCount: 1 } }] }))
    }
    return route.fulfill(json({}))
  })
  return imports
}

async function assertNoHorizontalOverflow(page, label) {
  const geometry = await page.evaluate(() => ({ scrollWidth: document.documentElement.scrollWidth, width: innerWidth }))
  assert(geometry.scrollWidth <= geometry.width + 1, `${label}: horizontal overflow ${geometry.scrollWidth} > ${geometry.width}`)
}

const sources = {
  local: {
    route: '/local-store?storageState=1',
    dialog: '.global-local-store-dialog',
    oneButton: '加入书架',
  },
  webdav: {
    route: '/settings?panel=webdav&storageState=1',
    dialog: '.global-webdav-dialog',
    oneButton: '加入书架',
  },
}

for (const source of Object.values(sources)) {
  source.openMulti = async (page, dialog) => {
    const rows = page.locator(`${dialog} .el-table__body-wrapper .el-checkbox`)
    assert(await rows.count() >= 2, 'workspace file manager must expose two selectable imported files')
    await rows.nth(0).click()
    await rows.nth(1).click()
    await page.locator(dialog).getByRole('button', { name: '加入书架 2', exact: true }).click()
  }
}

async function openMulti(page, source, dialog, isMobile) {
  await sources[source].openMulti(page, dialog, isMobile)
  await page.locator('.storage-import-mode-dialog').waitFor()
}

async function runSource(page, viewport, imports, source) {
  const config = sources[source]
  const importedAtStart = imports.length
  await page.goto(`${targetUrl}${config.route}`, { waitUntil: 'networkidle' })
  await page.locator(config.dialog).waitFor({ timeout: 10000 })

  const forbiddenActions = source === 'local'
    ? ['新建目录', '重命名', '下载', '导入当前目录', '导入筛选', '导入目录']
    : ['新建目录', '重命名', '加入目录']
  const actionLabels = await page.locator(`${config.dialog} button`).allTextContents()
  for (const action of forbiddenActions) {
    assert(!actionLabels.some(label => label.trim() === action), `${viewport.width} ${source}: removed operation ${action} must not be reachable`)
  }

  await page.locator(config.dialog).getByRole('button', { name: config.oneButton, exact: true }).first().click()
  await page.locator('.storage-import-single-dialog').waitFor()
  await page.locator('.storage-import-single-dialog').getByRole('button', { name: '确定导入', exact: true }).click()
  await page.locator('.storage-import-single-dialog').waitFor({ state: 'hidden' })
  assert(imports.length === importedAtStart + 1 && imports.at(-1).path === 'one.txt', `${viewport.width} ${source}: one valid item must skip the mode chooser`)
  assert(imports.at(-1).categoryIds.length === 0, `${viewport.width} ${source}: one-item confirmation must start ungrouped`)

  await openMulti(page, source, config.dialog, viewport.width <= 750)
  await page.locator('.storage-import-mode-dialog').getByRole('button', { name: '批量导入', exact: true }).click()
  await page.locator('.storage-import-groups-dialog').waitFor()
  await page.locator('.storage-import-groups-dialog').locator('.el-select').click()
  await page.getByText('导入组', { exact: true }).last().click()
  await page.locator('.storage-import-groups-dialog').getByRole('button', { name: '确定', exact: true }).click()
  await page.locator('.storage-import-groups-dialog').waitFor({ state: 'hidden' })
  assert(imports.length === importedAtStart + 3, `${viewport.width} ${source}: batch confirmation must write both rows`)
  assert(imports.at(-2).path === 'one.txt' && imports.at(-1).path === 'two.txt', `${viewport.width} ${source}: batch writes must preserve preview order`)
  assert(imports.at(-2).categoryIds.join(',') === '7' && imports.at(-1).categoryIds.join(',') === '7', `${viewport.width} ${source}: batch group must apply to every per-item write`)

  await openMulti(page, source, config.dialog, viewport.width <= 750)
  await page.locator('.storage-import-mode-dialog').getByRole('button', { name: '逐一确认导入', exact: true }).click()
  await page.locator('.storage-import-single-dialog').getByText('导入本地书籍（1/2）', { exact: true }).waitFor()
  await page.locator('.storage-import-single-dialog').getByRole('button', { name: '取消', exact: true }).click()
  await page.locator('.storage-import-single-dialog').getByText('导入本地书籍（2/2）', { exact: true }).waitFor()
  await page.locator('.storage-import-single-dialog').getByRole('button', { name: '确定导入', exact: true }).click()
  await page.locator('.storage-import-single-dialog').waitFor({ state: 'hidden' })
  assert(imports.length === importedAtStart + 4 && imports.at(-1).path === 'two.txt', `${viewport.width} ${source}: cancelling sequential item must advance without writing it`)

  await openMulti(page, source, config.dialog, viewport.width <= 750)
  await page.locator('.storage-import-mode-dialog').locator('.el-dialog__headerbtn').click()
  await page.locator('.storage-import-mode-dialog').waitFor({ state: 'hidden' })
  assert(imports.length === importedAtStart + 4, `${viewport.width} ${source}: closing multi-import chooser must cancel every remaining write`)

  await assertNoHorizontalOverflow(page, `${viewport.width} ${source} storage import state machine`)
  if (viewport.width <= 750) {
    await page.locator(config.dialog).getByRole('button', { name: config.oneButton, exact: true }).first().click()
    const geometry = await page.locator('.storage-import-single-dialog').evaluate(node => {
      const rect = node.getBoundingClientRect()
      return { left: rect.left, top: rect.top, width: rect.width, height: rect.height, viewportWidth: innerWidth, viewportHeight: innerHeight }
    })
    assert(Math.abs(geometry.left) <= 1 && Math.abs(geometry.top) <= 1, `${viewport.width} ${source}: storage confirmation must be fullscreen`)
    assert(Math.abs(geometry.width - geometry.viewportWidth) <= 1 && Math.abs(geometry.height - geometry.viewportHeight) <= 1, `${viewport.width} ${source}: storage confirmation must fill mobile viewport`)
    await page.locator('.storage-import-single-dialog').getByRole('button', { name: '取消', exact: true }).click()
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
  const imports = await installApiMocks(page)

  try {
    await runSource(page, viewport, imports, 'local')
    await runSource(page, viewport, imports, 'webdav')
    assert(failures.length === 0, failures.join('\n'))
    return `${viewport.width}x${viewport.height}`
  } finally {
    await context.close()
  }
}

async function run() {
  const browser = await openSmokeBrowser()
  try {
    const checks = []
    checks.push(await runViewport(browser, { width: 1440, height: 900 }))
    checks.push(await runViewport(browser, { width: 390, height: 844 }))
    checks.push(await runViewport(browser, { width: 360, height: 800 }))
    console.log(`workspace-storage-state-machine: ok ${checks.join(', ')} single=true batch=true sequential=true closeCancels=true`)
  } finally {
    await browser.close()
  }
}

run().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
