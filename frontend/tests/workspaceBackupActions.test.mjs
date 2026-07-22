import assert from 'node:assert/strict'
import test from 'node:test'
import { useWorkspaceBackupActions } from '../src/composables/useWorkspaceBackupActions.js'

function createActions(overrides = {}) {
  const calls = []
  const actions = useWorkspaceBackupActions({
    triggerBackup: async () => {
      calls.push(['trigger'])
      return { data: { name: 'backup-1.zip' } }
    },
    triggerPortableBackup: async () => {
      calls.push(['portable'])
      return { data: { name: 'portable-1.zip', localBooks: 2 } }
    },
    confirmBackup: async () => calls.push(['confirm-backup']),
    confirmPortable: async () => calls.push(['confirm-portable']),
    onSuccess: message => calls.push(['success', message]),
    onError: (error, fallback) => calls.push(['error', error, fallback]),
    ...overrides,
  })
  return { actions, calls }
}

test('ordinary workspace backup confirms then performs one direct trigger', async () => {
  const fixture = createActions()
  await fixture.actions.runBackup()
  assert.deepEqual(fixture.calls, [
    ['confirm-backup'],
    ['trigger'],
    ['success', '当前账户备份已保存：backup-1.zip'],
  ])
  assert.equal(fixture.actions.backupLoading.value, false)
})

test('cancelled direct backup performs no write', async () => {
  const fixture = createActions({
    confirmBackup: async () => {
      throw new Error('cancel')
    },
  })
  await fixture.actions.runBackup()
  assert.deepEqual(fixture.calls, [])
})

test('portable backup remains a separately confirmed and named extension', async () => {
  const fixture = createActions()
  await fixture.actions.runPortableBackup()
  assert.deepEqual(fixture.calls, [
    ['confirm-portable'],
    ['portable'],
    ['success', '完整本地书备份已保存：portable-1.zip（2 本）'],
  ])
  assert.equal(fixture.actions.portableBackupLoading.value, false)
})

test('direct backup failures release loading state and use truthful fallback text', async () => {
  const failure = new Error('disk full')
  const fixture = createActions({
    triggerBackup: async () => {
      throw failure
    },
  })
  await fixture.actions.runBackup()
  assert.deepEqual(fixture.calls, [
    ['confirm-backup'],
    ['error', failure, '保存当前账户备份失败'],
  ])
  assert.equal(fixture.actions.backupLoading.value, false)
})
