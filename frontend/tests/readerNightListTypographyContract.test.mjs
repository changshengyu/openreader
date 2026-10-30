import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

// 2026-10-30 user decision (docs/compat/reader-night-list-typography-fixed-baseline-second-audit-p1-contract.md):
// the Reader catalog, shelf and source lists use reader-dev's `.night` list colors for every night scheme;
// day keeps its colors as the var() fallbacks.
const source = path => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8')
const readerViewSource = source('views/Reader.vue')
const tocSource = source('components/reader/ReaderTocPanel.vue')
const shelfSource = source('components/reader/ReaderShelfPanel.vue')
const sourceSwitchSource = source('components/reader/SourceSwitchPanel.vue')
const desktopWorkspaceSource = source('components/reader/ReaderDesktopWorkspacePanel.vue')
const settingsSource = source('components/reader/ReaderSettingsPanel.vue')

const POPUP_TEXT = 'var\\(--reader-popup-text, var\\(--reader-text\\)\\)'
const mixed = percent => `color-mix\\(in srgb, ${POPUP_TEXT} ${percent}%, transparent\\)`

// Bodies of every top-level rule whose selector list is exactly `selector`.
function rule(src, selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const bodies = [...src.matchAll(new RegExp(`(?:^|\\n)${escaped} \\{([^}]*)\\}`, 'g'))].map(match => match[1])
  assert.ok(bodies.length, `missing CSS rule ${selector}`)
  return bodies.join('\n')
}

function frozenStyle(name) {
  const match = readerViewSource.match(new RegExp(`const ${name} = Object\\.freeze\\(\\{([\\s\\S]*?)\\}\\)`))
  assert.ok(match, `Reader.vue must define ${name} in one place`)
  return Object.fromEntries(
    [...match[1].matchAll(/'(--reader-[a-z-]+)':\s*'([^']+)'/g)].map(entry => [entry[1], entry[2]]),
  )
}

test('reader night list variables carry the reader-dev night list palette for every night scheme', () => {
  assert.deepEqual(frozenStyle('NIGHT_LIST_STYLE'), {
    '--reader-list-text': '#606266',
    '--reader-list-cached-text': '#444',
    '--reader-list-border': '#333',
    '--reader-list-active-text': '#ed4259',
    '--reader-list-active-bg': 'transparent',
    '--reader-shelf-title-text': '#888',
    '--reader-shelf-chapter-text': '#555',
    '--reader-source-chapter-text': '#888',
  })
  assert.match(readerViewSource, /\.\.\.\(reader\.themeType === 'night' \? NIGHT_LIST_STYLE : \{\}\)/)
})

test('catalog rows take night text, cached, current and separator colors from the list palette', () => {
  const template = tocSource.slice(0, tocSource.indexOf('</template>'))
  assert.match(template, /:class="\{ active: item\.index === currentIndex, cached: isCached\(item\) \}"/)

  const item = rule(tocSource, '.toc-item')
  assert.match(item, new RegExp(`color: var\\(--reader-list-text, ${POPUP_TEXT}\\);`))
  assert.match(item, /border-bottom: 1px solid var\(--reader-list-border, #f0f0f0\);/)
  assert.match(
    rule(tocSource, '.toc-item.cached'),
    new RegExp(`color: var\\(--reader-list-cached-text, var\\(--reader-list-text, ${POPUP_TEXT}\\)\\);`),
  )
  // Same specificity as hover/current, so the cached rule must come first for them to win.
  const cachedAt = tocSource.indexOf('\n.toc-item.cached {')
  assert.ok(cachedAt > 0 && cachedAt < tocSource.indexOf('\n.toc-item:hover {'), '.toc-item.cached must precede hover')
  assert.ok(cachedAt < tocSource.indexOf('\n.toc-item.active {'), '.toc-item.cached must precede the current chapter')

  const hover = rule(tocSource, '.toc-item:hover')
  assert.match(hover, /color: var\(--reader-list-active-text, #409eff\);/)
  assert.match(hover, /background: var\(--reader-list-active-bg, #f5f7fa\);/)
  const active = rule(tocSource, '.toc-item.active')
  assert.match(active, /color: var\(--reader-list-active-text, #409eff\);/)
  assert.match(active, /font-weight: 600;/)
  assert.match(active, /background: var\(--reader-list-active-bg, #ecf5ff\);/)
  assert.match(rule(tocSource, '.toc-item small'), /color: var\(--reader-shelf-chapter-text, var\(--app-text-muted, #909399\)\);/)
})

test('shelf cards take night title, unread count, chapter and separator colors from the list palette', () => {
  const card = rule(shelfSource, '.reader-shelf-card')
  assert.match(card, new RegExp(`color: var\\(--reader-shelf-title-text, ${POPUP_TEXT}\\);`))
  assert.match(card, /border-bottom: 1px solid var\(--reader-list-border, rgba\(160, 139, 91, 0\.22\)\);/)
  assert.match(rule(shelfSource, '.reader-shelf-title-line em'), /color: var\(--reader-shelf-title-text, var\(--reader-accent, #ed4259\)\);/)
  assert.match(
    rule(shelfSource, '.reader-shelf-chapter'),
    new RegExp(`color: var\\(--reader-shelf-chapter-text, ${mixed(72)}\\);`),
  )
})

test('source rows and the source title actions take night colors from the list palette', () => {
  assert.match(rule(sourceSwitchSource, '.source-item'), /border-bottom: 1px solid var\(--reader-list-border, #eee\);/)
  assert.match(rule(sourceSwitchSource, '.source-name'), new RegExp(`color: var\\(--reader-list-text, ${POPUP_TEXT}\\);`))
  assert.match(rule(sourceSwitchSource, '.source-time'), new RegExp(`color: var\\(--reader-list-text, ${mixed(72)}\\);`))
  for (const selector of ['.source-latest-chapter', '.source-status']) {
    assert.match(rule(sourceSwitchSource, selector), new RegExp(`color: var\\(--reader-source-chapter-text, ${mixed(72)}\\);`))
  }
  assert.match(
    rule(sourceSwitchSource, '.title-actions button:disabled'),
    new RegExp(`color: var\\(--reader-list-text, ${mixed(62)}\\);`),
  )
})

test('desktop catalog and shelf title actions use the night loading color', () => {
  assert.match(rule(desktopWorkspaceSource, '.reader-workspace-actions :deep(button:disabled)'), /color: var\(--reader-list-text, #aaa\);/)
})

test('font preview uses the ReadSettings night control surface at night', () => {
  const chrome = frozenStyle('NIGHT_CHROME_STYLE')
  assert.equal(chrome['--reader-font-preview-bg'], 'rgba(45, 45, 45, 0.5)')
  assert.equal(chrome['--reader-font-preview-border'], '#666')
  const preview = rule(settingsSource, '.font-preview')
  assert.match(preview, /color: var\(--reader-settings-text, #2c2a24\);/)
  assert.match(preview, /background: var\(--reader-font-preview-bg, #fffaf0\);/)
  assert.match(preview, /border: 1px solid var\(--reader-font-preview-border, #eadfca\);/)
})

test('day lists keep their current colors and popup text keeps its contrast resolution', () => {
  assert.match(readerViewSource, /'--reader-popup-text': effectiveReaderPopupTextColor\.value,/)
  assert.match(readerViewSource, /\.\.\.\(reader\.themeType === 'night' \? NIGHT_CHROME_STYLE : \{\}\)/)
  assert.match(rule(tocSource, '.toc-item'), /#f0f0f0/)
  assert.match(rule(tocSource, '.toc-item:hover'), /#409eff[\s\S]*#f5f7fa/)
  assert.match(rule(tocSource, '.toc-item.active'), /#409eff[\s\S]*#ecf5ff/)
  const gridActive = rule(tocSource, '.toc-list.desktop-grid .toc-item.active')
  assert.match(gridActive, /color: var\(--reader-accent, #ed4259\);/)
  assert.match(gridActive, /background: transparent;/)
  assert.match(rule(shelfSource, '.reader-shelf-card'), /rgba\(160, 139, 91, 0\.22\)/)
  assert.match(rule(shelfSource, '.reader-shelf-card:hover,\n.reader-shelf-card.active'), /color: var\(--reader-accent, #ed4259\);/)
  assert.match(rule(sourceSwitchSource, '.source-item'), /#eee/)
  assert.match(rule(sourceSwitchSource, '.source-item.selected .source-name'), /color: var\(--reader-accent, #ed4259\);/)
  assert.match(rule(desktopWorkspaceSource, '.reader-workspace-actions :deep(button:disabled)'), /#aaa/)
  assert.match(readerViewSource, /\.reader-shell\.mini-interface \.reader-mobile-primary-actions button:disabled \{\n  color: #606266;/)
  const preview = rule(settingsSource, '.font-preview')
  for (const day of ['#2c2a24', '#fffaf0', '#eadfca']) assert.match(preview, new RegExp(day))
})
