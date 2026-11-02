import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import test from 'node:test'

// 2026-10-30 user decision (docs/compat/night-element-plus-theme-fixed-baseline-second-audit-p1-contract.md):
// reader-dev App.vue#setTheme hangs its Element night overrides (App.vue 939–1141) off body.night-theme for
// every page and dialog. The expected values are reader-dev's rendered results, measured on Element UI 2.15
// in reader-dev's bundle order (Element's own rules load later and win ties), mapped onto Element Plus.
const sourceUrl = path => new URL(`../src/${path}`, import.meta.url)
const source = path => readFileSync(sourceUrl(path), 'utf8')
const NIGHT_CSS = 'styles/night-element.css'

const normalizeSelector = selector => selector
  .replace(/\s+/g, ' ')
  .replace(/\s*([,>()])\s*/g, '$1')
  .trim()

// Top-level rules of a plain stylesheet: [{ selectors, declarations }].
function parseRules(css) {
  const text = css.replace(/\/\*[\s\S]*?\*\//g, '')
  assert.doesNotMatch(text, /@/, 'the night stylesheet must not need at-rules')
  const rules = []
  for (const match of text.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    const selectors = []
    let depth = 0
    let current = ''
    for (const char of match[1]) {
      if (char === '(') depth += 1
      if (char === ')') depth -= 1
      if (char === ',' && depth === 0) {
        selectors.push(normalizeSelector(current))
        current = ''
      } else {
        current += char
      }
    }
    selectors.push(normalizeSelector(current))
    const declarations = {}
    for (const declaration of match[2].split(';')) {
      const colon = declaration.indexOf(':')
      if (colon < 0) continue
      declarations[declaration.slice(0, colon).trim()] = declaration.slice(colon + 1).trim().replace(/\s+/g, ' ')
    }
    rules.push({ selectors, declarations })
  }
  return rules
}

function nightRules() {
  assert.ok(existsSync(sourceUrl(NIGHT_CSS)), `src/${NIGHT_CSS} must exist`)
  return parseRules(source(NIGHT_CSS))
}

// Declarations of the rule(s) whose selector list contains every given selector.
function declarationsFor(...selectors) {
  const wanted = selectors.map(normalizeSelector)
  const matches = nightRules().filter(rule => wanted.every(selector => rule.selectors.includes(selector)))
  assert.ok(matches.length, `missing night rule for ${wanted.join(', ')}`)
  return Object.assign({}, ...matches.map(rule => rule.declarations))
}

function assertDeclarations(selectors, expected) {
  const declarations = declarationsFor(...[selectors].flat())
  for (const [property, value] of Object.entries(expected)) {
    assert.equal(declarations[property], value, `${[selectors].flat().join(', ')} { ${property} }`)
  }
}

test('control: day styling and existing night surfaces stay as they are', () => {
  const app = source('App.vue')
  assert.match(app, /document\.documentElement\.classList\.toggle\('dark-reader', themeType === 'night'\)/)
  assert.doesNotMatch(source('styles/tokens.css'), /--el-/)
  // Already reader-dev aligned (Index.vue `.night`, Index.vue:1264, BookInfo.vue:76).
  assert.match(source('styles/home-shelf.css'), /html\.dark-reader \.shelf-page \.shelf-search-wrapper \.el-input__wrapper \{\n  background: #444;/)
  assert.match(source('views/Home.vue'), /const shelfLoadingBackground = computed\(\(\) => reader\.themeType === 'night' \? '#222' : '#fff'\)/)
  assert.match(source('components/BookInfoPanel.vue'), /:effect="isNight \? 'dark' : 'light'"/)
})

test('App.vue toggles body.night-theme with the night theme type, except on the login page', () => {
  const app = source('App.vue')
  const darkReader = app.match(/watch\(\n  \(\) => readerStore\.themeType,\n  \(themeType\) => \{([\s\S]*?)\n  \},\n  \{ immediate: true \},\n\)/)
  assert.ok(darkReader, 'App.vue must keep its immediate themeType watcher')
  assert.match(darkReader[1], /document\.documentElement\.classList\.toggle\('dark-reader', themeType === 'night'\)/)
  // D6: the standalone login page keeps its light design; the in-app login dialog follows night.
  const nightTheme = app.match(/watch\(\n  \(\) => readerStore\.themeType === 'night' && !isLoginRoute\.value,\n  \(night\) => \{([\s\S]*?)\n  \},\n  \{ immediate: true \},\n\)/)
  assert.ok(nightTheme, 'App.vue must watch the night theme outside the login route')
  assert.match(nightTheme[1], /document\.body\.classList\.toggle\('night-theme', night\)/)
})

test('the night stylesheet loads after Element Plus and only styles under .night-theme', () => {
  const main = source('main.js')
  const elementCss = main.indexOf("import 'element-plus/dist/index.css'")
  const nightCss = main.indexOf("import './styles/night-element.css'")
  assert.ok(elementCss >= 0, 'main.js must import Element Plus CSS')
  assert.ok(nightCss > elementCss, 'main.js must import the night stylesheet after Element Plus CSS')
  // reader-dev bundles App.vue's night overrides before its components, so component styles win ties.
  assert.ok(nightCss < main.indexOf("import App from './App.vue'"), 'main.js must import the night stylesheet before App.vue')
  const rules = nightRules()
  assert.ok(rules.length > 10, 'the night stylesheet must carry the night-theme rules')
  for (const rule of rules) {
    for (const selector of rule.selectors) {
      assert.ok(selector === '.night-theme' || selector.startsWith('.night-theme '), `unscoped night rule: ${selector}`)
    }
  }
})

test('page background, message boxes and dialogs take reader-dev night colors', () => {
  assertDeclarations('.night-theme', { 'background-color': '#222', '--check-tip-text': '#bbb' })
  assertDeclarations('.night-theme .el-message-box', {
    '--el-messagebox-title-color': '#888',
    '--el-messagebox-content-color': '#777',
    'background-color': '#212121',
    border: '1px solid #212121',
    // Native controls inside message bodies (the EPUB catalog rule picker) follow the night inputs.
    '--el-input-bg-color': '#444',
    '--el-input-text-color': '#ddd',
    '--el-input-border-color': '#444',
  })
  assertDeclarations('.night-theme .el-dialog', { '--el-dialog-bg-color': '#222' })
  assertDeclarations('.night-theme .el-dialog__title', { color: '#bbb' })
})

test('buttons follow reader-dev night colors, including its cascade results', () => {
  assertDeclarations('.night-theme .el-button', { '--el-button-disabled-text-color': '#666' })
  const plainButtons = '.night-theme .el-button:not(.el-button--primary, .el-button--success, .el-button--warning, .el-button--danger, .el-button--info, .is-text, .is-link)'
  assertDeclarations(plainButtons, {
    '--el-button-text-color': '#ddd',
    '--el-button-bg-color': '#888',
    '--el-button-border-color': '#888',
    '--el-button-hover-text-color': '#eee',
    '--el-button-hover-bg-color': '#bbb',
    '--el-button-hover-border-color': '#bbb',
    '--el-button-active-text-color': '#eee',
    '--el-button-active-bg-color': '#bbb',
    '--el-button-active-border-color': '#bbb',
  })
  assertDeclarations('.night-theme .el-button--primary:not(.is-plain, .is-text, .is-link)', {
    '--el-button-bg-color': '#185798',
    '--el-button-border-color': '#185798',
    '--el-button-hover-bg-color': '#2b67bb',
    '--el-button-hover-border-color': '#2b67bb',
    '--el-button-active-bg-color': '#2b67bb',
    '--el-button-active-border-color': '#2b67bb',
  })
  // D4: reader-dev's generic `.el-button:hover` outranks Element's typed hovers.
  assertDeclarations('.night-theme :is(.el-button--success, .el-button--warning, .el-button--danger, .el-button--info):not(.is-plain, .is-text, .is-link)', {
    '--el-button-hover-text-color': '#eee',
    '--el-button-hover-bg-color': '#bbb',
    '--el-button-hover-border-color': '#bbb',
    '--el-button-active-text-color': '#eee',
    '--el-button-active-bg-color': '#bbb',
    '--el-button-active-border-color': '#bbb',
  })
  // reader-dev styles :focus like :hover.
  assertDeclarations('.night-theme .el-button:not(.is-text, .is-link, .is-disabled):focus', {
    color: 'var(--el-button-hover-text-color)',
    'background-color': 'var(--el-button-hover-bg-color)',
    'border-color': 'var(--el-button-hover-border-color)',
  })
  assertDeclarations([
    '.night-theme .el-button.is-text:not(.is-disabled):is(:hover, :focus)',
    '.night-theme .el-button.is-link:not(.is-disabled):is(:hover, :focus)',
  ], { color: '#66b1ff', 'background-color': 'transparent', 'border-color': 'transparent' })
  // D4: disabled buttons keep Element's surface (reader-dev's Element `.is-disabled` rule wins the tie).
  for (const rule of nightRules()) {
    assert.equal(rule.declarations['--el-button-disabled-bg-color'], undefined, 'disabled button surface must stay Element default')
    assert.equal(rule.declarations['--el-button-disabled-border-color'], undefined, 'disabled button border must stay Element default')
  }
})

test('inputs, selects and their dropdowns take reader-dev night colors', () => {
  assertDeclarations(['.night-theme .el-input', '.night-theme .el-textarea'], {
    '--el-input-bg-color': '#444',
    '--el-input-text-color': '#ddd',
    '--el-input-border-color': '#444',
    '--el-input-hover-border-color': '#444',
    '--el-input-focus-border-color': '#444',
    '--el-disabled-border-color': '#444',
  })
  assertDeclarations('.night-theme .el-select', {
    '--el-input-text-color': '#ddd',
    '--el-select-multiple-input-color': '#ddd',
    '--el-select-disabled-border': '#444',
  })
  assertDeclarations('.night-theme .el-select__wrapper:not(.is-disabled)', { 'background-color': '#444' })
  assertDeclarations([
    '.night-theme .el-select__wrapper',
    '.night-theme .el-select__wrapper.is-hovering',
    '.night-theme .el-select__wrapper.is-focused',
  ], { 'box-shadow': '0 0 0 1px #444 inset' })
  assertDeclarations('.night-theme .el-select .el-tag.el-tag--info', {
    '--el-tag-bg-color': '#777',
    '--el-tag-border-color': '#777',
    '--el-tag-text-color': '#ddd',
  })
  assertDeclarations(['.night-theme .el-input-number__increase', '.night-theme .el-input-number__decrease'], {
    color: '#fff',
    'background-color': '#909399',
    'border-color': '#909399',
  })
  assertDeclarations(['.night-theme .el-input-number__increase:hover', '.night-theme .el-input-number__decrease:hover'], {
    color: 'var(--el-color-primary)',
  })
  assertDeclarations('.night-theme .el-select__popper.el-popper', {
    '--el-bg-color-overlay': '#333',
    '--el-border-color-light': '#333',
    '--el-fill-color-light': '#444',
    '--el-text-color-regular': '#ddd',
  })
  assertDeclarations('.night-theme .el-select-dropdown.is-multiple .el-select-dropdown__item.is-selected', { 'background-color': '#444' })
  assertDeclarations('.night-theme .el-select-dropdown.is-multiple .el-select-dropdown__item.is-hovering', { 'background-color': '#555' })
  assertDeclarations('.night-theme .el-popper > .el-popper__arrow::before', { background: '#333', 'border-color': '#333' })
  assertDeclarations('.night-theme .el-dropdown__popper.el-popper', {
    '--el-bg-color-overlay': '#444',
    '--el-border-color-light': '#444',
  })
  assertDeclarations('.night-theme .el-dropdown-menu', {
    '--el-dropdown-menuItem-hover-fill': '#666',
    '--el-dropdown-menuItem-hover-color': '#66b1ff',
    '--el-text-color-regular': '#bbb',
  })
})

test('tables, checkboxes, tabs and pagination take reader-dev night colors', () => {
  assertDeclarations('.night-theme .el-table', {
    '--el-table-bg-color': 'transparent',
    '--el-table-tr-bg-color': '#222',
    '--el-table-header-bg-color': '#222',
    '--el-table-expanded-cell-bg-color': 'transparent',
    '--el-table-text-color': '#888',
    '--el-table-border-color': '#555',
    '--el-table-row-hover-bg-color': '#444',
  })
  assertDeclarations('.night-theme .el-table__inner-wrapper::before', { 'background-color': 'transparent' })
  assertDeclarations('.night-theme .el-table--enable-row-hover .el-table__body tr:hover > td.el-table__cell', { color: '#ccc' })
  assertDeclarations('.night-theme .el-checkbox', { '--el-checkbox-bg-color': '#bbb' })
  assertDeclarations('.night-theme .el-tabs__item', { '--el-text-color-primary': '#ddd' })
  assertDeclarations(['.night-theme .el-tabs__nav-next:not(.is-disabled)', '.night-theme .el-tabs__nav-prev:not(.is-disabled)'], { color: '#aaa' })
  assertDeclarations('.night-theme .el-tabs__nav-wrap::after', { 'background-color': '#444' })
  assertDeclarations('.night-theme .el-pagination', {
    '--el-pagination-bg-color': '#444',
    '--el-pagination-button-color': '#ddd',
    '--el-pagination-button-disabled-color': '#ddd',
    '--el-pagination-button-disabled-bg-color': '#444',
  })
})

test('every 已选择 N 个 counter takes the night check-tip color and keeps its day color', () => {
  const scopedRule = (path, selector) => {
    const css = source(path)
    const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    const match = css.match(new RegExp(`(?:^|\\n)${escaped} \\{([^}]*)\\}`))
    assert.ok(match, `${path} must keep ${selector}`)
    return match[1]
  }
  // reader-dev `.night-theme .check-tip`; each counter keeps its current day color as the fallback.
  for (const [path, selector, dayColor] of [
    ['components/WebDAVBrowser.vue', '.check-tip', 'var(--app-text-muted)'],
    ['components/workspace/LocalStoreManager.vue', '.check-tip', 'var(--app-text-muted)'],
    ['components/overlays/BookManagementBatchFooter.vue', '.check-tip', 'var(--app-text-muted)'],
    ['components/workspace/SourceManager.vue', '.source-dialog-footer .check-tip', 'var(--app-text-muted)'],
    ['components/workspace/SourceTransferOverlay.vue', '.source-import-footer .check-tip', 'var(--app-text-muted)'],
    ['components/overlays/OverlayUserManagement.vue', '.check-tip', 'var(--app-text-muted)'],
    ['components/overlays/OverlayReplaceRules.vue', '.replace-manager-footer span', 'var(--app-text-muted)'],
    ['components/overlays/OverlayBookmarks.vue', '.reader-dialog-footer span', 'var(--el-text-color-secondary)'],
    ['components/rss/RSSImportDialog.vue', '.rss-import-count', 'var(--app-text)'],
  ]) {
    assert.ok(
      scopedRule(path, selector).includes(`color: var(--check-tip-text, ${dayColor});`),
      `${path} ${selector} must take --check-tip-text at night and keep ${dayColor} by day`,
    )
  }
  // The protected-user label shares the muted day color but is not a selection counter.
  assert.match(scopedRule('components/overlays/OverlayUserManagement.vue', '.protected-user-label'), /color: var\(--app-text-muted\);/)
})

test('custom dialog headers take the night title color like reader-dev span.el-dialog__title', () => {
  // reader-dev custom title slots reuse span.el-dialog__title; OpenReader's inherit from the header.
  assertDeclarations('.night-theme .el-dialog__header', { color: '#bbb' })
  const rss = source('components/RSSManager.vue')
  const title = rss.match(/\n\.rss-dialog-title-text \{([^}]*)\}/)
  assert.ok(title, 'RSSManager keeps its title rule')
  assert.doesNotMatch(title[1], /(^|\s)color:/, 'the RSS dialog title must inherit the header color')
})

test('the light home sidebar selects keep their text color at night', () => {
  // D5: the sidebar keeps its current look until it follows reader-dev Index.vue `.night`.
  const layout = source('layouts/AppLayout.vue')
  const wrapper = layout.match(/\n\.setting-select :deep\(\.el-select__wrapper\) \{([^}]*)\}/)
  assert.ok(wrapper, 'AppLayout keeps its sidebar select surface rule')
  assert.match(wrapper[1], /background: #fffdf8;/)
  assert.match(wrapper[1], /--el-input-text-color: var\(--el-text-color-regular\);/)
  assert.match(wrapper[1], /--el-select-multiple-input-color: var\(--el-text-color-regular\);/)
})

test('OpenReader-only catalog tag, empty states and loading masks get the decided night look', () => {
  // D2: reader-dev's night tag convention (BookInfo.vue, Explore.vue, Index.vue).
  const toc = source('components/reader/ReaderTocPanel.vue')
  assert.match(toc, /<el-tag v-if="!showMeta && isCached\(item\)" size="small" type="success" :effect="isNight \? 'dark' : 'plain'">/)
  assert.match(toc, /isNight: \{\n    type: Boolean,\n    default: false,\n  \},/)
  const reader = source('views/Reader.vue')
  const tocPanels = [...reader.matchAll(/<ReaderTocPanel\b[^>]*>/g)].map(match => match[0])
  assert.equal(tocPanels.length, 2, 'Reader.vue renders the desktop and mobile catalogs')
  for (const panel of tocPanels) assert.match(panel, /:is-night="isNightTheme"/)
  assert.match(reader, /const isNightTheme = computed\(\(\) => reader\.themeType === 'night'\)/)

  // D3: Element Plus's own dark empty illustration and the reader-dev night loading background.
  assertDeclarations('.night-theme .el-empty', {
    '--el-empty-fill-color-0': '#000',
    '--el-empty-fill-color-1': '#4b4b52',
    '--el-empty-fill-color-2': '#36383d',
    '--el-empty-fill-color-3': '#1e1e20',
    '--el-empty-fill-color-4': '#262629',
    '--el-empty-fill-color-5': '#202124',
    '--el-empty-fill-color-6': '#212224',
    '--el-empty-fill-color-7': '#1b1c1f',
    '--el-empty-fill-color-8': '#1c1d1f',
    '--el-empty-fill-color-9': '#18181a',
  })
  assertDeclarations('.night-theme .el-loading-mask', { 'background-color': '#222' })
})

test('the EPUB catalog rule picker follows the night input colors and keeps its day colors', () => {
  const picker = source('composables/useReaderLocalTocRulePicker.js')
  const style = picker.match(/style: '([^']+)'/)
  assert.ok(style, 'the EPUB rule picker keeps one inline style')
  assert.match(style[1], /border:1px solid var\(--el-input-border-color, var\(--el-border-color\)\)/)
  assert.match(style[1], /background:var\(--el-input-bg-color, var\(--el-bg-color\)\)/)
  assert.match(style[1], /color:var\(--el-input-text-color, var\(--el-text-color-primary\)\)/)
})
