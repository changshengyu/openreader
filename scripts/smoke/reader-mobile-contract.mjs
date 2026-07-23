#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'
const readerUrl = process.env.SMOKE_READER_URL || `${targetUrl.replace(/\/$/, '')}/books/1/read?chapter=0`
const smokeBgImage = 'data:image/svg+xml,%3Csvg xmlns=%22http://www.w3.org/2000/svg%22 width=%2236%22 height=%2236%22%3E%3Crect width=%2236%22 height=%2236%22 fill=%22%23d8c49a%22/%3E%3C/svg%3E'

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

async function installApiMocks(page, readerSettings = {}) {
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
          pageMode: 'auto',
          autoTheme: false,
          theme: 'parchment',
          themeType: 'day',
          customBgImage: smokeBgImage,
          customBgImageList: [smokeBgImage],
          fontSize: 18,
          lineHeight: 1.8,
          paragraphSpace: 0.2,
          columnWidth: 800,
          ...readerSettings,
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
        ].concat(Array.from({ length: 48 }, (_, index) => (
          `移动工具层滚动契约段落 ${index + 1}：正文需要足够长，才能验证顶部和底部按钮确实移动阅读容器。`
        ))).join('\n'),
      }))
    }
    if (path === '/books/1/search' && method === 'GET') {
      const keyword = url.searchParams.get('q') || ''
      return route.fulfill(json({
        list: [
          {
            chapterId: 11,
            chapterIndex: 0,
            chapterTitle: '第一章',
            excerpt: `移动工具层滚动${keyword} 1`,
            query: keyword,
            resultCountWithinChapter: 0,
            lineIndex: 3,
            offset: 80,
            percent: 0.08,
          },
          {
            chapterId: 11,
            chapterIndex: 0,
            chapterTitle: '第一章',
            excerpt: `移动工具层滚动${keyword} 5`,
            query: keyword,
            resultCountWithinChapter: 4,
            lineIndex: 7,
            offset: 240,
            percent: 0.24,
          },
        ],
        lastIndex: 0,
        hasMore: false,
        total: 2,
        incomplete: true,
        unavailableChapters: 1,
        truncated: false,
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

async function assertWorkspaceOpen(page, viewport, label, { primary = false, contentSized = false, heightRange = null } = {}) {
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
      top: Math.round(rect.top),
      height: Math.round(rect.height),
      zIndex: Number(window.getComputedStyle(workspace).zIndex || 0),
      visibleDrawers,
      role: workspace.getAttribute('role'),
      text: workspace.innerText,
      hasLabel: workspace.innerText.includes(expectedLabel),
      hasGenericHeader: Boolean(header),
      paddingTop: window.getComputedStyle(workspace).paddingTop,
      paddingBottom: window.getComputedStyle(workspace).paddingBottom,
      hasPrimaryBody: Boolean(workspace.querySelector('.reader-mobile-primary-popover-body')),
      toolbarZIndex: Number(window.getComputedStyle(document.querySelector('.reader-mobile-top.visible')).zIndex || 0),
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
    assert(parseFloat(workspaceState.paddingTop) >= 58, `${viewport.width}: ${label} primary root must reserve the mobile tool strip, padding ${workspaceState.paddingTop}`)
    assert(workspaceState.paddingBottom === '0px', `${viewport.width}: ${label} primary root bottom padding ${workspaceState.paddingBottom}`)
    assert(workspaceState.hasPrimaryBody, `${viewport.width}: ${label} primary popover missing owned content body`)
    assert(workspaceState.zIndex > 8, `${viewport.width}: ${label} primary popover must paint above reader content`)
    assert(workspaceState.toolbarZIndex > workspaceState.zIndex, `${viewport.width}: ${label} mobile toolbar must stay interactive above the primary popover`)
  }
  if (contentSized) {
    assert(workspaceState.top === 0, `${viewport.width}: ${label} primary popover top ${workspaceState.top}`)
    assert(workspaceState.height >= 300, `${viewport.width}: ${label} primary popover height ${workspaceState.height}`)
    assert(workspaceState.height < viewport.height - 40, `${viewport.width}: ${label} must be a content-sized popover, not a fullscreen drawer (${workspaceState.height})`)
  }
  if (heightRange) {
    const [min, max] = heightRange
    assert(
      workspaceState.height >= min && workspaceState.height <= max,
      `${viewport.width}: ${label} primary popover height ${workspaceState.height}, expected ${min}-${max}`,
    )
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
    JSON.stringify(state.map(item => item.label)) === JSON.stringify(['首页', '书架', '书源', '目录', '设置']),
    `${viewport.width}: mobile Reader top-tool order must match reader-dev`,
  )
  assert(state.find(item => item.label === '书源')?.disabled === false, `${viewport.width}: Reader source entry must remain available`)
}

async function assertMobileFloatNavigationContract(page, viewport, initialGeometry) {
  const state = await page.evaluate(() => {
    const buttons = [...document.querySelectorAll('.reader-mobile-float-left.visible button')]
    return {
      titles: buttons.map(button => button.title),
      rects: buttons.map(button => {
        const rect = button.getBoundingClientRect()
        return { top: rect.top, bottom: rect.bottom, left: rect.left, right: rect.right }
      }),
    }
  })
  assert(
    JSON.stringify(state.titles) === JSON.stringify(['书签', '搜索正文', '书籍信息', '顶部', '底部']),
    `${viewport.width}: mobile left float controls ${JSON.stringify(state.titles)}`,
  )
  state.rects.forEach((rect, index) => {
    assert(rect.top >= 0 && rect.bottom <= viewport.height, `${viewport.width}: float control ${index} must stay inside viewport`)
    if (index > 0) {
      assert(rect.top >= state.rects[index - 1].bottom, `${viewport.width}: float controls ${index - 1}/${index} must not overlap`)
    }
  })

  const scrollState = await page.evaluate(() => {
    const element = document.querySelector('.reader-shell.document-scroll')
      ? (document.scrollingElement || document.documentElement)
      : document.querySelector('.reader-content')
    return {
      top: element?.scrollTop || 0,
      max: (element?.scrollHeight || 0) - (window.visualViewport?.height || element?.clientHeight || 0),
    }
  })
  assert(scrollState.max > 100, `${viewport.width}: fixture must be scrollable for top/bottom controls`)

  await page.locator('.reader-mobile-float-left.visible button[title="底部"]').click()
  await page.waitForFunction(() => {
    const element = document.querySelector('.reader-shell.document-scroll')
      ? (document.scrollingElement || document.documentElement)
      : document.querySelector('.reader-content')
    const height = window.visualViewport?.height || element?.clientHeight || 0
    return element && element.scrollTop >= element.scrollHeight - height - 2
  })
  assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: bottom action must keep toolbar visible`)

  await page.locator('.reader-mobile-float-left.visible button[title="顶部"]').click()
  await page.waitForFunction(() => {
    const element = document.querySelector('.reader-shell.document-scroll')
      ? (document.scrollingElement || document.documentElement)
      : document.querySelector('.reader-content')
    return element?.scrollTop === 0
  })
  assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: top action must keep toolbar visible`)
  const geometry = await readerGeometry(page)
  assertClose(geometry.paragraphLeft, initialGeometry.paragraphLeft, 1, `${viewport.width}: top/bottom actions should not shift paragraph left`)
  assertClose(geometry.paragraphRight, initialGeometry.paragraphRight, 1, `${viewport.width}: top/bottom actions should not shift paragraph right`)
}

async function assertMobilePageProgressContract(page, viewport, initialGeometry) {
  await page.waitForFunction(() => {
    const input = document.querySelector('.reader-mobile-bottom.visible .mobile-progress-slider')
    return input && Number(input.max) > 1 && /^第 \d+\/\d+ 页$/.test(input.parentElement?.innerText || '')
  })
  const initial = await page.evaluate(() => {
    const input = document.querySelector('.reader-mobile-bottom.visible .mobile-progress-slider')
    const content = document.querySelector('.reader-shell.document-scroll')
      ? (document.scrollingElement || document.documentElement)
      : document.querySelector('.reader-content')
    const progressButton = document.querySelector('.reader-mobile-bottom.visible .mobile-chapter-progress')
    return {
      min: Number(input.min),
      max: Number(input.max),
      value: Number(input.value),
      label: input.parentElement?.querySelector('span')?.textContent?.trim() || '',
      scrollTop: content?.scrollTop || 0,
      progressText: progressButton?.textContent?.trim() || '',
    }
  })
  assert(initial.min === 1, `${viewport.width}: mobile page slider min ${initial.min}`)
  assert(initial.max > 1, `${viewport.width}: mobile page slider must expose rendered pages`)
  assert(initial.value === 1, `${viewport.width}: initial mobile page must be 1, got ${initial.value}`)
  assert(initial.label === `第 1/${initial.max} 页`, `${viewport.width}: mobile page label ${initial.label}`)
  assert(/^阅读进度: \d+%$/.test(initial.progressText), `${viewport.width}: bottom progress text ${initial.progressText}`)
  assert(!initial.progressText.includes('第一章'), `${viewport.width}: bottom progress must not duplicate the chapter title`)

  const routeBefore = await page.url()
  await page.locator('.reader-mobile-bottom.visible .mobile-progress-slider').evaluate((input) => {
    input.value = input.max
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
  await page.waitForFunction((max) => (
    document.querySelector('.reader-mobile-bottom.visible .mobile-progress-slider-row span')?.textContent?.trim()
      === `第 ${max}/${max} 页`
  ), initial.max)
  assert(await page.evaluate(() => {
    const element = document.querySelector('.reader-shell.document-scroll')
      ? (document.scrollingElement || document.documentElement)
      : document.querySelector('.reader-content')
    return element?.scrollTop || 0
  }) === initial.scrollTop, `${viewport.width}: page slider input must not move content before change`)
  assert(await page.url() === routeBefore, `${viewport.width}: page slider input must not change the Reader route`)

  await page.locator('.reader-mobile-bottom.visible .mobile-progress-slider').evaluate((input) => {
    input.dispatchEvent(new Event('change', { bubbles: true }))
  })
  await page.waitForFunction(() => {
    const element = document.querySelector('.reader-shell.document-scroll')
      ? (document.scrollingElement || document.documentElement)
      : document.querySelector('.reader-content')
    const height = window.visualViewport?.height || element?.clientHeight || 0
    return element && element.scrollTop >= element.scrollHeight - height - 2
  })
  assert(await page.url() === routeBefore, `${viewport.width}: page slider change must stay in the current chapter route`)
  assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: page slider change must keep toolbar visible`)

  await page.locator('.reader-mobile-bottom.visible .mobile-progress-slider').evaluate((input) => {
    input.value = '1'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    input.dispatchEvent(new Event('change', { bubbles: true }))
  })
  await page.waitForFunction(() => {
    const element = document.querySelector('.reader-shell.document-scroll')
      ? (document.scrollingElement || document.documentElement)
      : document.querySelector('.reader-content')
    return element?.scrollTop === 0
  })
  const geometry = await readerGeometry(page)
  assertClose(geometry.paragraphLeft, initialGeometry.paragraphLeft, 1, `${viewport.width}: page slider should not shift paragraph left`)
  assertClose(geometry.paragraphRight, initialGeometry.paragraphRight, 1, `${viewport.width}: page slider should not shift paragraph right`)
}

async function assertWorkspaceClosed(page, viewport, label) {
  await page.waitForFunction(() => !document.querySelector('.reader-mobile-workspace'), null, { timeout: 10000 })
  assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: toolbar should remain visible after closing ${label}`)
}

async function assertPrimaryPopoverKeepsChromeInteractive(page, viewport, label) {
  const toolLabel = label === '来源' ? '书源' : label
  const activeTool = mobileTopTool(page, toolLabel)
  const state = await page.evaluate(() => {
    const active = document.querySelector('.reader-mobile-top.visible .mobile-tool-button')
    const rect = active?.getBoundingClientRect()
    const hit = rect && document.elementFromPoint(rect.left + rect.width / 2, rect.top + rect.height / 2)
    return {
      hitIsTool: hit === active || active?.contains(hit),
      dismissVisible: Boolean(document.querySelector('.reader-mobile-primary-dismiss')),
    }
  })
  assert(state.dismissVisible, `${viewport.width}: ${label} must install a click-away layer`)
  assert(state.hitIsTool === true, `${viewport.width}: ${label} toolbar tool must remain above the primary popover`)
  await activeTool.click()
  await assertWorkspaceClosed(page, viewport, label)
  await activeTool.click()
  await assertWorkspaceOpen(page, viewport, label, { primary: true })
}

async function closePrimaryWorkspace(page, viewport, label) {
  const bounds = await page.locator('.reader-mobile-workspace').evaluate((element) => {
    const rect = element.getBoundingClientRect()
    return { bottom: rect.bottom }
  })
  const y = Math.min(viewport.height - 130, Math.ceil(bounds.bottom + 24))
  await page.mouse.click(Math.round(viewport.width / 2), y)
  await assertWorkspaceClosed(page, viewport, label)
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

async function assertSelectedTextReplaceRuleEditor(page, viewport, { fullscreen, touch = false }) {
  const paragraph = page.locator('.reader-body p').first()
  const selectedText = (await paragraph.textContent())?.trim() || ''
  assert(selectedText, `${viewport.width}: reader fixture must include selectable text`)
  if (touch) {
    await paragraph.evaluate((node) => {
      const rect = node.getBoundingClientRect()
      const event = new Event('touchstart', { bubbles: true, cancelable: true })
      Object.defineProperty(event, 'touches', {
        value: [{ clientX: rect.left + 12, clientY: rect.top + 12, identifier: 1 }],
      })
      node.dispatchEvent(event)
    })
    await page.waitForTimeout(720)
    await paragraph.evaluate((node) => {
      const rect = node.getBoundingClientRect()
      const selection = window.getSelection()
      const range = document.createRange()
      range.selectNodeContents(node)
      selection?.removeAllRanges()
      selection?.addRange(range)
      const event = new Event('touchend', { bubbles: true, cancelable: true })
      Object.defineProperty(event, 'touches', { value: [] })
      Object.defineProperty(event, 'changedTouches', {
        value: [{ clientX: rect.left + 12, clientY: rect.top + 12, identifier: 1 }],
      })
      node.dispatchEvent(event)
    })
  } else {
    await paragraph.evaluate((node) => {
      const selection = window.getSelection()
      const range = document.createRange()
      range.selectNodeContents(node)
      selection?.removeAllRanges()
      selection?.addRange(range)
      node.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, button: 0 }))
    })
  }

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
    const chromeState = await page.evaluate(() => ({
      topClass: document.querySelector('.reader-mobile-top')?.className || '',
      shellClass: document.querySelector('.reader-shell')?.className || '',
    }))
    assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: selected-text editor must preserve the reader toolbar (${JSON.stringify(chromeState)})`)
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

async function assertDirectNumericSettingEdit(page, viewport) {
  const row = page.locator('.settings-body .setting-row').filter({ hasText: '亮度' }).first()
  const valueButton = row.locator('.reader-setting-stepper-value')
  await valueButton.click()
  const input = row.locator('.reader-setting-stepper-input')
  await input.fill('87')
  await input.press('Enter')
  assert((await valueButton.textContent())?.trim() === '87', `${viewport.width}: brightness center value must accept direct numeric input`)
  const brightness = await page.locator('.reader-shell').evaluate(node => (
    node.style.getPropertyValue('--reader-brightness')
  ))
  assert(brightness === '87%', `${viewport.width}: direct brightness input must update the reader, got ${brightness}`)
  const renderLayer = await page.locator('.reader-page').evaluate(node => ({
    dimOpacity: node.style.getPropertyValue('--reader-dim-opacity'),
    filter: getComputedStyle(node).filter,
    overlayBackground: getComputedStyle(node, '::after').backgroundColor,
    overlayPointerEvents: getComputedStyle(node, '::after').pointerEvents,
  }))
  assert(Math.abs(Number(renderLayer.dimOpacity) - 0.13) < 0.001, `${viewport.width}: brightness overlay alpha mismatch ${JSON.stringify(renderLayer)}`)
  assert(renderLayer.filter === 'none', `${viewport.width}: brightness must not filter the scrolling reader ${JSON.stringify(renderLayer)}`)
  assert(renderLayer.overlayBackground === 'rgba(0, 0, 0, 0.13)', `${viewport.width}: brightness overlay color mismatch ${JSON.stringify(renderLayer)}`)
  assert(renderLayer.overlayPointerEvents === 'none', `${viewport.width}: brightness overlay must not intercept input ${JSON.stringify(renderLayer)}`)
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

async function assertDesktopPrimaryPopover(page, label, [minHeight, maxHeight]) {
  await page.waitForSelector('.reader-desktop-workspace', { timeout: 10000 })
  const state = await page.evaluate(() => {
    const workspace = document.querySelector('.reader-desktop-workspace')
    const rail = document.querySelector('.reader-left-rail')
    const pageEl = document.querySelector('.reader-page')
    const rect = workspace?.getBoundingClientRect()
    const railRect = rail?.getBoundingClientRect()
    const pageRect = pageEl?.getBoundingClientRect()
    return {
      count: document.querySelectorAll('.reader-desktop-workspace').length,
      left: Math.round(rect?.left || 0),
      top: Math.round(rect?.top || 0),
      width: Math.round(rect?.width || 0),
      height: Math.round(rect?.height || 0),
      panel: ['shelf', 'source', 'toc', 'settings'].find(panel => workspace?.classList.contains(`workspace-panel-${panel}`)) || '',
      railRight: Math.round(railRect?.right || 0),
      pageLeft: Math.round(pageRect?.left || 0),
      pageRight: Math.round(pageRect?.right || 0),
      zIndex: Number(window.getComputedStyle(workspace).zIndex || 0),
    }
  })
  const expectedPanel = { 书架: 'shelf', 书源: 'source', 目录: 'toc', 设置: 'settings' }[label]
  assert(state.count === 1, `desktop: ${label} must have exactly one primary popover`)
  assert(state.panel === expectedPanel, `desktop: ${label} must expose ${expectedPanel}, received ${state.panel}`)
  assert(state.top === 0, `desktop: ${label} popover top ${state.top}`)
  assert(state.left >= state.railRight + 10, `desktop: ${label} must begin after the left rail (${state.left}/${state.railRight})`)
  assert(Math.abs(state.left - (state.pageLeft + 5)) <= 1, `desktop: ${label} left ${state.left}, expected reader frame + 5 (${state.pageLeft})`)
  assert(Math.abs((state.left + state.width) - (state.pageRight - 6)) <= 1, `desktop: ${label} right ${state.left + state.width}, expected reader frame - 6 (${state.pageRight})`)
  assert(state.height >= minHeight && state.height <= maxHeight, `desktop: ${label} height ${state.height}, expected ${minHeight}-${maxHeight}`)
  assert(state.height < 560, `desktop: ${label} must be a content-sized Popover, not a full-height workspace (${state.height})`)
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
  const expectedExcerpts = (Array.isArray(excerpt) ? excerpt : [excerpt]).filter(Boolean)
  assert(
    expectedExcerpts.length > 0 && state.readonlyValues.some(value => expectedExcerpts.some(expected => value.includes(expected))),
    `${viewport.width}: bookmark form missing readonly excerpt`,
  )
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

async function createBookmarkFromSelectedText(page, viewport, { fullscreen, touch = false }) {
  const paragraph = page.locator('.reader-body p').first()
  const selectedText = (await paragraph.textContent())?.trim() || ''
  assert(selectedText, `${viewport.width}: reader fixture must include bookmark-selectable text`)
  if (touch) {
    await paragraph.evaluate((node) => {
      const rect = node.getBoundingClientRect()
      const event = new Event('touchstart', { bubbles: true, cancelable: true })
      Object.defineProperty(event, 'touches', {
        value: [{ clientX: rect.left + 12, clientY: rect.top + 12, identifier: 1 }],
      })
      node.dispatchEvent(event)
    })
    await page.waitForTimeout(420)
    await paragraph.evaluate((node) => {
      const rect = node.getBoundingClientRect()
      const selection = window.getSelection()
      const range = document.createRange()
      range.selectNodeContents(node)
      selection?.removeAllRanges()
      selection?.addRange(range)
      const event = new Event('touchend', { bubbles: true, cancelable: true })
      Object.defineProperty(event, 'touches', { value: [] })
      Object.defineProperty(event, 'changedTouches', {
        value: [{ clientX: rect.left + 12, clientY: rect.top + 12, identifier: 1 }],
      })
      node.dispatchEvent(event)
    })
  } else {
    await paragraph.evaluate((node) => {
      const selection = window.getSelection()
      const range = document.createRange()
      range.selectNodeContents(node)
      selection?.removeAllRanges()
      selection?.addRange(range)
      node.dispatchEvent(new MouseEvent('mouseup', { bubbles: true, button: 0 }))
    })
  }

  const chooser = page.locator('.el-message-box').last()
  await chooser.getByRole('button', { name: '添加书签', exact: true }).click()
  await assertBookmarkFormContext(page, viewport, { fullscreen, excerpt: selectedText })
  const form = page.locator('.global-bookmark-form-dialog')
  await form.locator('textarea').last().fill('选中文字创建')
  await form.getByRole('button', { name: '确定', exact: true }).click()
  await form.waitFor({ state: 'hidden', timeout: 10000 })
  return selectedText
}

async function createBookmarkFromCurrentParagraph(page, viewport, { fullscreen }) {
  const dialog = page.locator('.global-bookmark-dialog')
  const addButton = dialog.getByRole('button', { name: '添加当前段落', exact: true })
  assert(await addButton.count() === 1, `${viewport.width}: Reader bookmark manager must expose one add-current-paragraph action`)
  const focusedParagraph = await page.locator('.reader-content').evaluate((content) => {
    const usesDocumentScroll = document.querySelector('.reader-shell.document-scroll') !== null
    const bounds = usesDocumentScroll
      ? { top: 0, bottom: window.visualViewport?.height || window.innerHeight, height: window.visualViewport?.height || window.innerHeight }
      : content.getBoundingClientRect()
    const anchor = bounds.top + Math.min(bounds.height * 0.32, 180)
    const rows = [...content.querySelectorAll('[data-reader-block]')]
      .map(node => ({ node, rect: node.getBoundingClientRect() }))
      .filter(({ node, rect }) => (
        String(node.textContent || '').trim()
        && rect.bottom >= bounds.top + 8
        && rect.top <= bounds.bottom - 8
      ))
    const anchored = rows.find(({ rect }) => rect.top <= anchor && rect.bottom >= anchor)
    const selected = anchored || rows.sort((left, right) => (
      Math.abs(left.rect.top - anchor) - Math.abs(right.rect.top - anchor)
    ))[0]
    return String(selected?.node?.textContent || '').trim()
  })
  assert(focusedParagraph, `${viewport.width}: current viewport must expose one bookmark paragraph`)
  await addButton.click()
  await assertBookmarkFormContext(page, viewport, {
    fullscreen,
    excerpt: focusedParagraph,
  })
  const form = page.locator('.global-bookmark-form-dialog')
  assert(
    await form.locator('textarea[readonly]').inputValue() === focusedParagraph,
    `${viewport.width}: current-paragraph bookmark must not append following paragraphs`,
  )
  await form.locator('textarea').last().fill('当前段落创建')
  await form.getByRole('button', { name: '确定', exact: true }).click()
  await form.waitFor({ state: 'hidden', timeout: 10000 })
  assert(await dialog.isVisible(), `${viewport.width}: saving current-paragraph bookmark must keep the manager open`)
  await dialog.getByText('当前段落创建', { exact: true }).waitFor({ state: 'visible', timeout: 10000 })
}

async function exerciseBookmarkManager(page, viewport, { fullscreen, selectedText }) {
  const dialog = page.locator('.global-bookmark-dialog')
  await createBookmarkFromCurrentParagraph(page, viewport, { fullscreen })
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
  const orderedNotes = ['已通过根级表单更新', '选中文字创建', '当前段落创建', '导入三', '导入四']
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

async function exerciseContentSearch(page, viewport, { mobile }) {
  const dialog = page.locator('.global-content-search-dialog')
  const chromeWasVisible = mobile
    ? await page.locator('.reader-mobile-top.visible').count()
    : await page.locator('.reader-left-rail').count()
  const input = dialog.getByPlaceholder('搜索书籍内容')
  await input.fill('契约段落')
  await input.press('Enter')
  await dialog.getByText('有 1 章加载失败，搜索结果不完整，请检查书源或网络后重试', { exact: true })
    .waitFor({ state: 'visible', timeout: 10000 })
  const rows = dialog.locator('.el-table__body-wrapper tbody tr')
  assert(await rows.count() === 2, `${viewport.width}: content search must render the complete mocked result page`)
  if (mobile) {
    assert(await page.locator('.reader-mobile-top.visible').count() === chromeWasVisible, `${viewport.width}: search interaction must preserve mobile Reader chrome`)
  }

  await rows.nth(1).click()
  await dialog.waitFor({ state: 'hidden', timeout: 10000 })
  const highlighted = page.locator('.reader-search-active')
  await highlighted.waitFor({ state: 'visible', timeout: 10000 })
  assert((await highlighted.textContent())?.includes('滚动契约段落 5'), `${viewport.width}: search must highlight the requested fifth occurrence`)
  const query = await page.evaluate(() => Object.fromEntries(new URLSearchParams(location.search)))
  assert(query.chapter === '0' && query.match === '4' && query.q === '契约段落', `${viewport.width}: search result route metadata ${JSON.stringify(query)}`)
  if (mobile) {
    assert(await page.locator('.reader-mobile-top.visible').count() === chromeWasVisible, `${viewport.width}: result jump must preserve mobile Reader chrome`)
  }
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
    const firstConfigScheme = document.querySelector('.config-scheme')
    const firstFontOption = document.querySelector('.font-family-option')
    const labelRect = label?.getBoundingClientRect()
    const controlRect = control?.getBoundingClientRect()
    const activeThemeRect = activeTheme?.getBoundingClientRect()
    const selectionButtonRect = firstSelectionButton?.getBoundingClientRect()
    const configSchemeRect = firstConfigScheme?.getBoundingClientRect()
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
      firstSelectionButtonWidth: selectionButtonRect?.width ?? null,
      firstConfigSchemeWidth: configSchemeRect?.width ?? null,
      firstConfigSchemeHeight: configSchemeRect?.height ?? null,
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
  assertClose(geometry.firstSelectionButtonWidth, 78, 1, `${viewport.width}: settings selection button width`)
  assertClose(geometry.firstSelectionButtonHeight, 34, 1, `${viewport.width}: settings selection button height`)
  assertClose(geometry.firstConfigSchemeWidth, 78, 1, `${viewport.width}: settings configuration scheme width`)
  assertClose(geometry.firstConfigSchemeHeight, 34, 1, `${viewport.width}: settings configuration scheme height`)
  assertClose(geometry.firstFontOptionWidth, 78, 1, `${viewport.width}: settings font option width`)
  assertClose(geometry.firstFontOptionHeight, 34, 1, `${viewport.width}: settings font option height`)
}

async function assertSettingsFirstScreenDensity(page, viewport) {
  const state = await page.evaluate(() => {
    const list = document.querySelector('.settings-list')
    const listRect = list?.getBoundingClientRect()
    const labels = ['特殊模式', '配置方案', '方案类型', '阅读主题'].map((label) => {
      const node = [...document.querySelectorAll('.settings-body .setting-label')].find(item => item.textContent?.trim() === label)
      const rect = node?.closest('.setting-row')?.getBoundingClientRect()
      return { label, top: rect?.top ?? null, bottom: rect?.bottom ?? null }
    })
    const warning = document.querySelector('.setting-help')
    const warningRect = warning?.getBoundingClientRect()
    const warningZone = warning?.closest('.selection-zone')?.getBoundingClientRect()
    const configScheme = document.querySelector('.config-scheme')
    const configStyle = configScheme ? window.getComputedStyle(configScheme) : null
    return {
      listTop: listRect?.top ?? null,
      listBottom: listRect?.bottom ?? null,
      labels,
      warningInsideSelectionZone: Boolean(warning?.closest('.selection-zone')),
      warningTop: warningRect?.top ?? null,
      warningBottom: warningRect?.bottom ?? null,
      warningZoneTop: warningZone?.top ?? null,
      warningZoneBottom: warningZone?.bottom ?? null,
      configSchemeBorderRadius: configStyle?.borderTopLeftRadius ?? '',
      documentWidth: document.documentElement.scrollWidth,
      viewportWidth: innerWidth,
    }
  })
  assert(state.listTop !== null && state.listBottom !== null, `${viewport.width}: settings list missing`)
  assert(state.warningInsideSelectionZone, `${viewport.width}: special-mode warning must stay inside its option zone`)
  assert(state.warningTop >= state.warningZoneTop && state.warningBottom <= state.warningZoneBottom, `${viewport.width}: special-mode warning must be bounded by its option zone`)
  assert(state.configSchemeBorderRadius === '2px', `${viewport.width}: configuration scheme border radius ${state.configSchemeBorderRadius}`)
  for (const row of state.labels) {
    assert(row.top !== null && row.bottom !== null, `${viewport.width}: missing first-screen settings row ${row.label}`)
    assert(row.top >= state.listTop - 1 && row.bottom <= state.listBottom + 1, `${viewport.width}: ${row.label} must remain visible in the initial settings list`)
  }
  assert(state.documentWidth <= state.viewportWidth + 1, `${viewport.width}: settings initial screen must not overflow horizontally`)
}

async function assertSettingsFixedTitle(page, viewport, { desktop = false } = {}) {
  const state = await page.evaluate(({ desktop }) => {
    const title = document.querySelector('.settings-title')
    const list = document.querySelector('.settings-list')
    const outer = desktop
      ? document.querySelector('.reader-desktop-workspace .reader-workspace-body')
      : document.querySelector('.reader-mobile-primary-settings')
    const titleRect = title?.getBoundingClientRect()
    const listRect = list?.getBoundingClientRect()
    const titleStyle = title ? window.getComputedStyle(title) : null
    const listStyle = list ? window.getComputedStyle(list) : null
    const outerStyle = outer ? window.getComputedStyle(outer) : null
    return {
      titleTop: titleRect?.top ?? null,
      titleBottom: titleRect?.bottom ?? null,
      listTop: listRect?.top ?? null,
      listHeight: listRect?.height ?? null,
      listClientHeight: list?.clientHeight ?? 0,
      listScrollHeight: list?.scrollHeight ?? 0,
      listOverflowY: listStyle?.overflowY ?? '',
      outerOverflowY: outerStyle?.overflowY ?? '',
      titleFontSize: titleStyle?.fontSize ?? '',
      titleLineHeight: titleStyle?.lineHeight ?? '',
      titleFontWeight: titleStyle?.fontWeight ?? '',
    }
  }, { desktop })
  assert(state.titleTop !== null && state.listTop !== null, `${viewport.width}: settings title/list missing`)
  assert(state.titleBottom < state.listTop, `${viewport.width}: fixed title must precede the scroll list`)
  assert(Math.abs(state.listHeight - state.listClientHeight) <= 1, `${viewport.width}: settings list must not have a nested height mismatch`)
  assert(state.listScrollHeight > state.listClientHeight + 20, `${viewport.width}: settings fixture must require list scrolling`)
  assert(state.listOverflowY === 'auto', `${viewport.width}: settings list overflow ${state.listOverflowY}`)
  assert(state.outerOverflowY === 'visible', `${viewport.width}: outer settings shell overflow ${state.outerOverflowY}`)
  assert(state.titleFontSize === '18px', `${viewport.width}: settings title font size ${state.titleFontSize}`)
  assert(state.titleLineHeight === '22px', `${viewport.width}: settings title line height ${state.titleLineHeight}`)
  assert(state.titleFontWeight === '400', `${viewport.width}: settings title font weight ${state.titleFontWeight}`)

  const beforeTop = state.titleTop
  const moved = await page.evaluate(() => {
    const list = document.querySelector('.settings-list')
    if (!list) return null
    list.scrollTop = Math.min(180, Math.max(1, list.scrollHeight - list.clientHeight))
    return list.scrollTop
  })
  assert(moved > 0, `${viewport.width}: settings list did not scroll`)
  const after = await page.evaluate(({ desktop }) => {
    const title = document.querySelector('.settings-title')
    const list = document.querySelector('.settings-list')
    const outer = desktop
      ? document.querySelector('.reader-desktop-workspace .reader-workspace-body')
      : document.querySelector('.reader-mobile-primary-settings')
    return {
      titleTop: title?.getBoundingClientRect().top ?? null,
      listScrollTop: list?.scrollTop ?? 0,
      outerScrollTop: outer?.scrollTop ?? 0,
    }
  }, { desktop })
  assertClose(after.titleTop, beforeTop, 1, `${viewport.width}: settings title must stay fixed while list scrolls`)
  assert(after.listScrollTop > 0, `${viewport.width}: settings list scroll position must change`)
  assert(after.outerScrollTop === 0, `${viewport.width}: outer settings shell must not scroll`)
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
  await page.locator('.reader-left-rail button[title="书架"]').click()
  await assertDesktopPrimaryPopover(page, '书架', [380, 410])
  await page.locator('.reader-left-rail button[title="书源"]').click()
  await assertDesktopPrimaryPopover(page, '书源', [380, 410])
  await page.locator('.reader-left-rail button[title="目录"]').click()
  await assertDesktopPrimaryPopover(page, '目录', [380, 410])
  await page.locator('.reader-left-rail button[title="设置"]').click()
  await assertDesktopPrimaryPopover(page, '设置', [470, 520])
  await page.waitForSelector('.reader-desktop-workspace .settings-body', { timeout: 10000 })
  await assertSettingsRowGeometry(page, viewport)
  await assertSettingsFirstScreenDensity(page, viewport)
  await assertSettingsFixedTitle(page, viewport, { desktop: true })
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
  await exerciseContentSearch(page, viewport, { mobile: false })
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
  await assertSelectedTextReplaceRuleEditor(page, viewport, { fullscreen: true, touch: true })
  const selectedBookmarkText = await createBookmarkFromSelectedText(page, viewport, { fullscreen: true, touch: true })

  const initialTopVisible = await page.locator('.reader-mobile-top.visible').count()
  assert(initialTopVisible === 1, `${viewport.width}: mobile toolbar should be visible by default`)
  await assertMobileTopToolContract(page, viewport)
  const initialGeometry = await readerGeometry(page)
  assertReaderGeometry(initialGeometry, viewport, 'initial')
  await assertMobilePageProgressContract(page, viewport, initialGeometry)

  await mobileTopTool(page, '书架').click()
  await assertWorkspaceOpen(page, viewport, '书架', { primary: true, contentSized: true, heightRange: [418, 488] })
  await assertPrimaryPopoverKeepsChromeInteractive(page, viewport, '书架')
  await closePrimaryWorkspace(page, viewport, '书架')

  await mobileTopTool(page, '书源').click()
  await assertWorkspaceOpen(page, viewport, '来源', { primary: true, contentSized: true, heightRange: [418, 488] })
  await assertPrimaryPopoverKeepsChromeInteractive(page, viewport, '来源')
  await closePrimaryWorkspace(page, viewport, '来源')
  await mobileTopTool(page, '目录').click()
  await assertWorkspaceOpen(page, viewport, '目录', { primary: true, contentSized: true, heightRange: [418, 488] })
  await assertPrimaryPopoverKeepsChromeInteractive(page, viewport, '目录')
  await closePrimaryWorkspace(page, viewport, '目录')
  await mobileTopTool(page, '设置').click()
  await assertWorkspaceOpen(page, viewport, '设置', { primary: true, contentSized: true, heightRange: [488, 588] })
  await assertPrimaryPopoverKeepsChromeInteractive(page, viewport, '设置')
  await assertSettingsRowGeometry(page, viewport)
  await assertSettingsFirstScreenDensity(page, viewport)
  await assertSettingsFixedTitle(page, viewport)
  await assertDirectNumericSettingEdit(page, viewport)
  await assertSettingsBackgroundGeometry(page, viewport)

  await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
  const afterPanelCenterTap = await page.locator('.reader-mobile-top.visible').count()
  assert(afterPanelCenterTap === 1, `${viewport.width}: center tap with panel open must not hide toolbar`)

  await closePrimaryWorkspace(page, viewport, '设置')
  await page.locator('.reader-mobile-float-left.visible button[title="书签"]').click()
  await assertGlobalReaderDialog(page, viewport, '.global-bookmark-dialog', '书签')
  await exerciseBookmarkManager(page, viewport, { fullscreen: true, selectedText: selectedBookmarkText })
  await page.locator('.reader-mobile-float-left.visible button[title="搜索正文"]').click()
  await assertGlobalReaderDialog(page, viewport, '.global-content-search-dialog', '搜索正文')
  await exerciseContentSearch(page, viewport, { mobile: true })
  await page.locator('.reader-mobile-float-left.visible button[title="书籍信息"]').click()
  await assertReaderBookInfoDialog(page, viewport, { fullscreen: true })
  await closeReaderBookInfoDialog(page)
  await assertMobileFloatNavigationContract(page, viewport, initialGeometry)
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

async function assertIPadAdaptiveWorkspace(page, viewport, label) {
  await page.waitForSelector('.reader-desktop-workspace', { timeout: 10000 })
  const state = await page.evaluate(() => {
    const workspace = document.querySelector('.reader-desktop-workspace')
    const rect = workspace?.getBoundingClientRect()
    const settingsList = workspace?.querySelector('.settings-list')
    const listStyle = settingsList ? window.getComputedStyle(settingsList) : null
    return {
      count: document.querySelectorAll('.reader-desktop-workspace').length,
      left: rect?.left ?? 0,
      right: rect?.right ?? 0,
      top: rect?.top ?? 0,
      bottom: rect?.bottom ?? 0,
      width: rect?.width ?? 0,
      height: rect?.height ?? 0,
      mobileWorkspaceCount: document.querySelectorAll('.reader-mobile-workspace').length,
      settingsOverflow: listStyle?.overflowY || '',
    }
  })
  assert(state.count === 1, `${viewport.width}x${viewport.height}: ${label} must open one desktop workspace`)
  assert(state.mobileWorkspaceCount === 0, `${viewport.width}x${viewport.height}: ${label} must not open a mobile workspace`)
  assert(state.left >= 0 && state.right <= viewport.width + 1, `${viewport.width}x${viewport.height}: ${label} horizontal bounds ${state.left}-${state.right}`)
  assert(state.top >= 0 && state.bottom <= viewport.height + 1, `${viewport.width}x${viewport.height}: ${label} vertical bounds ${state.top}-${state.bottom}`)
  assert(state.width < viewport.width - 40, `${viewport.width}x${viewport.height}: ${label} must not be a full-width phone panel (${state.width})`)
  assert(state.height < viewport.height, `${viewport.width}x${viewport.height}: ${label} must leave a close path (${state.height})`)
  if (label === '设置') {
    assert(state.settingsOverflow === 'auto', `${viewport.width}x${viewport.height}: settings list overflow ${state.settingsOverflow}`)
  }
}

async function closeIPadWorkspaceByOutsideTouch(page, viewport, label) {
  const before = await page.evaluate(() => {
    const workspace = document.querySelector('.reader-desktop-workspace')
    const content = document.querySelector('.reader-content')
    const body = document.querySelector('.reader-body')
    const rect = workspace?.getBoundingClientRect()
    return {
      workspaceBottom: rect?.bottom ?? 0,
      scrollTop: content?.scrollTop ?? 0,
      transform: body ? window.getComputedStyle(body).transform : '',
      url: window.location.href,
    }
  })
  const outsideY = Math.min(viewport.height - 24, Math.ceil(before.workspaceBottom + 32))
  assert(outsideY > before.workspaceBottom, `${viewport.width}x${viewport.height}: ${label} needs a visible outside-touch target`)
  await page.touchscreen.tap(Math.round(viewport.width / 2), outsideY)
  await page.waitForFunction(() => !document.querySelector('.reader-desktop-workspace'), null, { timeout: 10000 })
  const after = await page.evaluate(() => {
    const content = document.querySelector('.reader-content')
    const body = document.querySelector('.reader-body')
    return {
      scrollTop: content?.scrollTop ?? 0,
      transform: body ? window.getComputedStyle(body).transform : '',
      url: window.location.href,
    }
  })
  assert(after.scrollTop === before.scrollTop, `${viewport.width}x${viewport.height}: ${label} outside touch changed scroll ${before.scrollTop} -> ${after.scrollTop}`)
  assert(after.transform === before.transform, `${viewport.width}x${viewport.height}: ${label} outside touch changed page transform`)
  assert(after.url === before.url, `${viewport.width}x${viewport.height}: ${label} outside touch changed Reader route`)
}

async function closeIPadWorkspaceByVisibleControl(page, viewport, label) {
  const close = page.locator('.reader-desktop-workspace-close')
  const bounds = await close.evaluate((element) => {
    const rect = element.getBoundingClientRect()
    return {
      left: rect.left,
      right: rect.right,
      top: rect.top,
      bottom: rect.bottom,
      width: rect.width,
      height: rect.height,
    }
  })
  assert(bounds.width >= 44 && bounds.height >= 44, `${viewport.width}x${viewport.height}: ${label} close target ${bounds.width}x${bounds.height}`)
  assert(bounds.left >= 0 && bounds.right <= viewport.width, `${viewport.width}x${viewport.height}: ${label} close target horizontal bounds`)
  assert(bounds.top >= 0 && bounds.bottom <= viewport.height, `${viewport.width}x${viewport.height}: ${label} close target vertical bounds`)
  await page.touchscreen.tap(Math.round((bounds.left + bounds.right) / 2), Math.round((bounds.top + bounds.bottom) / 2))
  await page.waitForFunction(() => !document.querySelector('.reader-desktop-workspace'), null, { timeout: 10000 })
}

async function closeDesktopDialogWithHeader(page, selector, viewport, label) {
  const dialog = page.locator(selector)
  const close = dialog.locator('.el-dialog__headerbtn')
  const closeBounds = await close.evaluate((element) => {
    const rect = element.getBoundingClientRect()
    return { left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom }
  })
  assert(closeBounds.left >= 0 && closeBounds.right <= viewport.width, `${viewport.width}: ${label} close button must stay horizontally visible`)
  assert(closeBounds.top >= 0 && closeBounds.bottom <= viewport.height, `${viewport.width}: ${label} close button must stay vertically visible`)
  await close.click()
  await dialog.waitFor({ state: 'hidden', timeout: 10000 })
}

async function runIPadAdaptiveViewport(browser, viewport) {
  const context = await browser.newContext({
    viewport,
    hasTouch: true,
    deviceScaleFactor: 2,
    userAgent: 'Mozilla/5.0 (iPad; CPU OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1',
  })
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
  await page.waitForSelector('.reader-left-rail', { timeout: 10000 })
  await page.waitForSelector('.reader-body p', { timeout: 10000 })

  const initial = await page.evaluate(() => ({
    shellMini: document.querySelector('.reader-shell')?.classList.contains('mini-interface'),
    leftRail: document.querySelectorAll('.reader-left-rail').length,
    rightRail: document.querySelectorAll('.reader-right-rail').length,
    desktopProgress: document.querySelectorAll('.reader-page-control').length,
    mobileChrome: document.querySelectorAll('.reader-mobile-top').length,
    documentWidth: document.documentElement.scrollWidth,
  }))
  assert(initial.shellMini === false, `${viewport.width}x${viewport.height}: adaptive iPad must not expose mini-interface`)
  assert(initial.leftRail === 1 && initial.rightRail === 1, `${viewport.width}x${viewport.height}: adaptive iPad must mount both desktop rails`)
  assert(initial.desktopProgress === 1, `${viewport.width}x${viewport.height}: adaptive iPad must mount desktop progress`)
  assert(initial.mobileChrome === 0, `${viewport.width}x${viewport.height}: adaptive iPad must not mount mobile chrome`)
  assert(initial.documentWidth <= viewport.width + 1, `${viewport.width}x${viewport.height}: adaptive iPad document overflow ${initial.documentWidth}`)

  for (const label of ['书架', '书源', '目录', '设置']) {
    const tool = page.locator(`.reader-left-rail button[title="${label}"]`)
    await tool.click()
    await assertIPadAdaptiveWorkspace(page, viewport, label)
    await closeIPadWorkspaceByVisibleControl(page, viewport, label)
    await tool.click()
    await assertIPadAdaptiveWorkspace(page, viewport, label)
    await closeIPadWorkspaceByOutsideTouch(page, viewport, label)
    await tool.click()
    await assertIPadAdaptiveWorkspace(page, viewport, label)
    await tool.click()
    await page.waitForFunction(() => !document.querySelector('.reader-desktop-workspace'))
  }

  await page.locator('.reader-right-rail button[title="书签"]').click()
  await page.waitForSelector('.global-bookmark-dialog', { timeout: 10000 })
  await closeDesktopDialogWithHeader(page, '.global-bookmark-dialog', viewport, '书签')
  await page.locator('.reader-right-rail button[title="搜索正文"]').click()
  await page.waitForSelector('.global-content-search-dialog', { timeout: 10000 })
  await closeDesktopDialogWithHeader(page, '.global-content-search-dialog', viewport, '搜索正文')
  await page.locator('.reader-right-rail button[title="书籍信息"]').click()
  await assertReaderBookInfoDialog(page, viewport, { fullscreen: false })
  await closeDesktopDialogWithHeader(page, '.book-info-dialog', viewport, '书籍信息')

  assert(failures.length === 0, failures.join('\n'))
  await context.close()
}

async function runIPadForcedMobileViewport(browser, viewport) {
  const context = await browser.newContext({
    viewport,
    hasTouch: true,
    deviceScaleFactor: 2,
    userAgent: 'Mozilla/5.0 (iPad; CPU OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1',
  })
  await context.addInitScript((token) => {
    window.localStorage.setItem('openreader_token', token)
  }, fakeToken())
  const page = await context.newPage()
  await installApiMocks(page, { pageMode: 'mobile' })
  await page.goto(readerUrl, { waitUntil: 'networkidle' })
  await page.waitForSelector('.reader-mobile-top.visible', { timeout: 10000 })
  assert(await page.locator('.reader-shell.mini-interface').count() === 1, `${viewport.width}: explicit phone mode must retain mini scene`)
  assert(await page.locator('.reader-left-rail, .reader-right-rail').count() === 0, `${viewport.width}: explicit phone mode must not mix desktop rails`)
  for (const [toolLabel, panelLabel] of [
    ['书架', '书架'],
    ['书源', '来源'],
    ['目录', '目录'],
    ['设置', '设置'],
  ]) {
    await mobileTopTool(page, toolLabel).click()
    await assertWorkspaceOpen(page, viewport, panelLabel, { primary: true, contentSized: true })
    await assertPrimaryPopoverKeepsChromeInteractive(page, viewport, panelLabel)
    await mobileTopTool(page, toolLabel).click()
    await assertWorkspaceClosed(page, viewport, panelLabel)
  }
  await context.close()
}

async function main() {
  const browser = await openSmokeBrowser()
  try {
    await runDesktopViewport(browser)
    await runViewport(browser, { width: 390, height: 844 })
    await runViewport(browser, { width: 360, height: 800 })
    await runIPadAdaptiveViewport(browser, { width: 1024, height: 1366 })
    await runIPadAdaptiveViewport(browser, { width: 1366, height: 1024 })
    await runIPadForcedMobileViewport(browser, { width: 1024, height: 1366 })
    console.log('reader desktop/mobile/adaptive-iPad/forced-mobile-iPad contract smoke passed')
  } finally {
    await browser.close()
  }
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})
