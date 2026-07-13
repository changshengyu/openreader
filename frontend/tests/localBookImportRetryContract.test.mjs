import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const dialog = readFileSync(new URL('../src/components/LocalBookImportPreviewDialog.vue', import.meta.url), 'utf8')
const localStore = readFileSync(new URL('../src/views/LocalStore.vue', import.meta.url), 'utf8')
const webdav = readFileSync(new URL('../src/components/WebDAVBrowser.vue', import.meta.url), 'utf8')
const localStoreApi = readFileSync(new URL('../src/api/localStore.js', import.meta.url), 'utf8')
const webdavApi = readFileSync(new URL('../src/api/webdav.js', import.meta.url), 'utf8')

test('keeps a failed storage preview editable and retries it through the staged import token', () => {
  assert.match(dialog, /@click="reparse\(row\)"/, 'failed TXT/EPUB rows need an explicit reparse action')
  assert.match(dialog, /emit\('reparse',\s*toReparseRequest\(row\),\s*applyReparseResult\)/, 'the dialog must return the same row token and a result callback to its storage host')
  assert.match(dialog, /row\.importToken\s*=\s*result\.importToken\s*\|\|\s*result\.book\?\.importToken\s*\|\|\s*row\.importToken/, 'a failed reparse must retain its original staged token')
  assert.match(dialog, /v-if="isRuleConfigurable\(row\.path\)"/, 'rules must stay editable for a failed TXT/EPUB preview row')
})

test('sends tokenized preview items instead of re-reading LocalStore or WebDAV paths', () => {
  for (const [name, source, fn] of [
    ['LocalStore', localStoreApi, 'previewLocalStoreImport'],
    ['WebDAV', webdavApi, 'previewWebDAVImport'],
  ]) {
    assert.match(source, new RegExp(`export function ${fn}\\(paths\\)[\\s\\S]*?const items[\\s\\S]*?items\\.length\\s*\\?\\s*\\{ items \\}`), `${name} preview API must forward a tokenized items array unchanged`)
  }
})

test('wires the shared retry action through both root workspace storage dialogs', () => {
  for (const [name, source, fn] of [
    ['LocalStore', localStore, 'previewLocalStoreImport'],
    ['WebDAV', webdav, 'previewWebDAVImport'],
  ]) {
    assert.match(source, /@reparse="reparsePreviewItem"/, `${name} must receive the preview dialog reparse event`)
    assert.match(source, new RegExp(`async function reparsePreviewItem[\\s\\S]*?${fn}\\(\\[item\\]\\)`), `${name} must reparse one staged item rather than start a new path preview`)
  }
})
