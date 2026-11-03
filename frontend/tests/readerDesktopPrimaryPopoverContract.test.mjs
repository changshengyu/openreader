import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const source = path => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8')
const workspaceSource = source('components/reader/ReaderDesktopWorkspacePanel.vue')
const readerSource = source('views/Reader.vue')
const toolsSource = source('components/reader/ReaderDesktopTools.vue')
const progressSource = source('components/reader/ReaderDesktopProgress.vue')
const clickZonesSource = source('components/reader/ReaderClickZones.vue')

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

// docs/compat/reader-menu-chrome-panels-fixed-baseline-second-audit-p0-contract.md C4/C5: the visible face of
// reader-dev's desktop popover covers the reading frame inside its border, from the top.
test('desktop primary panels use the fixed-baseline top-anchored popover face', () => {
  const css = rule(workspaceSource, '.reader-desktop-workspace')
  assert.match(readerSource, /<ReaderDesktopWorkspacePanel[\s\S]*?:panel="desktopWorkspacePanel"/, 'Reader must identify the active desktop primary panel to its shell')
  assert.match(workspaceSource, /workspace-panel-\$\{panel\}/, 'desktop shell exposes a panel-specific class')
  assert.match(css, /position: fixed;/)
  assert.match(css, /top: 0;/, 'upstream desktop popovers begin at the reader top edge')
  assert.match(css, /left: 50%;[\s\S]*margin-left: calc\(var\(--reader-frame-width\) \/ -2\);/, 'the face starts inside the frame border')
  assert.match(css, /width: calc\(var\(--reader-frame-width\) - 1px\);/)
  assert.match(css, /max-height: 100dvh;/, 'desktop primary panel needs a viewport safety cap')
  assert.doesNotMatch(css, /bottom: 0|(^|\n)\s*height:/, 'content-sized, not a full-height workspace')
})

test('desktop primary panel internals retain upstream 300px lists while settings owns its inner scroll', () => {
  assert.match(rule(source('components/reader/ReaderShelfPanel.vue'), '.reader-shelf-list'), /height: 300px;/)
  assert.match(rule(source('components/reader/ReaderTocPanel.vue'), '.toc-list'), /height: 300px;/)
  assert.match(rule(source('components/reader/SourceSwitchPanel.vue'), '.source-switch-list'), /height: 300px;/)
  assert.doesNotMatch(workspaceSource, /reader-workspace-body|overflow: auto/, 'the shell never scrolls; settings scrolls its own list')
})

test('desktop primary panels retain both upstream dismissal paths and add one visible iPad close target', () => {
  assert.match(
    workspaceSource,
    /class="reader-desktop-workspace-close"[\s\S]*?aria-label="关闭阅读工具面板"[\s\S]*?@click\.stop="\$emit\('close'\)"/,
    'the shared workspace must expose one explicit close control for shelf, source, catalog, and settings',
  )
  const close = rule(workspaceSource, '.reader-desktop-workspace-close')
  assert.match(close, /width: 44px;[\s\S]*height: 44px;/, 'the visible iPad close control keeps a 44px touch target')
  assert.match(close, /position: fixed;[\s\S]*margin-left: calc\(var\(--reader-frame-width\) \/ 2 \+ 8px\);/, 'user decision D2: beside the panel, over the right tool column')
  assert.match(
    workspaceSource,
    /class="reader-workspace-dismiss"[\s\S]*?@pointerdown\.stop[\s\S]*?@click\.stop="\$emit\('close'\)"/,
    'outside pointer input must be consumed by the shared dismiss surface',
  )
})

test('desktop panel stack is explicit above content hit zones and below retained rails', () => {
  const zones = zIndex(clickZonesSource, '.reader-tap-zones')
  const dismiss = zIndex(workspaceSource, '.reader-workspace-dismiss')
  const panel = zIndex(workspaceSource, '.reader-desktop-workspace')
  const right = zIndex(toolsSource, '.reader-right-rail')
  const column = zIndex(progressSource, '.reader-page-control')
  const close = zIndex(workspaceSource, '.reader-desktop-workspace-close')
  const overlay = zIndex(clickZonesSource, '.click-zone-overlay')
  const left = zIndex(toolsSource, '.reader-left-rail')
  assert.ok(zones < dismiss && dismiss < panel, 'hit zones < dismiss layer < panel')
  assert.ok(panel < right && panel < column && panel < close, 'the right tool column stays pressable beside an open panel')
  // reader-dev: `.read-bar` z 100 < `.click-zone` z 120 < `.tool-bar` z 2001.
  assert.ok(right < overlay && column < overlay && overlay < left, '显示翻页区域 covers the right column but not the left rail')
})
