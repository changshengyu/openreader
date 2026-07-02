import assert from 'node:assert/strict'
import test from 'node:test'
import { useOverlayBackups } from '../src/composables/useOverlayBackups.js'

function createController(overrides = {}) {
  const calls = []
  const form = {
    append: (...args) => calls.push(['append', ...args]),
  }
  const controller = useOverlayBackups({
    triggerBackup: async () => {
      calls.push(['trigger'])
      return { data: { name: 'backup-1.zip' } }
    },
    listBackups: async () => {
      calls.push(['list'])
      return { data: [{ name: 'backup-1.zip', size: 10 }] }
    },
    downloadBackup: async name => {
      calls.push(['download', name])
      return { data: 'blob-data' }
    },
    restoreBackup: async payload => {
      calls.push(['restore', payload])
      return { data: { sources: 2, books: 3, progress: 4 } }
    },
    applyRestoreResult: async result => calls.push(['apply', result]),
    saveBlob: (...args) => calls.push(['save-blob', ...args]),
    createFormData: () => form,
    onSuccess: message => calls.push(['success', message]),
    onError: (...args) => calls.push(['error', ...args]),
    ...overrides,
  })
  return { calls, controller, form }
}

test('runs a backup and refreshes the WebDAV list', async () => {
  const fixture = createController()
  await fixture.controller.run()
  assert.deepEqual(fixture.calls, [
    ['trigger'],
    ['success', '备份已保存到 WebDAV：backup-1.zip'],
    ['list'],
  ])
  assert.deepEqual(fixture.controller.backups.value, [
    { name: 'backup-1.zip', size: 10 },
  ])
  assert.equal(fixture.controller.backupLoading.value, false)
  assert.equal(fixture.controller.listLoading.value, false)
})

test('reports list failures and always releases loading state', async () => {
  const failure = new Error('offline')
  const fixture = createController({
    listBackups: async () => {
      throw failure
    },
  })
  await fixture.controller.load()
  assert.deepEqual(fixture.calls, [
    ['error', failure, '加载备份列表失败'],
  ])
  assert.equal(fixture.controller.listLoading.value, false)
})

test('downloads backup blobs with the original filename', async () => {
  const fixture = createController()
  await fixture.controller.download({ name: 'backup-1.zip' })
  assert.deepEqual(fixture.calls, [
    ['download', 'backup-1.zip'],
    ['save-blob', 'blob-data', 'backup-1.zip'],
  ])
})

test('restores an uploaded package and applies returned state', async () => {
  const fixture = createController()
  const file = { name: 'restore.zip' }
  await fixture.controller.restore({ raw: file })
  assert.deepEqual(fixture.calls, [
    ['append', 'file', file],
    ['restore', fixture.form],
    ['success', '恢复完成：书源 2，书籍 3，进度 4'],
    ['apply', { sources: 2, books: 3, progress: 4 }],
  ])
  assert.equal(fixture.controller.restoreLoading.value, false)

  fixture.calls.length = 0
  await fixture.controller.restore({})
  assert.deepEqual(fixture.calls, [])
})
