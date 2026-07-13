import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const workflow = readFileSync(new URL('../src/composables/useStorageImportWorkflow.js', import.meta.url), 'utf8')
const overlay = readFileSync(new URL('../src/components/overlays/OverlayStorageImport.vue', import.meta.url), 'utf8')
const localStore = readFileSync(new URL('../src/views/LocalStore.vue', import.meta.url), 'utf8')
const webdav = readFileSync(new URL('../src/components/WebDAVBrowser.vue', import.meta.url), 'utf8')
const localStoreApi = readFileSync(new URL('../src/api/localStore.js', import.meta.url), 'utf8')
const webdavApi = readFileSync(new URL('../src/api/webdav.js', import.meta.url), 'utf8')

test('keeps a failed storage preview editable and retries it through the staged import token', () => {
  assert.match(overlay, /@click="reparse\(row\)"/, 'failed TXT/EPUB rows need an explicit root-level reparse action')
  assert.match(overlay, /v-if="isRuleConfigurable\(row\.path\)"/, 'rules must stay editable for a failed TXT/EPUB preview row')
  assert.match(workflow, /function toReparsePayload\(row\)[\s\S]*?importToken:\s*row\.importToken/, 'the reparse request must keep the server-issued staged token')
  assert.match(workflow, /row\.importToken\s*=\s*String\(result\?\.importToken\s*\|\|\s*book\.importToken\s*\|\|\s*row\.importToken/, 'a failed reparse must retain its original staged token')
})

test('sends tokenized preview items instead of re-reading LocalStore or WebDAV paths', () => {
  for (const [name, source, fn] of [
    ['LocalStore', localStoreApi, 'previewLocalStoreImport'],
    ['WebDAV', webdavApi, 'previewWebDAVImport'],
  ]) {
    assert.match(source, new RegExp(`export function ${fn}\\(paths\\)[\\s\\S]*?const items[\\s\\S]*?items\\.length\\s*\\?\\s*\\{ items \\}`), `${name} preview API must forward a tokenized items array unchanged`)
  }
})

test('wires both storage managers through the one root-level retry controller', () => {
  for (const [name, source, sourceName] of [
    ['LocalStore', localStore, 'local-store'],
    ['WebDAV', webdav, 'webdav'],
  ]) {
    assert.match(source, new RegExp(`overlay\\.openStorageImport\\('${sourceName}'`), `${name} must open the root storage controller instead of owning a retry dialog`)
  }
  assert.match(overlay, /previewLocalStoreImport\(payload\)/, 'the root controller must select the LocalStore staged preview endpoint')
  assert.match(overlay, /previewWebDAVImport\(payload\)/, 'the root controller must select the WebDAV staged preview endpoint')
})
