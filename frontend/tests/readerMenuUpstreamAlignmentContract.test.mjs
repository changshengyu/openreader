import assert from 'node:assert/strict'
import { existsSync, readFileSync, statSync } from 'node:fs'
import test from 'node:test'
import { reactive, ref } from 'vue'
import * as pagination from '../src/utils/readerPagination.js'
import { useReaderLayout } from '../src/composables/useReaderLayout.js'
import { useReaderProgressControls } from '../src/composables/useReaderProgressControls.js'
import { useReaderTools } from '../src/composables/useReaderTools.js'

// docs/compat/reader-menu-chrome-panels-fixed-baseline-second-audit-p0-contract.md: the Reader menu bars, their
// panels and their logic follow reader-dev's Reader.vue, BookShelf.vue, BookSource.vue and PopCatalog.vue as measured
// on the fixed upstream build. User decisions: D1 the catalog drops its ordinal line and cache tags; D2 the desktop
// close button stays, outside the panel.
const source = path => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8')
const mobileChrome = source('components/reader/ReaderMobileChrome.vue')
const desktopTools = source('components/reader/ReaderDesktopTools.vue')
const desktopProgress = source('components/reader/ReaderDesktopProgress.vue')
const cachePanel = source('components/reader/ReaderCachePanel.vue')
const mobileWorkspace = source('components/reader/ReaderMobileWorkspacePanel.vue')
const desktopWorkspace = source('components/reader/ReaderDesktopWorkspacePanel.vue')
const tocPanel = source('components/reader/ReaderTocPanel.vue')
const shelfPanel = source('components/reader/ReaderShelfPanel.vue')
const clickZones = source('components/reader/ReaderClickZones.vue')
const readerView = source('views/Reader.vue')
const mainSource = source('main.js')

function rule(src, selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const bodies = [...src.matchAll(new RegExp(`(?:^|\\n)${escaped} \\{([^}]*)\\}`, 'g'))].map(match => match[1])
  assert.ok(bodies.length, `missing CSS rule ${selector}`)
  return bodies.join('\n')
}

function zIndex(src, selector) {
  const value = rule(src, selector).match(/z-index: (\d+);/)
  assert.ok(value, `${selector} must declare its z-index`)
  return Number(value[1])
}

function template(src) {
  return src.slice(src.indexOf('<template>'), src.lastIndexOf('</template>'))
}

function block(src, startPattern) {
  const start = src.search(startPattern)
  assert.ok(start >= 0, `missing ${startPattern}`)
  const tag = src.slice(start).match(/^<([a-z-]+)/)[1]
  const end = src.indexOf(`</${tag}>`, start)
  return src.slice(start, end)
}

test('reader-dev chapter progress is the integer chapter percentage', () => {
  assert.equal(typeof pagination.readerChapterProgressLabel, 'function')
  assert.equal(pagination.readerChapterProgressLabel({ chapterIndex: 2, totalChapters: 12 }), '25%')
  assert.equal(pagination.readerChapterProgressLabel({ chapterIndex: 0, totalChapters: 3 }), '33%')
  assert.equal(pagination.readerChapterProgressLabel({ chapterIndex: 11, totalChapters: 12 }), '100%')
  assert.equal(pagination.readerChapterProgressLabel({ chapterIndex: 0, totalChapters: 0 }), '')
})

test('reader-dev vertical pages use the content height and the two-line scroll offset', () => {
  assert.equal(typeof pagination.readerVerticalPageCount, 'function')
  assert.equal(typeof pagination.readerVerticalCurrentPage, 'function')
  assert.equal(typeof pagination.readerVerticalPageScrollTop, 'function')
  const typography = { fontSize: 18, lineHeight: 1.8, paragraphSpace: 0.2 }
  // Fixed-upstream fixture: 6104px of content in an 844px window gives 第 1/8 页.
  assert.equal(pagination.readerVerticalPageCount({ contentHeight: 6104, viewportHeight: 844, ...typography }), 8)
  assert.equal(pagination.readerVerticalPageCount({ contentHeight: 6104, viewportHeight: 800, ...typography }), 9)
  assert.equal(pagination.readerVerticalPageCount({ contentHeight: 0, viewportHeight: 844, ...typography }), 1)
  const current = scrollTop => pagination.readerVerticalCurrentPage({ scrollTop, viewportHeight: 844, pageCount: 8, ...typography })
  assert.equal(current(0), 1)
  assert.equal(current(2316), 4)
  assert.equal(current(5418), 8)
  assert.equal(current(99999), 8)
  assert.equal(pagination.readerVerticalPageScrollTop({ page: 1, viewportHeight: 844 }), 0)
  assert.equal(pagination.readerVerticalPageScrollTop({ page: 3, viewportHeight: 844 }), 1668)
})

test('reader-dev readWidthConfig steps the reading width down by 20px', () => {
  assert.equal(typeof pagination.readerReadWidthConfig, 'function')
  assert.equal(pagination.readerReadWidthConfig({ readWidth: 800, windowWidth: 1440 }), 800)
  assert.equal(pagination.readerReadWidthConfig({ readWidth: 800, windowWidth: 1024 }), 800)
  assert.equal(pagination.readerReadWidthConfig({ readWidth: 800, windowWidth: 900 }), 760)
  assert.equal(pagination.readerReadWidthConfig({ readWidth: 960, windowWidth: 1000 }), 860)
  assert.match(readerView, /readerReadWidthConfig\(/)
  assert.match(rule(readerView, '.reader-shell'), /--reader-frame-width: var\(--reader-read-width, 800px\);/)
})

test('the reader layout counts vertical pages like reader-dev computePages and scrollHandler', () => {
  const block = (top, height) => ({ getBoundingClientRect: () => ({ top, bottom: top + height, height }) })
  const contentEl = ref({ clientWidth: 390, clientHeight: 844, scrollHeight: 6262, scrollTop: 0 })
  const state = {
    reader: reactive({ mode: 'page', fontSize: 18, lineHeight: 1.8, paragraphSpace: 0.2 }),
    contentEl,
    contentBody: ref({
      scrollWidth: 390,
      scrollHeight: 6262,
      querySelectorAll: selector => (selector === '.chapter-content' ? [block(73, 6104)] : []),
    }),
    page: ref(0),
    pageCount: ref(1),
    pageWidth: ref(0),
    pageHeight: ref(0),
    windowWidth: ref(390),
  }
  const controller = useReaderLayout({
    ...state,
    windowTarget: {
      innerWidth: 390,
      innerHeight: 844,
      getComputedStyle: () => ({ paddingLeft: '0px', paddingRight: '0px', paddingTop: '0px', paddingBottom: '0px' }),
    },
    getScrollStep: () => 772,
    getViewportWidth: () => 390,
  })
  controller.update()
  assert.equal(state.pageCount.value, 8)
  assert.equal(state.page.value, 0)
  contentEl.value.scrollTop = 2316
  controller.update()
  assert.equal(state.page.value, 3)
  contentEl.value.scrollTop = 5418
  controller.update()
  assert.equal(state.page.value, 7)
})

test('the bottom bar shows reader-dev progress and seeks pages like showPage', () => {
  const animations = []
  const controls = useReaderProgressControls({
    contentEl: ref({ scrollTop: 0, scrollHeight: 6262, clientHeight: 844 }),
    contentBody: ref(null),
    chapters: ref(Array.from({ length: 12 }, (_, id) => ({ id }))),
    currentIndex: ref(2),
    page: ref(0),
    pageCount: ref(8),
    progressVersion: ref(0),
    isContinuousScrollRead: ref(false),
    windowTarget: { innerHeight: 844 },
    getMode: () => 'page',
    getAnimateDuration: () => 300,
    getCurrentChapterPercent: () => 0.5,
    navigate: async () => {},
    applyLocalProgress: () => {},
    saveProgress: () => {},
    scheduleProgressSave: () => {},
    scrollAnimator: {
      cancel: () => {},
      scrollTo: (element, top, duration, onFinish) => {
        animations.push({ top, duration })
        element.scrollTop = top
        onFinish?.()
        return true
      },
    },
  })
  assert.equal(controls.bookProgressLabel.value, '25%')
  controls.handleMobilePageProgressChange({ target: { value: '3' } })
  assert.deepEqual(animations, [{ top: 1668, duration: 300 }])
})

test('chapter buttons stay enabled and report the reader-dev boundary messages', () => {
  const calls = []
  const notices = []
  const currentIndex = ref(0)
  const tools = useReaderTools({
    currentIndex,
    chapterCount: () => 12,
    mobileChromeVisible: ref(true),
    goChapter: index => calls.push(['chapter', index]),
    toggleChrome: () => calls.push(['toggle']),
    notify: message => notices.push(message),
    closePrimaryPanels: () => calls.push(['close']),
    actions: {},
  })
  tools.handleMobileChromeAction('previous')
  tools.handleDesktopToolAction('previous')
  currentIndex.value = 11
  tools.handleMobileChromeAction('next')
  tools.handleDesktopToolAction('next')
  assert.deepEqual(notices, ['本章是第一章', '本章是第一章', '本章是最后一章', '本章是最后一章'])
  assert.deepEqual(calls.filter(call => call[0] === 'chapter'), [])
  tools.handleDesktopToolAction('previous')
  currentIndex.value = 4
  tools.handleMobileChromeAction('next')
  assert.deepEqual(calls.filter(call => call[0] === 'chapter'), [['chapter', 10], ['chapter', 5]])

  assert.doesNotMatch(template(mobileChrome), /:disabled=/)
  assert.doesNotMatch(template(desktopProgress), /:disabled=/)
})

test('a float or bottom action closes an open primary panel, like the reader-dev popover document click', () => {
  const calls = []
  const tools = useReaderTools({
    currentIndex: ref(3),
    chapterCount: () => 12,
    mobileChromeVisible: ref(true),
    goChapter: index => calls.push(['chapter', index]),
    toggleChrome: () => calls.push(['toggle']),
    notify: () => {},
    closePrimaryPanels: () => calls.push(['close']),
    actions: {
      shelf: () => calls.push(['shelf']),
      night: () => calls.push(['night']),
      bookmarks: () => calls.push(['bookmarks']),
      cache: () => calls.push(['cache']),
    },
  })
  tools.handleMobileChromeAction('night')
  tools.handleMobileChromeAction('bookmarks')
  tools.handleMobileChromeAction('cache')
  tools.handleMobileChromeAction('next')
  tools.handleDesktopToolAction('bookmarks')
  tools.handleMobileChromeAction('shelf')
  assert.deepEqual(calls, [
    ['close'], ['night'],
    ['close'], ['bookmarks'],
    ['close'], ['cache'],
    ['close'], ['chapter', 4],
    ['close'], ['bookmarks'],
    ['shelf'],
  ])
})

test('the toolbar glyphs come from the reader-dev iconfont', () => {
  const font = new URL('../src/assets/fonts/iconfont.woff', import.meta.url)
  assert.ok(existsSync(font), 'the reader-dev iconfont.woff must ship with the Reader')
  assert.equal(statSync(font).size, 30972, 'iconfont.woff must be the fixed-upstream file')
  const cssUrl = new URL('../src/styles/reader-iconfont.css', import.meta.url)
  assert.ok(existsSync(cssUrl), 'the iconfont @font-face must be declared')
  const css = source('styles/reader-iconfont.css')
  assert.match(css, /font-family: "iconfont";\s*src: url\("\.\.\/assets\/fonts\/iconfont\.woff"\) format\("woff"\);/)
  assert.match(mainSource, /import '\.\/styles\/reader-iconfont\.css'/)

  const top = block(template(mobileChrome), /<header class="reader-mobile-top/)
  const topOrder = [...top.matchAll(/'action', '([a-z]+)'/g)].map(match => match[1])
  assert.deepEqual(topOrder, ['home', 'shelf', 'source', 'toc', 'settings'])
  const glyphs = src => [...src.matchAll(/<span class="iconfont">&#(\d+);<\/span>/g)].map(match => Number(match[1]))
  assert.deepEqual(glyphs(top), [58920, 58892, 58905, 58971])
  assert.match(top, /<el-icon><Menu \/><\/el-icon>/)

  const rail = block(template(desktopTools), /<aside class="reader-left-rail/)
  assert.deepEqual([...rail.matchAll(/'action', '([a-z]+)'/g)].map(match => match[1]), ['shelf', 'source', 'toc', 'settings', 'home', 'top', 'bottom'])
  assert.deepEqual(glyphs(rail), [58892, 58905, 58971, 58920, 58914, 58915])
  assert.deepEqual(glyphs(template(desktopProgress)), [58920, 58913])
  assert.deepEqual(glyphs(block(template(mobileChrome), /<footer class="reader-mobile-bottom/)), [58920, 58913])

  for (const src of [mobileChrome, desktopTools]) {
    for (const stale of ['ArrowUpBold', 'ArrowDownBold', 'Notebook', 'Grid', 'Setting,', 'List,']) {
      assert.ok(!src.includes(stale), `${stale} is not a reader-dev toolbar icon`)
    }
    assert.match(src, /\bTop,/)
    assert.match(src, /\bBottom,/)
  }
})

test('the mobile toolbar, float buttons and bottom bar have the reader-dev geometry', () => {
  const top = rule(mobileChrome, '.reader-mobile-top.visible')
  for (const line of ['top: 0;', 'left: 0;', 'width: 100vw;', 'background-image: var(--reader-popup-bg-image);']) {
    assert.ok(top.includes(line), `.reader-mobile-top.visible needs ${line}`)
  }
  assert.doesNotMatch(top, /border|box-shadow|color-mix/)
  assert.match(rule(mobileChrome, '.reader-mobile-top .tools'), /display: flex;[\s\S]*justify-content: space-around;/)
  const tool = rule(mobileChrome, '.mobile-tool-button')
  for (const line of ['box-sizing: content-box;', 'width: 58px;', 'height: 48px;', 'margin: -1px 0 0;', 'padding: 12px 0 0;']) {
    assert.ok(tool.includes(line), `.mobile-tool-button needs ${line}`)
  }

  assert.match(rule(mobileChrome, '.reader-mobile-float-tools.visible'), /bottom: 135px;/)
  assert.match(rule(mobileChrome, '.reader-mobile-float-left'), /left: 20px;/)
  assert.match(rule(mobileChrome, '.reader-mobile-float-right'), /right: 20px;[\s\S]*margin-bottom: 20px;/)
  const float = rule(mobileChrome, '.reader-mobile-float-tools button')
  for (const line of ['width: 36px;', 'height: 36px;', 'margin: 20px 0 0;', 'border: 0;', 'border-radius: 100%;', 'background-image: var(--reader-popup-bg-image);']) {
    assert.ok(float.includes(line), `float buttons need ${line}`)
  }
  assert.doesNotMatch(float, /box-shadow|color-mix/)
  assert.ok(!mobileChrome.includes('cache-zone-visible'), 'the cache zone must not move the float buttons')

  const bottom = rule(mobileChrome, '.reader-mobile-bottom.visible')
  assert.match(bottom, /background-image: var\(--reader-popup-bg-image\);/)
  assert.doesNotMatch(bottom, /border|box-shadow|color-mix|grid/)
  assert.match(rule(mobileChrome, '.mobile-progress-slider-row'), /padding: 10px 36px;/)
  const tools = rule(mobileChrome, '.reader-mobile-bottom .tools')
  for (const line of ['display: flex;', 'justify-content: space-around;', 'height: 45px;', 'padding: 0 15px;']) {
    assert.ok(tools.includes(line), `bottom tools need ${line}`)
  }
  const cache = rule(cachePanel, '.reader-cache-zone')
  for (const line of ['display: flex;', 'justify-content: space-between;', 'padding: 10px 36px;']) {
    assert.ok(cache.includes(line), `the cache zone needs ${line}`)
  }
  assert.doesNotMatch(cachePanel, /grid-template-columns/)
})

test('the desktop rails sit beside the reader-dev readWidthConfig frame', () => {
  const rail = rule(desktopTools, '.reader-left-rail')
  for (const line of ['left: 50%;', 'margin-left: calc(var(--reader-frame-width) / -2 - 68px);', 'background-image: var(--reader-popup-bg-image);']) {
    assert.ok(rail.includes(line), `.reader-left-rail needs ${line}`)
  }
  assert.doesNotMatch(rail, /bottom: 0|backdrop-filter|color-mix|border/)
  const item = rule(desktopTools, '.rail-item')
  for (const line of ['box-sizing: content-box;', 'width: 58px;', 'height: 48px;', 'margin: -1px 0 0;', 'padding: 12px 0 0;', 'border: 1px solid var(--reader-chrome-border, rgba(0, 0, 0, 0.1));']) {
    assert.ok(item.includes(line), `.rail-item needs ${line}`)
  }
  assert.doesNotMatch(desktopTools, /\.rail-item\.active|\.rail-item:hover|\.round-tool:hover|\.round-tool\.active|active: activePanel/)

  const right = rule(desktopTools, '.reader-right-rail')
  for (const line of ['left: 50%;', 'margin-left: calc(var(--reader-frame-width) / 2 + 12px);', 'bottom: 135px;', 'padding-bottom: 20px;']) {
    assert.ok(right.includes(line), `.reader-right-rail needs ${line}`)
  }
  const round = rule(desktopTools, '.round-tool')
  for (const line of ['width: 36px;', 'height: 36px;', 'margin: 20px 0 0;', 'border: 0;', 'border-radius: 100%;', 'background-image: var(--reader-popup-bg-image);']) {
    assert.ok(round.includes(line), `.round-tool needs ${line}`)
  }
  assert.doesNotMatch(round, /box-shadow/)

  const column = rule(desktopProgress, '.reader-page-control')
  for (const line of ['left: 50%;', 'margin-left: calc(var(--reader-frame-width) / 2 + 8px);', 'background-image: var(--reader-popup-bg-image);']) {
    assert.ok(column.includes(line), `.reader-page-control needs ${line}`)
  }
  const step = rule(desktopProgress, '.progress-box,\n.page-step')
  for (const line of ['box-sizing: content-box;', 'width: 42px;', 'height: 31px;', 'margin: -1px 0 0;', 'padding: 12px 0 0;', 'border: 1px solid var(--reader-chrome-border, rgba(0, 0, 0, 0.1));']) {
    assert.ok(step.includes(line), `progress column items need ${line}`)
  }
  assert.match(rule(desktopProgress, '.desktop-cache-zone'), /top: 0;[\s\S]*right: 55px;/)
})

test('mobile primary panels cover the toolbar while float buttons and the bottom bar stay above the dismiss layer', () => {
  const panel = rule(mobileWorkspace, '.reader-mobile-workspace-primary')
  for (const line of ['top: 0;', 'left: 0;', 'padding: calc(20px + env(safe-area-inset-top)) 20px 23px;', 'color: #606266;', 'font-family: initial;', 'line-height: 1.4;', 'background-image: var(--reader-popup-bg-image);']) {
    assert.ok(panel.includes(line), `.reader-mobile-workspace-primary needs ${line}`)
  }
  assert.doesNotMatch(panel, /58px|color-mix|backdrop-filter/)
  const dismiss = zIndex(readerView, '.reader-shell.mini-interface .reader-mobile-primary-dismiss')
  const floats = zIndex(mobileChrome, '.reader-mobile-float-tools.visible')
  const bottom = zIndex(mobileChrome, '.reader-mobile-bottom.visible')
  const top = zIndex(mobileChrome, '.reader-mobile-top.visible')
  const primary = zIndex(mobileWorkspace, '.reader-mobile-workspace-primary')
  assert.ok(dismiss < floats && dismiss < bottom, 'float buttons and the bottom bar stay clickable while a panel is open')
  assert.ok(floats < top && bottom < top && top < primary, 'an open panel covers the toolbar')
})

test('desktop primary panels span the reader-dev popover face and keep the close button outside', () => {
  const panel = rule(desktopWorkspace, '.reader-desktop-workspace')
  for (const line of ['top: 0;', 'left: 50%;', 'margin-left: calc(var(--reader-frame-width) / -2);', 'width: calc(var(--reader-frame-width) - 1px);', 'padding: 20px 24px 24px;', 'color: #606266;', 'font-family: initial;', 'line-height: 1.4;', 'background-image: var(--reader-popup-bg-image);']) {
    assert.ok(panel.includes(line), `.reader-desktop-workspace needs ${line}`)
  }
  assert.doesNotMatch(panel, /filter|border-left|border-right|box-shadow/)
  const section = block(template(desktopWorkspace), /<section/)
  assert.ok(!section.includes('reader-desktop-workspace-close'), 'the close button is outside the panel')
  assert.ok(!section.includes('reader-workspace-head'), 'shelf and catalog draw their own title zone')
  const close = rule(desktopWorkspace, '.reader-desktop-workspace-close')
  for (const line of ['position: fixed;', 'top: 8px;', 'left: 50%;', 'margin-left: calc(var(--reader-frame-width) / 2 + 8px);', 'width: 44px;', 'height: 44px;']) {
    assert.ok(close.includes(line), `the close button needs ${line}`)
  }
})

test('the catalog and shelf lists follow PopCatalog.vue and BookShelf.vue', () => {
  const toc = template(tocPanel)
  assert.doesNotMatch(toc, /el-tag|<small/)
  assert.match(toc, /目录 <span v-if="chapters\.length">\(\{\{ chapters\.length \}\}\)<\/span>/)
  const item = rule(tocPanel, '.toc-item')
  for (const line of ['box-sizing: content-box;', 'width: 50%;', 'height: 40px;', "font: 16px / 40px PingFangSC-Regular, HelveticaNeue-Light, 'Helvetica Neue Light', 'Microsoft YaHei', sans-serif;", 'border-bottom: 1px solid var(--reader-list-border, #eee);']) {
    assert.ok(item.includes(line), `.toc-item needs ${line}`)
  }
  assert.match(rule(tocPanel, '.toc-item .log-text'), /margin-right: 26px;/)
  assert.match(rule(tocPanel, '.toc-item.active .log-text'), /color: #eb4259;/)
  assert.match(rule(tocPanel, '.toc-item .log-text.cached'), /color: var\(--reader-list-cached-text, #bbb\);/)
  assert.match(tocPanel, /@media \(max-width: 500px\) \{[\s\S]*?\.toc-item \{[\s\S]*?width: 100%;/)
  assert.doesNotMatch(tocPanel, /\.toc-item:hover|desktop-grid/)

  const shelf = template(shelfPanel)
  assert.match(shelf, /书架\(\{\{ books\.length \}\}\)/)
  assert.ok(!shelf.includes('尚未阅读'))
  assert.doesNotMatch(shelfPanel, /repeat\(4|\.reader-shelf-card:hover/)
  assert.match(rule(shelfPanel, '.reader-shelf-card.active strong'), /color: #eb4259;/)
  assert.match(rule(shelfPanel, '.reader-shelf-chapter'), /margin-top: 6px;[\s\S]*color: var\(--reader-shelf-chapter-text, #888\);/)
  assert.match(rule(shelfPanel, '.reader-shelf-list'), /height: 300px;[\s\S]*gap: 16px;/)
})

test('显示翻页区域 is the reader-dev click-through overlay', () => {
  const overlay = block(template(clickZones), /<div v-if="showOverlay"/)
  assert.deepEqual([...overlay.matchAll(/<span>([^<]+)<\/span>/g)].map(match => match[1]), ['点击前一页', '点击显示菜单', '点击后一页'])
  const root = rule(clickZones, '.click-zone-overlay')
  for (const line of ['position: fixed;', 'background: #333;', 'opacity: 0.8;', 'color: #fff;', 'font-size: 14px;', 'pointer-events: none;']) {
    assert.ok(root.includes(line), `.click-zone-overlay needs ${line}`)
  }
  assert.match(rule(clickZones, '.click-zone-prev'), /background: #43987324;/)
  assert.match(rule(clickZones, '.click-zone-next'), /background: #6b1a7324;/)
  assert.match(rule(clickZones, '.click-zone-menu'), /background: #636060;/)
  const close = rule(clickZones, '.click-zone-close')
  for (const line of ['bottom: 20px;', 'height: 45px;', 'pointer-events: all;']) {
    assert.ok(close.includes(line), `the overlay close bar needs ${line}`)
  }
})

test('Kindle day chrome and the mobile running title use the reader-dev tip colors', () => {
  const kindle = readerView.match(/const KINDLE_DAY_CHROME_STYLE = Object\.freeze\(\{([\s\S]*?)\}\)/)
  assert.ok(kindle, 'Reader.vue must define the reader-dev .kindle-page day chrome')
  assert.match(kindle[1], /'--reader-chrome-label': '#444'/)
  assert.match(kindle[1], /'--reader-chrome-border': '#fefefefe'/)
  const head = rule(readerView, '.reader-shell.mini-interface .reader-page-head')
  assert.match(head, /color: var\(--reader-chrome-label, var\(--reader-chrome-text, rgba\(0, 0, 0, 0\.4\)\)\);/)
  assert.doesNotMatch(head, /opacity/)
})
