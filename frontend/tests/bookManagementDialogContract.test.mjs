import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))

function read(relative) {
  return readFileSync(resolve(__dirname, relative), 'utf8')
}

test('hosts one normal-page BookManage dialog with fixed-upstream geometry', () => {
  const manager = read('../src/components/overlays/OverlayBookManagement.vue')
  const host = read('../src/components/GlobalOverlayHost.vue')

  assert.match(manager, /<el-dialog/)
  assert.match(manager, /v-if="isNormalPage"/)
  assert.match(manager, /v-model="overlay\.bookManageVisible"/)
  assert.match(manager, /title="书架管理"/)
  assert.match(manager, /width="min\(1000px, max\(750px, 70vw\)\)"/)
  assert.match(manager, /top="max\(15dvh, calc\(\(100dvh - 584px\) \/ 2\)\)"/)
  assert.match(manager, /:fullscreen="isMobile"/)
  assert.match(manager, /const isNormalPage = computed\(\(\) => reader\.pageType === 'normal'\)/)
  assert.match(manager, /watch\(isNormalPage,[\s\S]*?overlay\.bookManageVisible = false/)
  assert.match(manager, /class="global-book-manage-dialog"/)
  assert.doesNotMatch(manager, /<el-drawer/)
  assert.match(host, /<OverlayBookManagement\s+:is-mobile="isMobileOverlay"\s*\/>/)
})

test('uses the same fixed-upstream table on desktop and mobile', () => {
  const manager = read('../src/components/overlays/OverlayBookManagement.vue')
  const table = read('../src/components/overlays/BookManagementTable.vue')

  assert.equal((manager.match(/<BookManagementTable\b/g) || []).length, 1)
  assert.doesNotMatch(manager, /BookManagementDesktopTable|BookManagementMobileList/)
  assert.doesNotMatch(manager, /progressLabel|localBookSearchText|selectAllManagedBooks|toggleManagedBook/)
  assert.match(table, /:height="isMobile \? 'calc\(100dvh - 226px\)' : 'min\(358px, calc\(70dvh - 226px\)\)'"/)
  assert.match(table, /<el-table-column\s+type="selection"\s+width="25"\s+:fixed="isMobile"/)
  assert.match(table, /prop="title"[\s\S]*?label="书名名"[\s\S]*?min-width="100"[\s\S]*?:fixed="isMobile"/)
  assert.match(table, /prop="author"[\s\S]*?label="作者"[\s\S]*?min-width="100"/)
  assert.match(table, /label="分组"\s+min-width="120"/)
  assert.match(table, /label="章节"\s+min-width="120"/)
  assert.match(table, /label="操作"\s+width="100px"/)
  assert.doesNotMatch(table, /阅读进度|fixed="right"|远程书籍|本地书籍|最新：/)
})

test('restores exact upstream search, cache, export, and footer surfaces', () => {
  const manager = read('../src/components/overlays/OverlayBookManagement.vue')
  const toolbar = read('../src/components/overlays/BookManagementToolbar.vue')
  const actions = read('../src/components/overlays/BookManagementActions.vue')
  const footer = read('../src/components/overlays/BookManagementBatchFooter.vue')

  assert.match(manager, /❗️只能缓存文本内容/)
  assert.match(manager, /<template #footer>[\s\S]*?<BookManagementBatchFooter/)
  assert.match(toolbar, /placeholder="搜索书名或作者"/)
  assert.match(toolbar, /:prefix-icon="Search"/)
  assert.doesNotMatch(toolbar, /全选|清空|搜索书名、作者或文件名/)

  const server = actions.indexOf('缓存到服务器')
  const browser = actions.indexOf('缓存到浏览器')
  const deleteServer = actions.indexOf('删除服务器缓存')
  const deleteBrowser = actions.indexOf('删除浏览器缓存')
  assert(server >= 0 && server < browser && browser < deleteServer && deleteServer < deleteBrowser)
  assert.match(actions, /<Loading\s*\/>[\s\S]*?缓存中/)
  assert.doesNotMatch(actions, /停止|cacheProgress|cancel-cache/)
  assert.match(actions, /导出为TXT/)
  assert.match(actions, /导出为Epub/)
  assert.doesNotMatch(actions, /导出为 TXT|导出书籍数据/)

  assert.match(footer, /批量删除/)
  assert.match(footer, /批量添加分组/)
  assert.match(footer, /批量移除分组/)
  assert.match(footer, /已选择 \{\{ selectedCount \}\} 个/)
  assert.doesNotMatch(footer, /:disabled="!selectedCount"|更多批量操作|批量缓存到服务器|批量清服务器缓存|批量导出/)
})

test('preserves the query while close clears only selection and every open forces a full shelf read', () => {
  const manager = read('../src/components/overlays/OverlayBookManagement.vue')

  assert.match(manager, /bookshelf\.loadBooks\(\{ force: true, all: true \}\)/)
  assert.match(manager, /if \(!visible\) \{[\s\S]*?clearManagedSelection\(\)[\s\S]*?return/)
  assert.doesNotMatch(manager, /if \(!visible\) \{[\s\S]*?manageKeyword\.value = ''/)
  assert.match(manager, /const q = manageKeyword\.value\.trim\(\)\.toLowerCase\(\)/)
  assert.match(manager, /String\(book\.title \|\| ''\)\.toLowerCase\(\)\.includes\(q\)/)
  assert.match(manager, /String\(book\.author \|\| ''\)\.toLowerCase\(\)\.includes\(q\)/)
  assert.doesNotMatch(manager, /localBookSearchText|normalizeLocalBookSearch|progressLabel/)
})
