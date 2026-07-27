import assert from 'node:assert/strict'
import test from 'node:test'
import { useOverlayUserManagement } from '../src/composables/useOverlayUserManagement.js'

function deferred() {
  let resolve
  let reject
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function createController(overrides = {}) {
  const calls = []
  let timerTask
  const userStore = {
    profile: { id: 1 },
    async loadMe() {
      calls.push(['load-me'])
      this.profile = { id: 1 }
    },
  }
  const controller = useOverlayUserManagement({
    userStore,
    getCurrentUserId: () => userStore.profile?.id || null,
    isActive: () => true,
    listUsers: async () => {
      calls.push(['list'])
      return {
        data: [
          { id: 1, username: 'current', role: 'user' },
          { id: 2, username: 'admin', role: 'admin' },
          { id: 3, username: 'reader', role: 'user' },
        ],
      }
    },
    createUser: async payload => calls.push(['create', payload]),
    resetUserPassword: async (...args) => calls.push(['reset-password', ...args]),
    deleteUsers: async ids => {
      calls.push(['delete', ids])
      return { data: { deleted: ids.length } }
    },
    setUserSourcesAsDefault: async id => {
      calls.push(['set-default-sources', id])
      return { data: { count: id } }
    },
    resetUserSources: async ids => {
      calls.push(['reset-sources', ids])
      return { data: { reset: ids.length } }
    },
    updateUser: async (...args) => calls.push(['update', ...args]),
    prompt: async () => ({ value: 'secret12' }),
    confirm: async (...args) => calls.push(['confirm', ...args]),
    onSuccess: message => calls.push(['success', message]),
    onWarning: message => calls.push(['warning', message]),
    onError: (...args) => calls.push(['error', ...args]),
    setTimeout: task => {
      calls.push(['set-timeout'])
      timerTask = task
      return 9
    },
    clearTimeout: id => calls.push(['clear-timeout', id]),
    ...overrides,
  })
  return {
    calls,
    controller,
    runTimer: async () => timerTask?.(),
    userStore,
  }
}

test('loads users while keeping all real accounts selectable for source reset', async () => {
  const fixture = createController()
  fixture.controller.selectedUserIds.value = [1, 2, 3]
  await fixture.controller.load()
  assert.deepEqual(fixture.controller.selectedUserIds.value, [1, 2, 3])
  assert.equal(fixture.controller.isSelectable(fixture.controller.users.value[0]), true)
  assert.equal(fixture.controller.isSelectable(fixture.controller.users.value[1]), true)
  assert.equal(fixture.controller.isSelectable(fixture.controller.users.value[2]), true)
  assert.equal(fixture.controller.isDeletable(fixture.controller.users.value[0]), false)
  assert.equal(fixture.controller.isDeletable(fixture.controller.users.value[1]), false)
  assert.equal(fixture.controller.isDeletable(fixture.controller.users.value[2]), true)
  assert.equal(fixture.controller.isMutable(fixture.controller.users.value[0]), false)
  assert.equal(fixture.controller.isMutable(fixture.controller.users.value[1]), false)
  assert.equal(fixture.controller.isMutable(fixture.controller.users.value[2]), true)
  assert.equal(fixture.controller.usersLoading.value, false)
})

test('debounces active user update events and ignores inactive overlays', async () => {
  const fixture = createController()
  fixture.controller.handleUpdated()
  fixture.controller.handleUpdated()
  assert.deepEqual(fixture.calls, [
    ['set-timeout'],
    ['clear-timeout', 9],
    ['set-timeout'],
  ])
  await fixture.runTimer()
  assert.equal(fixture.calls.filter(call => call[0] === 'list').length, 1)

  const inactive = createController({ isActive: () => false })
  inactive.controller.handleUpdated()
  assert.deepEqual(inactive.calls, [])
})

test('protects selection and validates before creating a user', async () => {
  const fixture = createController()
  await fixture.controller.load()
  fixture.controller.changeSelection(fixture.controller.users.value)
  assert.deepEqual(fixture.controller.selectedUserIds.value, [1, 2, 3])
  fixture.controller.toggleSelection(3, false)
  assert.deepEqual(fixture.controller.selectedUserIds.value, [1, 2])
  fixture.controller.toggleSelection(3, true)
  assert.deepEqual(fixture.controller.selectedUserIds.value, [1, 2, 3])

  fixture.calls.length = 0
  fixture.controller.openCreateDialog()
  fixture.controller.draft.username = 'user-'
  fixture.controller.draft.password = '1234567'
  await fixture.controller.create()
  assert.deepEqual(fixture.calls, [
    ['warning', '用户名至少 5 位且只能包含字母或数字，密码至少 8 位'],
  ])

  fixture.calls.length = 0
  fixture.controller.draft.username = '  reader2  '
  fixture.controller.draft.password = 'secret12'
  await fixture.controller.create()
  assert.deepEqual(fixture.calls[0], ['create', {
    username: 'reader2',
    password: 'secret12',
    canEditSources: true,
    canAccessStore: true,
    canAccessWebdav: true,
  }])
  assert.equal(fixture.controller.createDialogVisible.value, false)
  assert.equal(fixture.controller.creatingUser.value, false)
})

test('resets passwords while treating prompt cancellation as a no-op', async () => {
  const fixture = createController()
  await fixture.controller.resetPassword({ id: 3, username: 'reader' })
  assert.deepEqual(fixture.calls, [
    ['reset-password', 3, { password: 'secret12' }],
    ['success', '重置密码成功'],
  ])

  const cancelled = createController({
    prompt: async () => {
      throw 'cancel'
    },
  })
  await cancelled.controller.resetPassword({ id: 3, username: 'reader' })
  assert.deepEqual(cancelled.calls, [])
})

test('deletes selected users before reloading without exposing the non-upstream cleanup action', async () => {
  const fixture = createController()
  await fixture.controller.load()
  fixture.calls.length = 0
  fixture.controller.selectedUserIds.value = [1, 2, 3]
  await fixture.controller.removeSelected()
  assert.deepEqual(fixture.calls.slice(0, 4), [
    ['confirm', '确认要删除所选择的 1 个用户吗？该用户空间内的书架、进度、书签和设置也会删除。', '批量删除用户', { type: 'warning' }],
    ['delete', [3]],
    ['success', '删除用户成功：1 个'],
    ['list'],
  ])
  assert.deepEqual(fixture.controller.selectedUserIds.value, [])

  assert.equal('cleanupInactive' in fixture.controller, false)
  assert.equal('cleanupLoading' in fixture.controller, false)
})

test('sets a target user source snapshot as default with the upstream confirmation', async () => {
  const fixture = createController()
  await fixture.controller.setDefaultSources({ id: 2, username: 'admin' })
  assert.deepEqual(fixture.calls, [
    ['confirm', '确认要将用户admin的书源设为默认书源（新用户有效）吗?', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    }],
    ['set-default-sources', 2],
    ['success', '设置成功'],
    ['list'],
  ])
  assert.equal(fixture.controller.defaultingSourceUserId.value, null)
})

test('resets every selected source namespace while account deletion stays protected', async () => {
  const fixture = createController()
  await fixture.controller.load()
  fixture.controller.changeSelection(fixture.controller.users.value)
  fixture.calls.length = 0

  await fixture.controller.resetSelectedSources()

  assert.deepEqual(fixture.calls.slice(0, 4), [
    ['confirm', '确认要删除所选择的用户书源吗?', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning',
    }],
    ['reset-sources', [1, 2, 3]],
    ['success', '操作成功'],
    ['list'],
  ])
  assert.deepEqual(fixture.controller.selectedUserIds.value, [])
  assert.equal(fixture.controller.resettingSources.value, false)
})

test('reloads users after a permission update fails', async () => {
  const failure = new Error('offline')
  const fixture = createController({
    updateUser: async () => {
      throw failure
    },
  })
  await fixture.controller.updatePermission({
    id: 3,
    canEditSources: false,
    canAccessStore: true,
    canAccessWebdav: false,
    bookLimit: 10,
    sourceLimit: 20,
  })
  assert.deepEqual(fixture.calls, [
    ['error', failure, '更新用户失败'],
    ['list'],
  ])
})

test('clears only manager list state when the user dialog closes', async () => {
  const fixture = createController()
  await fixture.controller.load()
  fixture.controller.selectedUserIds.value = [3]
  fixture.controller.openCreateDialog()

  fixture.controller.resetManager()

  assert.deepEqual(fixture.controller.users.value, [])
  assert.deepEqual(fixture.controller.selectedUserIds.value, [])
  assert.equal(fixture.controller.createDialogVisible.value, true, 'the independent create-user dialog must survive a manager-only close')
})

test('does not announce or reload after an admin mutation crosses authenticated sessions', async () => {
  const response = deferred()
  let current = true
  const fixture = createController({
    operationGuard: {
      begin: key => ({ key }),
      canCommit: () => current,
      reset: () => {
        current = false
      },
    },
    createUser: async payload => {
      fixture.calls.push(['create', payload])
      return response.promise
    },
  })
  fixture.controller.openCreateDialog()
  fixture.controller.draft.username = 'reader2'
  fixture.controller.draft.password = 'secret12'

  const pending = fixture.controller.create()
  current = false
  response.resolve({ data: { id: 3 } })
  await pending

  assert.equal(fixture.calls.some(([kind]) => kind === 'success'), false)
  assert.equal(fixture.calls.some(([kind]) => kind === 'list'), false)
  assert.equal(fixture.controller.createDialogVisible.value, true)
})
