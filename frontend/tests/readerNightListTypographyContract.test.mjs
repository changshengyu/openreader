import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

// 2026-10-30 user decision (docs/compat/reader-night-list-typography-fixed-baseline-second-audit-p1-contract.md):
// the Reader catalog, shelf and source lists use reader-dev's `.night` list colors for every night scheme.
// 2026-11-03 reader-menu contract: the day fallbacks are reader-dev's day list colors too.
const source = path => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8')
const readerViewSource = source('views/Reader.vue')
const tocSource = source('components/reader/ReaderTocPanel.vue')
const shelfSource = source('components/reader/ReaderShelfPanel.vue')
const sourceSwitchSource = source('components/reader/SourceSwitchPanel.vue')
const settingsSource = source('components/reader/ReaderSettingsPanel.vue')


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
    '--reader-shelf-title-text': '#888',
    '--reader-shelf-chapter-text': '#555',
    '--reader-source-chapter-text': '#888',
  })
  assert.match(readerViewSource, /\.\.\.\(reader\.themeType === 'night' \? NIGHT_LIST_STYLE : \{\}\)/)
})

test('catalog rows take night text, cached and separator colors from the list palette', () => {
  const template = tocSource.slice(0, tocSource.indexOf('</template>'))
  assert.match(template, /:class="\{ active: item\.index === currentIndex \}"/)
  assert.match(template, /<span class="log-text" :class="\{ cached: isBrowserCached\(item\) \}">/)
  // reader-dev dims only chapters in the browser cache (PopCatalog cachedCataMap);
  // every imported local-book chapter has a server cachePath and must not dim.
  assert.match(tocSource, /function isBrowserCached\(item\) \{\n  return Boolean\(props\.browserCachedMap\?\.\[item\?\.index\]\)\n\}/)

  const item = rule(tocSource, '.toc-item')
  assert.match(item, /color: var\(--reader-list-text, inherit\);/)
  assert.match(item, /border-bottom: 1px solid var\(--reader-list-border, #eee\);/)
  assert.match(rule(tocSource, '.toc-item .log-text.cached'), /color: var\(--reader-list-cached-text, #bbb\);/)
  // Same specificity as the current chapter, so the cached rule must come first for it to win.
  const cachedAt = tocSource.indexOf('\n.toc-item .log-text.cached {')
  assert.ok(cachedAt > 0 && cachedAt < tocSource.indexOf('\n.toc-item.active .log-text {'), 'cached must precede the current chapter')
  assert.match(rule(tocSource, '.toc-item.active .log-text'), /color: #eb4259;/)
  assert.doesNotMatch(tocSource, /\.toc-item:hover|<small/)
})

test('shelf cards take night title, unread count, chapter and separator colors from the list palette', () => {
  assert.match(rule(shelfSource, '.reader-shelf-card'), /border-bottom: 1px solid var\(--reader-list-border, #eee\);/)
  assert.match(rule(shelfSource, '.reader-shelf-title-line'), /color: var\(--reader-shelf-title-text, inherit\);/)
  assert.match(rule(shelfSource, '.reader-shelf-chapter'), /color: var\(--reader-shelf-chapter-text, #888\);/)
  assert.match(rule(shelfSource, '.reader-shelf-card.active strong'), /color: #eb4259;/)
})

test('source rows and the source title actions take night colors from the list palette', () => {
  const item = rule(sourceSwitchSource, '.source-item')
  assert.match(item, /color: var\(--reader-list-text, inherit\);/)
  assert.match(item, /border-bottom: 1px solid var\(--reader-list-border, #eee\);/)
  for (const selector of ['.source-latest-chapter', '.source-status']) {
    assert.match(rule(sourceSwitchSource, selector), /color: var\(--reader-source-chapter-text, #888\);/)
  }
  assert.match(rule(sourceSwitchSource, '.source-item.selected .source-name'), /color: #eb4259;/)
  assert.match(rule(sourceSwitchSource, '.title-actions button:disabled'), /color: #606266;/)
})

test('catalog and shelf title actions use the reader-dev loading color', () => {
  assert.match(rule(shelfSource, '.title-btn.loading'), /color: #606266;/)
  assert.match(rule(tocSource, '.title-btn .refresh-btn.loading'), /color: #606266;/)
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

test('popup text keeps its contrast resolution while the panels use reader-dev text defaults', () => {
  assert.match(readerViewSource, /'--reader-popup-text': effectiveReaderPopupTextColor\.value,/)
  assert.match(readerViewSource, /\.\.\.\(reader\.themeType === 'night' \? NIGHT_CHROME_STYLE : \{\}\)/)
  const preview = rule(settingsSource, '.font-preview')
  for (const day of ['#2c2a24', '#fffaf0', '#eadfca']) assert.match(preview, new RegExp(day))
})
