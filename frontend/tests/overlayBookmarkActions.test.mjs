import assert from 'node:assert/strict'
import test from 'node:test'
import { useOverlayBookmarkActions } from '../src/composables/useOverlayBookmarkActions.js'

function createController(overrides = {}) {
  const calls = []
  let book = { id: 7, title: '测试书' }
  const controller = useOverlayBookmarkActions({
    getBook: () => book,
    closePanel: () => calls.push(['close']),
    navigate: route => calls.push(['navigate', route]),
    removeMany: async rows => calls.push(['remove-many', rows]),
    importPayloads: async rows => {
      calls.push(['import', rows])
      return rows.map((row, index) => ({ ...row, id: index + 1 }))
    },
    confirm: async (...args) => calls.push(['confirm', ...args]),
    onSuccess: message => calls.push(['success', message]),
    onInvalidImport: message => calls.push(['invalid', message]),
    onError: (...args) => calls.push(['error', ...args]),
    ...overrides,
  })
  return {
    calls,
    controller,
    setBook(value) {
      book = value
    },
  }
}

test('closes the panel and navigates to the bookmark position', () => {
  const fixture = createController()
  fixture.controller.jump({
    chapterIndex: 3,
    offset: 25,
    percent: '0.4',
  })
  assert.deepEqual(fixture.calls, [
    ['close'],
    ['navigate', {
      name: 'reader',
      params: { id: 7 },
      query: { chapter: 3, offset: 25, percent: 0.4 },
    }],
  ])
  fixture.calls.length = 0
  fixture.setBook(null)
  fixture.controller.jump({ chapterIndex: 1 })
  assert.deepEqual(fixture.calls, [])
})

test('removes selected bookmarks with the upstream batch confirmation', async () => {
  const fixture = createController()
  const rows = [{ id: 1 }, { id: 2 }]
  await fixture.controller.removeMany(rows)
  assert.deepEqual(fixture.calls, [
    ['confirm', '确认要删除所选择的 2 条书签吗？', '批量删除书签', { type: 'warning' }],
    ['remove-many', rows],
    ['success', '书签已删除'],
  ])
})

test('normalizes imported bookmarks before confirmation and creation', async () => {
  const fixture = createController()
  await fixture.controller.importRows([{
    durChapterIndex: 2,
    chapterName: '第三章',
    bookText: '摘录',
  }])
  assert.equal(fixture.calls[0][0], 'confirm')
  assert.deepEqual(fixture.calls[1], ['import', [{
    chapterIndex: 2,
    offset: 0,
    percent: 0,
    title: '第三章',
    excerpt: '摘录',
    note: '',
  }]])
  assert.deepEqual(fixture.calls[2], ['success', '已导入 1 条书签'])

  fixture.calls.length = 0
  await fixture.controller.importRows([])
  assert.deepEqual(fixture.calls, [
    ['invalid', '书签文件没有可导入内容'],
  ])
})
