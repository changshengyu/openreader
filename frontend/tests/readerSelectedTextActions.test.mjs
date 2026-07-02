import assert from 'node:assert/strict'
import test from 'node:test'
import { useReaderSelectedTextActions } from '../src/composables/useReaderSelectedTextActions.js'

function createController(overrides = {}) {
  const calls = []
  const controller = useReaderSelectedTextActions({
    getBook: () => ({ title: '测试书', url: 'https://example.com/book' }),
    confirm: async () => 'confirm',
    prompt: async () => ({ value: '替换内容' }),
    createBookmark: async text => calls.push(['bookmark', text]),
    createReplaceRule: async payload => calls.push(['rule', payload]),
    dispatchRulesUpdated: () => calls.push(['dispatch']),
    onSuccess: message => calls.push(['success', message]),
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

test('creates scoped replacement rules and broadcasts the update', async () => {
  const fixture = createController()
  const text = '这是一段超过二十四个字符的选中文字用于验证规则名称截断逻辑'
  await fixture.controller.operate(`  ${text}  `)
  assert.deepEqual(fixture.calls, [
    ['rule', {
      name: `${text.slice(0, 24)}...`,
      pattern: text,
      replacement: '替换内容',
      scope: '测试书;https://example.com/book',
      isRegex: false,
      enabled: true,
    }],
    ['dispatch'],
    ['success', '过滤规则已添加'],
  ])
})

test('does not create a rule after cancelling the replacement prompt', async () => {
  const fixture = createController({
    prompt: async () => {
      throw new Error('cancel')
    },
  })
  await fixture.controller.operate('选中文字')
  assert.deepEqual(fixture.calls, [])
})
