import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const workflow = readFileSync(new URL('../src/composables/useStorageImportWorkflow.js', import.meta.url), 'utf8')
const overlay = readFileSync(new URL('../src/components/overlays/OverlayStorageImport.vue', import.meta.url), 'utf8')
const directDialog = readFileSync(new URL('../src/components/overlays/OverlayBookImport.vue', import.meta.url), 'utf8')
const localStoreApi = readFileSync(new URL('../src/api/localStore.js', import.meta.url), 'utf8')
const webdavApi = readFileSync(new URL('../src/api/webdav.js', import.meta.url), 'utf8')

test('preserves a preview import token through LocalStore/WebDAV confirmation', () => {
  assert.match(workflow, /importToken:\s*String\(item\?\.importToken\s*\|\|\s*book\.importToken\s*\|\|\s*''\)/, 'preview rows must retain the server-issued staged-input token')
  assert.match(workflow, /function toImportPayload\(row\)[\s\S]*?path:\s*row\.path,[\s\S]*?importToken:\s*row\.importToken,[\s\S]*?title:\s*row\.title\.trim\(\)/, 'confirmation must submit the same token alongside editable metadata')
  assert.match(overlay, /importFromLocalStore\(\[item\], categoryIds\)/, 'the root controller must submit each LocalStore item separately')
  assert.match(overlay, /importFromWebDAV\(\[item\], categoryIds\)/, 'the root controller must submit each WebDAV item separately')
  assert.match(localStoreApi, /items\.length\s*\?\s*\{ items, categoryIds \}/, 'LocalStore must forward preview rows without rebuilding them')
  assert.match(webdavApi, /items\.length\s*\?\s*\{ items, categoryIds \}/, 'WebDAV must forward preview rows without rebuilding them')
})

test('keeps direct TXT rule-retry context visible after an empty catalogue response', () => {
  assert.match(directDialog, /v-else-if="previewError"/, 'the direct-import preview must retain a visible retry hint instead of a blank state')
  assert.match(directDialog, /no readable chapters.*return fallback/s, 'server no-readable-chapter errors must use the actionable retry message')
})
