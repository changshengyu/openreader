import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))

function read(relative) {
  return readFileSync(resolve(__dirname, relative), 'utf8')
}

test('hosts BookManage in an upstream-style root dialog with compact fullscreen behavior', () => {
  const manager = read('../src/components/overlays/OverlayBookManagement.vue')
  const host = read('../src/components/GlobalOverlayHost.vue')

  assert.match(manager, /<el-dialog/)
  assert.doesNotMatch(manager, /<el-drawer/)
  assert.match(manager, /v-model="overlay\.bookManageVisible"/)
  assert.match(manager, /title="书架管理"/)
  assert.match(manager, /:fullscreen="isMobile"/)
  assert.match(manager, /class="global-book-manage-dialog"/)
  assert.match(manager, /BookManagementDesktopTable/)
  assert.match(manager, /BookManagementMobileList/)
  assert.match(manager, /BookManagementBatchFooter/)
  assert.match(host, /<OverlayBookManagement\s+:is-mobile="isMobileOverlay"\s*\/>/)
})

test('keeps management state as one overlay controller rather than a route or parallel drawer', () => {
  const manager = read('../src/components/overlays/OverlayBookManagement.vue')
  const layout = read('../src/layouts/AppLayout.vue')

  assert.match(layout, /\{ key: 'bookManage', label: '书籍管理', action: \(\) => overlay\.openBookManage\(\) \}/)
  assert.match(manager, /useOverlayBookManagement/)
  assert.match(manager, /\(\) => overlay\.bookManageVisible/)
  assert.doesNotMatch(layout, /\{ key: 'bookManage',[^\n]*route:/)
})

test('matches the upstream BookManage cache menu and footer surface', () => {
  const manager = read('../src/components/overlays/OverlayBookManagement.vue')
  const actions = read('../src/components/overlays/BookManagementActions.vue')
  const footer = read('../src/components/overlays/BookManagementBatchFooter.vue')

  assert.match(manager, /❗️只能缓存文本内容/)
  const server = actions.indexOf('缓存到服务器')
  const browser = actions.indexOf('缓存到浏览器')
  const deleteServer = actions.indexOf('删除服务器缓存')
  const deleteBrowser = actions.indexOf('删除浏览器缓存')
  assert(server >= 0 && server < browser && browser < deleteServer && deleteServer < deleteBrowser)
  assert.doesNotMatch(actions, /导出书籍数据/)
  assert.doesNotMatch(footer, /更多批量操作|批量缓存到服务器|批量清服务器缓存|批量导出/)
  assert.match(footer, /批量删除/)
  assert.match(footer, /批量添加分组/)
  assert.match(footer, /批量移除分组/)
})
