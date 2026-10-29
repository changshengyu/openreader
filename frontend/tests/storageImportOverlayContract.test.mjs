import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const host = readFileSync(new URL('../src/components/GlobalOverlayHost.vue', import.meta.url), 'utf8')
const overlayStore = readFileSync(new URL('../src/stores/overlay.js', import.meta.url), 'utf8')
const overlay = readFileSync(new URL('../src/components/overlays/OverlayStorageImport.vue', import.meta.url), 'utf8')
const localStore = readFileSync(new URL('../src/components/workspace/LocalStoreManager.vue', import.meta.url), 'utf8')
const webdav = readFileSync(new URL('../src/components/WebDAVBrowser.vue', import.meta.url), 'utf8')

test('mounts one root storage-import controller and exposes a shared overlay request', () => {
  assert.match(host, /<OverlayStorageImport\s+:is-mobile="isMobileOverlay"/, 'GlobalOverlayHost must own the only storage-import surface')
  assert.match(overlayStore, /storageImportVisible:\s*false/, 'overlay state needs one storage import visibility flag')
  assert.match(overlayStore, /storageImportRequest:\s*null/, 'overlay state must retain the source request independently of either manager')
  assert.match(overlayStore, /openStorageImport\(source, paths\)/, 'both file managers must enter the same storage import controller')
  assert.match(overlay, /useStorageImportWorkflow/, 'the root dialog must use the tested storage import state machine')
  assert.match(overlay, /previewLocalStoreImport|previewWebDAVImport/, 'the controller must select the source-specific token preview endpoint')
  assert.match(overlay, /importFromLocalStore|importFromWebDAV/, 'the controller must select the source-specific staged import endpoint')
})

test('keeps file managers as source launchers instead of competing import dialogs', () => {
  for (const [name, source] of [['LocalStore', localStore], ['WebDAV', webdav]]) {
    assert.match(source, /overlay\.openStorageImport\(/, `${name} must open the shared import controller`)
    assert.doesNotMatch(source, /LocalBookImportPreviewDialog/, `${name} must not retain a second import confirmation dialog`)
    assert.doesNotMatch(source, /previewDialog\s*=\s*ref/, `${name} must not own local import dialog state`)
    assert.doesNotMatch(source, /resultDialog\s*=\s*ref|importResultDialog\s*=\s*ref/, `${name} must not own an alternate import result flow`)
  }
})

// reader-dev Index.vue 895-1001: cover, labelled name/author/group/rule rows,
// a "内置规则" rule select, the rule textarea and "章节列表(N)".
test('the single-book confirmation mirrors the reader-dev import dialog fields', () => {
  const single = overlay.slice(overlay.indexOf("phase === 'single'"))
  assert.match(single, /<BookCover\b[^>]*:book="importCoverBook\(currentRow\)"/, 'the dialog shows the cover slot')
  for (const label of ['书名：', '作者：', '分组：', '规则：']) {
    assert.ok(single.includes(`<span class="storage-import-label">${label}</span>`), `missing ${label} label`)
  }
  assert.match(single, /placeholder="内置规则"/, 'the TXT rule select uses the upstream placeholder')
  assert.match(single, /<el-input\s[^>]*type="textarea"[^>]*v-model="currentRuleText"/, 'the dialog shows the effective rule in a textarea')
  assert.match(single, /章节列表\(\{\{ currentRow\.chapters\.length \}\}\)/, 'the chapter list title uses the upstream format')
  assert.match(overlay, /const currentRuleText = computed\(\{[\s\S]*?get: \(\) => currentRow\.value\?\.tocRule \|\| currentRow\.value\?\.detectedTocRule \|\| ''/, 'the textarea shows the chosen or detected rule')
})

test('the confirmation cover uses the preview cover', () => {
  assert.match(
    overlay,
    /function importCoverBook\(row\) \{\s*return \{ title: row\?\.title \|\| '', author: row\?\.author \|\| '', coverUrl: row\?\.coverUrl \|\| '' \}\s*\}/,
  )
})
