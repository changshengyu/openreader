import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const __dirname = dirname(fileURLToPath(import.meta.url))

function read(relative) {
  return readFileSync(resolve(__dirname, relative), 'utf8')
}

test('keeps old book detail URLs as shared BookInfo overlay redirects', () => {
  const router = read('../src/router/index.js')
  assert.match(router, /path:\s*'\/books\/:id'[\s\S]*name:\s*'book-detail'[\s\S]*redirect:\s*to =>/)
  assert.match(router, /bookInfo:\s*to\.params\.id/)
  assert.doesNotMatch(router, /const BookDetail = \(\) => import\('\.\.\/views\/BookDetail\.vue'\)/)
  assert.equal(existsSync(resolve(__dirname, '../src/views/BookDetail.vue')), false)
})

test('removes canonical BookDetail navigation from workspace BookInfo actions', () => {
  const home = read('../src/views/Home.vue')
  const search = read('../src/views/Search.vue')
  const discover = read('../src/views/Discover.vue')
  const readerPanels = read('../src/composables/useReaderPanels.js')

  assert.match(home, /function goEditBook\(book\)\s*\{[\s\S]*overlay\.openBookEdit\(book\)/)
  assert.doesNotMatch(search, /完整详情/)
  assert.doesNotMatch(discover, /完整详情/)
  assert.doesNotMatch(readerPanels, /完整详情/)
})

test('AppLayout owns route bookInfo query hydration into the shared overlay', () => {
  const layout = read('../src/layouts/AppLayout.vue')
  assert.match(layout, /route\.query\.bookInfo/)
  assert.match(layout, /api\.get\(`\/books\/\$\{id\}`\)/)
  assert.match(layout, /overlay\.openBookInfo\(mergedBook/)
  assert.doesNotMatch(layout, /buildBookInfo(Read|StartRead)Actions/)
})

test('keeps BookInfo action state in the shared overlay instead of entry contexts', () => {
  const search = read('../src/views/Search.vue')
  const discover = read('../src/views/Discover.vue')
  const layout = read('../src/layouts/AppLayout.vue')
  const bookInfoOverlay = read('../src/components/overlays/OverlayBookInfo.vue')

  for (const source of [search, discover, layout]) {
    assert.doesNotMatch(source, /build(BookInfo(Read|StartRead)|Search(Add|Existing))BookActions/)
    assert.doesNotMatch(source, /actions:\s*build/)
  }
  assert.doesNotMatch(bookInfoOverlay, /overlay\.bookInfoOptions\.actions/)
  assert.match(bookInfoOverlay, /function addBookInfoToShelf\(\)/)
})
