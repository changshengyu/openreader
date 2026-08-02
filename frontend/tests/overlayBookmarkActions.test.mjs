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
    onInvalidSelection: message => calls.push(['invalid-selection', message]),
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
    ['confirm', '确认要删除所选择的书签吗?', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    }],
    ['remove-many', rows],
    ['success', '删除书签成功'],
  ])
})

test('reports an upstream empty-selection error without confirmation or mutation', async () => {
  const fixture = createController()
  await fixture.controller.removeMany([])
  assert.deepEqual(fixture.calls, [
    ['invalid-selection', '请选择需要删除的书签'],
  ])
})

test('treats closed delete and import confirmations as no-op actions', async () => {
  const deleteFixture = createController({
    confirm: async (...args) => {
      deleteFixture.calls.push(['confirm', ...args])
      throw 'cancel'
    },
  })
  await deleteFixture.controller.removeMany([{ id: 1 }])
  assert.equal(deleteFixture.calls.length, 1)
  assert.equal(deleteFixture.calls[0][0], 'confirm')

  const importFixture = createController({
    confirm: async (...args) => {
      importFixture.calls.push(['confirm', ...args])
      throw 'close'
    },
  })
  await importFixture.controller.importRows([{ chapterIndex: 0, bookText: '正文' }])
  assert.equal(importFixture.calls.length, 1)
  assert.equal(importFixture.calls[0][0], 'confirm')
})

test('uses the fixed-upstream delete failure fallback after confirmation', async () => {
  const failure = new Error('delete failed')
  const fixture = createController({
    removeMany: async () => { throw failure },
  })
  await fixture.controller.removeMany([{ id: 1 }])
  assert.deepEqual(fixture.calls.at(-1), ['error', failure, '删除书签失败'])
  assert.equal(fixture.calls.some(call => call[0] === 'success'), false)
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
  assert.deepEqual(fixture.calls[0], [
    'confirm',
    '确认要导入文件中的1条书签吗?',
    '提示',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    },
  ])
  assert.deepEqual(fixture.calls[2], ['success', '导入书签成功'])

  fixture.calls.length = 0
  await fixture.controller.importRows([{
    chapterIndex: 4,
    chapterName: '只有章节',
    content: '只有备注',
  }])
  assert.deepEqual(fixture.calls, [
    ['invalid', '书签文件没有可导入内容'],
  ])

  fixture.calls.length = 0
  await fixture.controller.importRows([])
  assert.deepEqual(fixture.calls, [
    ['invalid', '书签文件没有可导入内容'],
  ])
})
