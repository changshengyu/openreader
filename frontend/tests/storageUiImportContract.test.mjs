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

test('keeps direct CBZ import while storage managers use their distinct upstream format gates', () => {
  assert.match(directImport, /accept="[^"]*\.cbz[^"]*"/i, 'direct import remains an approved OpenReader parser entry')
  assert.doesNotMatch(localStore, /accept=/, 'upstream LocalStore upload must manage arbitrary files instead of filtering chooser extensions')
  assert.doesNotMatch(webdav, /\(txt\|text\|md\|epub\|pdf\|umd\|cbz\)/i, 'WebDAV must not expose the OpenReader-only CBZ/PDF/Markdown import set')
})
