import assert from 'node:assert/strict'
import test from 'node:test'
import {
  normalizeOverlayReplaceRule,
  normalizeOverlayReplaceRuleImport,
  useOverlayReplaceRules,
} from '../src/composables/useOverlayReplaceRules.js'

function createController(overrides = {}) {
  const calls = []
  let timerTask
  const controller = useOverlayReplaceRules({
    isActive: () => true,
    listReplaceRules: async () => {
      calls.push(['list'])
      return {
        data: [
          { id: 1, name: '旧规则', pattern: 'ad', isEnabled: false },
          { id: 2, name: '正文', pattern: 'body', isRegex: false },
        ],
      }
    },
    createReplaceRule: async payload => calls.push(['create', payload]),
    updateReplaceRule: async (...args) => calls.push(['update', ...args]),
    deleteReplaceRule: async id => calls.push(['delete', id]),
    deleteReplaceRules: async ids => {
      calls.push(['delete-many', ids])
      return { data: { deletedIds: ids } }
    },
    testReplaceRule: async payload => {
      calls.push(['test', payload])
      return { data: { changed: true, output: '正文' } }
    },
    upsertReplaceRules: async rows => {
      calls.push(['upsert', rows])
      return { data: { created: 1, updated: 1, skipped: 1 } }
    },
    confirm: async (...args) => calls.push(['confirm', ...args]),
    notifyUpdated: () => calls.push(['notify']),
    onSuccess: message => calls.push(['success', message]),
    onWarning: message => calls.push(['warning', message]),
    onError: (...args) => calls.push(['error', ...args]),
    setTimeout: task => {
      calls.push(['set-timeout'])
      timerTask = task
      return 7
    },
    clearTimeout: id => calls.push(['clear-timeout', id]),
    ...overrides,
  })
  return {
    calls,
    controller,
    runTimer: async () => timerTask?.(),
  }
}

function deferred() {
  let resolve
  const promise = new Promise(done => {
    resolve = done
  })
  return { promise, resolve }
}

function expiringOperationGuard() {
  let active = true
  return {
    begin: key => ({ key }),
    canCommit: () => active,
    invalidate: () => {
      active = false
    },
    reset: () => {
      active = false
    },
    expire: () => {
      active = false
    },
  }
}

test('normalizes legacy stored rules and imported rule fields separately', () => {
  assert.deepEqual(normalizeOverlayReplaceRule({
    id: 1,
    pattern: 'ad',
    scope: ' ',
    isEnabled: false,
  }), {
    id: 1,
    pattern: 'ad',
    scope: ' ',
    isEnabled: false,
    isRegex: false,
    enabled: false,
  })
  assert.deepEqual(normalizeOverlayReplaceRuleImport({
    rules: [
      { title: '广告', regex: ' ad ', replace: '', scope: '', isEnabled: false },
      { title: '空规则' },
      { pattern: ' missing name ' },
    ],
  }), [
    {
      name: '广告',
      pattern: ' ad ',
      replacement: '',
      scope: '*',
      isRegex: false,
      enabled: false,
    },
    {
      name: '空规则',
      pattern: '',
      replacement: '',
      scope: '*',
      isRegex: false,
      enabled: true,
    },
    {
      name: '',
      pattern: ' missing name ',
      replacement: '',
      scope: '*',
      isRegex: false,
      enabled: true,
    },
  ])
})

test('loads normalized rules and debounces only remote visible updates', async () => {
  const fixture = createController()
  fixture.controller.selectedIds.value = [1, 9]
  await fixture.controller.load()
  assert.deepEqual(fixture.controller.selectedIds.value, [1])
  assert.equal(fixture.controller.rules.value[0].enabled, false)
  assert.equal(fixture.controller.rules.value[1].isRegex, false)

  fixture.calls.length = 0
  fixture.controller.handleUpdated({ detail: { local: true } })
  assert.deepEqual(fixture.calls, [])
  fixture.controller.handleUpdated({})
  fixture.controller.handleUpdated({})
  assert.deepEqual(fixture.calls, [
    ['set-timeout'],
    ['clear-timeout', 7],
    ['set-timeout'],
  ])
  await fixture.runTimer()
  assert.equal(fixture.calls.filter(call => call[0] === 'list').length, 1)
})

test('imports normalized JSON rules and resets the file input', async () => {
  const fixture = createController()
  const target = {
    files: [{
      text: async () => JSON.stringify([
        { name: '广告', pattern: 'ad' },
        { name: '正文', pattern: 'body', isRegex: true },
        { name: '', pattern: 'invalid-but-counted' },
      ]),
    }],
    value: 'rules.json',
  }
  await fixture.controller.importFile({ target })
  assert.equal(target.value, '')
  assert.equal(fixture.controller.importing.value, false)
  assert.equal(fixture.calls[0][0], 'confirm')
  assert.equal(fixture.calls[0][1], '确认要导入文件中的3条替换规则吗?')
  assert.equal(fixture.calls[1][0], 'upsert')
  assert.equal(fixture.calls[1][1].length, 3)
  assert.equal(fixture.calls[1][1][2].name, '')
  assert.deepEqual(fixture.calls.slice(-3), [
    ['success', '导入替换规则成功：新增 1，更新 1，跳过 1'],
    ['list'],
    ['notify'],
  ])
})

test('opens fresh and existing editors and saves create or update payloads', async () => {
  const fixture = createController()
  fixture.controller.openEditor()
  assert.equal(fixture.controller.editingId.value, null)
  fixture.controller.draft.value.name = '广告规则'
  fixture.controller.draft.value.pattern = ' ad '
  fixture.controller.draft.value.scope = '*'
  await fixture.controller.save()
  assert.equal(fixture.calls[0][0], 'create')
  assert.equal(fixture.calls[0][1].pattern, ' ad ')
  assert.equal(fixture.controller.dialogVisible.value, false)

  fixture.calls.length = 0
  fixture.controller.openEditor({
    id: 2,
    name: '正文',
    pattern: 'body',
    isRegex: false,
  })
  await fixture.controller.save()
  assert.equal(fixture.calls[0][0], 'update')
  assert.equal(fixture.calls[0][1], 2)
})

test('requires the same name, pattern, and scope fields as the upstream editor', async () => {
  const fixture = createController()
  fixture.controller.openEditor()
  fixture.controller.draft.value.pattern = '广告'
  await fixture.controller.save()
  assert.deepEqual(fixture.calls, [['warning', '规则名不能为空']])

  fixture.calls.length = 0
  fixture.controller.draft.value.name = '广告规则'
  await fixture.controller.save()
  assert.deepEqual(fixture.calls, [['warning', '替换范围不能为空']])
})

test('toggles rules while preserving hidden compatibility fields and failure reload semantics', async () => {
  const fixture = createController()
  const rule = {
    id: 2,
    name: '正文',
    group: '净化',
    pattern: 'body',
    isRegex: false,
    order: 7,
  }
  await fixture.controller.toggle(rule, false)
  assert.equal(rule.enabled, false)
  assert.equal(rule.isEnabled, false)
  assert.equal(fixture.calls[0][2].group, '净化')
  assert.equal(fixture.calls[0][2].order, 7)

  const failure = new Error('offline')
  const failed = createController({
    updateReplaceRule: async () => {
      throw failure
    },
  })
  await failed.controller.toggle({ id: 2, pattern: 'body' }, true)
  assert.deepEqual(failed.calls, [
    ['error', failure, '更新替换规则失败'],
    ['list'],
  ])
})

test('toggle updates the visible switch immediately like the upstream v-model', async () => {
  const pendingUpdate = deferred()
  const fixture = createController({
    updateReplaceRule: async () => pendingUpdate.promise,
  })
  const rule = {
    id: 2,
    name: '正文',
    pattern: 'body',
    scope: '*',
    isEnabled: true,
  }

  const toggleTask = fixture.controller.toggle(rule, false)
  assert.equal(rule.enabled, false)
  assert.equal(rule.isEnabled, false)

  pendingUpdate.resolve()
  await toggleTask
})

test('batch deletes selected rules and clears their selections', async () => {
  const fixture = createController()
  await fixture.controller.load()
  fixture.controller.selectedIds.value = [1, 2]
  await fixture.controller.removeSelected()
  assert.deepEqual(fixture.controller.rules.value, [])
  assert.deepEqual(fixture.controller.selectedIds.value, [])
})

test('clears only manager list state when its root dialog closes', async () => {
  const fixture = createController()
  await fixture.controller.load()
  fixture.controller.selectedIds.value = [1]
  fixture.controller.openEditor({ id: 2, name: '正文', pattern: 'body' })

  fixture.controller.resetManager()

  assert.deepEqual(fixture.controller.rules.value, [])
  assert.deepEqual(fixture.controller.selectedIds.value, [])
  assert.equal(fixture.controller.dialogVisible.value, true, 'the independent editor must survive a manager-only close')
})

test('manager close does not retire an in-flight sibling editor save', async () => {
  const pendingUpdate = deferred()
  const managerOperationGuard = expiringOperationGuard()
  const editorOperationGuard = expiringOperationGuard()
  const updateCalls = []
  let managerVisible = true
  const fixture = createController({
    isActive: () => managerVisible,
    managerOperationGuard,
    editorOperationGuard,
    updateReplaceRule: async (...args) => {
      updateCalls.push(args)
      await pendingUpdate.promise
    },
  })
  fixture.controller.openEditor({
    id: 2,
    name: '正文',
    group: '净化',
    pattern: 'body',
    scope: '*',
    order: 7,
  })

  const saveTask = fixture.controller.save()
  managerVisible = false
  fixture.controller.resetManager()
  pendingUpdate.resolve()
  await saveTask

  assert.equal(updateCalls.length, 1)
  assert.equal(fixture.controller.dialogVisible.value, false)
  assert.equal(fixture.controller.saving.value, false)
  assert.deepEqual(fixture.calls, [
    ['success', '编辑替换规则成功'],
    ['notify'],
  ])
})

test('an old batch-delete confirmation cannot mutate after authentication changes', async () => {
  const pendingConfirmation = deferred()
  const operationGuard = expiringOperationGuard()
  const fixture = createController({
    operationGuard,
    confirm: () => pendingConfirmation.promise,
  })

  fixture.controller.selectedIds.value = [7]
  const removeTask = fixture.controller.removeSelected()
  operationGuard.expire()
  pendingConfirmation.resolve()
  await removeTask

  assert.deepEqual(fixture.calls, [])
})
