import assert from 'node:assert/strict'
import test from 'node:test'
import {
  createReaderSelectedTextReplaceRuleDraft,
  useReaderSelectedTextActions,
} from '../src/composables/useReaderSelectedTextActions.js'

function createController(overrides = {}) {
  const calls = []
  const controller = useReaderSelectedTextActions({
    getBook: () => ({ title: '测试书', url: 'https://example.com/book' }),
    confirm: async () => 'confirm',
    createBookmark: async text => calls.push(['bookmark', text]),
    now: () => new Date('2026-07-11T01:02:03'),
    openReplaceRuleEditor: payload => calls.push(['open-editor', payload]),
    ...overrides,
  })
  return { calls, controller }
}

test('creates a bookmark when the operation dialog is cancelled', async () => {
  const fixture = createController({
    confirm: async () => {
      throw 'cancel'
    },
  })
  await fixture.controller.operate('选中文字')
  assert.deepEqual(fixture.calls, [['bookmark', '选中文字']])
})

test('closes the operation dialog without creating anything', async () => {
  const fixture = createController({
    confirm: async () => {
      throw 'close'
    },
  })
  await fixture.controller.operate('选中文字')
  assert.deepEqual(fixture.calls, [])
})

test('creates the upstream selected-text draft without saving before editor confirmation', async () => {
  const fixture = createController()
  const text = '这是一段选中文字'
  await fixture.controller.operate(text)
  assert.deepEqual(fixture.calls, [
    ['open-editor', {
      name: '文本替换 2026-07-11 01:02:03',
      pattern: text,
      replacement: '',
      scope: '测试书;https://example.com/book',
      isRegex: false,
      enabled: true,
    }],
  ])
})

test('builds the same full editor draft from the original selected text', () => {
  assert.deepEqual(createReaderSelectedTextReplaceRuleDraft({
    text: '第一段\n第二段',
    book: { title: '范围书', url: 'local://scope' },
    now: new Date('2026-07-11T23:59:58'),
  }), {
    name: '文本替换 2026-07-11 23:59:58',
    pattern: '第一段\n第二段',
    replacement: '',
    scope: '范围书;local://scope',
    isRegex: false,
    enabled: true,
  })
})

test('does not open an editor for empty selected text', async () => {
  const fixture = createController()
  await fixture.controller.operate('   ')
  assert.deepEqual(fixture.calls, [])
})
