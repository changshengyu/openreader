import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const __dirname = dirname(fileURLToPath(import.meta.url))

function read(relative) {
  return readFileSync(resolve(__dirname, relative), 'utf8')
}

test('Reader delegates bookmark and content search UI to the shared App-level overlays', () => {
  const reader = read('../src/views/Reader.vue')
  const panels = read('../src/composables/useReaderPanels.js')

  assert.doesNotMatch(reader, /ReaderBookmarkPanel/)
  assert.doesNotMatch(reader, /ReaderSearchPanel/)
  assert.doesNotMatch(reader, /showBookmarkDrawer/)
  assert.doesNotMatch(reader, /showSearchDrawer/)
  assert.doesNotMatch(reader, /useBookContentSearch/)
  assert.match(panels, /options\.openBookmarksOverlay\(currentBook\)/)
  assert.match(panels, /options\.openContentSearchOverlay\(currentBook\)/)
})

test('shared bookmarks and content search use upstream-style dialogs, including mobile fullscreen mode', () => {
  const bookmarks = read('../src/components/overlays/OverlayBookmarks.vue')
  const search = read('../src/components/overlays/OverlayBookContentSearch.vue')
  const host = read('../src/components/GlobalOverlayHost.vue')

  assert.match(bookmarks, /<el-dialog/)
  assert.match(bookmarks, /:fullscreen="isMobile"/)
  assert.doesNotMatch(bookmarks, /<el-drawer/)
  assert.match(search, /<el-dialog/)
  assert.match(search, /:fullscreen="isMobile"/)
  assert.doesNotMatch(search, /<el-drawer/)
  assert.match(host, /<OverlayBookContentSearch[\s\S]*:is-mobile="isMobileOverlay"/)
})

test('cache controls remain in the reader bars instead of opening a workspace or drawer', () => {
  const reader = read('../src/views/Reader.vue')
  const desktopProgress = read('../src/components/reader/ReaderDesktopProgress.vue')
  const mobileChrome = read('../src/components/reader/ReaderMobileChrome.vue')
  const cacheZone = read('../src/components/reader/ReaderCachePanel.vue')

  assert.doesNotMatch(reader, /showCacheDrawer/)
  assert.doesNotMatch(reader, /<el-drawer[\s\S]*缓存章节/)
  assert.match(reader, /showCacheContentZone/)
  assert.match(desktopProgress, /ReaderCachePanel/)
  assert.match(mobileChrome, /ReaderCachePanel/)
  assert.match(cacheZone, /reader-cache-zone/)
  assert.match(cacheZone, /visible/)
})
