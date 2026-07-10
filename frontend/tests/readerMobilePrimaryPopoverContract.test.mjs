import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const workspaceSource = readFileSync(new URL('../src/components/reader/ReaderMobileWorkspacePanel.vue', import.meta.url), 'utf8')
const readerSource = readFileSync(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')

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
