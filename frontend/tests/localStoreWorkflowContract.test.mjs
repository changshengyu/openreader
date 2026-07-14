import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('../src/views/LocalStore.vue', import.meta.url), 'utf8')

test('keeps LocalStore as the upstream current-directory manager without recursive or directory import controls', () => {
  assert.doesNotMatch(source, /recursiveScan|importCurrentDirectory|importFiltered|importDirectory/, 'upstream LocalStore must not expose a recursive mixed tree or directory-import path')
  assert.doesNotMatch(source, /createDirectory|renameItem|downloadItem/, 'upstream LocalStore must not grow directory, rename, or download product actions')
  assert.match(source, /lastModified/, 'the upstream table must retain a modification-time field')
  assert.match(source, /showMoreItems/, 'the 101-item reveal state remains explicit')
})
