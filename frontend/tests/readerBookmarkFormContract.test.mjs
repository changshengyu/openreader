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
  assert.match(form, /:fullscreen="isMobile"/)
  assert.match(form, /label="书名"/)
  assert.match(form, /label="作者"/)
  assert.match(form, /label="章节"/)
  assert.match(form, /label="内容"/)
  assert.match(form, /readonly/)
  assert.match(form, /createBookmark\(/)
  assert.match(form, /updateBookmark\(/)
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
