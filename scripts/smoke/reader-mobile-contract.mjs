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

  const initial = await page.evaluate(() => {
    const top = document.querySelector('.reader-mobile-top')
    const pageEl = document.querySelector('.reader-page')
    const body = document.querySelector('.reader-body')
    const firstParagraph = document.querySelector('.reader-body p')
    const pageStyle = window.getComputedStyle(pageEl)
    const bodyStyle = window.getComputedStyle(body)
    const paragraphStyle = window.getComputedStyle(firstParagraph)
    return {
      topVisible: top?.classList.contains('visible'),
      pagePaddingLeft: pageStyle.paddingLeft,
      pagePaddingRight: pageStyle.paddingRight,
      pageTextAlign: pageStyle.textAlign,
      bodyTextAlign: bodyStyle.textAlign,
      paragraphTextAlign: paragraphStyle.textAlign,
    }
  })
  assert(initial.topVisible, `${viewport.width}: mobile toolbar should be visible by default`)
  assert(initial.pagePaddingLeft === '16px', `${viewport.width}: left padding ${initial.pagePaddingLeft}`)
  assert(initial.pagePaddingRight === '16px', `${viewport.width}: right padding ${initial.pagePaddingRight}`)
  assert(initial.pageTextAlign === 'justify', `${viewport.width}: page text-align ${initial.pageTextAlign}`)
  assert(initial.bodyTextAlign === 'justify', `${viewport.width}: body text-align ${initial.bodyTextAlign}`)
  assert(initial.paragraphTextAlign === 'justify', `${viewport.width}: paragraph text-align ${initial.paragraphTextAlign}`)

  await page.getByRole('button', { name: /设置/ }).click()
  await page.waitForFunction(() => Array.from(document.querySelectorAll('.el-drawer')).some((element) => {
    const rect = element.getBoundingClientRect()
    const style = window.getComputedStyle(element)
    return rect.width > 0 && rect.height > 0 && style.display !== 'none' && style.visibility !== 'hidden'
  }), null, { timeout: 10000 })
  const afterSettings = await page.locator('.reader-mobile-top.visible').count()
  assert(afterSettings === 1, `${viewport.width}: toolbar should remain visible after opening settings`)

  await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
  const afterPanelCenterTap = await page.locator('.reader-mobile-top.visible').count()
  assert(afterPanelCenterTap === 1, `${viewport.width}: center tap with panel open must not hide toolbar`)

  await page.keyboard.press('Escape')
  await page.waitForTimeout(250)
  await page.mouse.click(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
  await page.waitForTimeout(120)
  const afterCenterTap = await page.locator('.reader-mobile-top.visible').count()
  assert(afterCenterTap === 0, `${viewport.width}: center tap without panel should hide toolbar`)

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
