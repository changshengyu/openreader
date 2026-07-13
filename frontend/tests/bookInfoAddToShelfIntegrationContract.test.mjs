import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))

function read(relative) {
  return readFileSync(resolve(__dirname, relative), 'utf8')
}

test('OverlayBookInfo owns the one shared BookInfo add-to-shelf transaction', () => {
  const search = read('../src/views/Search.vue')
  const discover = read('../src/views/Discover.vue')
  const overlay = read('../src/stores/overlay.js')
  const bookInfoOverlay = read('../src/components/overlays/OverlayBookInfo.vue')
  const panel = read('../src/components/BookInfoPanel.vue')

  for (const source of [search, discover]) {
    assert.doesNotMatch(source, /useBookInfoAddToShelf/)
    assert.doesNotMatch(source, /addToShelf\.addRemoteBook\(/)
    assert.doesNotMatch(source, /buildSearch(Add|Existing)BookActions/)
  }

  assert.match(overlay, /selectBookAddCategories\(initialCategoryIds = \[\]\)/)
  assert.match(overlay, /finishBookAddCategories\(categoryIds = null\)/)
  assert.match(bookInfoOverlay, /useBookInfoAddToShelf/)
  assert.match(bookInfoOverlay, /addToShelf\.addRemoteBook\(/)
  assert.match(bookInfoOverlay, /overlay\.selectBookAddCategories/)
  assert.match(bookInfoOverlay, /:show-add-action="canAddBookInfoToShelf"/)
  assert.match(panel, /v-else-if="showAddAction"/)
  assert.match(panel, /加入书架/)
  assert.doesNotMatch(panel, /加入并阅读/)
})
