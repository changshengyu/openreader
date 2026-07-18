import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderSelection } from '../src/composables/useReaderSelection.js'
import {
  normalizeReaderSelectionText,
  readerSelectionBelongsToRoot,
} from '../src/utils/readerSelection.js'

test('preserves original selected reader text while rejecting whitespace-only selections', () => {
  assert.equal(normalizeReaderSelectionText('  第一段\n\n第二段  '), '  第一段\n\n第二段  ')
  assert.equal(normalizeReaderSelectionText('abcdef', 4), 'abcd')
  assert.equal(normalizeReaderSelectionText(' \n '), '')
})

test('accepts selection containers only inside the reader root', () => {
  const inside = {}
  const root = { contains: value => value === inside }
  assert.equal(readerSelectionBelongsToRoot(root, inside), true)
  assert.equal(readerSelectionBelongsToRoot(root, {}), false)
})

test('does not start delayed selection polling for an ordinary tap without a selection', () => {
  const originalWindow = globalThis.window
  const originalSetTimeout = globalThis.setTimeout
  const originalClearTimeout = globalThis.clearTimeout
  const timers = []
  globalThis.window = {
    Node: { ELEMENT_NODE: 1 },
    getSelection: () => ({
      rangeCount: 0,
      toString: () => '',
    }),
  }
  globalThis.setTimeout = (callback, delay) => {
    timers.push({ callback, delay })
    return timers.length
  }
  globalThis.clearTimeout = () => {}

  try {
    const selection = useReaderSelection({
      contentBody: ref({ contains: () => true }),
      getAction: () => '操作弹窗',
      onOperate: () => {},
    })
    assert.equal(selection.schedule(0, { retry: false }), false)
    assert.deepEqual(timers, [], 'a short page tap must not poll selection during animation')
  } finally {
    globalThis.window = originalWindow
    globalThis.setTimeout = originalSetTimeout
    globalThis.clearTimeout = originalClearTimeout
  }
})
