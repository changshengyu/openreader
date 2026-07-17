import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderSelection } from '../src/composables/useReaderSelection.js'

test('waits for a mobile selection that settles after touchend', async () => {
  const originalWindow = globalThis.window
  const selectedNode = { nodeType: 1 }
  const root = { contains: node => node === selectedNode }
  let text = ''
  const selection = {
    rangeCount: 1,
    toString: () => text,
    getRangeAt: () => ({ commonAncestorContainer: selectedNode }),
    removeAllRanges: () => {},
  }
  globalThis.window = {
    Node: { ELEMENT_NODE: 1 },
    getSelection: () => selection,
  }
  const operated = []
  try {
    const controller = useReaderSelection({
      contentBody: ref(root),
      getAction: () => '操作弹窗',
      onOperate: value => operated.push(value),
      retryInterval: 10,
      retryWindow: 80,
    })
    controller.schedule(10)
    setTimeout(() => { text = '移动端稍后稳定的选中文字' }, 25)
    await new Promise(resolve => setTimeout(resolve, 110))
    assert.deepEqual(operated, ['移动端稍后稳定的选中文字'])
  } finally {
    globalThis.window = originalWindow
  }
})
