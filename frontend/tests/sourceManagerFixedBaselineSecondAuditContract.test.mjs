import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const manager = readFileSync(new URL('../src/components/workspace/SourceManager.vue', import.meta.url), 'utf8')
const overlay = readFileSync(new URL('../src/components/overlays/OverlaySources.vue', import.meta.url), 'utf8')
const transfer = readFileSync(new URL('../src/composables/useSourceTransfer.js', import.meta.url), 'utf8')

function count(source, pattern) {
  return (source.match(pattern) || []).length
}

function assertInOrder(source, values, message) {
  let previous = -1
  for (const value of values) {
    const index = source.indexOf(value)
    assert.ok(index >= 0, `${message}: missing ${value}`)
    assert.ok(index > previous, `${message}: ${value} is out of order`)
    previous = index
  }
}

test('owns normal/failure source management as one fixed-baseline dialog without a transfer background shell', () => {
  assert.match(overlay, /<SourceManager/)
  assert.match(overlay, /<SourceTransferOverlay/)
  assert.match(overlay, /isManagerIntent/)
  assert.doesNotMatch(overlay, /<el-dialog/, 'the owner must not wrap import/remote in a second manager dialog')

  assert.match(manager, /:title="isFailureMode \? '失效书源管理' : '书源管理'"/)
  assert.match(manager, /width="min\(1000px, max\(750px, 70vw\)\)"/)
  assert.match(manager, /top="max\(15dvh, calc\(\(100dvh - 584px\) \/ 2\)\)"/)
  assert.match(manager, /:fullscreen="isMobile"/)
  assert.match(manager, /v-if="isNormalPage"/)
  assert.doesNotMatch(manager, /destroy-on-close/)
})

test('restores the exact normal manager action, group, single-table, pagination, and footer structure', () => {
  assertInOrder(manager, ['新增', '导出', '恢复默认', '清空'], 'normal title actions')
  assert.equal(count(manager, /<el-table(?:\s|>)/g), 1, 'desktop and mobile must share one source table')
  assertInOrder(manager, ['type="selection"', '书源名称', '书源链接', '书架书籍', '操作'], 'normal source columns')
  assert.match(manager, /width="25"[^>]*:fixed="isMobile"[^>]*:selectable="isSourceSelectable"/s)
  assert.match(manager, /prop="name"\s+label="书源名称"\s+min-width="120"\s+:fixed="isMobile"/s)
  assert.match(manager, /target="_blank"/)
  assert.match(manager, /row\.usedBookNames/)
  assert.match(manager, /width="100px"/)
  assert.match(manager, /sourcePageSize\s*=\s*ref\(25\)/)
  assert.match(manager, /\[25, 50, 100, 200, 300, 400\]/)
  assert.match(manager, /layout="total, sizes, prev, pager, next"/)
  assert.match(manager, /:pager-count="isMobile \? 5 : 7"/)
  assertInOrder(manager, ['批量删除', '已选择 {{ selection.length }} 个', '取消'], 'normal footer')

  assert.doesNotMatch(manager, /mobile-source-card|mobile-source-list|source-batch-footer/)
  assert.doesNotMatch(manager, /<el-drawer/)
  assert.doesNotMatch(manager, /启用选中|停用选中|设置分组|停用失败|只看失败|导出选中|更多批量操作/)
  assert.doesNotMatch(manager, /@click="openDebug|@click="deleteSource\(/)
})

test('restores cached failure entry and the upstream failure form instead of a permanent health toolbar', () => {
  assert.match(manager, /const checkConfig = reactive\(\{\s*keyword: '斗罗大陆',\s*timeout: 5000,\s*concurrent: 5,?\s*\}\)/s)
  assert.match(manager, /v-if="isFailureMode"[^>]*class="source-check-form"/)
  assertInOrder(manager, ['搜索词：', '超时(ms)：', '并发数：'], 'failure form')
  assert.match(manager, /:min="1000"[^>]*:max="15000"[^>]*:step="500"/s)
  assert.match(manager, /:min="3"[^>]*:max="15"[^>]*:step="1"/s)
  assert.match(manager, /请输入搜索关键词/)
  assert.match(manager, /listInvalidSources/)
  assert.match(manager, /isFailureMode[\s\S]*?await loadInvalidSourceHealth/)
  assert.doesNotMatch(manager, /isFailureMode[\s\S]*?await checkInvalidSources\(\)/)
  assert.match(manager, /检测书源/)
  assert.match(manager, /checkProgress/)
  const failureGroups = manager.match(/const failureGroupOrder = \[([\s\S]*?)\n\]/)?.[1] || ''
  assertInOrder(failureGroups, ['UnknownHostException', 'ConnectException: Failed to connect', 'SocketException: Connection reset', 'SSLHandshakeException', 'responseCode: 307', 'responseCode: 513', 'timeout'], 'failure group order')
  assert.match(manager, /visibleFailureCategory\(source\.errorMessage\)/)
  assert.match(manager, /code === 'source_request_failed'[\s\S]*?'ConnectException: Failed to connect'/)
})

test('uses one full reader-dev JSON editor and preserves explicit validation messages', () => {
  assert.match(manager, /class="source-json-editor-dialog"/)
  assert.match(manager, /title="编辑书源"/)
  assert.match(manager, /class="source-json-editor"/)
  assert.match(manager, /JSON\.stringify\([^)]*,\s*null,\s*4\)/)
  assert.match(manager, /书源名称不能为空/)
  assert.match(manager, /书源链接不能为空/)
  assert.match(manager, /书源必须是JSON格式/)
  assert.match(manager, /保存书源成功/)
  assert.match(manager, /bookSourceName:\s*'新增书源'/)
  assert.match(manager, /ruleContent:\s*\{\s*content:\s*''\s*\}/s)
  assert.match(manager, /ruleToc:\s*\{[\s\S]*?chapterList:\s*''[\s\S]*?chapterName:\s*''[\s\S]*?chapterUrl:\s*''/)
  assert.doesNotMatch(manager, /<el-form-item/)
})

test('keeps transfer preview confirmation explicit and starts with no selected sources', () => {
  assert.match(transfer, /checkedImportSourceIndexes\.value = \[\]/)
  assert.doesNotMatch(transfer, /checkedImportSourceIndexes\.value = selectableIndexes\(list\)/)
  assert.match(transfer, /书源文件错误/)
  assert.match(transfer, /远程书源文件错误/)
  assert.match(transfer, /部分使用了Javascript和Webview的书源未勾选/)
  assert.match(transfer, /请选择需要导入的源/)
  assert.match(transfer, /导入书源成功/)
})
