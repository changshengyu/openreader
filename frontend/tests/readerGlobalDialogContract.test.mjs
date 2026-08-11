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
  assert.match(search, /width="min\(1000px, max\(750px, 70vw\)\)"/)
  assert.match(search, /top="max\(15dvh, calc\(\(100dvh - 584px\) \/ 2\)\)"/)
  assert.match(search, /:height="isMobile \? 'calc\(100dvh - 184px\)' : 'min\(400px, calc\(70dvh - 184px\)\)'"/)
  assert.match(search, /size="small"/)
  assert.match(search, /:prefix-icon="Search"/)
  assert.match(search, /class="content-search-title-input"/)
  assert.match(search, /property="chapterTitle"[^>]*min-width="100"/)
  assert.match(search, /property="resultText"[^>]*min-width="250"/)
  assert.doesNotMatch(search, /max-height="520"/)
  assert.doesNotMatch(search, /v-loading="loading"/)
  assert.doesNotMatch(search, /<el-empty/)
  assert.doesNotMatch(search, /scope\.row\.excerpt \|\| scope\.row\.resultText \|\| '—'/)
  assert.doesNotMatch(search, /searchInputRef\.value\?\.focus/)
  assert.match(search, /const isNormalPage = computed/)
  assert.match(search, /v-if="isNormalPage"/)
  assert.match(search, /bookContentSearchBookIdentity/)
  assert.match(search, /searchNotice/)
  assert.match(search, /incomplete/)
  assert.doesNotMatch(search, /useRouter|router\.push/, 'a live result selection must not create browser history')
  assert.match(search, /requestSearchBookContentJump/, 'every row click must emit a repeatable Reader intent')
  assert.match(host, /<OverlayBookContentSearch[\s\S]*:is-mobile="isMobileOverlay"/)
})

test('content search footer keeps upstream left actions and right cancel without equal-width mobile buttons', () => {
  const search = read('../src/components/overlays/OverlayBookContentSearch.vue')

  assert.match(search, /class="reader-dialog-footer-left"/)
  assert.match(search, /reader-dialog-footer-left[\s\S]*加载中[\s\S]*加载更多/)
  assert.match(search, /reader-dialog-footer-left[\s\S]*跳转上次位置/)
  assert.match(search, /reader-dialog-footer[\s\S]*reader-dialog-footer-left[\s\S]*>取消</)
  assert.doesNotMatch(search, /reader-dialog-footer > \*[\s\S]*flex: 1 1 auto/)
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
  assert.match(cacheZone, /import \{ Close \} from '@element-plus\/icons-vue'/)
  assert.match(cacheZone, /title="取消缓存"/)
  assert.match(cacheZone, /aria-label="取消缓存"/)
  assert.match(cacheZone, /<Close\s*\/>/)
  assert.doesNotMatch(cacheZone, />取消<\/button>/)
  assert.match(cacheZone, /\.reader-cache-zone\s*\{[\s\S]*background:\s*inherit[\s\S]*border:\s*0[\s\S]*box-shadow:\s*none/)
  assert.match(cacheZone, /\.reader-cache-actions button,[\s\S]*\.reader-cache-status button\s*\{[\s\S]*border:\s*0/)
})
