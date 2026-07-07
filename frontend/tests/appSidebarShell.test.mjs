import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'
import test from 'node:test'

const __dirname = dirname(fileURLToPath(import.meta.url))
const layoutPath = resolve(__dirname, '../src/layouts/AppLayout.vue')
const source = readFileSync(layoutPath, 'utf8')

function indexOfRequired(fragment) {
  const index = source.indexOf(fragment)
  assert.notEqual(index, -1, `missing fragment: ${fragment}`)
  return index
}

test('keeps sidebar bottom icons outside the scrollable navigation content', () => {
  const scrollStart = indexOfRequired('<div class="app-sidebar-scroll">')
  const scrollEnd = indexOfRequired('</div>\n\n      <div class="sidebar-bottom-icons"')
  const bottomIcons = indexOfRequired('<div class="sidebar-bottom-icons" aria-label="侧栏快捷入口">')
  const asideEnd = indexOfRequired('</aside>')

  assert.ok(scrollStart < scrollEnd, 'scroll container should close before bottom icons')
  assert.ok(scrollEnd < bottomIcons, 'bottom icons should not be nested in the scroll container')
  assert.ok(bottomIcons < asideEnd, 'bottom icons should remain inside the sidebar frame')
})

test('locks sidebar bottom icons to the frame and isolates pointer events', () => {
  assert.match(source, /\.sidebar-bottom-icons\s*\{[\s\S]*position:\s*absolute;[\s\S]*bottom:\s*30px;[\s\S]*left:\s*36px;[\s\S]*pointer-events:\s*none;/)
  assert.match(source, /\.sidebar-bottom-icon\s*\{[\s\S]*pointer-events:\s*auto;/)
  assert.match(source, /\.app-shell\.mobile-shell \.sidebar-bottom-icons\s*\{[\s\S]*position:\s*absolute;[\s\S]*bottom:\s*30px;[\s\S]*left:\s*36px;[\s\S]*pointer-events:\s*none;/)
  assert.match(source, /transform:\s*translateX\(calc\(-1 \* var\(--mobile-nav-drag-offset, 0px\)\)\);/)
})
