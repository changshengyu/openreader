import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const __dirname = dirname(fileURLToPath(import.meta.url))

function read(relative) {
  return readFileSync(resolve(__dirname, relative), 'utf8')
}

test('moves BookmarkForm ownership to the global overlay host', () => {
  const host = read('../src/components/GlobalOverlayHost.vue')
  const store = read('../src/stores/overlay.js')
  const reader = read('../src/views/Reader.vue')

  assert.match(host, /OverlayBookmarkForm/)
  assert.match(store, /bookmarkFormVisible/)
  assert.match(store, /openBookmarkForm\(/)
  assert.match(store, /finishBookmarkForm\(/)
  assert.doesNotMatch(reader, /ReaderBookmarkFormDialog/)
  assert.doesNotMatch(reader, /useBookBookmarks/)
  assert.equal(existsSync(resolve(__dirname, '../src/components/reader/ReaderBookmarkFormDialog.vue')), false)
})

test('global bookmark form preserves upstream readonly context and mobile dialog behavior', () => {
  const form = read('../src/components/overlays/OverlayBookmarkForm.vue')

  assert.match(form, /<el-dialog/)
  assert.match(form, /width="min\(1000px, max\(750px, 70vw\)\)"/)
  assert.match(form, /top="max\(15dvh, calc\(\(100dvh - 584px\) \/ 2\)\)"/)
  assert.match(form, /:fullscreen="isMobile"/)
  assert.match(form, /label="书名"/)
  assert.match(form, /label="作者"/)
  assert.match(form, /label="章节"/)
  assert.match(form, /label="内容"/)
  assert.match(form, /readonly/)
  assert.match(form, /createBookmark\(/)
  assert.match(form, /updateBookmark\(/)
  assert.match(
    form,
    /updateBookmark\(currentDraft\.id,\s*\{\s*note:\s*payload\.note,?\s*\}\)/s,
  )
  assert.doesNotMatch(form, /updateBookmark\(currentDraft\.id,\s*payload\)/)
})

test('bookmark manager restores the fixed-upstream dialog geometry, columns, and direct field rendering', () => {
  const bookmarks = read('../src/components/overlays/OverlayBookmarks.vue')

  assert.match(bookmarks, /width="min\(1000px, max\(750px, 70vw\)\)"/)
  assert.match(bookmarks, /top="max\(15dvh, calc\(\(100dvh - 584px\) \/ 2\)\)"/)
  assert.match(
    bookmarks,
    /:height="isMobile \? 'calc\(100dvh - 184px\)' : 'min\(400px, calc\(70dvh - 184px\)\)'"/,
  )
  assert.match(bookmarks, /type="selection"\s+width="25"\s+:fixed="isMobile"/)
  assert.match(bookmarks, /label="书籍"[\s\S]*?:fixed="isMobile"[\s\S]*?\{\{ bookIdentity \}\}/)
  assert.match(bookmarks, /label="操作"\s+width="100"/)
  assert.doesNotMatch(bookmarks, /label="操作"[^>]*fixed="right"/)
  assert.doesNotMatch(bookmarks, /scope\.row\.(?:excerpt|note) \|\| '—'/)
  assert.doesNotMatch(bookmarks, /:disabled="!selectedRows\.length"/)
  assert.match(bookmarks, /if \(!Array\.isArray\(rows\)\)/)
  assert.doesNotMatch(bookmarks, /!Array\.isArray\(rows\) \|\| !rows\.length/)
  assert.match(bookmarks, /importRows\(rows\)/)
})

test('bookmark list delegates editing to the global form instead of nesting an editor dialog', () => {
  const bookmarks = read('../src/components/overlays/OverlayBookmarks.vue')
  const actions = read('../src/composables/useOverlayBookmarkActions.js')

  assert.match(bookmarks, /overlay\.openBookmarkForm\(/)
  assert.doesNotMatch(bookmarks, /editorVisible/)
  assert.doesNotMatch(bookmarks, /bookmark-editor/)
  assert.doesNotMatch(actions, /editorVisible/)
  assert.doesNotMatch(actions, /saveEdit/)
})

test('Reader-opened bookmark manager can add one frozen current paragraph through the shared form', () => {
  const bookmarks = read('../src/components/overlays/OverlayBookmarks.vue')
  const reader = read('../src/views/Reader.vue')
  const store = read('../src/stores/overlay.js')

  assert.match(store, /bookmarkCreateDraft/)
  assert.match(store, /openBookmark\(book,\s*options\s*=\s*\{\}\)/)
  assert.match(bookmarks, /v-if="canAddCurrentParagraph"[\s\S]*?>添加当前段落</)
  assert.match(bookmarks, /openBookmarkForm\([\s\S]*?bookmarkCreateDraft[\s\S]*?mode:\s*'create'/)
  assert.match(reader, /createDraft:\s*currentBookmarkDraft\(\)/)
  assert.match(reader, /getCurrentContext:\s*currentBookmarkParagraphContext/)
  assert.match(reader, /readerBookmarkText\(paragraph\)/)
  assert.doesNotMatch(reader, /function currentVisibleExcerpt\([\s\S]*?captureReaderBookmarkExcerpt/)
})
