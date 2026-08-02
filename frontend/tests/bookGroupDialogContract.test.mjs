import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))

function read(relative) {
  return readFileSync(resolve(__dirname, relative), 'utf8')
}

test('hosts BookGroup set and manage modes in one fixed-upstream table dialog', () => {
  const groups = read('../src/components/overlays/OverlayBookGroups.vue')
  const host = read('../src/components/GlobalOverlayHost.vue')

  assert.match(groups, /<el-dialog/)
  assert.doesNotMatch(groups, /<el-drawer/)
  assert.match(groups, /v-if="isNormalPage"/)
  assert.match(groups, /v-model="overlay\.bookGroupVisible"/)
  assert.match(groups, /:title="overlay\.bookGroupMode === 'set' \? '设置分组' : '分组管理'"/)
  assert.match(groups, /width="min\(1000px, max\(750px, 70vw\)\)"/)
  assert.match(groups, /top="max\(15dvh, calc\(\(100dvh - 584px\) \/ 2\)\)"/)
  assert.match(groups, /:fullscreen="isMobile"/)
  assert.match(groups, /class="global-book-group-dialog"/)
  assert.equal((groups.match(/<el-table(?:\s|>)/g) || []).length, 1, 'BookGroup must render one table for both modes')
  assert.match(groups, /:data="groupRows"/)
  assert.match(groups, /:height="isMobile \? 'calc\(100dvh - 184px\)' : 'min\(400px, calc\(70dvh - 184px\)\)'"/)
  assert.match(groups, /type="selection"[\s\S]*?width="25"[\s\S]*?v-if="isSetMode"/)
  assert.match(groups, /prop="name"[\s\S]*?label="分组名"[\s\S]*?min-width="100"/)
  assert.match(groups, /class="group-drag-icon"/)
  assert.match(groups, /prop="show"[\s\S]*?label="显示"[\s\S]*?min-width="80"[\s\S]*?v-if="!isSetMode"/)
  assert.match(groups, /label="操作"\s+width="100px"/)
  assert.match(groups, /@click="createCategory"/, 'set mode must retain upstream add-group')
  assert.match(groups, /renameGroup\(row\)/, 'set mode rows must retain upstream edit action')
  assert.ok(groups.indexOf('@click="createCategory"') < groups.indexOf('@click="saveSetting"'), 'add-group must precede confirm')
  assert.doesNotMatch(groups, /@row-click=/)
  assert.doesNotMatch(groups, /active-text=|inactive-text=/)
  assert.doesNotMatch(groups, /<el-empty/)
  assert.doesNotMatch(groups, /description|groupBookCount\(row\)\s*}}\s*本/)
  assert.doesNotMatch(groups, /grid-template-columns/)
  assert.match(host, /<OverlayBookGroups\s+:is-mobile="isMobileOverlay"\s*\/>/)
})

test('manages built-ins and custom categories as one ordered projection', () => {
  const groups = read('../src/components/overlays/OverlayBookGroups.vue')
  const controller = read('../src/composables/useOverlayBookGroups.js')
  const shelf = read('../src/stores/bookshelf.js')

  assert.match(controller, /bookshelf\.bookGroups/)
  assert.match(controller, /displayBookGroupName/)
  assert.match(controller, /reorderBookGroupKeys/)
  assert.match(groups, /row\.kind === 'category'/)
  assert.match(shelf, /bookGroups:/)
  assert.match(shelf, /loadBookGroups/)
})

test('keeps book-group state as one global overlay controller instead of a route or drawer', () => {
  const groups = read('../src/components/overlays/OverlayBookGroups.vue')
  const layout = read('../src/layouts/AppLayout.vue')

  assert.match(layout, /\{ key: 'bookGroup', label: '分组管理', action: \(\) => overlay\.openBookGroup\('manage'\) \}/)
  assert.doesNotMatch(layout, /\{ key: 'bookGroup',[^\n]*route:/)
  assert.match(groups, /useOverlayBookGroups/)
  assert.match(groups, /\(\) => overlay\.bookGroupVisible/)
  assert.match(groups, /reader\.pageType === 'normal'/)
  assert.match(groups, /loadBookGroups\(\{ force: true \}\)/)
  assert.match(groups, /watch\(bookGroupProjectionRevision/)
  assert.match(groups, /watch\(categoryProjectionRevision/)
})
