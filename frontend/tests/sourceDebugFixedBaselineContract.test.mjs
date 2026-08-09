import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import test from 'node:test'

const router = readFileSync(new URL('../src/router/index.js', import.meta.url), 'utf8')
const layout = readFileSync(new URL('../src/layouts/AppLayout.vue', import.meta.url), 'utf8')
const sourceManager = readFileSync(new URL('../src/components/workspace/SourceManager.vue', import.meta.url), 'utf8')
const sourceAPI = readFileSync(new URL('../src/api/sources.js', import.meta.url), 'utf8')
const debugViewURL = new URL('../src/views/SourceDebug.vue', import.meta.url)

function assertLabelsInOrder(source, labels) {
  let previous = -1
  for (const label of labels) {
    const index = source.indexOf(label)
    assert(index >= 0, `missing ${label}`)
    assert(index > previous, `${label} is out of fixed-baseline order`)
    previous = index
  }
}

test('Index opens the fixed-baseline standalone debugger in one named tab', () => {
  const sourceSection = layout.slice(layout.indexOf("key: 'sources'"), layout.indexOf("key: 'bookshelf'"))
  assert.match(sourceSection, /key:\s*'sourceDebug'[\s\S]*?action:\s*openSourceDebugWorkspace/)
  assert.match(layout, /function\s+openSourceDebugWorkspace\s*\(/)
  assert.match(layout, /router\.resolve\(\{\s*name:\s*'source-debug'/)
  assert.match(layout, /window\.open\([^)]*'_target'/s, 'upstream uses one reusable named debugger tab')
  assert.doesNotMatch(sourceSection, /openSourceManage\('debug'\)/)
})

test('router owns the canonical and legacy source-debug paths outside the Index overlay', () => {
  assert.match(router, /const SourceDebug = \(\) => import\('\.\.\/views\/SourceDebug\.vue'\)/)
  assert.match(router, /path:\s*'\/source-debug'[\s\S]*?name:\s*'source-debug'[\s\S]*?component:\s*SourceDebug/)
  assert.match(router, /alias:\s*\[[^\]]*'\/bookSourceDebug'[^\]]*'\/bookSourceDebug\/'/)
  assert.match(router, /action === 'debug'[\s\S]*?name:\s*'source-debug'/, 'the old /sources?action=debug link must translate to the standalone workspace')
})

test('SourceManager no longer owns the incorrect three independent probe dialog', () => {
  assert.doesNotMatch(sourceManager, /<el-dialog[^>]*title="书源调试"/)
  assert.doesNotMatch(sourceManager, /debugKeyword|debugBookURL|debugChapterURL|useSearchResultForChapter|useChapterForContent/)
  assert.doesNotMatch(sourceManager, /testSourceSearch|testSourceChapter|testSourceContent/)
  assert.doesNotMatch(sourceManager, /intent === 'debug'/)
})

test('standalone debugger restores the editor, command rail, output tabs, and automatic stream action', () => {
  assert.equal(existsSync(debugViewURL), true, 'SourceDebug.vue must exist')
  const view = readFileSync(debugViewURL, 'utf8')

  assert.match(view, /class="source-debug-workspace"/)
  assert.match(view, /class="source-debug-rule-pane"/)
  assert.match(view, /class="source-debug-command-rail"/)
  assert.match(view, /class="source-debug-output-pane"/)
  assertLabelsInOrder(view, ['基本', '搜索', '发现', '详情', '目录', '正文', '其它规则'])
  assertLabelsInOrder(view, ['推送源', '拉取源', '编辑源', '生成源', '清空表单', '撤销操作', '重做操作', '调试源', '保存源'])
  assertLabelsInOrder(view, ['编辑源', '调试源', '源列表', '帮助信息'])
  assert.match(view, /ref\('我的'\)/, 'the debug keyword must default to 我的')
  assert.match(view, /@keyup\.enter="runDebug"/)
  const runDebug = view.match(/async function runDebug\(\) \{([\s\S]*?)\n\}/)?.[1] || ''
  assert(runDebug.indexOf('saveCurrentSource') >= 0, 'debug must save the current editor source first')
  assert(runDebug.indexOf('saveCurrentSource') < runDebug.indexOf('debugSourceStream'), 'the stream must not start before save succeeds')
  assert.match(view, /AbortController/)
  assert.match(view, /onBeforeUnmount\([\s\S]*?abort\(/)
  assert.match(view, /importSources\(createSourceImportForm\(rows\)\)/, 'push must synchronize the debugger list into the app')
  assert.match(view, /sources\.value\.map\(sourceSnapshot\)/, 'pull must refresh the debugger list from the app')
  assert.match(view, /parseImportSourceList|exportLocalSources|deleteSelectedLocalSource|clearLocalSources/)
  assert.match(view, /URLSearchParams\(window\.location\.hash/)
  assert.match(view, /window\.history\.replaceState/)
  assert.match(view, /buildBookSourcePayload|sourceToEditorSnapshot/, 'the debugger must reuse the shared source conversion')
  assert.doesNotMatch(view, /function\s+emptySource|function\s+emptyRules/)
})

test('source debug API uses a cancellable Bearer POST stream and never puts JWT in the URL', () => {
  assert.match(sourceAPI, /export async function debugSourceStream\(id, keyword, options = \{\}\)/)
  const fn = sourceAPI.slice(sourceAPI.indexOf('export async function debugSourceStream'), sourceAPI.length)
  assert.match(fn, /fetch\(`\/api\/sources\/\$\{id\}\/debug\/stream`/)
  assert.match(fn, /method:\s*'POST'/)
  assert.match(fn, /Authorization:\s*`Bearer \$\{token\}`/)
  assert.match(fn, /signal:\s*options\.signal/)
  assert.doesNotMatch(fn, /[?&](token|jwt)=/i)
})
