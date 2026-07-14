import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import {
  isLocalStoreImportable,
  isWebDAVImportable,
} from '../src/utils/storageImportable.js'

const localStore = readFileSync(new URL('../src/views/LocalStore.vue', import.meta.url), 'utf8')
const webdav = readFileSync(new URL('../src/components/WebDAVBrowser.vue', import.meta.url), 'utf8')

test('keeps reader-dev format gates distinct for LocalStore and WebDAV', () => {
  for (const name of ['book.txt', 'book.epub', 'book.umd']) {
    assert.equal(isLocalStoreImportable(name), true, `LocalStore must offer ${name}`)
    assert.equal(isWebDAVImportable(name), true, `WebDAV must offer ${name}`)
  }
  assert.equal(isLocalStoreImportable('comic.cbz'), true, 'LocalStore must retain the upstream CBZ entry')
  assert.equal(isWebDAVImportable('comic.cbz'), false, 'WebDAV must not advertise the LocalStore-only CBZ entry')
  for (const name of ['legacy.text', 'notes.md', 'paper.pdf', 'archive.zip']) {
    assert.equal(isLocalStoreImportable(name), false, `LocalStore must not advertise ${name}`)
    assert.equal(isWebDAVImportable(name), false, `WebDAV must not advertise ${name}`)
  }
})

test('keeps only upstream file-manager actions in the root workspace surfaces', () => {
  assert.doesNotMatch(localStore, /createDirectory|renameItem|downloadItem|importCurrentDirectory|importFiltered|importDirectory|recursiveScan/, 'LocalStore must remove non-upstream operations')
  assert.doesNotMatch(webdav, /createFolder|renameItem|importDirectory/, 'WebDAV must remove non-upstream operations')
  assert.match(localStore, /multiple/, 'LocalStore upload must support the upstream multi-file chooser')
  assert.match(webdav, /multiple/, 'WebDAV upload must support the upstream multi-file chooser')
  assert.match(localStore, /isLocalStoreImportable/, 'LocalStore must use the source-specific format gate')
  assert.match(webdav, /isWebDAVImportable/, 'WebDAV must use the source-specific format gate')
})
