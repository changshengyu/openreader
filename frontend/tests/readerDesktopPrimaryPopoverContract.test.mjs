import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const workspaceSource = readFileSync(new URL('../src/components/reader/ReaderDesktopWorkspacePanel.vue', import.meta.url), 'utf8')
const readerSource = readFileSync(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')

function desktopWorkspaceBlock() {
  const match = workspaceSource.match(/\.reader-desktop-workspace\s*\{([\s\S]*?)\n\}/)
  assert.ok(match, 'desktop workspace root CSS block must exist')
  return match[1]
}

test('desktop primary panels use the fixed-baseline top-anchored popover frame', () => {
  const css = desktopWorkspaceBlock()
  assert.match(readerSource, /<ReaderDesktopWorkspacePanel[\s\S]*?:panel="desktopWorkspacePanel"/, 'Reader must identify the active desktop primary panel to its shell')
  assert.match(workspaceSource, /workspace-panel-\$\{panel\}/, 'desktop shell must expose a panel-specific class for upstream bounds')
  assert.match(css, /position:\s*fixed/, 'desktop primary panel must remain anchored to the viewport')
  assert.match(css, /top:\s*0/, 'upstream desktop popovers begin at the reader top edge')
  assert.match(css, /left:\s*calc\(50vw\s*-\s*var\(--reader-frame-width\)\s*\/\s*2\s*\+\s*5px\)/, 'desktop popover must begin 5px inside the reader frame')
  assert.match(css, /width:\s*calc\(var\(--reader-frame-width\)\s*-\s*9px\)/, 'desktop popover must preserve the upstream frame-right inset')
  assert.match(css, /height:\s*auto/, 'desktop primary panel must size to its content')
  assert.match(css, /max-height:\s*100dvh/, 'desktop primary panel needs a viewport safety cap')
  assert.doesNotMatch(css, /bottom:\s*0/, 'desktop primary panel must not retain the rewritten full-height workspace')
})

test('desktop primary panel internals retain upstream 300px lists while settings owns its inner scroll', () => {
  assert.match(
    workspaceSource,
    /workspace-panel-shelf[\s\S]*?workspace-panel-toc[\s\S]*?workspace-panel-source[\s\S]*?height:\s*300px/,
    'desktop shelf, catalog, and source panels must keep the upstream 300px list viewport',
  )
  assert.match(
    workspaceSource,
    /workspace-panel-settings[\s\S]*?\.reader-workspace-body\s*\{[\s\S]*?max-height:\s*none;[\s\S]*?overflow:\s*visible/,
    'desktop settings shell must leave scrolling to the upstream-style inner settings list',
  )
})
