import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const workspaceSource = readFileSync(new URL('../src/components/reader/ReaderMobileWorkspacePanel.vue', import.meta.url), 'utf8')
const readerSource = readFileSync(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')
const mobileChromeSource = readFileSync(new URL('../src/components/reader/ReaderMobileChrome.vue', import.meta.url), 'utf8')

test('primary reader popovers expose an upstream-like root without generic workspace chrome', () => {
  assert.match(workspaceSource, /primary:\s*\{/, 'mobile workspace must expose a primary-popover presentation')
  assert.match(workspaceSource, /reader-mobile-workspace-primary/, 'primary workspace must have a dedicated root class')
  assert.match(workspaceSource, /\.reader-mobile-workspace-primary\s*\{[\s\S]*?padding:\s*0/, 'primary workspace root must not reserve generic toolbar padding')
  assert.match(workspaceSource, /\.reader-mobile-workspace-primary\s*\{[\s\S]*?backdrop-filter:\s*none/, 'primary workspace must not add a glass drawer effect over the upstream popover')

  const primaryPanels = readerSource.match(/<ReaderMobileWorkspacePanel[\s\S]*?\/>|<ReaderMobileWorkspacePanel[\s\S]*?<\/ReaderMobileWorkspacePanel>/g) || []
  const markedPrimaryPanels = primaryPanels.filter(block => /\bprimary\b/.test(block) && /:show-header="false"/.test(block))
  assert.equal(markedPrimaryPanels.length, 4, 'shelf, source, toc, and settings must use the primary-popover shell without a generic header')
  assert.match(readerSource, /reader-mobile-primary-popover-body/, 'primary panels must own their inner popover padding and title layout')
})

test('primary mobile popovers reproduce upstream click blocking and content-sized bounds', () => {
  assert.match(
    workspaceSource,
    /\.reader-mobile-workspace-primary\s*\{[\s\S]*?z-index:\s*10/,
    'the primary popover must paint above the retained mobile tool state',
  )
  assert.match(
    workspaceSource,
    /\.reader-mobile-workspace-primary\s*\{[\s\S]*?height:\s*auto[\s\S]*?max-height:\s*100dvh/,
    'primary popover roots must be content-sized and capped rather than a full-height drawer',
  )
  assert.match(
    workspaceSource,
    /\.reader-mobile-workspace-primary\s+\.reader-mobile-workspace-body\s*\{[\s\S]*?height:\s*auto[\s\S]*?max-height:\s*100dvh/,
    'the primary popover body must not reintroduce a forced viewport height',
  )
  assert.match(mobileChromeSource, /z-index:\s*8/, 'reader chrome stays mounted beneath a primary popover')
  assert.match(readerSource, /reader-mobile-primary-dismiss/, 'outside-popover clicks need a dedicated click-away layer')
  assert.match(readerSource, /@click\.stop="closeReaderPrimaryPanels"/, 'the click-away layer must close primary popovers without reaching reader taps')
  assert.match(
    readerSource,
    /reader-mobile-primary-shelf[\s\S]*?reader-mobile-primary-source[\s\S]*?height:\s*300px/,
    'shelf, source, and catalog keep the upstream 300px scroll region',
  )
  assert.match(
    readerSource,
    /reader-mobile-primary-settings\s*\{[\s\S]*?max-height:\s*calc\(45vh\s*\+\s*96px\)[\s\S]*?overflow:\s*auto/,
    'settings keeps its upstream-style bounded scroll region instead of a fullscreen drawer',
  )
})
