import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const source = path => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8')
const workspaceSource = source('components/reader/ReaderMobileWorkspacePanel.vue')
const readerSource = source('views/Reader.vue')
const mobileChromeSource = source('components/reader/ReaderMobileChrome.vue')

function rule(src, selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const bodies = [...src.matchAll(new RegExp(`(?:^|\\n)${escaped} \\{([^}]*)\\}`, 'g'))].map(match => match[1])
  assert.ok(bodies.length, `missing CSS rule ${selector}`)
  return bodies.join('\n')
}

// docs/compat/reader-menu-chrome-panels-fixed-baseline-second-audit-p0-contract.md B1/B2: reader-dev App.vue pins
// the mini `.popper-component` at the top over the toolbar; each panel draws its own title zone.
test('primary reader popovers expose the reader-dev popover face without generic workspace chrome', () => {
  const root = rule(workspaceSource, '.reader-mobile-workspace-primary')
  assert.match(root, /padding: calc\(20px \+ env\(safe-area-inset-top\)\) 20px 23px;/, 'the panels keep reader-dev 20px inset and 23px foot')
  assert.doesNotMatch(root, /backdrop-filter|58px/, 'no glass effect and no room reserved for the toolbar')
  assert.doesNotMatch(workspaceSource, /reader-mobile-workspace-head|reader-mobile-workspace-close|name="actions"/, 'no generic header or close')

  const primaryPanels = readerSource.match(/<ReaderMobileWorkspacePanel[\s\S]*?<\/ReaderMobileWorkspacePanel>/g) || []
  assert.equal(primaryPanels.length, 4, 'shelf, catalog, source and settings use the popover face')
  for (const panel of primaryPanels) assert.doesNotMatch(panel, /\bprimary\b|show-header|reader-mobile-primary-title/)
  assert.match(source('components/reader/ReaderShelfPanel.vue'), /<div class="title-zone">/)
  assert.match(source('components/reader/ReaderTocPanel.vue'), /<div class="title-zone">/)
})

test('primary mobile popovers cover the tool strip like reader-dev and use content-sized bounds', () => {
  const root = rule(workspaceSource, '.reader-mobile-workspace-primary')
  assert.match(root, /top: 0;[\s\S]*left: 0;[\s\S]*z-index: 13;/, 'the panel is pinned at the top above the toolbar')
  assert.match(root, /max-height: 100dvh;/, 'content-sized, capped by the viewport')
  assert.doesNotMatch(root, /(^|\n)\s*height:/, 'no forced height')
  assert.match(rule(mobileChromeSource, '.reader-mobile-top.visible'), /z-index: 12;/, 'the toolbar sits under an open panel')
  assert.match(readerSource, /reader-mobile-primary-dismiss/, 'outside-popover clicks need a dedicated click-away layer')
  assert.match(readerSource, /@click\.stop="closeReaderPrimaryPanels"/, 'the click-away layer closes primary popovers without reaching reader taps')
  assert.match(rule(source('components/reader/ReaderShelfPanel.vue'), '.reader-shelf-list'), /height: 300px;/)
  assert.match(rule(source('components/reader/ReaderTocPanel.vue'), '.toc-list'), /height: 300px;/)
  assert.match(rule(source('components/reader/SourceSwitchPanel.vue'), '.source-switch-list'), /height: 300px;/)
})
