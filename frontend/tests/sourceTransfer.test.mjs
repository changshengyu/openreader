import assert from 'node:assert/strict'
import test from 'node:test'
import {
  analyzeSourceCompatibility,
  importSourceName,
  importSourceTags,
  importSourceURL,
  parseImportSourceList,
  sourceImportMessage,
  useSourceTransfer,
} from '../src/composables/useSourceTransfer.js'

function createController(overrides = {}) {
  const calls = []
  const selection = [{ id: 2 }, { id: 4 }]
  const controller = useSourceTransfer({
    previewRemoteSource: async url => ({
      data: { sources: [{ name: `远程 ${url}`, baseUrl: url }] },
    }),
    importSources: async form => {
      calls.push(['import', form])
      return { data: { imported: 1, updated: 1, skipped: 1 } }
    },
    exportSources: async ids => {
      calls.push(['export', ids])
      return { data: '[{"name":"书源"}]' }
    },
    reloadSources: async () => calls.push(['reload']),
    getSelection: () => selection,
    download: (data, filename) => calls.push(['download', data, filename]),
    onInfo: message => calls.push(['info', message]),
    onWarning: message => calls.push(['warning', message]),
    onSuccess: message => calls.push(['success', message]),
    onError: (error, fallback) => calls.push(['error', fallback, error]),
    ...overrides,
  })
  return { calls, controller, selection }
}

test('normalizes supported source JSON shapes and display fields', () => {
  const source = {
    bookSourceName: '兼容源',
    bookSourceUrl: 'https://example.com',
    ruleSearch: '@js: return 1',
    loginUrl: 'webView:https://example.com/login',
  }

  assert.deepEqual(parseImportSourceList({ bookSources: [source] }), [source])
  assert.deepEqual(parseImportSourceList({ sources: [source] }), [source])
  assert.deepEqual(parseImportSourceList(source), [source])
  assert.deepEqual(parseImportSourceList({ invalid: true }), [])
  assert.equal(importSourceName(source), '兼容源')
  assert.equal(importSourceURL(source), 'https://example.com')
  assert.equal(importSourceTags(source), '@Javascript @WebView')
  assert.equal(
    sourceImportMessage({ imported: 2, updated: 1, skipped: 3 }),
    '新增 2 个，更新 1 个，跳过 3 个',
  )
})

test('classifies only executable source fields and keeps dormant fields non-blocking', () => {
  const supported = analyzeSourceCompatibility({
    bookSourceName: '名称里提到 JavaScript 和 {{普通文字}}',
    bookSourceComment: '注释中的 @js: 与 webView: 不是执行入口',
    header: '{"User-Agent":"OpenReader"}',
    searchUrl: 'https://example.com/search?key={{key}}&page={{page}}',
    ruleSearch: { name: '.name@text' },
    rules: JSON.stringify({
      searchUrl: 'https://example.com/search?key={{key}}',
      exploreUrl: 'https://example.com/explore?page={{page}}',
      bookNameRule: '.name|text',
    }),
  })
  assert.equal(supported.blocking, false)
  assert.equal(supported.status, 'supported')
  assert.deepEqual(supported.tags, [])

  const dormant = analyzeSourceCompatibility({
    ruleToc: { preUpdateJs: 'return chapterList' },
    ruleContent: { webJs: 'return response', sourceRegex: 'window.DATA' },
  })
  assert.equal(dormant.blocking, false)
  assert.equal(dormant.status, 'preserved-dormant')
  assert.deepEqual(dormant.tags, [])
  assert.deepEqual(dormant.dormantFields.sort(), [
    'ruleContent.sourceRegex',
    'ruleContent.webJs',
    'ruleToc.preUpdateJs',
  ])
})

test('detects every source entry point rejected by the current runtime', () => {
  const fixtures = [
    [{ header: '@js:return headers' }, 'dynamic-header'],
    [{ header: '<js>return headers</js>' }, 'dynamic-header'],
    [{ header: '', headerMap: '<js>return headers</js>' }, 'dynamic-header'],
    [{ loginCheckJs: 'return result' }, 'login-check'],
    [{ loginCheckJs: '', loginCheckJS: 'return result' }, 'login-check'],
    [{ ruleSearch: { name: '@js:return book.name' } }, 'rule-script'],
    [{ ruleBookInfo: { name: '<js>return book.name</js>' } }, 'rule-script'],
    [{ rules: JSON.stringify({ bookNameRule: '{{book.name}}' }) }, 'rule-template'],
    [{ loginUrl: 'webView:https://example.com/login' }, 'webview'],
  ]

  for (const [source, reason] of fixtures) {
    const result = analyzeSourceCompatibility(source)
    assert.equal(result.blocking, true, `${reason} must not be selected by default`)
    assert.ok(result.reasons.includes(reason), `${reason} must have a stable reason`)
  }
  assert.equal(importSourceTags(fixtures[0][0]), '@Javascript')
  assert.equal(importSourceTags(fixtures.at(-1)[0]), '@WebView')
})

test('preselects compatible sources and preserves check-all semantics', () => {
  const fixture = createController()
  fixture.controller.openImportPreview([
    { name: '普通源' },
    { name: '脚本源', rule: '@js: return true' },
    { name: 'WebView 源', login: 'webView:https://example.com' },
    { name: '动态头源', header: '<js>return headers</js>' },
    { name: '登录检测源', loginCheckJs: 'return result' },
    { name: '固定基准未消费字段', ruleToc: { preUpdateJs: 'return list' } },
  ])

  assert.deepEqual(fixture.controller.checkedImportSourceIndexes.value, [0, 5])
  assert.equal(fixture.controller.importCheckAll.value, true)
  assert.equal(fixture.controller.importCheckIndeterminate.value, false)
  assert.deepEqual(fixture.calls, [[
    'info',
    '部分使用 Javascript 或 WebView 的书源未默认勾选',
  ]])

  fixture.controller.toggleImportCheckAll(false)
  assert.deepEqual(fixture.controller.checkedImportSourceIndexes.value, [])
  fixture.controller.toggleImportCheckAll(true)
  assert.deepEqual(fixture.controller.checkedImportSourceIndexes.value, [0, 5])
})

test('imports a local file through preview and saves only selected sources', async () => {
  const fixture = createController()
  await fixture.controller.importFile({
    raw: {
      text: async () => JSON.stringify([
        { name: '源一' },
        { name: '源二' },
      ]),
    },
  })
  fixture.controller.checkedImportSourceIndexes.value = [1]
  await fixture.controller.saveSelectedImportSources()

  const importCall = fixture.calls.find(call => call[0] === 'import')
  const uploaded = importCall[1].get('file')
  assert.deepEqual(JSON.parse(await uploaded.text()), [{ name: '源二' }])
  assert.deepEqual(fixture.calls.slice(-2), [
    ['success', '新增 1 个，更新 1 个，跳过 1 个'],
    ['reload'],
  ])
  assert.equal(fixture.controller.showImportPreview.value, false)
  assert.equal(fixture.controller.importPreviewSaving.value, false)
})

test('previews a trimmed remote URL and resets the remote dialog', async () => {
  const fixture = createController()
  fixture.controller.showRemote.value = true
  fixture.controller.remoteURL.value = ' https://example.com/sources.json '
  await fixture.controller.importRemote()

  assert.equal(fixture.controller.showRemote.value, false)
  assert.equal(fixture.controller.remoteURL.value, '')
  assert.equal(fixture.controller.remoteLoading.value, false)
  assert.equal(fixture.controller.showImportPreview.value, true)
  assert.equal(
    fixture.controller.importPreviewSources.value[0].baseUrl,
    'https://example.com/sources.json',
  )
})

test('exports selected sources with the existing filename and feedback', async () => {
  const fixture = createController()
  await fixture.controller.exportSources()

  assert.deepEqual(fixture.calls, [
    ['export', [2, 4]],
    ['download', '[{"name":"书源"}]', 'bookSources-selected.json'],
    ['success', '已导出 2 个书源'],
  ])
})
