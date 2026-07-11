import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('../src/views/LocalStore.vue', import.meta.url), 'utf8')

test('starts LocalStore in the upstream current-directory mode and keeps recursive scan explicit', () => {
  assert.match(source, /const recursiveScan\s*=\s*ref\(false\)/, 'LocalStore must start with the root/current-directory listing, not a recursive mixed tree')
  assert.match(source, /v-model="recursiveScan"[\s\S]*?@change="load"/, 'recursive traversal must remain an explicit refreshable user choice')
  assert.match(source, /importPaths\(\[currentPath\.value\]\)/, 'directory import may still use the deliberate recursive importer')
})
