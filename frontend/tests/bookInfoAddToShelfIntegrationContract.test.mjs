import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))

function read(relative) {
  return readFileSync(resolve(__dirname, relative), 'utf8')
}

test('result cards confirm groups while BookInfo keeps the upstream direct add action', () => {
  const search = read('../src/views/Search.vue')
  const discover = read('../src/views/Discover.vue')
  const overlay = read('../src/stores/overlay.js')
  const bookInfoOverlay = read('../src/components/overlays/OverlayBookInfo.vue')
  const panel = read('../src/components/BookInfoPanel.vue')
  const results = read('../src/components/RemoteBookResultList.vue')

  for (const source of [search, discover]) {
    assert.match(source, /useRemoteBookAddToShelf/)
    assert.match(source, /addRemoteBookWithCategories/)
    assert.match(source, /@add="addResultToShelf"/)
    assert.match(source, /:is-night="reader\.themeType === 'night'"/)
    assert.doesNotMatch(source, /buildSearch(Add|Existing)BookActions/)
  }

  assert.match(overlay, /selectBookAddCategories\(initialCategoryIds = \[\]\)/)
  assert.match(overlay, /finishBookAddCategories\(categoryIds = null\)/)
  assert.match(results, /\$emit\('add', book\)/)
  assert.match(results, /加入书架/)
  assert.match(results, /:effect="isNight \? 'dark' : 'light'"/)
  assert.match(bookInfoOverlay, /useRemoteBookAddToShelf/)
  assert.match(bookInfoOverlay, /addToShelf\.addRemoteBook\(/)
  assert.doesNotMatch(bookInfoOverlay, /overlay\.selectBookAddCategories/)
  assert.match(bookInfoOverlay, /:show-add-action="canAddBookInfoToShelf"/)
  assert.match(panel, /v-else-if="showAddAction"/)
  assert.match(panel, /加入书架/)
  assert.doesNotMatch(panel, /加入并阅读/)
})
