import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))

function read(relative) {
  return readFileSync(resolve(__dirname, relative), 'utf8')
}

test('hosts BookGroup set and manage modes in one upstream-style root dialog', () => {
  const groups = read('../src/components/overlays/OverlayBookGroups.vue')
  const host = read('../src/components/GlobalOverlayHost.vue')

  assert.match(groups, /<el-dialog/)
  assert.doesNotMatch(groups, /<el-drawer/)
  assert.match(groups, /v-model="overlay\.bookGroupVisible"/)
  assert.match(groups, /:title="overlay\.bookGroupMode === 'set' \? '设置分组' : '分组管理'"/)
  assert.match(groups, /:fullscreen="isMobile"/)
  assert.match(groups, /class="global-book-group-dialog"/)
  assert.match(groups, /v-if="overlay\.bookGroupMode === 'set'"/)
  assert.match(groups, /v-else/)
  assert.match(host, /<OverlayBookGroups\s+:is-mobile="isMobileOverlay"\s*\/>/)
})

test('keeps book-group state as one global overlay controller instead of a route or drawer', () => {
  const groups = read('../src/components/overlays/OverlayBookGroups.vue')
  const layout = read('../src/layouts/AppLayout.vue')

  assert.match(layout, /\{ key: 'bookGroup', label: '分组管理', action: \(\) => overlay\.openBookGroup\('manage'\) \}/)
  assert.doesNotMatch(layout, /\{ key: 'bookGroup',[^\n]*route:/)
  assert.match(groups, /useOverlayBookGroups/)
  assert.match(groups, /\(\) => overlay\.bookGroupVisible/)
})
