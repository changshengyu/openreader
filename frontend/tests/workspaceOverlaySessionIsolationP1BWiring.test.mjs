import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

function read(path) {
  return readFileSync(new URL(path, import.meta.url), 'utf8')
}

test('remaining account-private overlay roots use the lifecycle authenticated guard', () => {
  const files = [
    '../src/components/overlays/OverlayBookManagement.vue',
    '../src/components/overlays/OverlayBookGroups.vue',
    '../src/components/overlays/OverlayBookmarks.vue',
    '../src/components/overlays/OverlayBookmarkForm.vue',
    '../src/components/overlays/OverlayBookContentSearch.vue',
    '../src/components/overlays/OverlayStorageImport.vue',
    '../src/components/workspace/LocalStoreManager.vue',
    '../src/components/workspace/SourceManager.vue',
    '../src/components/overlays/OverlayReplaceRules.vue',
    '../src/components/RSSManager.vue',
    '../src/layouts/AppLayout.vue',
  ]

  for (const file of files) {
    assert.match(
      read(file),
      /useAuthenticatedOperationGuard/,
      `${file} must retire account-private operations on session invalidation`,
    )
  }
})

test('remaining async controllers accept the shared operation guard', () => {
  const files = [
    '../src/composables/useOverlayBookBatchActions.js',
    '../src/composables/useOverlayBookItemActions.js',
    '../src/composables/useOverlayBookGroups.js',
    '../src/composables/useBookBookmarks.js',
    '../src/composables/useOverlayBookmarkActions.js',
    '../src/composables/useStorageImportWorkflow.js',
    '../src/composables/useBookContentSearch.js',
    '../src/composables/useSourceTransfer.js',
    '../src/composables/useOverlayReplaceRules.js',
    '../src/composables/useWorkspaceBackupActions.js',
  ]

  for (const file of files) {
    const source = read(file)
    assert.match(source, /operationGuard/)
    assert.match(source, /canCommit/)
  }
})

test('RSS source editor exposes a stable private-overlay root for browser lifecycle checks', () => {
  assert.match(
    read('../src/components/rss/RSSJsonEditorDialog.vue'),
    /class="rss-source-editor-dialog"/,
  )
})
