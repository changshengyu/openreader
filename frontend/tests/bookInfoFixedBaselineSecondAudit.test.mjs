import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'
import { findShelfBookByURL } from '../src/utils/bookInfoIdentity.js'

const __dirname = dirname(fileURLToPath(import.meta.url))

function read(relative) {
  return readFileSync(resolve(__dirname, relative), 'utf8')
}

test('BookInfo uses the one fixed-upstream dialog structure and geometry', () => {
  const dialog = read('../src/components/BookInfoDialog.vue')
  const panel = read('../src/components/BookInfoPanel.vue')
  const cover = read('../src/components/BookCover.vue')

  assert.match(dialog, /width="500px"/)
  assert.match(dialog, /:fullscreen="isMiniInterface"/)
  assert.doesNotMatch(dialog, /progress|chapters|statusLabel|statusType|browserCacheCount/)

  assert.match(panel, /class="book-info-container"/)
  assert.match(panel, /class="book-cover"/)
  assert.match(panel, /class="book-name"/)
  assert.match(panel, /class="book-kind"/)
  assert.match(panel, /class="book-props"/)
  assert.match(panel, /class="book-intro"/)
  assert.match(panel, /size="book-info"/)
  assert.doesNotMatch(panel, /variant|showStats|wordCount|progressLabel|chapterCount|browserCacheCount|cover-edit-label/)

  assert.match(cover, /book-cover-shared\.size-book-info/)
  assert.match(cover, /\.book-cover-shared\.size-book-info img[\s\S]*width:\s*auto[\s\S]*height:\s*150px/)
})

test('BookInfo preserves upstream field, kind, intro, and action semantics', () => {
  const panel = read('../src/components/BookInfoPanel.vue')
  const overlay = read('../src/components/overlays/OverlayBookInfo.vue')

  assert.match(panel, /String\(value \|\| ''\)\.split\(','\)/)
  assert.doesNotMatch(panel, /\.slice\(0,\s*8\)/)
  assert.match(panel, /split\('\\n'\)/)
  assert.match(panel, /replace\(\/\^\\s\+\/g,\s*''\)/)
  assert.doesNotMatch(panel, /split\(\/\\n\+\/|\.filter\(Boolean\)/)
  assert.match(panel, /v-if="inShelf"[\s\S]*inline-update-switch/)
  assert.match(panel, /:effect="isNight \? 'dark' : 'light'"/)

  assert.match(overlay, /\? '本地'\s*:\s*'未知书源'/)
  assert.match(overlay, /findShelfBookByURL/)
  assert.doesNotMatch(overlay, /refreshBookInfoBrowserCacheCount|browser-cache-count/)
})

test('BookInfo URL identity cannot be replaced by a colliding remote result id', () => {
  const identity = read('../src/utils/bookInfoIdentity.js')

  assert.match(identity, /export function findShelfBookByURL/)
  assert.match(identity, /bookInfoURL/)
  assert.match(identity, /if \(targetURL\)/)
  assert.match(identity, /allowIdFallback/)

  const shelf = [
    { id: 7, title: '书架 A', url: 'https://books.example/a' },
    { id: 8, title: '书架 B', url: 'https://books.example/b' },
  ]
  assert.equal(
    findShelfBookByURL({ id: 7, url: 'https://books.example/b' }, shelf),
    shelf[1],
    'URL must win even when the remote result id collides with another shelf row',
  )
  assert.equal(
    findShelfBookByURL({ id: 7, url: 'https://books.example/remote' }, shelf),
    null,
  )
  assert.equal(
    findShelfBookByURL({ id: 7 }, shelf, { allowIdFallback: true }),
    shelf[0],
  )
})
