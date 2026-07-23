#!/usr/bin/env node

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = process.env.TARGET_URL || 'http://127.0.0.1:4173'
const fixtureText = Array.from({ length: 320 }, (_, index) => (
  `第${index + 1}段。春风过处，纸页微明。此段用于验证切换阅读设置后仍停留在相同正文位置。`
)).join('\n')

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function token() {
  const payload = Buffer.from(JSON.stringify({ userId: 1, sub: '1' })).toString('base64url')
  return `open.${payload}.reader`
}

function response(data, status = 200) {
  return { status, contentType: 'application/json', body: JSON.stringify(data) }
}

function scheme(name, mode, configDefaultType) {
  return {
    name,
    configDefaultType,
    builtin: true,
    mode,
    pageType: 'normal',
    pageMode: 'auto',
    clickMethod: 'auto',
    selectionAction: '操作弹窗',
    fontFamily: 'system',
    chineseFont: '简体',
    fontSize: mode === 'flip' ? 24 : 18,
    fontWeight: mode === 'flip' ? 500 : 400,
    theme: mode === 'flip' ? 'dark' : 'parchment',
    themeType: mode === 'flip' ? 'night' : 'day',
    lineHeight: mode === 'flip' ? 2.2 : 1.8,
    paragraphSpace: mode === 'flip' ? 0.6 : 0.2,
    columnWidth: 800,
    animateDuration: 0,
  }
}

function baseSettings(mode = 'page', overrides = {}) {
  return {
    mode,
    pageType: 'normal',
    pageMode: 'auto',
    clickMethod: 'auto',
    selectionAction: '操作弹窗',
    theme: 'parchment',
    themeType: 'day',
    fontFamily: 'system',
    chineseFont: '简体',
    fontSize: 18,
    fontWeight: 400,
    lineHeight: 1.8,
    paragraphSpace: 0.2,
    columnWidth: 800,
    animateDuration: 0,
    autoTheme: false,
    customConfigName: '上下方案',
    customConfigList: [
      scheme('上下方案', 'page', '白天默认'),
      scheme('左右方案', 'flip', '黑夜默认'),
    ],
    ...overrides,
  }
}

async function installApiMocks(page, settings) {
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async route => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    const book = {
      id: 1,
      title: '设置位置契约测试',
      author: 'OpenReader',
      sourceId: 2,
      sourceName: '测试书源',
      url: 'https://source.example/book/settings-position',
      bookUrl: 'https://source.example/book/settings-position',
      chapterCount: 2,
      categoryIds: [],
      progress: null,
    }
    if (path === '/me') return route.fulfill(response({ id: 1, username: 'smoke', role: 'admin' }))
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(response({
        key: 'reader',
        updatedAt: '2026-07-23T00:00:00Z',
        value: settings,
      }))
    }
    if (path === '/settings/reader' && method === 'PUT') {
      return route.fulfill(response({
        key: 'reader',
        updatedAt: '2026-07-23T00:00:01Z',
        value: settings,
      }))
    }
    if (path === '/books/1') return route.fulfill(response(book))
    if (path === '/books') return route.fulfill(response([book]))
    if (path === '/books/1/chapters') {
      return route.fulfill(response([
        { id: 11, index: 0, title: '第一章' },
        { id: 12, index: 1, title: '第二章' },
      ]))
    }
    if (/^\/books\/1\/chapters\/\d+\/content$/.test(path)) {
      const index = Number(path.match(/chapters\/(\d+)\/content/)?.[1] || 0)
      return route.fulfill(response({
        chapter: { id: 11 + index, index, title: `第${index + 1}章` },
        content: fixtureText,
      }))
    }
    if (path === '/progress/1') return route.fulfill(response({}))
    if (path === '/sources') return route.fulfill(response([{ id: 2, name: '测试书源', enabled: true }]))
    if (path === '/categories') return route.fulfill(response([]))
    return route.fulfill(response({}))
  })
}

async function openReader(browser, viewport, settings, options = {}) {
  const context = await browser.newContext({
    viewport,
    hasTouch: true,
    isMobile: viewport.width <= 750,
    colorScheme: options.colorScheme || 'light',
  })
  await context.addInitScript(value => localStorage.setItem('openreader_token', value), token())
  const page = await context.newPage()
  const errors = []
  page.on('console', message => {
    if (message.type() !== 'error') return
    if (message.text().includes('/ws/sync')) return
    errors.push(`console: ${message.text()}`)
  })
  page.on('pageerror', error => errors.push(`pageerror: ${error.message}`))
  await installApiMocks(page, settings)
  await page.goto(`${targetUrl.replace(/\/$/, '')}/books/1/read?chapter=0`, { waitUntil: 'networkidle' })
  await page.waitForSelector('.reader-body p', { timeout: 15_000 })
  return { context, errors, page }
}

async function positionAtParagraph(page, index = 96) {
  const anchor = await page.evaluate((paragraphIndex) => {
    const paragraphs = [...document.querySelectorAll('.chapter-content[data-index="0"] p[data-reader-block]')]
    const target = paragraphs[paragraphIndex]
    if (!target) return null
    const documentScroll = document.querySelector('.reader-shell.document-scroll') !== null
    const viewport = documentScroll
      ? (document.scrollingElement || document.documentElement)
      : document.querySelector('.reader-content')
    const viewportRect = documentScroll
      ? { top: 0 }
      : viewport.getBoundingClientRect()
    viewport.scrollTop = Math.max(
      0,
      viewport.scrollTop + target.getBoundingClientRect().top - viewportRect.top - 140,
    )
    return {
      chapterIndex: 0,
      paragraphIndex,
      paragraphPos: Number(target.dataset.pos),
      text: target.textContent,
    }
  }, index)
  assert(anchor, `paragraph ${index} is missing`)
  await page.waitForTimeout(100)
  return captureVisibleParagraph(page)
}

async function captureVisibleParagraph(page) {
  const anchor = await page.evaluate(() => {
    const content = document.querySelector('.reader-content')
    const documentScroll = document.querySelector('.reader-shell.document-scroll') !== null
    const viewport = documentScroll
      ? { left: 0, top: 0, right: window.innerWidth, bottom: window.innerHeight, height: window.innerHeight }
      : content?.getBoundingClientRect()
    if (!viewport) return null
    const anchorY = viewport.top + Math.min(viewport.height * 0.32, 180)
    const paragraphs = [...document.querySelectorAll('.chapter-content p[data-reader-block]')]
      .map((node, paragraphIndex) => ({
        node,
        paragraphIndex,
        rect: node.getBoundingClientRect(),
      }))
      .filter(({ rect }) => (
        rect.bottom >= viewport.top + 8
        && rect.top <= viewport.bottom - 8
        && rect.right >= viewport.left + 8
        && rect.left <= viewport.right - 8
      ))
    const selected = paragraphs.find(({ rect }) => (
      rect.top <= anchorY && rect.bottom >= anchorY
    )) || [...paragraphs].sort((a, b) => (
      Math.abs(a.rect.top - anchorY) - Math.abs(b.rect.top - anchorY)
    ))[0]
    if (!selected) return null
    const chapter = selected.node.closest('.chapter-content')
    const chapterNodes = [...chapter.querySelectorAll('p[data-reader-block]')]
    return {
      chapterIndex: Number(chapter.dataset.index),
      paragraphIndex: chapterNodes.indexOf(selected.node),
      paragraphPos: Number(selected.node.dataset.pos),
      text: selected.node.textContent,
    }
  })
  assert(anchor, 'no visible paragraph anchor was found')
  return anchor
}

async function assertAnchorVisible(page, anchor, label) {
  await page.waitForFunction((expected) => {
    const chapter = document.querySelector(`.chapter-content[data-index="${expected.chapterIndex}"]`)
    const nodes = [...(chapter?.querySelectorAll('p[data-reader-block]') || [])]
    const target = nodes.find(node => (
      Number(node.dataset.pos) === expected.paragraphPos
      && node.textContent === expected.text
    )) || nodes[expected.paragraphIndex]
    const rect = target?.getBoundingClientRect()
    const content = document.querySelector('.reader-content')
    const documentScroll = document.querySelector('.reader-shell.document-scroll') !== null
    const viewport = documentScroll
      ? { left: 0, top: 0, right: window.innerWidth, bottom: window.innerHeight }
      : content?.getBoundingClientRect()
    return Boolean(
      rect
      && viewport
      && rect.bottom > viewport.top
      && rect.top < viewport.bottom
      && rect.right > viewport.left
      && rect.left < viewport.right
    )
  }, anchor, { timeout: 3_000 }).catch(() => {})
  const state = await page.evaluate((expected) => {
    const chapter = document.querySelector(`.chapter-content[data-index="${expected.chapterIndex}"]`)
    const nodes = [...(chapter?.querySelectorAll('p[data-reader-block]') || [])]
    const target = nodes.find(node => (
      Number(node.dataset.pos) === expected.paragraphPos
      && node.textContent === expected.text
    )) || nodes[expected.paragraphIndex]
    const rect = target?.getBoundingClientRect()
    const content = document.querySelector('.reader-content')
    const shell = document.querySelector('.reader-shell')
    const documentScroll = shell?.classList.contains('document-scroll')
    const viewport = documentScroll
      ? { left: 0, top: 0, right: window.innerWidth, bottom: window.innerHeight }
      : content?.getBoundingClientRect()
    return {
      chapterIndex: Number(target?.closest('.chapter-content')?.dataset?.index),
      contentScrollTop: content?.scrollTop || 0,
      documentScroll,
      mode: ['page', 'flip', 'scroll', 'scroll2'].find(mode => shell?.classList.contains(mode)),
      pageTransform: document.querySelector('.reader-body')
        ? getComputedStyle(document.querySelector('.reader-body')).transform
        : '',
      rootScrollTop: (document.scrollingElement || document.documentElement)?.scrollTop || 0,
      targetRect: rect ? {
        bottom: rect.bottom,
        left: rect.left,
        right: rect.right,
        top: rect.top,
      } : null,
      targetFound: Boolean(target),
      viewport: viewport ? {
        bottom: viewport.bottom,
        left: viewport.left,
        right: viewport.right,
        top: viewport.top,
      } : null,
      visible: Boolean(
        rect
        && viewport
        && rect.bottom > viewport.top
        && rect.top < viewport.bottom
        && rect.right > viewport.left
        && rect.left < viewport.right
      ),
    }
  }, anchor)
  assert(state.targetFound, `${label}: captured paragraph disappeared`)
  assert(state.chapterIndex === anchor.chapterIndex, `${label}: chapter changed ${JSON.stringify(state)}`)
  assert(state.visible, `${label}: captured paragraph is outside the new viewport ${JSON.stringify(state)}`)
  assert(
    state.rootScrollTop > 0 || state.contentScrollTop > 0 || state.mode === 'flip',
    `${label}: position fell back to chapter start ${JSON.stringify(state)}`,
  )
  return state
}

async function ensureMobileSettings(page, viewport) {
  if (!await page.locator('.reader-mobile-top.visible').count()) {
    await page.touchscreen.tap(Math.round(viewport.width / 2), Math.round(viewport.height / 2))
    await page.waitForSelector('.reader-mobile-top.visible')
  }
  if (!await page.locator('.reader-mobile-primary-settings').count()) {
    await page.locator('.reader-mobile-top.visible .mobile-tool-button').filter({ hasText: '设置' }).click()
    await page.waitForSelector('.reader-mobile-primary-settings .settings-body')
  }
  return page.locator('.reader-mobile-primary-settings')
}

async function ensureDesktopSettings(page) {
  if (!await page.locator('.reader-desktop-workspace .settings-body').count()) {
    await page.locator('.reader-left-rail button[title="设置"]').click()
    await page.waitForSelector('.reader-desktop-workspace .settings-body')
  }
  return page.locator('.reader-desktop-workspace')
}

async function clickSettingOption(panel, label, option) {
  const row = panel.locator('.setting-row').filter({ hasText: label }).first()
  await row.scrollIntoViewIfNeeded()
  await row.locator('.selection-button').filter({ hasText: option }).first().click()
}

async function assertMobileModeAndSchemeTransactions(browser, viewport) {
  const { context, errors, page } = await openReader(browser, viewport, baseSettings('page'))
  const panel = await ensureMobileSettings(page, viewport)
  let anchor = await positionAtParagraph(page)

  await clickSettingOption(panel, '翻页方式', '左右滑动')
  await page.waitForSelector('.reader-shell.flip')
  await assertAnchorVisible(page, anchor, `${viewport.width}: page -> flip`)
  assert(await page.locator('.reader-mobile-top.visible').count() === 1, `${viewport.width}: mode switch hid tools`)
  assert(await page.locator('.reader-mobile-primary-settings').count() === 1, `${viewport.width}: mode switch closed settings`)

  anchor = await captureVisibleParagraph(page)
  await clickSettingOption(panel, '翻页方式', '上下滑动')
  await page.waitForSelector('.reader-shell.page.document-scroll')
  await assertAnchorVisible(page, anchor, `${viewport.width}: flip -> page`)

  anchor = await captureVisibleParagraph(page)
  await clickSettingOption(panel, '配置方案', '左右方案')
  await page.waitForSelector('.reader-shell.flip')
  await assertAnchorVisible(page, anchor, `${viewport.width}: scheme -> flip with typography`)

  anchor = await captureVisibleParagraph(page)
  await clickSettingOption(panel, '特殊模式', '正常')
  await clickSettingOption(panel, '特殊模式', '简洁')
  await page.waitForSelector('.reader-shell.flip')
  await assertAnchorVisible(page, anchor, `${viewport.width}: normal -> Kindle`)

  assert(errors.length === 0, `${viewport.width}: browser errors ${JSON.stringify(errors)}`)
  await context.close()
}

async function assertAutomaticThemeTransaction(browser) {
  const viewport = { width: 390, height: 844 }
  const settings = baseSettings('page', { autoTheme: true })
  const { context, errors, page } = await openReader(browser, viewport, settings, { colorScheme: 'light' })
  await positionAtParagraph(page, 112)
  const anchor = await captureVisibleParagraph(page)
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.waitForSelector('.reader-shell.flip')
  await assertAnchorVisible(page, anchor, 'automatic night scheme')
  assert(errors.length === 0, `automatic theme browser errors ${JSON.stringify(errors)}`)
  await context.close()
}

async function assertIPadPageModeTransaction(browser) {
  const viewport = { width: 1024, height: 1366 }
  const { context, errors, page } = await openReader(browser, viewport, baseSettings('page'))
  const panel = await ensureDesktopSettings(page)
  await positionAtParagraph(page, 72)
  let anchor = await captureVisibleParagraph(page)
  await clickSettingOption(panel, '页面模式', '手机模式')
  await page.waitForSelector('.reader-shell.mini-interface.document-scroll')
  await page.waitForSelector('.reader-mobile-primary-settings .settings-body')
  await assertAnchorVisible(page, anchor, 'iPad adaptive -> mobile')
  assert(await page.locator('.reader-mobile-top.visible').count() === 1, 'iPad page-mode switch hid tools')

  anchor = await captureVisibleParagraph(page)
  const mobilePanel = page.locator('.reader-mobile-primary-settings')
  await clickSettingOption(mobilePanel, '页面模式', '自适应')
  await page.waitForSelector('.reader-desktop-workspace .settings-body')
  await assertAnchorVisible(page, anchor, 'iPad mobile -> adaptive')
  assert(errors.length === 0, `iPad browser errors ${JSON.stringify(errors)}`)
  await context.close()
}

async function main() {
  const browser = await openSmokeBrowser()
  try {
    for (const viewport of [{ width: 390, height: 844 }, { width: 360, height: 800 }]) {
      await assertMobileModeAndSchemeTransactions(browser, viewport)
    }
    await assertAutomaticThemeTransaction(browser)
    await assertIPadPageModeTransaction(browser)
    console.log('reader settings-position contract smoke passed')
  } finally {
    await browser.close()
  }
}

main().catch(error => {
  console.error(error.stack || error.message)
  process.exit(1)
})
