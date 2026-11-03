#!/usr/bin/env node

// Reader menu bars and primary panels browser contract
// (docs/compat/reader-menu-chrome-panels-fixed-baseline-second-audit-p0-contract.md).
// Expected values were measured on the fixed reader-dev web build with the same fixture: a 12-chapter book read at
// chapter 3 (60 paragraphs of the same text), six shelf books and four sources.
// Mobile 390x844 / 360x800 (touch) and desktop 1440x900 / 1024x1366, day; mobile 390x844 and desktop 1440x900 at
// night. Every viewport runs to the end and reports all of its failed checks; SMOKE_VIEWPORTS=<regex> limits them.

import { openSmokeBrowser } from './playwright-runtime.mjs'

const targetUrl = (process.env.TARGET_URL || 'http://127.0.0.1:5173').replace(/\/$/, '')
const viewportFilter = process.env.SMOKE_VIEWPORTS ? new RegExp(process.env.SMOKE_VIEWPORTS) : null

const VIEWPORTS = [
  { label: '390x844', width: 390, height: 844, mobile: true },
  { label: '360x800', width: 360, height: 800, mobile: true },
  { label: '1440x900', width: 1440, height: 900, mobile: false },
  { label: '1024x1366', width: 1024, height: 1366, mobile: false },
  { label: '390x844 night', width: 390, height: 844, mobile: true, night: true },
  { label: '1440x900 night', width: 1440, height: 900, mobile: false, night: true },
]
const BOOK_TITLE = '菜单对照书'
const AUTHOR = '对照作者'
const CURRENT = 2
const TITLES = Array.from({ length: 12 }, (_, index) => `第${index + 1}章 菜单对照章节${index + 1}`)
const SHELF = [
  { title: BOOK_TITLE, durIndex: CURRENT, total: 12 },
  { title: '书架第二本', durIndex: 0, total: 30 },
  { title: '书架第三本', durIndex: 5, total: 6 },
  { title: '书架第四本很长很长的书名用于截断显示测试', durIndex: 10, total: 120 },
  { title: '书架第五本', durIndex: 1, total: 2 },
  { title: '书架第六本', durIndex: 0, total: 1 },
]
const SOURCES = [
  { name: '当前来源', latest: TITLES[11], time: 18, current: true },
  { name: '备用来源一', latest: '第十二章 备用', time: 34 },
  { name: '备用来源二', latest: '', time: 0 },
  { name: '备用来源三很长很长的名字', latest: '第十章 很长很长的最新章节标题用于截断显示', time: 120 },
]
const RED = 'rgb(235, 66, 89)'
const TOLERANCE = 1.5

function json(data, status = 200) {
  return { status, contentType: 'application/json', body: JSON.stringify(data) }
}

function fakeToken() {
  const payload = Buffer.from(JSON.stringify({ userId: 1, sub: '1' })).toString('base64url')
  return `open.${payload}.reader`
}

function paragraphs(index) {
  return Array.from({ length: 60 }, (_, p) => (
    `${TITLES[index]}第${p + 1}段。菜单对照正文，用于比较上游与 OpenReader 阅读页工具栏和弹出面板的布局与逻辑。`
  ))
}

function shelfBooks() {
  return SHELF.map((item, i) => ({
    id: i + 1,
    title: item.title,
    author: AUTHOR,
    sourceId: 2,
    sourceName: '当前来源',
    bookUrl: i === 0 ? 'https://current.example/book' : `https://example.test/book/${i}`,
    chapterCount: item.total,
    latestChapterTitle: `第${item.total}章`,
    createdAt: new Date(Date.UTC(2026, 9, 1) - i * 1000).toISOString(),
    updatedAt: new Date(Date.UTC(2026, 9, 1) - i * 1000).toISOString(),
    progress: {
      bookId: i + 1,
      chapterIndex: item.durIndex,
      chapterTitle: i === 0 ? TITLES[item.durIndex] : `第${item.durIndex + 1}章`,
      updatedAt: new Date(Date.UTC(2026, 9, 2) - i * 1000).toISOString(),
    },
  }))
}

async function installMocks(page, viewport) {
  await page.route(/^https?:\/\/[^/]+\/ws\/sync.*$/, route => route.abort())
  await page.route(/^https?:\/\/[^/]+\/api\/.*$/, async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    const path = url.pathname.replace(/^\/api/, '')
    const method = request.method()
    if (path === '/me') return route.fulfill(json({ id: 1, username: 'menu-smoke', role: 'admin' }))
    if (path === '/settings/reader' && method === 'GET') {
      return route.fulfill(json({
        key: 'reader',
        updatedAt: '2026-11-03T00:00:00Z',
        value: {
          mode: 'page',
          pageMode: 'auto',
          autoTheme: false,
          theme: viewport.night ? 'dark' : 'parchment',
          themeType: viewport.night ? 'night' : 'day',
          fontSize: 18,
          lineHeight: 1.8,
          paragraphSpace: 0.2,
          columnWidth: 800,
          animateDuration: 0,
        },
      }))
    }
    if (path === '/settings/reader' && method === 'PUT') return route.fulfill(json({ key: 'reader', updatedAt: '2026-11-03T00:00:01Z', value: {} }))
    const books = shelfBooks()
    if (path === '/books/1' && method === 'GET') return route.fulfill(json({ ...books[0], progress: { chapterIndex: CURRENT, offset: 0 } }))
    if (path === '/books' && method === 'GET') return route.fulfill(json(books))
    if (path === '/books/1/chapters') return route.fulfill(json(TITLES.map((title, index) => ({ id: index + 11, bookId: 1, index, title }))))
    const content = path.match(/^\/books\/1\/chapters\/(\d+)\/content$/)
    if (content) {
      const index = Number(content[1])
      return route.fulfill(json({ chapter: { id: index + 11, bookId: 1, index, title: TITLES[index] }, content: paragraphs(index).join('\n'), format: 'text' }))
    }
    if (path === '/books/1/source-candidates') {
      return route.fulfill(json(SOURCES.map((source, i) => ({
        sourceId: i + 2,
        sourceName: source.name,
        group: '默认',
        title: BOOK_TITLE,
        author: AUTHOR,
        bookUrl: i === 0 ? 'https://current.example/book' : `https://alt${i}.example/book`,
        latestChapterTitle: source.latest,
        time: source.time,
        current: Boolean(source.current),
        type: 0,
      }))))
    }
    if (path === '/progress' && method === 'PUT') return route.fulfill(json({ ...(request.postDataJSON?.() || {}), bookId: 1 }))
    if (path === '/progress/1') return route.fulfill(json({ chapterIndex: CURRENT, offset: 0 }))
    if (path === '/sources') return route.fulfill(json([{ id: 2, name: '当前来源', group: '默认', enabled: true }]))
    if (['/books/1/bookmarks', '/categories', '/book-groups', '/replace-rules'].includes(path)) return route.fulfill(json([]))
    return route.fulfill(json({}))
  })
}

async function openReader(browser, viewport, chapter = CURRENT) {
  const context = await browser.newContext({
    viewport: { width: viewport.width, height: viewport.height },
    deviceScaleFactor: 1,
    ...(viewport.mobile ? { hasTouch: true, isMobile: true } : {}),
  })
  await context.addInitScript((token) => {
    localStorage.setItem('openreader_token', token)
    localStorage.setItem('reader', JSON.stringify({ mode: 'page', settingsScope: 'user:1', progressScope: 'user:1' }))
  }, fakeToken())
  const page = await context.newPage()
  await installMocks(page, viewport)
  await page.goto(`${targetUrl}/books/1/read?chapter=${chapter}`, { waitUntil: 'domcontentloaded' })
  await page.waitForSelector('.reader-body [data-reader-block]', { timeout: 20_000 })
  await page.waitForTimeout(500)
  return { context, page }
}

function rects(page, selector) {
  return page.evaluate((sel) => [...document.querySelectorAll(sel)].map((node) => {
    const rect = node.getBoundingClientRect()
    const style = getComputedStyle(node)
    return {
      x: rect.x,
      y: rect.y,
      w: rect.width,
      h: rect.height,
      // The iconfont glyphs are private-use characters; keep the readable text.
      text: (node.innerText || '').replace(/[\ue000-\uf8ff]/g, '').trim().replace(/\s+/g, ' '),
      color: style.color,
      backgroundColor: style.backgroundColor,
      backgroundImage: style.backgroundImage,
      boxShadow: style.boxShadow,
      borderTop: style.borderTopWidth,
      borderBottom: style.borderBottomWidth,
      borderLeftColor: style.borderLeftColor,
      fontSize: style.fontSize,
      pointerEvents: style.pointerEvents,
      display: style.display,
    }
  }).filter(rect => rect.display !== 'none' && rect.w > 0 && rect.h > 0), selector)
}

function hitInside(page, x, y, selector) {
  return page.evaluate(({ px, py, sel }) => Boolean(document.elementFromPoint(px, py)?.closest(sel)), { px: x, py: y, sel: selector })
}

async function tap(page, viewport, x, y) {
  if (viewport.mobile) await page.touchscreen.tap(x, y)
  else await page.mouse.click(x, y)
}

async function clickCenter(page, viewport, selector) {
  const box = await page.locator(selector).first().boundingBox()
  if (!box) throw new Error(`missing ${selector}`)
  await tap(page, viewport, box.x + box.width / 2, box.y + box.height / 2)
}

function topTool(viewport, label) {
  return viewport.mobile
    ? `.reader-mobile-top .mobile-tool-button:has-text("${label}")`
    : `.reader-left-rail .rail-item:has-text("${label}")`
}

async function scrollTop(page) {
  return page.evaluate(() => {
    const documentScroll = Boolean(document.querySelector('.reader-shell.document-scroll'))
    const scroller = documentScroll ? (document.scrollingElement || document.documentElement) : document.querySelector('.reader-content')
    return Math.round(scroller?.scrollTop || 0)
  })
}

async function runViewport(browser, viewport) {
  const failures = []
  const near = (actual, expected, what) => {
    if (typeof actual !== 'number' || Math.abs(actual - expected) > TOLERANCE) failures.push(`${what}: expected ${expected}, got ${actual}`)
  }
  const equal = (actual, expected, what) => {
    if (JSON.stringify(actual) !== JSON.stringify(expected)) failures.push(`${what}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`)
  }
  const ok = (value, what) => {
    if (!value) failures.push(what)
  }
  const { width: W, height: H } = viewport
  const popupImage = viewport.night ? 'none' : 'popup_0.png'
  const surface = (rect, what) => {
    if (!rect) return failures.push(`${what}: missing`)
    if (viewport.night) equal(rect.backgroundColor, 'rgb(18, 18, 18)', `${what} night surface`)
    else ok(rect.backgroundImage.includes(popupImage), `${what} uses the popup texture (got ${rect.backgroundImage.slice(0, 60)})`)
    equal(rect.boxShadow, 'none', `${what} box-shadow`)
  }
  const icon = viewport.night ? 'rgb(102, 102, 102)' : 'rgb(0, 0, 0)'
  const label = viewport.night ? 'rgb(102, 102, 102)' : 'rgba(0, 0, 0, 0.4)'
  const floatIcon = viewport.night ? 'rgb(102, 102, 102)' : 'rgb(18, 18, 18)'

  const { context, page } = await openReader(browser, viewport)
  try {
    if (viewport.mobile) {
      const [top] = await rects(page, '.reader-mobile-top.visible')
      surface(top, 'top bar')
      if (top) {
        near(top.y, 0, 'top bar y'); near(top.w, W, 'top bar width'); near(top.h, 59, 'top bar height')
        equal(top.borderBottom, '0px', 'top bar border')
      }
      const tools = await rects(page, '.reader-mobile-top .mobile-tool-button')
      equal(tools.map(tool => tool.text), ['首页', '书架', '书源', '目录', '设置'], 'top tool order')
      const gap = (W - 5 * 58) / 5
      tools.forEach((tool, i) => {
        near(tool.x, gap / 2 + i * (58 + gap), `top tool ${i} x`)
        near(tool.y, -1, `top tool ${i} y`); near(tool.w, 58, `top tool ${i} width`); near(tool.h, 60, `top tool ${i} height`)
        equal(tool.color, icon, `top tool ${i} icon color`)
      })
      const labels = await rects(page, '.reader-mobile-top .icon-text')
      equal(labels.map(item => [item.color, item.fontSize]), Array(5).fill([label, '12px']), 'top tool labels')
      // Glyph and label stack from the top of each tool: glyph 22px tall at y=11, label at y=33.
      labels.forEach((item, i) => near(item.y, 33, `top tool ${i} label y`))
      const topGlyphs = await rects(page, '.reader-mobile-top .iconfont')
      equal(topGlyphs.length, 4, 'top tool glyphs')
      topGlyphs.forEach((item, i) => { near(item.y, 11, `top glyph ${i} y`); near(item.h, 22, `top glyph ${i} height`) })
      const glyph = await page.evaluate(() => {
        const node = document.querySelector('.reader-mobile-top .iconfont')
        return node ? getComputedStyle(node).fontFamily : null
      })
      equal(glyph, 'iconfont', 'top tool glyph font')

      const left = await rects(page, '.reader-mobile-float-left button')
      equal(left.length, 5, 'left float buttons')
      left.forEach((button, i) => {
        near(button.x, 20, `left float ${i} x`); near(button.y, H - 395 + 56 * i, `left float ${i} y`)
        near(button.w, 36, `left float ${i} size`)
        equal(button.color, floatIcon, `left float ${i} color`)
        surface(button, `left float ${i}`)
      })
      const right = await rects(page, '.reader-mobile-float-right button')
      right.forEach((button, i) => {
        near(button.x, W - 56, `right float ${i} x`); near(button.y, H - 135 - 20 - 56 * right.length + 20 + 56 * i, `right float ${i} y`)
        surface(button, `right float ${i}`)
      })

      const [bottom] = await rects(page, '.reader-mobile-bottom.visible')
      surface(bottom, 'bottom bar')
      if (bottom) {
        near(bottom.y, H - 103, 'bottom bar y'); near(bottom.h, 103, 'bottom bar height')
        equal(bottom.borderTop, '0px', 'bottom bar border')
      }
      const [slider] = await rects(page, '.mobile-progress-slider-row')
      if (slider) { near(slider.y, H - 103, 'slider row y'); near(slider.h, 58, 'slider row height') } else failures.push('slider row missing')
      const pages = Math.ceil(6104 / (H - 72))
      const [tip] = await rects(page, '.mobile-progress-slider-row .progress-tip')
      equal(tip?.text, `第 1/${pages} 页`, 'page tip')
      equal(tip?.color, label, 'page tip color')
      const bottomTools = await rects(page, '.reader-mobile-bottom .tools > *')
      equal(bottomTools.map(tool => tool.text), ['上一章', '阅读进度: 25%', '下一章'], 'bottom tools')
      bottomTools.forEach((tool, i) => {
        near(tool.y, H - 46, `bottom tool ${i} y`); near(tool.h, 45, `bottom tool ${i} height`)
        equal(tool.color, icon, `bottom tool ${i} color`)
      })
      const stepGlyphs = await rects(page, '.reader-mobile-bottom .mobile-chapter-step .iconfont')
      equal(stepGlyphs.length, 2, 'chapter step glyphs')
      stepGlyphs.forEach((item, i) => near(item.y, H - 46, `chapter step glyph ${i} y`))
      if (bottomTools.length === 3) {
        const half = bottomTools[0].x - 15
        near(bottomTools[1].x - (bottomTools[0].x + bottomTools[0].w), half * 2, 'bottom tools space-around')
      }

      await clickCenter(page, viewport, '.mobile-chapter-progress')
      await page.waitForTimeout(300)
      const [cache] = await rects(page, '.reader-cache-zone')
      if (cache) {
        near(cache.y, H - 85, 'cache zone y'); near(cache.h, 40, 'cache zone height'); near(cache.w, W, 'cache zone width')
        equal(cache.text, '缓存章节 后面50章 后面100章 后面全部', 'cache zone text')
      } else failures.push('cache zone missing')
      const leftAfter = await rects(page, '.reader-mobile-float-left button')
      near(leftAfter[0]?.y, H - 395, 'cache zone keeps the float buttons')
      await clickCenter(page, viewport, '.mobile-chapter-progress')
      await page.waitForTimeout(300)
    } else {
      const frame = 800
      const [rail] = await rects(page, '.reader-left-rail')
      surface(rail, 'left rail')
      if (rail) { near(rail.x, W / 2 - frame / 2 - 68, 'left rail x'); near(rail.y, 0, 'left rail y'); near(rail.w, 60, 'left rail width'); near(rail.h, 427, 'left rail height') }
      const items = await rects(page, '.reader-left-rail .rail-item')
      equal(items.map(item => item.text), ['书架', '书源', '目录', '设置', '首页', '顶部', '底部'], 'left rail order')
      items.forEach((item, i) => {
        near(item.y, -1 + 61 * i, `rail item ${i} y`); near(item.w, 60, `rail item ${i} width`); near(item.h, 62, `rail item ${i} height`)
        equal(item.color, icon, `rail item ${i} color`)
        equal(item.borderLeftColor, viewport.night ? 'rgb(68, 68, 68)' : 'rgba(0, 0, 0, 0.1)', `rail item ${i} border`)
      })
      const railLabels = await rects(page, '.reader-left-rail .icon-text')
      railLabels.forEach((item, i) => near(item.y, 34 + 61 * i, `rail label ${i} y`))
      const railGlyphs = await rects(page, '.reader-left-rail .iconfont')
      equal(railGlyphs.length, 6, 'rail glyphs (书源 is an Element icon)')
      railGlyphs.forEach((item, i) => near(item.y, 12 + 61 * [0, 2, 3, 4, 5, 6][i], `rail glyph ${i} y`))
      const rounds = await rects(page, '.reader-right-rail .round-tool')
      rounds.forEach((button, i) => {
        near(button.x, W / 2 + frame / 2 + 12, `right float ${i} x`)
        near(button.y, H - 155 - 56 * rounds.length + 20 + 56 * i, `right float ${i} y`)
        near(button.w, 36, `right float ${i} size`)
        equal(button.color, floatIcon, `right float ${i} color`)
        surface(button, `right float ${i}`)
      })
      const column = await rects(page, '.reader-page-control > button')
      equal(column.map(item => item.text), ['25%', '', ''], 'progress column')
      column.forEach((item, i) => {
        near(item.x, W / 2 + frame / 2 + 8, `progress column ${i} x`); near(item.y, H - 133 + 44 * i, `progress column ${i} y`)
        near(item.w, 44, `progress column ${i} width`); near(item.h, 45, `progress column ${i} height`)
      })
      const columnGlyphs = await rects(page, '.reader-page-control .iconfont')
      columnGlyphs.forEach((item, i) => near(item.y, H - 133 + 44 * (i + 1) + 13, `progress column glyph ${i} y`))
      await clickCenter(page, viewport, '.progress-box')
      await page.waitForTimeout(300)
      const [cache] = await rects(page, '.reader-cache-zone')
      if (cache) {
        near(cache.x, W / 2 + frame / 2 + 52 - 55 - 372, 'desktop cache x'); near(cache.y, H - 132, 'desktop cache y')
        near(cache.w, 372, 'desktop cache width'); near(cache.h, 40, 'desktop cache height')
      } else failures.push('desktop cache zone missing')
      await clickCenter(page, viewport, '.progress-box')
      await page.waitForTimeout(300)
    }

    // Primary panels.
    const panelSelector = viewport.mobile ? '.reader-mobile-workspace-primary' : '.reader-desktop-workspace'
    const panelX = viewport.mobile ? 0 : W / 2 - 400
    const panelW = viewport.mobile ? W : 799
    const inset = viewport.mobile ? 20 : 24
    const heights = viewport.mobile
      ? { 书架: 389.19, 书源: 391, 目录: 389.19, 设置: 472.8 + (H - 844) * 0.45 }
      : { 书架: 390.19, 书源: 392, 目录: 390.19, 设置: 94 + H * 0.45 }
    for (const name of ['书架', '书源', '目录', '设置']) {
      await clickCenter(page, viewport, topTool(viewport, name))
      await page.waitForTimeout(600)
      const [panel] = await rects(page, panelSelector)
      if (!panel) { failures.push(`${name} panel missing`); continue }
      surface(panel, `${name} panel`)
      near(panel.x, panelX, `${name} panel x`); near(panel.y, 0, `${name} panel y`); near(panel.w, panelW, `${name} panel width`)
      near(panel.h, heights[name], `${name} panel height`)
      const titleSelector = name === '设置' ? '.settings-title' : `${panelSelector} .title-zone .title`
      const [title] = await rects(page, titleSelector)
      near(title?.x, panelX + inset, `${name} title x`); near(title?.y, 20, `${name} title y`)
      if (viewport.mobile) ok(await hitInside(page, W / 2, 20, panelSelector), `${name} panel covers the top bar`)
      if (name === '书架') {
        equal(title?.text, '书架(6)', 'shelf title')
        const books = await rects(page, `${panelSelector} .reader-shelf-card`)
        near(books[0]?.w, panelW - 2 * inset, 'shelf item width (single column)')
        near(books[1]?.x, books[0]?.x, 'shelf is one column')
        near(books[0]?.h, 64.98, 'shelf item height')
        const [name0] = await rects(page, `${panelSelector} .reader-shelf-card.active strong`)
        equal(name0?.color, RED, 'current book name color')
        const [chapter0] = await rects(page, `${panelSelector} .reader-shelf-chapter`)
        equal(chapter0?.color, viewport.night ? 'rgb(85, 85, 85)' : 'rgb(136, 136, 136)', 'shelf chapter color')
      }
      if (name === '目录') {
        equal(title?.text, '目录 (12)', 'catalog title')
        equal((await rects(page, `${panelSelector} .el-tag`)).length, 0, 'catalog has no cache tags')
        const rows = await rects(page, `${panelSelector} .toc-item`)
        near(rows[0]?.h, 41, 'catalog row height')
        near(rows[0]?.w, viewport.mobile && W <= 500 ? panelW - 2 * inset : (panelW - 2 * inset) / 2, 'catalog row width')
        const [active] = await rects(page, `${panelSelector} .toc-item.active .log-text`)
        equal(active?.color, RED, 'current chapter color')
        // PopCatalog jumpToCurrent: the current row goes to the list top as far as the list can scroll.
        const scroll = await page.evaluate((sel) => {
          const list = document.querySelector(`${sel} .toc-list`)
          const active = list?.querySelector('.toc-item.active')
          if (!list || !active) return null
          const offset = active.getBoundingClientRect().top - list.getBoundingClientRect().top + list.scrollTop
          return { actual: list.scrollTop, expected: Math.min(offset, list.scrollHeight - list.clientHeight) }
        }, panelSelector)
        near(scroll?.actual, scroll?.expected ?? -1, 'current chapter scrolled toward the list top')
      }
      if (name === '书源') equal(title?.text, '来源(4)', 'source title')
      // Close by tapping the reading content below the panel: only the panel closes.
      const before = await scrollTop(page)
      await tap(page, viewport, Math.round(W / 2), Math.round(H * 0.72))
      await page.waitForTimeout(500)
      equal((await rects(page, panelSelector)).length, 0, `${name} panel closes on an outside tap`)
      equal(await scrollTop(page), before, `${name} outside tap does not page`)
      if (viewport.mobile) ok((await rects(page, '.reader-mobile-top.visible')).length === 1, `${name} outside tap keeps the toolbar`)
    }

    // A float button pressed while a panel is open runs and closes the panel.
    await clickCenter(page, viewport, topTool(viewport, '书架'))
    await page.waitForTimeout(500)
    const nightSelector = viewport.mobile ? '.reader-mobile-float-right button:last-child' : '.reader-right-rail .round-tool:last-child'
    await clickCenter(page, viewport, nightSelector)
    await page.waitForTimeout(700)
    equal((await rects(page, panelSelector)).length, 0, 'float button closes the open panel')
    const toggled = await page.evaluate(() => document.querySelector('.reader-shell')?.getAttribute('style') || '')
    ok(viewport.night ? !toggled.includes('--reader-chrome-text: #666') : toggled.includes('--reader-chrome-text: #666'), 'float day/night button ran while the panel was open')
    await clickCenter(page, viewport, nightSelector)
    await page.waitForTimeout(700)

    // Boundary message instead of a disabled chapter button.
    await page.evaluate(() => {
      const router = document.querySelector('#app').__vue_app__.config.globalProperties.$router
      return router.push({ query: { ...router.currentRoute.value.query, chapter: '0' } })
    })
    await page.waitForSelector('.chapter-content[data-index="0"]', { timeout: 10_000 })
    await page.waitForTimeout(500)
    const previous = viewport.mobile ? '.reader-mobile-bottom .mobile-chapter-step' : '.reader-page-control .chapter-step'
    equal(await page.locator(previous).first().isDisabled(), false, 'previous chapter button stays enabled')
    await clickCenter(page, viewport, previous)
    await page.waitForTimeout(300)
    const toast = await page.locator('.reader-toast').first().textContent().catch(() => '')
    equal(String(toast).trim(), '本章是第一章', 'first chapter message')
    ok(new URL(page.url()).searchParams.get('chapter') === '0', 'previous at the first chapter keeps the route')

    // 显示翻页区域: the overlay lets taps through.
    await clickCenter(page, viewport, topTool(viewport, '设置'))
    await page.waitForTimeout(500)
    await page.getByText('显示翻页区域', { exact: true }).first().click()
    await page.waitForTimeout(400)
    const pieces = await rects(page, '.click-zone-overlay > *')
    equal(pieces.map(piece => piece.text), ['点击前一页', '点击显示菜单', '点击后一页', '关闭'], 'click zone labels')
    const [overlay] = await rects(page, '.click-zone-overlay')
    equal(overlay?.pointerEvents, 'none', 'click zone overlay lets taps through')
    if (overlay) { near(overlay.w, W, 'click zone width'); near(overlay.h, H, 'click zone height') }
    const closeBar = pieces[3]
    if (closeBar) { near(closeBar.y, H - 65, 'click zone close bar y'); near(closeBar.h, 45, 'click zone close bar height') }
    // Lower half, outside the menu box and clear of the float buttons.
    const beforeTap = await scrollTop(page)
    await tap(page, viewport, Math.round(W / 2), Math.round(H * 0.75))
    await page.waitForTimeout(500)
    ok(await scrollTop(page) > beforeTap, 'a tap on the lower half pages through the overlay')
    await page.locator('.click-zone-close').click()
    await page.waitForTimeout(300)
    equal((await rects(page, '.click-zone-overlay')).length, 0, 'click zone closes')
  } catch (error) {
    failures.push(`harness: ${String(error?.message || error).split('\n')[0]}`)
  } finally {
    await context.close()
  }
  return failures
}

const browser = await openSmokeBrowser()
let failed = 0
try {
  for (const viewport of VIEWPORTS) {
    if (viewportFilter && !viewportFilter.test(viewport.label)) continue
    const failures = await runViewport(browser, viewport)
    if (failures.length) {
      failed += failures.length
      console.error(`reader-menu ${viewport.label}: ${failures.length} failed`)
      for (const failure of failures) console.error(`  - ${failure}`)
    } else {
      console.log(`reader-menu ${viewport.label}: ok`)
    }
  }
} finally {
  await browser.close()
}
if (failed) {
  console.error(`reader-menu-upstream-contract: ${failed} failed checks`)
  process.exit(1)
}
console.log('reader-menu-upstream-contract: ok')
