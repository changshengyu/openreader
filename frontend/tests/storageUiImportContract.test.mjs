import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const localStoreOverlay = readFileSync(new URL('../src/components/overlays/OverlayLocalStore.vue', import.meta.url), 'utf8')
const webdavOverlay = readFileSync(new URL('../src/components/overlays/OverlayWebDAV.vue', import.meta.url), 'utf8')
const localStore = readFileSync(new URL('../src/views/LocalStore.vue', import.meta.url), 'utf8')
const webdav = readFileSync(new URL('../src/components/WebDAVBrowser.vue', import.meta.url), 'utf8')
const directImport = readFileSync(new URL('../src/components/overlays/OverlayBookImport.vue', import.meta.url), 'utf8')

test('uses the upstream storage-manager dialog labels without a competing embedded title', () => {
  assert.match(localStoreOverlay, /title="书仓文件管理"/, 'LocalStore root dialog must retain the upstream title')
  assert.match(webdavOverlay, /title="WebDAV文件管理"/, 'WebDAV root dialog must retain the upstream title')
  assert.doesNotMatch(localStore, /embedded-store-title/, 'LocalStore body must not duplicate the root dialog title')
  assert.doesNotMatch(webdav, /<strong>\{\{ title \}\}<\/strong>/, 'WebDAV body must not duplicate the root dialog title')
})

test('keeps CBZ reachable from every frontend local-import entry point', () => {
  for (const [name, source] of [
    ['direct import', directImport],
    ['LocalStore', localStore],
  ]) {
    assert.match(source, /accept="[^"]*\.cbz[^"]*"/i, `${name} upload chooser must allow CBZ`)
  }
  assert.match(webdav, /\(txt\|text\|md\|epub\|pdf\|umd\|cbz\)/i, 'WebDAV listing must mark CBZ as importable')
})
