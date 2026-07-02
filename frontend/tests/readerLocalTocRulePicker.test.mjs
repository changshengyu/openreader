import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderLocalTocRulePicker } from '../src/composables/useReaderLocalTocRulePicker.js'

function createController(overrides = {}) {
  const calls = []
  const book = ref({ id: 7, tocRule: '^第.+章$' })
  const isEPUBLocalBook = ref(false)
  const controller = useReaderLocalTocRulePicker({
    book,
    isEPUBLocalBook,
    prompt: async (...args) => {
      calls.push(['prompt', ...args])
      return { value: '^卷.+$' }
    },
    confirm: async (...args) => {
      calls.push(['confirm', ...args])
      return true
    },
    ...overrides,
  })
  return { book, calls, controller, isEPUBLocalBook }
}

test('returns the TXT regular expression from the existing prompt contract', async () => {
  const fixture = createController()
  assert.equal(await fixture.controller.choose(), '^卷.+$')
  assert.deepEqual(fixture.calls[0].slice(0, 3), [
    'prompt',
    '填写 TXT 目录行正则，留空则使用默认目录规则。',
    '修改目录规则',
  ])
  assert.deepEqual(fixture.calls[0][3], {
    confirmButtonText: '刷新目录',
    cancelButtonText: '取消',
    inputType: 'textarea',
    inputValue: '^第.+章$',
    inputPlaceholder: '^第.+章.*$',
  })
})

test('keeps blank TXT rules and maps prompt cancellation to null', async () => {
  const blank = createController({
    prompt: async () => ({ value: '' }),
  })
  assert.equal(await blank.controller.choose(), '')

  const cancelled = createController({
    prompt: async () => {
      throw new Error('cancel')
    },
  })
  assert.equal(await cancelled.controller.choose(), null)
})

test('selects an EPUB TOC priority and preserves confirmation cancellation', async () => {
  const fixture = createController({
    confirm: async selector => {
      fixture.calls.push(['confirm', selector])
      selector.props.onChange({ target: { value: 'toc+spin' } })
      return true
    },
  })
  fixture.isEPUBLocalBook.value = true
  fixture.book.value.tocRule = 'spin+toc'
  assert.equal(await fixture.controller.choose(), 'toc+spin')
  const selector = fixture.calls[0][1]
  assert.equal(selector.type, 'select')
  assert.equal(selector.children.length, 6)

  const cancelled = createController({
    confirm: async () => {
      throw new Error('cancel')
    },
  })
  cancelled.isEPUBLocalBook.value = true
  assert.equal(await cancelled.controller.choose(), null)
})
