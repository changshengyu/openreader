import assert from 'node:assert/strict'
import test from 'node:test'
import {
  BOOK_SOURCE_RULE_KEYS,
  buildBookSourcePayload,
  buildReaderDevBookSource,
  createBookSourceForm,
  sourceToEditorSnapshot,
} from '../src/utils/bookSourceEditor.js'

test('shared source editor converts reader-dev aliases and nested rules once', () => {
  const snapshot = sourceToEditorSnapshot({
    bookSourceName: '上游书源',
    bookSourceUrl: 'https://source.example',
    bookSourceGroup: '分组',
    searchUrl: '/search?key={{key}}',
    exploreUrl: '/explore?page={{page}}',
    ruleSearch: {
      bookList: '.book',
      name: '.name@text',
      bookUrl: 'a@href',
    },
    ruleBookInfo: {
      name: 'h1@text',
      tocUrl: '.toc@href',
    },
    ruleToc: {
      chapterList: '.chapter',
      chapterName: '.chapter-name@text',
      chapterUrl: 'a@href',
    },
    ruleContent: {
      content: '.content@text',
    },
  })

  assert.equal(snapshot.form.name, '上游书源')
  assert.equal(snapshot.form.baseUrl, 'https://source.example')
  assert.equal(snapshot.form.group, '分组')
  assert.equal(snapshot.rules.searchUrl, '/search?key={keyword}')
  assert.equal(snapshot.rules.exploreUrl, '/explore?page={page}')
  assert.equal(snapshot.rules.bookNameRule, '.name|text')
  assert.equal(snapshot.rules.bookUrlRule, 'a|attr:href')
  assert.equal(snapshot.rules.tocUrlRule, '.toc|attr:href')
  assert.equal(snapshot.rules.contentRule, '.content|text')
})

test('shared source editor payload keeps complete non-empty rules without writable-file duplication', () => {
  const form = createBookSourceForm({ name: '测试源', baseUrl: 'https://source.example' })
  const payload = buildBookSourcePayload(form, {
    searchUrl: '/search?q={keyword}',
    contentRule: '.content|text',
    empty: '',
    textReplaceRules: [{ pattern: '广告', replacement: '' }],
  })
  const rules = JSON.parse(payload.rules)

  assert.equal(payload.name, '测试源')
  assert.equal(rules.searchUrl, '/search?q={keyword}')
  assert.equal(rules.contentRule, '.content|text')
  assert.equal('empty' in rules, false)
  assert.deepEqual(rules.textReplaceRules, [{ pattern: '广告', replacement: '' }])
  assert(BOOK_SOURCE_RULE_KEYS.includes('explorePaginationRule'))
})

test('shared source editor generates a reader-dev compatible lossless JSON snapshot', () => {
  const exported = buildReaderDevBookSource({
    name: '测试源',
    group: '分组',
    baseUrl: 'https://source.example',
    enabled: true,
    enabledExplore: true,
  }, {
    searchUrl: '/search?q={keyword}&page={page}',
    exploreUrl: '/explore?page={page}',
    bookListRule: '.book',
    bookNameRule: '.name|text',
    bookUrlRule: 'a|attr:href',
    exploreBookNameRule: '.explore-name|text',
    tocUrlRule: '.toc|attr:href',
    chapterListRule: '.chapter',
    chapterNameRule: '.chapter-name|text',
    chapterUrlRule: 'a|attr:href',
    contentRule: '.content|html',
    paginationRule: '.next|attr:href',
  })

  assert.equal(exported.bookSourceName, '测试源')
  assert.equal(exported.bookSourceUrl, 'https://source.example')
  assert.equal(exported.searchUrl, '/search?q={{key}}&page={{page}}')
  assert.equal(exported.ruleSearch.name, '.name@text')
  assert.equal(exported.ruleSearch.bookUrl, 'a@href')
  assert.equal(exported.ruleExplore.name, '.explore-name@text')
  assert.equal(exported.ruleBookInfo.tocUrl, '.toc@href')
  assert.equal(exported.ruleToc.chapterName, '.chapter-name@text')
  assert.equal(exported.ruleContent.content, '.content@html')
  assert.equal(JSON.parse(exported.rules).paginationRule, '.next|attr:href')

  const roundTrip = sourceToEditorSnapshot(exported)
  assert.equal(roundTrip.form.name, '测试源')
  assert.equal(roundTrip.rules.searchUrl, '/search?q={keyword}&page={page}')
  assert.equal(roundTrip.rules.paginationRule, '.next|attr:href')
})

test('shared source editor preserves unknown reader-dev top-level fields without executing them', () => {
  const source = {
    bookSourceName: '扩展字段源',
    bookSourceUrl: 'https://source-extra.example',
    enabled: true,
    enabledExplore: true,
    loginUi: [{ name: '账号', type: 'text' }],
    customExtension: {
      mode: 'preserve-only',
      script: '@js:never-execute()',
    },
    ruleToc: {
      preUpdateJs: '@js:preserve-toc()',
      chapterList: '.chapter',
    },
    ruleContent: {
      content: '.content',
      webJs: '@js:preserve-content()',
    },
  }

  const snapshot = sourceToEditorSnapshot(source)
  const payload = buildBookSourcePayload(snapshot.form, snapshot.rules)
  const reopened = buildReaderDevBookSource(
    sourceToEditorSnapshot(payload).form,
    sourceToEditorSnapshot(payload).rules,
  )

  assert.deepEqual(reopened.loginUi, source.loginUi)
  assert.deepEqual(reopened.customExtension, source.customExtension)
  assert.equal(reopened.ruleToc.preUpdateJs, '@js:preserve-toc()')
  assert.equal(reopened.ruleContent.webJs, '@js:preserve-content()')
  assert.doesNotMatch(reopened.rules, /__openreaderSourceExtra/)
})
