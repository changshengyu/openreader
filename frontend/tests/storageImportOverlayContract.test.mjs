import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const host = readFileSync(new URL('../src/components/GlobalOverlayHost.vue', import.meta.url), 'utf8')
const overlayStore = readFileSync(new URL('../src/stores/overlay.js', import.meta.url), 'utf8')
const overlay = readFileSync(new URL('../src/components/overlays/OverlayStorageImport.vue', import.meta.url), 'utf8')
const localStore = readFileSync(new URL('../src/views/LocalStore.vue', import.meta.url), 'utf8')
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
