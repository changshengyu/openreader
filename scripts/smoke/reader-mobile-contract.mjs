#!/usr/bin/env node

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'
const readerUrl = process.env.SMOKE_READER_URL || `${targetUrl.replace(/\/$/, '')}/books/1/read?chapter=0`
const defaultChromePath = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
const smokeBgImage = 'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%2236%22 height=%2236%22%3E%3Crect width=%2236%22 height=%2236%22 fill=%22%23d8c49a%22/%3E%3C/svg%3E'

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
      console.error('Playwright is required for reader mobile contract smoke.')
      console.error(`Original import error: ${error.message}`)
      process.exit(2)
    }
  }
}

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function assertClose(actual, expected, tolerance, message) {
  if (Math.abs(actual - expected) > tolerance) {
    throw new Error(`${message}: expected ${expected}±${tolerance}, got ${actual}`)
  }
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

function savedReaderBook() {
  return {
    id: 1,
    title: '移动阅读契约测试',
    author: 'OpenReader',
    sourceId: 2,
    sourceName: '测试书源',
    url: 'https://source.example/book/mobile-reader-contract',
    bookUrl: 'https://source.example/book/mobile-reader-contract',
    chapterCount: 2,
    categoryIds: [],
    progress: null,
  }
}

async function installApiMocks(page) {
  const bookmarks = [{
    id: 101,
    chapterId: 11,
    chapterIndex: 0,
    offset: 0,
    percent: 0,
    title: '第一章',
    excerpt: '用于验证根级书签表单的摘录。',
    note: '原笔记',
  }]
  let nextBookmarkId = 102
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
          mode: 'scroll',
          pageMode: 'normal',
          theme: 'parchment',
          themeType: 'day',
          customBgImage: smokeBgImage,
          customBgImageList: [smokeBgImage],
          fontSize: 18,
          lineHeight: 1.8,
          paragraphSpace: 0.2,
          columnWidth: 800,
        },
      }))
    }
    if (path === '/settings/reader' && method === 'PUT') {
      return route.fulfill(json({
        key: 'reader',
        updatedAt: '2026-07-06T00:00:01Z',
        value: {},
      }))
    }
    if (path === '/books/1') {
      return route.fulfill(json(savedReaderBook()))
    }
    if (path === '/books') return route.fulfill(json([savedReaderBook()]))
    if (path === '/books/1/chapters') {
      return route.fulfill(json([
        { id: 11, index: 0, title: '第一章' },
        { id: 12, index: 1, title: '第二章' },
      ]))
    }
    if (path === '/books/1/chapters/0/content') {
      return route.fulfill(json({
        chapter: { id: 11, index: 0, title: '第一章' },
        content: [
          '春风过处，纸页微明。',
          '这一段用于验证移动端阅读正文左右留白对称，并保持两端对齐。',
          '点击中央区域应当只在没有面板时切换工具层。',
        ].join('\n'),
      }))
    }
    if (path === '/books/1/bookmarks' && method === 'GET') {
      return route.fulfill(json(bookmarks))
    }
    if (path === '/books/1/bookmarks' && method === 'POST') {
      const bookmark = { id: nextBookmarkId++, ...request.postDataJSON() }
      bookmarks.push(bookmark)
      return route.fulfill(json(bookmark, 201))
    }
    if (path === '/books/1/bookmarks/batch' && method === 'POST') {
      const created = request.postDataJSON().map(payload => ({ id: nextBookmarkId++, ...payload }))
      bookmarks.push(...created)
      return route.fulfill(json(created, 201))
    }
    if (path === '/books/1/bookmarks/batch-delete' && method === 'POST') {
      const ids = new Set(request.postDataJSON().ids)
      const deletedIds = bookmarks.filter(bookmark => ids.has(bookmark.id)).map(bookmark => bookmark.id)
      for (let index = bookmarks.length - 1; index >= 0; index -= 1) {
        if (ids.has(bookmarks[index].id)) bookmarks.splice(index, 1)
      }
      return route.fulfill(json({ deletedIds }))
    }
    if (path === '/bookmarks/101' && method === 'PUT') {
      const payload = request.postDataJSON()
      Object.assign(bookmarks[0], payload)
      return route.fulfill(json(bookmarks[0]))
    }
    if (path === '/progress/1') {
      return route.fulfill(json({}))
    }
    if (path === '/sources') {
      return route.fulfill(json([{ id: 2, name: '测试书源', enabled: true }]))
    }
    if (path === '/categories') {
      return route.fulfill(json([]))
    }
    return route.fulfill(json({}))
  })
}

async function assertWorkspaceOpen(page, viewport, label, { primary = false } = {}) {
  await page.waitForSelector('.reader-mobile-workspace', { timeout: 10000 })
  const topCount = await page.locator('.reader-mobile-top.visible').count()
  assert(topCount === 1, `${viewport.width}: toolbar should remain visible after opening ${label}`)
  const workspaceState = await page.evaluate((expectedLabel) => {
    const workspaces = Array.from(document.querySelectorAll('.reader-mobile-workspace'))
    const workspace = workspaces.at(-1)
    const rect = workspace.getBoundingClientRect()
    const header = workspace.querySelector('.reader-mobile-workspace-head')
    const visibleDrawers = Array.from(document.querySelectorAll('.el-drawer')).filter((element) => {
      const drawerRect = element.getBoundingClientRect()
      const style = window.getComputedStyle(element)
      return drawerRect.width > 0 && drawerRect.height > 0 && style.display !== 'none' && style.visibility !== 'hidden'
    }).length
    return {
      count: workspaces.length,
      width: Math.round(rect.width),
      left: Math.round(rect.left),
      visibleDrawers,
      role: workspace.getAttribute('role'),
      text: workspace.innerText,
      hasLabel: workspace.innerText.includes(expectedLabel),
      hasGenericHeader: Boolean(header),
      paddingTop: window.getComputedStyle(workspace).paddingTop,
      paddingBottom: window.getComputedStyle(workspace).paddingBottom,
      hasPrimaryBody: Boolean(workspace.querySelector('.reader-mobile-primary-popover-body')),
    }
  }, label)
  assert(workspaceState.count === 1, `${viewport.width}: exactly one mobile primary workspace should remain after opening ${label}`)
  assert(workspaceState.left === 0, `${viewport.width}: mobile workspace left ${workspaceState.left}`)
  assert(workspaceState.width === viewport.width, `${viewport.width}: mobile workspace width ${workspaceState.width}`)
  assert(workspaceState.visibleDrawers === 0, `${viewport.width}: mobile workspace must not use visible drawer`)
  assert(workspaceState.role === 'dialog', `${viewport.width}: mobile workspace role ${workspaceState.role}`)
  assert(workspaceState.hasLabel, `${viewport.width}: mobile workspace missing label ${label}`)
  if (primary) {
    assert(workspaceState.hasGenericHeader === false, `${viewport.width}: ${label} primary popover must not render generic workspace header`)
    assert(workspaceState.paddingTop === '0px', `${viewport.width}: ${label} primary root top padding ${workspaceState.paddingTop}`)
    assert(workspaceState.paddingBottom === '0px', `${viewport.width}: ${label} primary root bottom padding ${workspaceState.paddingBottom}`)
    assert(workspaceState.hasPrimaryBody, `${viewport.width}: ${label} primary popover missing owned content body`)
  }
}

function mobileTopTool(page, label) {
  return page.locator('.reader-mobile-top.visible .mobile-tool-button').filter({ hasText: label })
}

async function assertMobileTopToolContract(page, viewport) {
  const state = await page.evaluate(() => [...document.querySelectorAll('.reader-mobile-top.visible .mobile-tool-button')].map(button => ({
    label: button.innerText.trim(),
    disabled: button.disabled,
  })))
  assert(
    JSON.stringify(state.map(item => item.label)) === JSON.stringify(['书架', '书源', '目录', '设置', '首页']),
    `${viewport.width}: mobile Reader top-tool order must match reader-dev`,
  )
  assert(state.find(item => item.label === '书源')?.disabled === false, `${viewport.width}: Reader source entry must remain available`)
}

async function assertWorkspaceClosed(page, viewport, label) {
  await page.waitForFunction(() => !document.querySelector('.reader-mobile-workspace'), null, { timeout: 10000 })
  assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: toolbar should remain visible after closing ${label}`)
}

async function assertGlobalReaderDialog(page, viewport, selector, label) {
  await page.waitForSelector(selector, { timeout: 10000 })
  const state = await page.evaluate((target) => {
    const dialog = document.querySelector(target)
    const rect = dialog?.getBoundingClientRect()
    return {
      topTools: document.querySelectorAll('.reader-mobile-top.visible').length,
      dialogWidth: Math.round(rect?.width || 0),
      dialogHeight: Math.round(rect?.height || 0),
      workspaceCount: document.querySelectorAll('.reader-mobile-workspace').length,
      drawerCount: Array.from(document.querySelectorAll('.el-drawer')).filter((element) => {
        const rect = element.getBoundingClientRect()
        const style = window.getComputedStyle(element)
        return rect.width > 0 && rect.height > 0 && style.display !== 'none' && style.visibility !== 'hidden'
      }).length,
    }
  }, selector)
  assert(state.topTools === 1, `${viewport.width}: toolbar state must remain visible after opening ${label}`)
  assert(state.workspaceCount === 0, `${viewport.width}: ${label} must not create a reader workspace`)
  assert(state.drawerCount === 0, `${viewport.width}: ${label} must not use a drawer`)
  assert(state.dialogWidth === viewport.width, `${viewport.width}: ${label} dialog width ${state.dialogWidth}`)
  assert(state.dialogHeight === viewport.height, `${viewport.width}: ${label} dialog height ${state.dialogHeight}`)
  await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
  assert(
    await page.locator('.reader-mobile-top.visible').count() === 1,
    `${viewport.width}: ${label} dialog click must not pass through and toggle reader chrome`,
  )
}

async function closeGlobalReaderDialog(page, selector) {
  const dialog = page.locator(selector)
  await dialog.getByRole('button', { name: '取消', exact: true }).click()
  await dialog.waitFor({ state: 'hidden', timeout: 10000 })
}

async function assertSelectedTextReplaceRuleEditor(page, viewport, { fullscreen }) {
  const paragraph = page.locator('.reader-body p').first()
  const selectedText = (await paragraph.textContent())?.trim() || ''
  assert(selectedText, `${viewport.width}: reader fixture must include selectable text`)
  await paragraph.evaluate((node) => {
    const selection = window.getSelection()
    const range = document.createRange()
    range.selectNodeContents(node)
    selection?.removeAllRanges()
    selection?.addRange(range)
    node.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, button: 0 }))
  })

  const chooser = page.locator('.el-message-box').last()
  await chooser.getByRole('button', { name: '添加过滤规则', exact: true }).click()

  const editor = page.locator('.el-dialog').filter({ hasText: '新增替换规则' }).last()
  await editor.waitFor({ state: 'visible', timeout: 10000 })
  assert(await page.locator('.global-replace-dialog').count() === 0, `${viewport.width}: selected text must open only the direct editor, not the rule manager`)

  const pattern = editor.locator('.el-form-item').filter({ hasText: '匹配正则或文本' }).locator('input')
  const scope = editor.locator('.el-form-item').filter({ hasText: '替换范围' }).locator('input')
  assert(await pattern.inputValue() === selectedText, `${viewport.width}: direct editor must retain the exact selected text`)
  assert((await scope.inputValue()).startsWith('移动阅读契约测试;'), `${viewport.width}: direct editor must retain the active book scope`)

  const geometry = await editor.evaluate((node) => {
    const rect = node.getBoundingClientRect()
    return { width: Math.round(rect.width), height: Math.round(rect.height) }
  })
  if (fullscreen) {
    assert(geometry.width === viewport.width && geometry.height === viewport.height, `${viewport.width}: selected-text editor must be fullscreen on mobile`)
    assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: selected-text editor must preserve the reader toolbar`)
    await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
    assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: selected-text editor click must not pass through to reader chrome`)
  } else {
    assert(geometry.width >= 500 && geometry.width <= 540, `desktop: selected-text editor width ${geometry.width}`)
    assert(await page.locator('.reader-left-rail').count() === 1, 'desktop: selected-text editor must preserve reader rails')
  }

  await editor.getByRole('button', { name: '取消', exact: true }).click()
  await editor.waitFor({ state: 'hidden', timeout: 10000 })
  assert(await page.locator('.global-replace-dialog').count() === 0, `${viewport.width}: closing the direct editor must not reveal a manager`)
}

async function assertReaderBookInfoDialog(page, viewport, { fullscreen }) {
  const selector = '.book-info-dialog'
  await page.waitForSelector(selector, { timeout: 10000 })
  const state = await page.evaluate((target) => {
    const dialog = document.querySelector(target)
    const rect = dialog?.getBoundingClientRect()
    const text = dialog?.innerText || ''
    const visibleDrawers = Array.from(document.querySelectorAll('.el-drawer')).filter((element) => {
      const drawerRect = element.getBoundingClientRect()
      const style = window.getComputedStyle(element)
      return drawerRect.width > 0 && drawerRect.height > 0 && style.display !== 'none' && style.visibility !== 'hidden'
    }).length
    return {
      topTools: document.querySelectorAll('.reader-mobile-top.visible').length,
      leftRailVisible: Boolean(document.querySelector('.reader-left-rail')),
      width: Math.round(rect?.width || 0),
      height: Math.round(rect?.height || 0),
      workspaceCount: document.querySelectorAll('.reader-mobile-workspace').length,
      visibleDrawers,
      text,
    }
  }, selector)
  assert(state.workspaceCount === 0, `${viewport.width}: BookInfo must remain a root dialog, not a reader workspace`)
  assert(state.visibleDrawers === 0, `${viewport.width}: BookInfo must not use a drawer`)
  assert(state.text.includes('书籍信息'), `${viewport.width}: BookInfo dialog title missing`)
  assert(state.text.includes('移动阅读契约测试'), `${viewport.width}: BookInfo dialog missing active book title`)
  assert(state.text.includes('OpenReader'), `${viewport.width}: BookInfo dialog missing active book author`)
  const dialog = page.locator(selector)
  assert(await dialog.getByText('加入书架', { exact: true }).count() === 0, `${viewport.width}: URL-matched shelf Reader BookInfo must not expose add`)
  assert(await dialog.getByText('加入并阅读', { exact: true }).count() === 0, `${viewport.width}: shelf Reader BookInfo must not expose add-and-read`)
  assert(await dialog.getByText('开始阅读', { exact: true }).count() === 0, `${viewport.width}: shelf Reader BookInfo must not expose a second read action`)
  if (fullscreen) {
    assert(state.topTools === 1, `${viewport.width}: BookInfo must preserve mobile reader chrome`)
    assert(state.width === viewport.width, `${viewport.width}: BookInfo fullscreen width ${state.width}`)
    assert(state.height === viewport.height, `${viewport.width}: BookInfo fullscreen height ${state.height}`)
  } else {
    assert(state.leftRailVisible, 'desktop: BookInfo must preserve the reader left rail')
    assert(state.width >= 460 && state.width <= 520, `desktop: BookInfo dialog width ${state.width}`)
  }
  await page.locator(`${selector} .book-info-main`).click()
  if (fullscreen) {
    assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: BookInfo click must not pass through and toggle reader chrome`)
  }
}

async function closeReaderBookInfoDialog(page) {
  const readerURL = await page.url()
  const dialog = page.locator('.book-info-dialog')
  await dialog.locator('.el-dialog__headerbtn').click()
  await dialog.waitFor({ state: 'hidden', timeout: 10000 })
  assert(await page.url() === readerURL, 'closing Reader BookInfo must not change the reader route')
}

async function assertInlineMobileCacheZone(page, viewport) {
  await page.waitForSelector('.reader-mobile-bottom.visible .mobile-cache-zone.reader-cache-zone', { timeout: 10000 })
  const state = await page.evaluate(() => {
    const zone = document.querySelector('.reader-mobile-bottom.visible .mobile-cache-zone.reader-cache-zone')
    const bar = document.querySelector('.reader-mobile-bottom.visible')
    const zoneRect = zone?.getBoundingClientRect()
    const barRect = bar?.getBoundingClientRect()
    return {
      topTools: document.querySelectorAll('.reader-mobile-top.visible').length,
      workspaceCount: document.querySelectorAll('.reader-mobile-workspace').length,
      drawerCount: Array.from(document.querySelectorAll('.el-drawer')).filter((element) => {
        const rect = element.getBoundingClientRect()
        const style = window.getComputedStyle(element)
        return rect.width > 0 && rect.height > 0 && style.display !== 'none' && style.visibility !== 'hidden'
      }).length,
      zoneLeft: Math.round(zoneRect?.left || 0),
      zoneRight: Math.round(zoneRect?.right || 0),
      barLeft: Math.round(barRect?.left || 0),
      barRight: Math.round(barRect?.right || 0),
      text: zone?.innerText || '',
    }
  })
  assert(state.topTools === 1, `${viewport.width}: toolbar state must remain visible with cache zone open`)
  assert(state.workspaceCount === 0, `${viewport.width}: cache must not create a workspace`)
  assert(state.drawerCount === 0, `${viewport.width}: cache must not create a drawer`)
  assert(state.zoneLeft >= state.barLeft && state.zoneRight <= state.barRight, `${viewport.width}: cache zone must remain inside the read bar`)
  assert(state.text.includes('缓存章节') && state.text.includes('后面50章'), `${viewport.width}: inline cache controls missing`)
}

async function assertDesktopReaderDialog(page, selector, label) {
  await page.waitForSelector(selector, { timeout: 10000 })
  const state = await page.evaluate((target) => {
    const dialog = document.querySelector(target)
    const rect = dialog?.getBoundingClientRect()
    const visibleDrawers = Array.from(document.querySelectorAll('.el-drawer')).filter((element) => {
      const drawerRect = element.getBoundingClientRect()
      const style = window.getComputedStyle(element)
      return drawerRect.width > 0 && drawerRect.height > 0 && style.display !== 'none' && style.visibility !== 'hidden'
    }).length
    return {
      width: Math.round(rect?.width || 0),
      height: Math.round(rect?.height || 0),
      visibleDrawers,
      settingsOpen: document.querySelectorAll('.reader-desktop-workspace .settings-body').length,
    }
  }, selector)
  assert(state.width >= 760, `desktop: ${label} dialog width ${state.width}`)
  assert(state.height > 200, `desktop: ${label} dialog height ${state.height}`)
  assert(state.visibleDrawers === 0, `desktop: ${label} must not use a drawer`)
  assert(state.settingsOpen === 1, `desktop: ${label} must not close the active settings workspace`)
}

async function assertBookmarkFormContext(page, viewport, { fullscreen, excerpt = '用于验证根级书签表单的摘录。' }) {
  const selector = '.global-bookmark-form-dialog'
  await page.waitForSelector(selector, { timeout: 10000 })
  const state = await page.evaluate((target) => {
    const dialog = document.querySelector(target)
    const rect = dialog?.getBoundingClientRect()
    const readonlyValues = Array.from(dialog?.querySelectorAll('input[readonly], textarea[readonly]') || [])
      .map(element => element.value)
    return {
      width: Math.round(rect?.width || 0),
      height: Math.round(rect?.height || 0),
      readonlyValues,
      visibleDrawers: Array.from(document.querySelectorAll('.el-drawer')).filter((element) => {
        const drawerRect = element.getBoundingClientRect()
        const style = window.getComputedStyle(element)
        return drawerRect.width > 0 && drawerRect.height > 0 && style.display !== 'none' && style.visibility !== 'hidden'
      }).length,
    }
  }, selector)
  assert(state.visibleDrawers === 0, `${viewport.width}: bookmark form must not use a drawer`)
  assert(state.readonlyValues.includes('移动阅读契约测试'), `${viewport.width}: bookmark form missing readonly book title`)
  assert(state.readonlyValues.includes('OpenReader'), `${viewport.width}: bookmark form missing readonly author`)
  assert(state.readonlyValues.includes('第一章'), `${viewport.width}: bookmark form missing readonly chapter`)
  assert(state.readonlyValues.some(value => value.includes(excerpt)), `${viewport.width}: bookmark form missing readonly excerpt`)
  if (fullscreen) {
    assert(state.width === viewport.width, `${viewport.width}: bookmark form fullscreen width ${state.width}`)
    assert(state.height === viewport.height, `${viewport.width}: bookmark form fullscreen height ${state.height}`)
  } else {
    assert(state.width >= 600, `desktop: bookmark form width ${state.width}`)
  }
}

async function editBookmarkWithGlobalForm(page, viewport, { fullscreen }) {
  await page.locator('.global-bookmark-dialog .el-table__body-wrapper tbody tr').first()
    .getByRole('button', { name: '编辑', exact: true }).click()
  await assertBookmarkFormContext(page, viewport, { fullscreen })
  await page.locator('.global-bookmark-form-dialog textarea').last().fill('已通过根级表单更新')
  await page.locator('.global-bookmark-form-dialog').getByRole('button', { name: '确定', exact: true }).click()
  await page.locator('.global-bookmark-form-dialog').waitFor({ state: 'hidden', timeout: 10000 })
}

async function createBookmarkFromSelectedText(page, viewport, { fullscreen }) {
  const paragraph = page.locator('.reader-body p').first()
  const selectedText = (await paragraph.textContent())?.trim() || ''
  assert(selectedText, `${viewport.width}: reader fixture must include bookmark-selectable text`)
  await paragraph.evaluate((node) => {
    const selection = window.getSelection()
    const range = document.createRange()
    range.selectNodeContents(node)
    selection?.removeAllRanges()
    selection?.addRange(range)
    node.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, button: 0 }))
  })

  const chooser = page.locator('.el-message-box').last()
  await chooser.getByRole('button', { name: '添加书签', exact: true }).click()
  await assertBookmarkFormContext(page, viewport, { fullscreen, excerpt: selectedText })
  const form = page.locator('.global-bookmark-form-dialog')
  await form.locator('textarea').last().fill('选中文字创建')
  await form.getByRole('button', { name: '确定', exact: true }).click()
  await form.waitFor({ state: 'hidden', timeout: 10000 })
  return selectedText
}

async function exerciseBookmarkManager(page, viewport, { fullscreen, selectedText }) {
  const dialog = page.locator('.global-bookmark-dialog')
  await editBookmarkWithGlobalForm(page, viewport, { fullscreen })

  await dialog.locator('.bookmark-file-input').setInputFiles({
    name: 'bookmarks.json',
    mimeType: 'application/json',
    buffer: Buffer.from(JSON.stringify([
      { chapterIndex: 0, offset: 3, percent: 0.1, title: '第一章', excerpt: '导入正文三', note: '导入三' },
      { chapterIndex: 0, offset: 4, percent: 0.2, title: '第一章', excerpt: '导入正文四', note: '导入四' },
    ])),
  })
  await page.locator('.el-message-box').last().getByRole('button', { name: '确定', exact: true }).click()
  await dialog.getByText('导入四', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })

  const rowTexts = await dialog.locator('.el-table__body-wrapper tbody tr').evaluateAll(rows => rows.map(row => row.innerText))
  const orderedNotes = ['已通过根级表单更新', '选中文字创建', '导入三', '导入四']
  let previous = -1
  for (const note of orderedNotes) {
    const index = rowTexts.findIndex((text, rowIndex) => rowIndex > previous && text.includes(note))
    assert(index > previous, `${viewport.width}: bookmark manager must append ${note} in creation order before a sync reload`)
    previous = index
  }

  const importedRow = dialog.locator('.el-table__body-wrapper tbody tr').filter({ hasText: '导入三' }).first()
  await importedRow.locator('.el-checkbox').click()
  await dialog.getByRole('button', { name: '批量删除', exact: true }).click()
  await page.locator('.el-message-box').last().getByRole('button', { name: '确定', exact: true }).click()
  await dialog.getByText('导入三', { exact: true }).waitFor({ state: 'hidden', timeout: 10000 })

  const selectedBookmarkRow = dialog.locator('.el-table__body-wrapper tbody tr').filter({ hasText: '选中文字创建' }).first()
  await selectedBookmarkRow.getByRole('button', { name: '跳转', exact: true }).click()
  await dialog.waitFor({ state: 'hidden', timeout: 10000 })
  const query = await page.evaluate(() => Object.fromEntries(new URLSearchParams(location.search)))
  assert(
    query.chapter === '0' && Number.isFinite(Number(query.offset)) && Number(query.offset) >= 0 && Number.isFinite(Number(query.percent)),
    `${viewport.width}: bookmark jump must preserve a valid saved chapter and position, got ${JSON.stringify(query)}`,
  )
  assert(String(query.bookmark || '').includes(selectedText), `${viewport.width}: bookmark jump must retain paragraph context for stale-offset recovery`)
}

async function assertInlineDesktopCacheZone(page) {
  await page.waitForSelector('.reader-page-control .desktop-cache-zone.reader-cache-zone', { timeout: 10000 })
  const state = await page.evaluate(() => {
    const zone = document.querySelector('.reader-page-control .desktop-cache-zone.reader-cache-zone')
    const progress = document.querySelector('.reader-page-control .progress-box')
    const zoneRect = zone?.getBoundingClientRect()
    const progressRect = progress?.getBoundingClientRect()
    return {
      zoneRight: Math.round(zoneRect?.right || 0),
      progressLeft: Math.round(progressRect?.left || 0),
      text: zone?.innerText || '',
      visibleDrawers: Array.from(document.querySelectorAll('.el-drawer')).filter((element) => {
        const rect = element.getBoundingClientRect()
        const style = window.getComputedStyle(element)
        return rect.width > 0 && rect.height > 0 && style.display !== 'none' && style.visibility !== 'hidden'
      }).length,
    }
  })
  assert(state.zoneRight <= state.progressLeft, `desktop: cache zone must sit inside the read bar left of progress control`)
  assert(state.text.includes('缓存章节') && state.text.includes('后面100章'), 'desktop: inline cache controls missing')
  assert(state.visibleDrawers === 0, 'desktop: cache must not use a drawer')
}

async function assertSettingsRowGeometry(page, viewport) {
  const geometry = await page.evaluate(() => {
    const firstRow = document.querySelector('.settings-body .setting-row')
    const label = firstRow?.querySelector('.setting-label')
    const control = firstRow ? Array.from(firstRow.children).find(element => !element.classList.contains('setting-label')) : null
    const activeTheme = document.querySelector('.theme-item.active')
    const firstSelectionButton = firstRow?.querySelector('.selection-button')
    const firstFontOption = document.querySelector('.font-family-option')
    const labelRect = label?.getBoundingClientRect()
    const controlRect = control?.getBoundingClientRect()
    const activeThemeRect = activeTheme?.getBoundingClientRect()
    const selectionButtonRect = firstSelectionButton?.getBoundingClientRect()
    const fontOptionRect = firstFontOption?.getBoundingClientRect()
    const labelStyle = label ? window.getComputedStyle(label) : null
    const activeThemeStyle = activeTheme ? window.getComputedStyle(activeTheme) : null
    return {
      labelLeft: labelRect?.left ?? null,
      labelTop: labelRect?.top ?? null,
      controlLeft: controlRect?.left ?? null,
      controlTop: controlRect?.top ?? null,
      labelLineHeight: labelStyle?.lineHeight ?? '',
      activeThemeBorderColor: activeThemeStyle?.borderTopColor ?? '',
      activeThemeWidth: activeThemeRect?.width ?? null,
      activeThemeHeight: activeThemeRect?.height ?? null,
      firstSelectionButtonHeight: selectionButtonRect?.height ?? null,
      firstFontOptionWidth: fontOptionRect?.width ?? null,
      firstFontOptionHeight: fontOptionRect?.height ?? null,
    }
  })
  assert(geometry.labelLeft !== null && geometry.controlLeft !== null, `${viewport.width}: missing settings first row geometry`)
  assertClose(geometry.controlLeft - geometry.labelLeft, 72, 1, `${viewport.width}: settings control column offset`)
  assertClose(geometry.controlTop, geometry.labelTop, 2, `${viewport.width}: settings label and control should share a row`)
  assert(geometry.labelLineHeight === '36px', `${viewport.width}: settings label line-height ${geometry.labelLineHeight}`)
  assert(geometry.activeThemeBorderColor === 'rgb(237, 66, 89)', `${viewport.width}: active theme border ${geometry.activeThemeBorderColor}`)
  assertClose(geometry.activeThemeWidth, 34, 1, `${viewport.width}: settings theme item width`)
  assertClose(geometry.activeThemeHeight, 34, 1, `${viewport.width}: settings theme item height`)
  assertClose(geometry.firstSelectionButtonHeight, 34, 1, `${viewport.width}: settings selection button height`)
  assertClose(geometry.firstFontOptionWidth, 78, 1, `${viewport.width}: settings font option width`)
  assertClose(geometry.firstFontOptionHeight, 34, 1, `${viewport.width}: settings font option height`)
}

async function assertSettingsBackgroundGeometry(page, viewport) {
  await page.locator('.theme-custom-button').click()
  await page.waitForSelector('.content-bg-preview', { timeout: 10000 })
  const themeModeButtons = page.locator('.custom-theme-mode .selection-button')
  assert(await themeModeButtons.count() === 2, `${viewport.width}: custom theme must expose day/night mode buttons`)
  assert(await themeModeButtons.filter({ hasText: '白天' }).getAttribute('class').then(value => value?.includes('active')), `${viewport.width}: custom theme should start in day mode`)
  await themeModeButtons.filter({ hasText: '黑夜' }).click()
  await page.waitForFunction(() => document.documentElement.classList.contains('dark-reader'))
  assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: custom night mode must not hide toolbar`)
  assert(await themeModeButtons.filter({ hasText: '黑夜' }).getAttribute('class').then(value => value?.includes('active')), `${viewport.width}: custom night mode should become active`)
  const geometry = await page.evaluate(() => {
    const preview = document.querySelector('.content-bg-preview')
    const upload = document.querySelector('.upload-bg-btn')
    const deleteIcon = document.querySelector('.delete-bg-icon')
    const previewRect = preview?.getBoundingClientRect()
    const uploadRect = upload?.getBoundingClientRect()
    const uploadStyle = upload ? window.getComputedStyle(upload) : null
    const deleteStyle = deleteIcon ? window.getComputedStyle(deleteIcon) : null
    return {
      previewWidth: previewRect?.width ?? null,
      previewHeight: previewRect?.height ?? null,
      uploadLeft: uploadRect?.left ?? null,
      uploadTop: uploadRect?.top ?? null,
      previewRight: previewRect?.right ?? null,
      previewTop: previewRect?.top ?? null,
      previewBottom: previewRect?.bottom ?? null,
      uploadColor: uploadStyle?.color ?? '',
      deleteTop: deleteStyle?.top ?? '',
      deleteRight: deleteStyle?.right ?? '',
      deleteColor: deleteStyle?.color ?? '',
      hasCardOverlay: Boolean(document.querySelector('.bg-image-option, .bg-image-delete')),
    }
  })
  assertClose(geometry.previewWidth, 36, 1, `${viewport.width}: settings background preview width`)
  assertClose(geometry.previewHeight, 36, 1, `${viewport.width}: settings background preview height`)
  assert(geometry.uploadLeft !== null && geometry.previewRight !== null, `${viewport.width}: missing settings background upload geometry`)
  assert(geometry.uploadLeft >= geometry.previewRight, `${viewport.width}: settings background upload should follow preview inline`)
  assert(geometry.uploadTop >= geometry.previewTop - 1 && geometry.uploadTop <= geometry.previewBottom + 1, `${viewport.width}: settings background upload should stay on preview row`)
  assert(geometry.uploadColor === 'rgb(237, 66, 89)', `${viewport.width}: settings background upload color ${geometry.uploadColor}`)
  assert(geometry.deleteTop === '-6px', `${viewport.width}: settings background delete top ${geometry.deleteTop}`)
  assert(geometry.deleteRight === '-6px', `${viewport.width}: settings background delete right ${geometry.deleteRight}`)
  assert(geometry.deleteColor === 'rgb(237, 66, 89)', `${viewport.width}: settings background delete color ${geometry.deleteColor}`)
  assert(geometry.hasCardOverlay === false, `${viewport.width}: settings background should not keep card overlay classes`)
}

async function readerGeometry(page) {
  return page.evaluate(() => {
    const viewportWidth = window.innerWidth
    const pageEl = document.querySelector('.reader-page')
    const body = document.querySelector('.reader-body')
    const firstParagraph = document.querySelector('.reader-body p')
    const pageRect = pageEl.getBoundingClientRect()
    const bodyRect = body.getBoundingClientRect()
    const paragraphRect = firstParagraph.getBoundingClientRect()
    const pageStyle = window.getComputedStyle(pageEl)
    const bodyStyle = window.getComputedStyle(body)
    const paragraphStyle = window.getComputedStyle(firstParagraph)
    return {
      viewportWidth,
      pageLeft: pageRect.left,
      pageRight: viewportWidth - pageRect.right,
      bodyLeft: bodyRect.left,
      bodyRight: viewportWidth - bodyRect.right,
      paragraphLeft: paragraphRect.left,
      paragraphRight: viewportWidth - paragraphRect.right,
      pagePaddingLeft: pageStyle.paddingLeft,
      pagePaddingRight: pageStyle.paddingRight,
      pageTextAlign: pageStyle.textAlign,
      bodyTextAlign: bodyStyle.textAlign,
      paragraphTextAlign: paragraphStyle.textAlign,
    }
  })
}

function assertReaderGeometry(geometry, viewport, label) {
  assertClose(geometry.pageLeft, 0, 1, `${viewport.width} ${label}: reader page left`)
  assertClose(geometry.pageRight, 0, 1, `${viewport.width} ${label}: reader page right`)
  assertClose(geometry.bodyLeft, 16, 1, `${viewport.width} ${label}: reader body left gap`)
  assertClose(geometry.bodyRight, 16, 1, `${viewport.width} ${label}: reader body right gap`)
  assertClose(geometry.paragraphLeft, 16, 1, `${viewport.width} ${label}: paragraph left gap`)
  assertClose(geometry.paragraphRight, 16, 1, `${viewport.width} ${label}: paragraph right gap`)
  assert(geometry.pagePaddingLeft === '16px', `${viewport.width} ${label}: left padding ${geometry.pagePaddingLeft}`)
  assert(geometry.pagePaddingRight === '16px', `${viewport.width} ${label}: right padding ${geometry.pagePaddingRight}`)
  assert(geometry.pageTextAlign === 'justify', `${viewport.width} ${label}: page text-align ${geometry.pageTextAlign}`)
  assert(geometry.bodyTextAlign === 'justify', `${viewport.width} ${label}: body text-align ${geometry.bodyTextAlign}`)
  assert(geometry.paragraphTextAlign === 'justify', `${viewport.width} ${label}: paragraph text-align ${geometry.paragraphTextAlign}`)
}

async function closeWorkspace(page, method = 'close-button') {
  if (method === 'settings-toggle') {
    await page.locator('.reader-mobile-top.visible .mobile-tool-button').filter({ hasText: '设置' }).click()
  } else {
    await page.getByRole('button', { name: '关闭' }).click()
  }
  await page.waitForFunction(() => !document.querySelector('.reader-mobile-workspace'), null, { timeout: 10000 })
}

async function runDesktopViewport(browser) {
  const viewport = { width: 1440, height: 900 }
  const context = await browser.newContext({ viewport })
  await context.addInitScript((token) => {
    window.localStorage.setItem('openreader_token', token)
  }, fakeToken())
  const page = await context.newPage()
  const failures = []
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    const text = message.text()
    if (text.includes('/ws/sync') && text.includes('WebSocket connection')) return
    failures.push(text)
  })
  page.on('pageerror', error => failures.push(error.message))
  await installApiMocks(page)
  await page.goto(readerUrl, { waitUntil: 'networkidle' })
  await page.waitForSelector('.reader-body p', { timeout: 10000 })
  await assertSelectedTextReplaceRuleEditor(page, viewport, { fullscreen: false })
  const selectedBookmarkText = await createBookmarkFromSelectedText(page, viewport, { fullscreen: false })
  await page.locator('.reader-left-rail button[title="设置"]').click()
  await page.waitForSelector('.reader-desktop-workspace .settings-body', { timeout: 10000 })
  await page.locator('.theme-custom-button').click()

  const themeModeButtons = page.locator('.custom-theme-mode .selection-button')
  assert(await themeModeButtons.count() === 2, 'desktop: custom theme must expose day/night mode buttons')
  assert(await themeModeButtons.filter({ hasText: '白天' }).getAttribute('class').then(value => value?.includes('active')), 'desktop: custom theme should start in day mode')
  await themeModeButtons.filter({ hasText: '黑夜' }).click()
  await page.waitForFunction(() => document.documentElement.classList.contains('dark-reader'))
  assert(await page.locator('.reader-desktop-workspace .settings-body').count() === 1, 'desktop: switching custom night mode must keep settings open')
  assert(await page.locator('.reader-right-rail button[title="日间模式"]').count() === 1, 'desktop: semantic night mode must update the rail toggle')
  await page.locator('.reader-right-rail button[title="书签"]').click()
  await assertDesktopReaderDialog(page, '.global-bookmark-dialog', '书签')
  await exerciseBookmarkManager(page, viewport, { fullscreen: false, selectedText: selectedBookmarkText })
  await page.locator('.reader-right-rail button[title="搜索正文"]').click()
  await assertDesktopReaderDialog(page, '.global-content-search-dialog', '搜索正文')
  await closeGlobalReaderDialog(page, '.global-content-search-dialog')
  await page.locator('.reader-right-rail button[title="书籍信息"]').click()
  await assertReaderBookInfoDialog(page, viewport, { fullscreen: false })
  await closeReaderBookInfoDialog(page)
  await page.locator('.reader-page-control .progress-box').click()
  await assertInlineDesktopCacheZone(page)
  await page.locator('.reader-page-control .progress-box').click()
  await page.waitForFunction(() => !document.querySelector('.reader-page-control .desktop-cache-zone.reader-cache-zone'), null, { timeout: 10000 })
  assert(failures.length === 0, failures.join('\n'))
  await context.close()
}

async function runViewport(browser, viewport) {
  const context = await browser.newContext({ viewport })
  await context.addInitScript((token) => {
    window.localStorage.setItem('openreader_token', token)
  }, fakeToken())
  const page = await context.newPage()
  const failures = []
  page.on('console', (message) => {
    if (message.type() !== 'error') return
    const text = message.text()
    if (text.includes('/ws/sync') && text.includes('WebSocket connection')) return
    failures.push(text)
  })
  page.on('pageerror', error => failures.push(error.message))
  await installApiMocks(page)
  await page.goto(readerUrl, { waitUntil: 'networkidle' })
  try {
    await page.waitForSelector('.reader-mobile-top.visible', { timeout: 10000 })
  } catch (error) {
    const state = await page.evaluate(() => ({
      href: window.location.href,
      bodyText: document.body.innerText.slice(0, 500),
      hasReaderShell: Boolean(document.querySelector('.reader-shell')),
      mobileTopClass: document.querySelector('.reader-mobile-top')?.className || '',
      appHtml: document.querySelector('#app')?.innerHTML.slice(0, 500) || '',
    }))
    throw new Error(`${error.message}\nState: ${JSON.stringify(state, null, 2)}\nFailures: ${failures.join('\n')}`)
  }
  await page.waitForSelector('.reader-body p', { timeout: 10000 })
  await assertSelectedTextReplaceRuleEditor(page, viewport, { fullscreen: true })
  const selectedBookmarkText = await createBookmarkFromSelectedText(page, viewport, { fullscreen: true })

  const initialTopVisible = await page.locator('.reader-mobile-top.visible').count()
  assert(initialTopVisible === 1, `${viewport.width}: mobile toolbar should be visible by default`)
  await assertMobileTopToolContract(page, viewport)
  const initialGeometry = await readerGeometry(page)
  assertReaderGeometry(initialGeometry, viewport, 'initial')

  await mobileTopTool(page, '书架').click()
  await assertWorkspaceOpen(page, viewport, '书架', { primary: true })
  await mobileTopTool(page, '书架').click()
  await assertWorkspaceClosed(page, viewport, '书架')

  await mobileTopTool(page, '书架').click()
  await assertWorkspaceOpen(page, viewport, '书架', { primary: true })
  await mobileTopTool(page, '书源').click()
  await assertWorkspaceOpen(page, viewport, '来源', { primary: true })
  await mobileTopTool(page, '目录').click()
  await assertWorkspaceOpen(page, viewport, '目录', { primary: true })
  await mobileTopTool(page, '设置').click()
  await assertWorkspaceOpen(page, viewport, '设置', { primary: true })
  await assertSettingsRowGeometry(page, viewport)
  await assertSettingsBackgroundGeometry(page, viewport)

  await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
  const afterPanelCenterTap = await page.locator('.reader-mobile-top.visible').count()
  assert(afterPanelCenterTap === 1, `${viewport.width}: center tap with panel open must not hide toolbar`)

  await closeWorkspace(page, 'settings-toggle')
  await page.locator('.reader-mobile-float-left.visible button[title="书签"]').click()
  await assertGlobalReaderDialog(page, viewport, '.global-bookmark-dialog', '书签')
  await exerciseBookmarkManager(page, viewport, { fullscreen: true, selectedText: selectedBookmarkText })
  await page.locator('.reader-mobile-float-left.visible button[title="搜索正文"]').click()
  await assertGlobalReaderDialog(page, viewport, '.global-content-search-dialog', '搜索正文')
  await closeGlobalReaderDialog(page, '.global-content-search-dialog')
  await page.locator('.reader-mobile-float-left.visible button[title="书籍信息"]').click()
  await assertReaderBookInfoDialog(page, viewport, { fullscreen: true })
  await closeReaderBookInfoDialog(page)
  await page.locator('.reader-mobile-bottom.visible button[title="缓存章节"]').click()
  await assertInlineMobileCacheZone(page, viewport)
  await page.locator('.reader-mobile-bottom.visible button[title="缓存章节"]').click()
  await page.waitForFunction(() => !document.querySelector('.reader-mobile-bottom.visible .mobile-cache-zone.reader-cache-zone'), null, { timeout: 10000 })

  await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
  await page.waitForTimeout(120)
  const afterCenterTap = await page.locator('.reader-mobile-top.visible').count()
  assert(afterCenterTap === 0, `${viewport.width}: center tap without panel should hide toolbar`)
  const hiddenChromeGeometry = await readerGeometry(page)
  assertReaderGeometry(hiddenChromeGeometry, viewport, 'chrome hidden')
  assertClose(hiddenChromeGeometry.paragraphLeft, initialGeometry.paragraphLeft, 1, `${viewport.width}: toolbar hide should not shift paragraph left`)
  assertClose(hiddenChromeGeometry.paragraphRight, initialGeometry.paragraphRight, 1, `${viewport.width}: toolbar hide should not shift paragraph right`)

  assert(failures.length === 0, failures.join('\n'))
  await context.close()
}

async function main() {
  const { chromium } = await loadPlaywright()
  const browser = await chromium.launch({
    headless: true,
    executablePath: process.env.CHROME_PATH || defaultChromePath,
  })
  try {
    await runDesktopViewport(browser)
    await runViewport(browser, { width: 390, height: 844 })
    await runViewport(browser, { width: 360, height: 800 })
    console.log('reader desktop/mobile contract smoke passed')
  } finally {
    await browser.close()
  }
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})
