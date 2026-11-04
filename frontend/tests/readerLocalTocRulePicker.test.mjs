import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { ref } from 'vue'
import { ElInput, ElOption, ElSelect } from 'element-plus'
import { useReaderLocalTocRulePicker } from '../src/composables/useReaderLocalTocRulePicker.js'

// reader-dev PopCatalog.vue 164–252 (fixed baseline fa22f27): "修改规则" opens one $msgbox for TXT and EPUB books
// with "请选择规则：", a filterable rule select and a textarea sharing the same value, refuses an unchanged rule
// with "未修改规则", and starts from the book's current rule. The old prompt-only assertions conflicted with
// that dialog and were rewritten by docs/compat/txt-toc-rule-selection-fixed-baseline-second-audit-p1-contract.md.

const TXT_RULES = [
  { id: -1, enable: true, name: '目录(去空白)', rule: 'rule-1', serialNumber: 0 },
  { id: -2, enable: true, name: '目录', rule: 'rule-2', serialNumber: 1 },
  { id: -4, enable: false, name: '目录(古典、轻小说备用)', rule: 'rule-4', serialNumber: 3 },
  { id: -19, enable: true, name: '目录(章回、话)', rule: 'rule-19', serialNumber: 18 },
]

const UPSTREAM_EPUB_RULES = [
  ['根据 Spin 获取章节，使用 Toc 补充章节名', 'spin+toc'],
  ['根据 Spin 获取章节，强制使用 Toc 章节名', 'spin<toc'],
  ['根据 Spin 获取章节', 'spin'],
  ['根据 Toc 获取章节，使用 Spin 补充章节名', 'toc+spin'],
  ['根据 Toc 获取章节，强制使用 Spin 章节名', 'toc<spin'],
  ['根据 Toc 获取章节', 'toc'],
]

// Renders the dialog body the way ElMessageBox would, without a DOM.
function dialogBody(config) {
  const render = config.message.type.setup()
  return () => {
    const root = render()
    const [label, select, input] = root.children
    return { root, label, select, input, options: select.children.default() }
  }
}

function createPicker({ tocRule = 'rule-1', epub = false, interact = async () => 'confirm', loadTxtTocRules } = {}) {
  const calls = []
  const book = ref({ id: 7, tocRule })
  const picker = useReaderLocalTocRulePicker({
    book,
    isEPUBLocalBook: ref(epub),
    msgbox: async config => {
      calls.push(['msgbox', config])
      return interact(config)
    },
    loadTxtTocRules: loadTxtTocRules || (async () => {
      calls.push(['load-rules'])
      return TXT_RULES
    }),
    notifyError: message => calls.push(['notify-error', message]),
    onError: (error, fallback) => calls.push(['on-error', error?.message, fallback]),
    // The pre-contract picker used a free-text prompt (TXT) and a native-select confirm (EPUB); record them so a
    // regression shows up as the wrong dialog rather than a missing function.
    prompt: async (...args) => {
      calls.push(['prompt', ...args])
      return { value: 'prompt-value' }
    },
    confirm: async (...args) => {
      calls.push(['confirm', ...args])
      return true
    },
  })
  return { book, calls, picker }
}

function assertUpstreamDialogs(calls, count = 1) {
  assert.deepEqual(
    calls.map(call => call[0]).filter(kind => ['msgbox', 'prompt', 'confirm'].includes(kind)),
    Array(count).fill('msgbox'),
    'the rule picker must open the upstream $msgbox dialog (select + textarea), not a prompt or native select',
  )
}

test('offers the built-in TXT rules in the upstream select-plus-textarea dialog', async () => {
  const fixture = createPicker({
    interact: async config => {
      const body = dialogBody(config)
      const first = body()
      assert.equal(first.root.type, 'div')
      assert.deepEqual(first.root.props.style, { textAlign: 'center' })
      assert.equal(first.label.type, 'span')
      assert.equal(first.label.children, '请选择规则：')
      assert.equal(first.select.type, ElSelect)
      assert.equal(first.select.props.modelValue, 'rule-1')
      assert.equal(first.select.props.size, 'small')
      assert.equal(first.select.props.filterable, true)
      assert.equal(first.select.props.placeholder, '内置规则')
      assert.deepEqual(
        first.options.map(option => [option.type === ElOption, option.props.label, option.props.value]),
        TXT_RULES.map(rule => [true, rule.name, rule.rule]),
      )
      assert.equal(first.input.type, ElInput)
      assert.equal(first.input.props.type, 'textarea')
      assert.equal(first.input.props.rows, 3)
      assert.equal(first.input.props.size, 'small')
      assert.deepEqual(first.input.props.style, { marginTop: '10px' })
      assert.equal(first.input.props.modelValue, 'rule-1')

      first.select.props['onUpdate:modelValue']('rule-19')
      const chosen = body()
      assert.equal(chosen.select.props.modelValue, 'rule-19')
      assert.equal(chosen.input.props.modelValue, 'rule-19', 'the textarea shows the chosen rule text')
      return 'confirm'
    },
  })
  const chosen = await fixture.picker.choose()
  assertUpstreamDialogs(fixture.calls)
  assert.equal(chosen, 'rule-19')
  const [, config] = fixture.calls.find(call => call[0] === 'msgbox')
  assert.equal(config.title, '修改目录规则')
  assert.equal(config.showCancelButton, true)
  assert.equal(config.confirmButtonText, '确定')
  assert.equal(config.cancelButtonText, '取消')
  assert.equal(config.customClass, 'reader-toc-rule-dialog')
  assert.equal(fixture.calls.some(call => call[0] === 'notify-error'), false)
})

test('returns an edited TXT regular expression from the shared textarea', async () => {
  const fixture = createPicker({
    interact: async config => {
      const body = dialogBody(config)
      body().input.props['onUpdate:modelValue']('^卷.+$')
      assert.equal(body().select.props.modelValue, '^卷.+$', 'the select shares the textarea value')
      return 'confirm'
    },
  })
  const edited = await fixture.picker.choose()
  assertUpstreamDialogs(fixture.calls)
  assert.equal(edited, '^卷.+$')
})

test('refuses an unchanged rule with the upstream message and no refresh value', async () => {
  const untouched = createPicker()
  const unchanged = await untouched.picker.choose()
  assertUpstreamDialogs(untouched.calls)
  assert.equal(unchanged, null)
  assert.deepEqual(untouched.calls.filter(call => call[0] === 'notify-error'), [['notify-error', '未修改规则']])

  const roundTrip = createPicker({
    interact: async config => {
      const body = dialogBody(config)
      body().select.props['onUpdate:modelValue']('rule-2')
      body().select.props['onUpdate:modelValue']('rule-1')
      return 'confirm'
    },
  })
  const restored = await roundTrip.picker.choose()
  assertUpstreamDialogs(roundTrip.calls)
  assert.equal(restored, null)
  assert.deepEqual(roundTrip.calls.filter(call => call[0] === 'notify-error'), [['notify-error', '未修改规则']])
})

test('maps cancel and close to null without a message', async () => {
  for (const action of ['cancel', 'close']) {
    const fixture = createPicker({
      interact: async config => {
        dialogBody(config)().select.props['onUpdate:modelValue']('rule-2')
        throw action
      },
    })
    const cancelled = await fixture.picker.choose()
    assertUpstreamDialogs(fixture.calls)
    assert.equal(cancelled, null)
    assert.equal(fixture.calls.some(call => call[0] === 'notify-error'), false)
  }
})

test('uses the same dialog with the six upstream EPUB rules', async () => {
  const fixture = createPicker({
    tocRule: 'spin+toc',
    epub: true,
    interact: async config => {
      const body = dialogBody(config)
      const first = body()
      assert.equal(first.select.type, ElSelect)
      assert.equal(first.input.type, ElInput)
      assert.deepEqual(first.options.map(option => [option.props.label, option.props.value]), UPSTREAM_EPUB_RULES)
      assert.equal(first.select.props.modelValue, 'spin+toc')
      first.select.props['onUpdate:modelValue']('toc+spin')
      return 'confirm'
    },
  })
  const epubRule = await fixture.picker.choose()
  assertUpstreamDialogs(fixture.calls)
  assert.equal(epubRule, 'toc+spin')
  assert.equal(fixture.calls.some(call => call[0] === 'load-rules'), false, 'EPUB books do not load TXT rules')
})

test('loads the TXT rules once and still opens the dialog when loading fails', async () => {
  let attempts = 0
  const fixture = createPicker({
    loadTxtTocRules: async () => {
      attempts += 1
      if (attempts === 1) throw new Error('rules unavailable')
      return TXT_RULES
    },
    interact: async config => {
      const body = dialogBody(config)
      body().input.props['onUpdate:modelValue']('^第.+回.*$')
      return 'confirm'
    },
  })
  const afterFailure = await fixture.picker.choose()
  assertUpstreamDialogs(fixture.calls)
  assert.equal(afterFailure, '^第.+回.*$')
  assert.deepEqual(fixture.calls.filter(call => call[0] === 'on-error'), [['on-error', 'rules unavailable', '加载目录规则失败']])
  const [, failedConfig] = fixture.calls.find(call => call[0] === 'msgbox')
  assert.equal(dialogBody(failedConfig)().options.length, 0, 'a failed load opens the dialog without options')

  assert.equal(await fixture.picker.choose(), '^第.+回.*$')
  assert.equal(await fixture.picker.choose(), '^第.+回.*$')
  assertUpstreamDialogs(fixture.calls, 3)
  assert.equal(attempts, 2, 'a failed load is retried once, then the loaded list is reused')
  const lastConfig = fixture.calls.filter(call => call[0] === 'msgbox').at(-1)[1]
  assert.equal(dialogBody(lastConfig)().options.length, TXT_RULES.length)
})

test('Reader wires the picker to ElMessageBox and the TXT rule list', () => {
  const reader = readFileSync(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')
  const wiring = reader.match(/useReaderLocalTocRulePicker\(\{[\s\S]*?\n\}\)/)
  assert.ok(wiring, 'Reader must create the local TOC rule picker')
  assert.match(wiring[0], /msgbox: \(\.\.\.args\) => ElMessageBox\(\.\.\.args\)/)
  assert.match(wiring[0], /loadTxtTocRules: async \(\) => \(await listTXTTocRules\(\)\)\.data/)
  assert.match(wiring[0], /notifyError: message => ElMessage\.error\(message\)/)
  assert.match(wiring[0], /onError: \(error, fallback\) => ElMessage\.error\(readError\(error, fallback\)\)/)
  assert.match(reader, /import \{[^}]*\blistTXTTocRules\b[^}]*\} from '\.\.\/api\/books'/)
})
