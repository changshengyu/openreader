import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

function read(path) {
  return readFileSync(new URL(path, import.meta.url), 'utf8')
}

test('high-risk workspace overlays share a lifecycle-aware authenticated operation guard', () => {
  const bookInfo = read('../src/components/overlays/OverlayBookInfo.vue')
  const storageImport = read('../src/components/overlays/OverlayStorageImport.vue')
  const webdav = read('../src/components/WebDAVBrowser.vue')
  const userManagement = read('../src/components/overlays/OverlayUserManagement.vue')

  for (const [name, source] of [
    ['BookInfo', bookInfo],
    ['StorageImport', storageImport],
    ['WebDAV', webdav],
    ['UserManage', userManagement],
  ]) {
    assert.match(
      source,
      /useAuthenticatedOperationGuard/,
      `${name} must invalidate pending operations on session invalidation and scope disposal`,
    )
  }
})

test('WebDAV restore checks the frozen identity before global restore convergence or reload', () => {
  const source = read('../src/components/WebDAVBrowser.vue')
  assert.match(
    source,
    /restoreBackupFile[\s\S]*?operations\.begin\(['"]restore['"]\)[\s\S]*?restoreWebDAVBackup[\s\S]*?operations\.canCommit\(operation\)[\s\S]*?applyRestoreResult/,
  )
  assert.match(
    source,
    /applyRestoreResult[\s\S]*?operations\.canCommit\(operation\)[\s\S]*?load\(/,
  )
})

test('session invalidation resets the component operation generation before token removal', () => {
  const source = read('../src/composables/useAuthenticatedOperationGuard.js')
  assert.match(source, /openreader:session-invalidated/)
  assert.match(source, /onScopeDispose/)
  assert.match(source, /operations\.reset\(\)/)
})
