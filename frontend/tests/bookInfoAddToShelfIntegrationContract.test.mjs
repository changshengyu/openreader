import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))

function read(relative) {
  return readFileSync(resolve(__dirname, relative), 'utf8')
}

test('Search and Discover route BookInfo additions through the shared category-confirmation controller', () => {
  const search = read('../src/views/Search.vue')
  const discover = read('../src/views/Discover.vue')
  const overlay = read('../src/stores/overlay.js')
  const host = read('../src/components/GlobalOverlayHost.vue')

  for (const source of [search, discover]) {
    assert.match(source, /useBookInfoAddToShelf/)
    assert.match(source, /addToShelf\.addRemoteBook\(/)
    assert.match(source, /overlay\.selectBookAddCategories/)
    assert.doesNotMatch(source, /remoteBookCreatePayload\([^\n]*targetCategoryIds\.value/)
  }

  assert.match(overlay, /selectBookAddCategories\(initialCategoryIds = \[\]\)/)
  assert.match(overlay, /finishBookAddCategories\(categoryIds = null\)/)
  assert.match(host, /OverlayBookAddToShelf/)
})
