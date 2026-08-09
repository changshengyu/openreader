import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import {
  createDefaultRSSSource,
  normalizeRSSSourceImport,
  parseRSSSortOptions,
  safeRSSImportIndexes,
} from '../src/utils/rssSourceImport.js'

const manager = readFileSync(new URL('../src/components/RSSManager.vue', import.meta.url), 'utf8')

test('manual RSS JSON draft uses the exact fixed-baseline defaults', () => {
  assert.deepEqual(createDefaultRSSSource(), {
    sourceName: '新增RSS源',
    sourceUrl: '',
    sourceIcon: '',
    sourceGroup: '',
    enabled: true,
    singleUrl: true,
    articleStyle: 0,
    ruleArticles: '',
    ruleTitle: '',
    rulePubDate: '',
    ruleImage: '',
    ruleLink: '',
    ruleContent: '',
    enableJs: true,
  })
})

test('RSS import keeps upstream names/defaults and does not invent blank identities', () => {
  assert.deepEqual(normalizeRSSSourceImport([
    { sourceName: '有效源', sourceUrl: ' https://rss.example/feed ', headerMap: { Token: 'x' } },
    { sourceName: '', sourceUrl: 'https://rss.example/blank-name' },
  ]), [
    {
      sourceName: '有效源',
      sourceUrl: 'https://rss.example/feed',
      header: '{"Token":"x"}',
      singleUrl: false,
    },
    { sourceName: '', sourceUrl: 'https://rss.example/blank-name', singleUrl: false },
  ])
})

test('safe RSS select-all includes index zero and excludes Javascript/WebView records', () => {
  assert.deepEqual(safeRSSImportIndexes([
    { sourceName: '第一个安全源', sourceUrl: 'https://rss.example/1' },
    { sourceName: 'JS 源', sourceUrl: 'https://rss.example/2', ruleTitle: '@js:x' },
    { sourceName: 'WebView 源', sourceUrl: 'https://rss.example/3', loginUrl: 'webView:https://example' },
    { sourceName: '第二个安全源', sourceUrl: 'https://rss.example/4' },
  ]), [0, 3])
})

test('visible RSS sort tabs parse only upstream newline name::url rows', () => {
  assert.deepEqual(parseRSSSortOptions({
    sourceUrl: 'https://rss.example/base',
    singleUrl: false,
    sortUrl: '新闻::/news\r\n科技::/tech&&不应拆分::/extra',
  }), [
    { name: '新闻', url: '/news' },
    { name: '科技', url: '/tech&&不应拆分::/extra' },
  ])
  assert.deepEqual(parseRSSSortOptions({ sourceUrl: 'https://rss.example/base', singleUrl: true }), [])
})

test('RSS manager uses JSON editor, checkbox import, and request ownership gates', () => {
  assert.match(manager, /<RSSJsonEditorDialog/)
  assert.match(manager, /<RSSImportDialog/)
  assert.match(manager, /const articleListRequestGate = createRSSArticleRequestGate\(\)/)
  assert.match(manager, /const articleLoadMoreRequestGate = createRSSArticleRequestGate\(\)/)
  assert.doesNotMatch(manager, /<el-form-item label="名称"/)
  assert.doesNotMatch(manager, /<el-collapse-item title="高级规则"/)
})
