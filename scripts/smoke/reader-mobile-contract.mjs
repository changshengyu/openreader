#!/usr/bin/env node

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:5173'
const readerUrl = process.env.SMOKE_READER_URL || `${targetUrl.replace(/\/$/, '')}/books/1/read?chapter=0`
const defaultChromePath = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'

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

async function installApiMocks(page) {
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
      return route.fulfill(json({
        id: 1,
        title: '移动阅读契约测试',
        author: 'OpenReader',
        sourceId: 2,
        chapterCount: 2,
        progress: null,
      }))
    }
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
    if (path === '/books/1/bookmarks') {
      return route.fulfill(json([]))
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

async function assertWorkspaceOpen(page, viewport, label) {
  await page.waitForSelector('.reader-mobile-workspace', { timeout: 10000 })
  const topCount = await page.locator('.reader-mobile-top.visible').count()
  assert(topCount === 1, `${viewport.width}: toolbar should remain visible after opening ${label}`)
  const workspaceState = await page.evaluate((expectedLabel) => {
    const workspace = document.querySelector('.reader-mobile-workspace')
    const rect = workspace.getBoundingClientRect()
    const header = workspace.querySelector('.reader-mobile-workspace-head')
    const visibleDrawers = Array.from(document.querySelectorAll('.el-drawer')).filter((element) => {
      const drawerRect = element.getBoundingClientRect()
      const style = window.getComputedStyle(element)
      return drawerRect.width > 0 && drawerRect.height > 0 && style.display !== 'none' && style.visibility !== 'hidden'
    }).length
    return {
      width: Math.round(rect.width),
      left: Math.round(rect.left),
      visibleDrawers,
      role: workspace.getAttribute('role'),
      text: workspace.innerText,
      hasLabel: workspace.innerText.includes(expectedLabel),
      hasGenericHeader: Boolean(header),
    }
  }, label)
  assert(workspaceState.left === 0, `${viewport.width}: mobile workspace left ${workspaceState.left}`)
  assert(workspaceState.width === viewport.width, `${viewport.width}: mobile workspace width ${workspaceState.width}`)
  assert(workspaceState.visibleDrawers === 0, `${viewport.width}: mobile workspace must not use visible drawer`)
  assert(workspaceState.role === 'dialog', `${viewport.width}: mobile workspace role ${workspaceState.role}`)
  assert(workspaceState.hasLabel, `${viewport.width}: mobile workspace missing label ${label}`)
  if (label === '设置') {
    assert(workspaceState.hasGenericHeader === false, `${viewport.width}: settings workspace must not render a duplicate generic header`)
  }
}

async function assertSettingsRowGeometry(page, viewport) {
  const geometry = await page.evaluate(() => {
    const firstRow = document.querySelector('.settings-body .setting-row')
    const label = firstRow?.querySelector('.setting-label')
    const control = firstRow ? Array.from(firstRow.children).find(element => !element.classList.contains('setting-label')) : null
    const activeTheme = document.querySelector('.theme-dot.active')
    const labelRect = label?.getBoundingClientRect()
    const controlRect = control?.getBoundingClientRect()
    const labelStyle = label ? window.getComputedStyle(label) : null
    const activeThemeStyle = activeTheme ? window.getComputedStyle(activeTheme) : null
    return {
      labelLeft: labelRect?.left ?? null,
      labelTop: labelRect?.top ?? null,
      controlLeft: controlRect?.left ?? null,
      controlTop: controlRect?.top ?? null,
      labelLineHeight: labelStyle?.lineHeight ?? '',
      activeThemeBorderColor: activeThemeStyle?.borderTopColor ?? '',
    }
  })
  assert(geometry.labelLeft !== null && geometry.controlLeft !== null, `${viewport.width}: missing settings first row geometry`)
  assertClose(geometry.controlLeft - geometry.labelLeft, 72, 1, `${viewport.width}: settings control column offset`)
  assertClose(geometry.controlTop, geometry.labelTop, 2, `${viewport.width}: settings label and control should share a row`)
  assert(geometry.labelLineHeight === '36px', `${viewport.width}: settings label line-height ${geometry.labelLineHeight}`)
  assert(geometry.activeThemeBorderColor === 'rgb(237, 66, 89)', `${viewport.width}: active theme border ${geometry.activeThemeBorderColor}`)
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

  const initialTopVisible = await page.locator('.reader-mobile-top.visible').count()
  assert(initialTopVisible === 1, `${viewport.width}: mobile toolbar should be visible by default`)
  const initialGeometry = await readerGeometry(page)
  assertReaderGeometry(initialGeometry, viewport, 'initial')

  await page.getByRole('button', { name: /设置/ }).click()
  await assertWorkspaceOpen(page, viewport, '设置')
  await assertSettingsRowGeometry(page, viewport)

  await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
  const afterPanelCenterTap = await page.locator('.reader-mobile-top.visible').count()
  assert(afterPanelCenterTap === 1, `${viewport.width}: center tap with panel open must not hide toolbar`)

  await closeWorkspace(page, 'settings-toggle')
  await page.getByRole('button', { name: /目录/ }).click()
  await assertWorkspaceOpen(page, viewport, '目录')
  await closeWorkspace(page)
  await page.locator('.reader-mobile-float-left.visible button[title="书签"]').click()
  await assertWorkspaceOpen(page, viewport, '书签')
  await closeWorkspace(page)
  await page.locator('.reader-mobile-float-left.visible button[title="搜索正文"]').click()
  await assertWorkspaceOpen(page, viewport, '搜索正文')
  await closeWorkspace(page)
  await page.locator('.reader-mobile-bottom.visible button[title="缓存章节"]').click()
  await assertWorkspaceOpen(page, viewport, '缓存章节')
  await closeWorkspace(page)

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
    await runViewport(browser, { width: 390, height: 844 })
    await runViewport(browser, { width: 360, height: 800 })
    console.log('reader mobile contract smoke passed')
  } finally {
    await browser.close()
  }
}

main().catch((error) => {
  console.error(error.stack || error.message)
  process.exit(1)
})
