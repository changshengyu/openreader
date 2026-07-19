import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive } from 'vue'
import { useOverlayBookGroups } from '../src/composables/useOverlayBookGroups.js'

function createController(overrides = {}) {
  const calls = []
  const overlay = reactive({
    bookGroupMode: 'manage',
    bookGroupVisible: true,
    bookInfoBook: null,
    bookInfoOptions: {},
  })
  const bookshelf = reactive({
    categories: [
      { id: 1, name: '玄幻', show: true },
      { id: 2, name: '历史', show: true },
      { id: 3, name: '科幻', show: true },
    ],
    bookGroups: [
      { key: 'builtin:all', kind: 'builtin', semantic: 'all', name: '全部', defaultName: '全部', show: true, sortOrder: -10 },
      { key: 'builtin:local', kind: 'builtin', semantic: 'local', name: '本地', defaultName: '本地', show: true, sortOrder: -9 },
      { key: 'builtin:audio', kind: 'builtin', semantic: 'audio', name: '音频', defaultName: '音频', show: true, sortOrder: -8 },
      { key: 'builtin:ungrouped', kind: 'builtin', semantic: 'ungrouped', name: '未分组', defaultName: '未分组', show: true, sortOrder: -7 },
      { key: 'category:1', kind: 'category', semantic: 'category', categoryId: 1, name: '玄幻', show: true, sortOrder: 10 },
      { key: 'category:2', kind: 'category', semantic: 'category', categoryId: 2, name: '历史', show: true, sortOrder: 20 },
      { key: 'category:3', kind: 'category', semantic: 'category', categoryId: 3, name: '科幻', show: true, sortOrder: 30 },
    ],
    books: [
      { id: 11, sourceId: 0, type: 0, categoryIds: [1] },
      { id: 12, sourceId: 9, type: 1, categoryIds: [1, 2] },
      { id: 13, sourceId: 9, type: 0, categoryIds: [] },
    ],
    upsertBook: book => calls.push(['upsert', book]),
    addCategory: async payload => calls.push(['add', payload]),
    renameCategory: async (id, payload) => calls.push(['rename', id, payload]),
    setCategoryVisible: async (id, show) => calls.push(['visible', id, show]),
    loadCategories: async options => calls.push(['load-categories', options]),
    loadBookGroups: async options => calls.push(['load-book-groups', options]),
    updateBuiltInBookGroup: async (key, payload) => calls.push(['update-builtin', key, payload]),
    removeCategory: async id => calls.push(['remove', id]),
    reorderBookGroupKeys: async keys => calls.push(['reorder-groups', keys]),
  })
  let sortableOptions
  const sortable = {
    destroy: () => calls.push(['destroy-sortable']),
  }
  const controller = useOverlayBookGroups({
    overlay,
    bookshelf,
    getManagedBooks: () => bookshelf.books,
    updateBookCategory: async (id, categoryIds) => {
      calls.push(['set-book-groups', id, categoryIds])
      return {
        data: {
          ...overlay.bookInfoBook,
          categoryIds,
        },
      }
    },
    categoryName: book => `分组:${book.categoryIds.join(',')}`,
    getBookProgress: () => ({ percent: 0.42 }),
    emitBookInfoUpdated: book => calls.push(['emit', book]),
    prompt: async () => ({ value: '新分组' }),
    confirm: async (...args) => calls.push(['confirm', ...args]),
    createSortable: (element, options) => {
      calls.push(['create-sortable', element])
      sortableOptions = options
      return sortable
    },
    nextFrame: async () => calls.push(['next-frame']),
    onSuccess: message => calls.push(['success', message]),
    onWarning: message => calls.push(['warning', message]),
    onError: (...args) => calls.push(['error', ...args]),
    ...overrides,
  })
  return {
    calls,
    controller,
    overlay,
    bookshelf,
    getSortableOptions: () => sortableOptions,
  }
}

test('prepares and saves the selected groups for the current book', async () => {
  const fixture = createController()
  fixture.overlay.bookGroupMode = 'set'
  fixture.overlay.bookInfoBook = { id: 9, categoryIds: [2] }
  fixture.controller.prepareOpen()

  assert.equal(fixture.controller.groupSetRows.value[0].description, '2 本')
  assert.equal(fixture.controller.groupSetRows.value[1].description, '1 本')
  assert.equal(fixture.controller.isBookGroupSelected({ id: 2 }), true)

  fixture.controller.toggleBookGroupSelection({ id: 1 })
  await fixture.controller.saveBookGroupSetting()

  assert.deepEqual(fixture.calls[0], ['set-book-groups', 9, [2, 1]])
  assert.deepEqual(fixture.overlay.bookInfoBook.categoryIds, [2, 1])
  assert.equal(fixture.overlay.bookInfoOptions.categoryName, '分组:2,1')
  assert.equal(fixture.overlay.bookInfoOptions.progress, 0.42)
  assert.equal(fixture.overlay.bookGroupVisible, false)
  assert.equal(fixture.controller.settingCategorySaving.value, false)
  assert.deepEqual(fixture.calls.at(-1), ['success', '分组已设置'])
})

test('keeps upstream BookGroup set semantics by rejecting an empty selection', async () => {
  const fixture = createController()
  fixture.overlay.bookGroupMode = 'set'
  fixture.overlay.bookInfoBook = { id: 9, categoryIds: [2] }
  fixture.controller.prepareOpen()
  fixture.controller.toggleBookGroupSelection({ id: 2 })

  await fixture.controller.saveBookGroupSetting()

  assert.deepEqual(fixture.calls, [
    ['warning', '请选择书籍分组'],
  ])
  assert.equal(fixture.overlay.bookGroupVisible, true)
  assert.deepEqual(fixture.overlay.bookInfoBook.categoryIds, [2])
  assert.equal(fixture.controller.settingCategorySaving.value, false)
})

test('prepares the current book selection when an open drawer changes mode', async () => {
  const fixture = createController()
  fixture.overlay.bookInfoBook = { id: 9, categoryIds: [1, 3] }

  await fixture.controller.handleModeChange('set')

  assert.equal(fixture.controller.isBookGroupSelected({ id: 1 }), true)
  assert.equal(fixture.controller.isBookGroupSelected({ id: 2 }), false)
  assert.equal(fixture.controller.isBookGroupSelected({ id: 3 }), true)
})

test('creates and renames custom groups while preserving cancellation semantics', async () => {
  const fixture = createController()
  await fixture.controller.createCategory()
  await fixture.controller.renameGroup({ id: 2, name: '旧名称' })

  assert.deepEqual(fixture.calls, [
    ['add', { name: '新分组' }],
    ['success', '分组已创建'],
    ['rename', 2, { name: '新分组' }],
    ['success', '分组已重命名'],
  ])

  const cancelled = createController({
    prompt: async () => {
      throw 'cancel'
    },
  })
  await cancelled.controller.createCategory()
  await cancelled.controller.renameGroup({ id: 2, name: '旧名称' })
  assert.deepEqual(cancelled.calls, [])
})

test('manages four built-in groups together with custom groups', async () => {
  const fixture = createController()
  fixture.controller.prepareOpen()

  assert.deepEqual(
    fixture.controller.groupManageRows.value.map(group => group.key),
    fixture.bookshelf.bookGroups.map(group => group.key),
  )
  assert.deepEqual(
    fixture.controller.groupManageRows.value.map(group => fixture.controller.groupBookCount(group)),
    [3, 1, 1, 1, 2, 1, 0],
  )
  assert.equal(
    fixture.controller.displayBookGroupName(fixture.bookshelf.bookGroups[0]),
    '全部(全部)',
  )

  await fixture.controller.renameGroup(fixture.bookshelf.bookGroups[2])
  await fixture.controller.toggleGroupVisibility(fixture.bookshelf.bookGroups[0], false)

  assert.deepEqual(fixture.calls, [
    ['update-builtin', 'audio', { name: '新分组' }],
    ['success', '分组已重命名'],
    ['update-builtin', 'all', { show: false }],
    ['success', '分组已隐藏'],
  ])
})

test('reloads category state when visibility changes fail', async () => {
  const failure = new Error('save failed')
  const fixture = createController()
  fixture.bookshelf.setCategoryVisible = async () => {
    throw failure
  }

  await fixture.controller.toggleGroupVisibility({ id: 2 }, false)

  assert.deepEqual(fixture.calls, [
    ['load-book-groups', { force: true }],
    ['error', failure, '修改分组显示状态失败'],
  ])
  assert.equal(fixture.controller.visibilitySavingId.value, null)
})

test('protects non-empty groups and deletes empty confirmed groups', async () => {
  const fixture = createController()
  await fixture.controller.deleteGroup(fixture.bookshelf.bookGroups[0])
  await fixture.controller.deleteGroup(fixture.bookshelf.bookGroups[4])
  await fixture.controller.deleteGroup(fixture.bookshelf.bookGroups[6])

  assert.deepEqual(fixture.calls, [
    ['warning', '内置分组不能删除'],
    ['warning', '分组内还有书籍，清空后才能删除'],
    ['confirm', '确定删除分组“科幻”吗？', '删除分组', { type: 'warning' }],
    ['remove', 3],
    ['success', '分组已删除'],
  ])
})

test('owns sortable lifecycle and persists the drafted group order', async () => {
  const fixture = createController()
  const tableBody = {}
  fixture.controller.prepareOpen()
  fixture.controller.groupManageTableRef.value = {
    $el: {
      querySelector: selector => {
        assert.equal(selector, '.el-table__body-wrapper tbody')
        return tableBody
      },
    },
  }

  await fixture.controller.handleBookGroupOpened()
  fixture.getSortableOptions().onEnd({ oldIndex: 0, newIndex: 2 })

  assert.deepEqual(fixture.controller.groupOrderDraftKeys.value, [
    'builtin:local',
    'builtin:audio',
    'builtin:all',
    'builtin:ungrouped',
    'category:1',
    'category:2',
    'category:3',
  ])
  assert.equal(fixture.controller.isGroupOrderDirty.value, true)
  await fixture.controller.saveGroupOrderDraft()
  await fixture.controller.handleModeChange('set')

  assert.deepEqual(fixture.calls, [
    ['next-frame'],
    ['create-sortable', tableBody],
    ['reorder-groups', [
      'builtin:local',
      'builtin:audio',
      'builtin:all',
      'builtin:ungrouped',
      'category:1',
      'category:2',
      'category:3',
    ]],
    ['success', '分组排序已更新'],
    ['destroy-sortable'],
  ])
  assert.equal(fixture.controller.groupOrderSaving.value, false)
})
