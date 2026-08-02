import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive } from 'vue'
import { useOverlayBookGroups } from '../src/composables/useOverlayBookGroups.js'

function deferred() {
  let resolve
  const promise = new Promise(resolvePromise => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

function createController(overrides = {}) {
  const calls = []
  const promptCalls = []
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
    option: (...args) => calls.push(['sortable-option', ...args]),
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
    prompt: async (...args) => {
      promptCalls.push(args)
      return { value: '新分组' }
    },
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
    promptCalls,
    getSortableOptions: () => sortableOptions,
  }
}

test('prepares and saves the selected groups for the current book', async () => {
  const fixture = createController()
  fixture.overlay.bookGroupMode = 'set'
  fixture.overlay.bookInfoBook = { id: 9, categoryIds: [2] }
  fixture.controller.prepareOpen()

  assert.equal(fixture.controller.groupSetRows.value[0].description, undefined)
  assert.equal(fixture.controller.groupSetRows.value[1].description, undefined)
  assert.deepEqual(fixture.controller.selectedCategoryIds.value, ['2'])

  fixture.controller.handleBookGroupSelectionChange([
    fixture.controller.groupSetRows.value[1],
    fixture.controller.groupSetRows.value[0],
  ])
  await fixture.controller.saveBookGroupSetting()

  assert.deepEqual(fixture.calls[0], ['set-book-groups', 9, [2, 1]])
  assert.deepEqual(fixture.overlay.bookInfoBook.categoryIds, [2, 1])
  assert.equal(fixture.overlay.bookInfoOptions.categoryName, '分组:2,1')
  assert.equal(fixture.overlay.bookInfoOptions.progress, 0.42)
  assert.equal(fixture.overlay.bookGroupVisible, false)
  assert.equal(fixture.controller.settingCategorySaving.value, false)
  assert.deepEqual(fixture.calls.at(-1), ['success', '设置成功'])
})

test('projects set and manage modes through one table data source', () => {
  const fixture = createController()

  fixture.overlay.bookGroupMode = 'manage'
  assert.deepEqual(
    fixture.controller.groupRows.value.map(row => row.key),
    fixture.bookshelf.bookGroups.map(row => row.key),
  )

  fixture.overlay.bookGroupMode = 'set'
  assert.deepEqual(
    fixture.controller.groupRows.value.map(row => Number(row.id)),
    [1, 2, 3],
  )
  fixture.controller.handleBookGroupSelectionChange([
    fixture.controller.groupRows.value[0],
    fixture.controller.groupRows.value[2],
  ])
  assert.deepEqual(fixture.controller.selectedCategoryIds.value, ['1', '3'])
})

test('keeps upstream BookGroup set semantics by rejecting an empty selection', async () => {
  const fixture = createController()
  fixture.overlay.bookGroupMode = 'set'
  fixture.overlay.bookInfoBook = { id: 9, categoryIds: [2] }
  fixture.controller.prepareOpen()
  fixture.controller.handleBookGroupSelectionChange([])

  await fixture.controller.saveBookGroupSetting()

  assert.deepEqual(fixture.calls, [
    ['error', null, '请选择书籍分组'],
  ])
  assert.equal(fixture.overlay.bookGroupVisible, true)
  assert.deepEqual(fixture.overlay.bookInfoBook.categoryIds, [2])
  assert.equal(fixture.controller.settingCategorySaving.value, false)
})

test('prepares the current book selection when the shared dialog changes mode', async () => {
  const fixture = createController()
  fixture.overlay.bookInfoBook = { id: 9, categoryIds: [1, 3] }

  await fixture.controller.handleModeChange('set')

  assert.deepEqual(fixture.controller.selectedCategoryIds.value, ['1', '3'])
})

test('creates and renames custom groups while preserving cancellation semantics', async () => {
  const fixture = createController()
  await fixture.controller.createCategory()
  await fixture.controller.renameGroup({ id: 2, name: '旧名称' })

  assert.deepEqual(fixture.calls, [
    ['add', { name: '新分组' }],
    ['success', '添加成功'],
    ['rename', 2, { name: '新分组' }],
    ['load-book-groups', { force: true }],
    ['success', '修改成功'],
  ])
  assert.equal(fixture.promptCalls[0][0], '')
  assert.equal(fixture.promptCalls[0][1], '添加分组')
  assert.equal(fixture.promptCalls[0][2].inputValidator(''), '分组名不能为空')
  assert.equal(fixture.promptCalls[0][2].inputValidator('有效'), true)
  assert.equal(fixture.promptCalls[1][0], '')
  assert.equal(fixture.promptCalls[1][1], '编辑分组')
  assert.equal(fixture.promptCalls[1][2].inputValue, '旧名称')

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
    ['load-book-groups', { force: true }],
    ['success', '修改成功'],
    ['update-builtin', 'all', { show: false }],
    ['load-book-groups', { force: true }],
    ['success', '修改成功'],
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
    ['error', failure, '修改失败'],
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
    ['confirm', '确认要删除该分组吗?', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    }],
    ['remove', 3],
    ['success', '删除分组成功'],
  ])
})

test('owns sortable lifecycle and persists the drafted group order', async () => {
  const fixture = createController()
  const tableBody = {}
  fixture.controller.prepareOpen()
  fixture.controller.groupTableRef.value = {
    $el: {
      querySelector: selector => {
        assert.equal(selector, '.el-table__body-wrapper tbody')
        return tableBody
      },
    },
  }

  await fixture.controller.handleBookGroupOpened()
  assert.equal(fixture.getSortableOptions().handle, '.group-drag-icon')
  assert.equal(fixture.getSortableOptions().animation, undefined)
  assert.equal(fixture.getSortableOptions().forceFallback, undefined)
  assert.equal(fixture.getSortableOptions().fallbackTolerance, undefined)
  fixture.getSortableOptions().onEnd({ oldIndex: 0, newIndex: 2 })

  assert.deepEqual(
    fixture.controller.groupManageRows.value.map(group => group.key),
    fixture.bookshelf.bookGroups.map(group => group.key),
    'Sortable must own the visible DOM order until the upstream-style save refresh',
  )
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
    ['sortable-option', 'disabled', false],
    ['reorder-groups', [
      'builtin:local',
      'builtin:audio',
      'builtin:all',
      'builtin:ungrouped',
      'category:1',
      'category:2',
      'category:3',
    ]],
    ['load-book-groups', { force: true }],
    ['next-frame'],
    ['destroy-sortable'],
    ['create-sortable', tableBody],
    ['sortable-option', 'disabled', false],
    ['success', '保存成功'],
    ['sortable-option', 'disabled', true],
    ['next-frame'],
  ])
  assert.equal(fixture.controller.groupOrderSaving.value, false)
})

test('submits an unchanged name like upstream instead of silently skipping the edit', async () => {
  const fixture = createController({
    prompt: async (...args) => {
      fixture.promptCalls.push(args)
      return { value: '旧名称' }
    },
  })

  await fixture.controller.renameGroup({ id: 2, name: '旧名称' })

  assert.deepEqual(fixture.calls, [
    ['rename', 2, { name: '旧名称' }],
    ['load-book-groups', { force: true }],
    ['success', '修改成功'],
  ])
})

test('does not apply a group response after the authenticated operation expires', async () => {
  const response = deferred()
  let current = true
  const fixture = createController({
    operationGuard: {
      begin: key => ({ key }),
      canCommit: () => current,
      reset: () => {},
    },
    updateBookCategory: async () => response.promise,
  })
  fixture.overlay.bookGroupMode = 'set'
  fixture.overlay.bookInfoBook = { id: 9, categoryIds: [2] }
  fixture.controller.prepareOpen()
  fixture.controller.handleBookGroupSelectionChange([
    fixture.controller.groupSetRows.value[1],
    fixture.controller.groupSetRows.value[0],
  ])

  const pending = fixture.controller.saveBookGroupSetting()
  current = false
  response.resolve({ data: { id: 9, categoryIds: [1, 2] } })
  await pending

  assert.equal(fixture.calls.some(([kind]) => kind === 'upsert'), false)
  assert.equal(fixture.calls.some(([kind]) => kind === 'emit'), false)
  assert.equal(fixture.calls.some(([kind]) => kind === 'success'), false)
  assert.equal(fixture.overlay.bookGroupVisible, true)
})
