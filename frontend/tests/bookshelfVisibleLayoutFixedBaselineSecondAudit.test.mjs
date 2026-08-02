import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import {
  filterShelfBooksByEditQuery,
  normalizeShelfEditQuery,
} from '../src/utils/shelfPresentation.js'

const home = readFileSync(new URL('../src/views/Home.vue', import.meta.url), 'utf8')
const preferences = readFileSync(new URL('../src/stores/preferences.js', import.meta.url), 'utf8')
const shelfCss = readFileSync(new URL('../src/styles/home-shelf.css', import.meta.url), 'utf8')

test('shelf edit search keeps the fixed-upstream trim and lowercase substring semantics', () => {
  const books = [
    { id: 1, title: 'A.B', author: 'Author One' },
    { id: 2, title: 'AB', author: 'Other' },
    { id: 3, title: 'Whitespace', author: 'First  Last' },
  ]

  assert.equal(normalizeShelfEditQuery('  A.B  '), 'a.b')
  assert.deepEqual(filterShelfBooksByEditQuery(books, 'A.B').map(book => book.id), [1])
  assert.deepEqual(filterShelfBooksByEditQuery(books, 'first  last').map(book => book.id), [3])
  assert.deepEqual(filterShelfBooksByEditQuery(books, '  ').map(book => book.id), [1, 2, 3])
})

test('Home counts the displayed shelf and keeps one fixed-upstream grid presentation', () => {
  assert.match(home, /书架 \(\{\{ displayedBooks\.length \}\}\)/)
  assert.doesNotMatch(home, /totalBookCount|effectiveShelfView|shelfView|list-view/)
  assert.doesNotMatch(home, /normalizeLocalBookSearch/)
  assert.match(preferences, /const SHELF_LAYOUT_VERSION = 3/)
  assert.match(preferences, /view:\s*'grid'/)
})

test('Home restores the upstream card metadata DOM and click ownership', () => {
  const card = home.slice(home.indexOf('v-for="book in displayedBooks"'), home.indexOf('</article>', home.indexOf('v-for="book in displayedBooks"')))
  assert.match(card, /class="book-row book"/)
  assert.match(card, /class="cover-img"[\s\S]*?<BookCover/)
  assert.match(card, /class="book-operation"/)
  assert.match(card, /class="name"/)
  assert.match(card, /class="sub"[\s\S]*class="author"[\s\S]*class="dot"[\s\S]*class="size"/)
  assert.match(card, /class="dur-chapter"/)
  assert.match(card, /class="last-chapter"/)
  assert(card.indexOf('class="cover-img"') < card.indexOf('class="book-operation"'))
  assert(card.indexOf('class="book-operation"') < card.indexOf('class="name"'))
  assert(card.indexOf('class="name"') < card.indexOf('class="sub"'))
  assert(card.indexOf('class="sub"') < card.indexOf('class="dur-chapter"'))
  assert(card.indexOf('class="dur-chapter"') < card.indexOf('class="last-chapter"'))
  assert.match(card, /class="cover-img"[\s\S]*?@click\.stop="openDetail\(book\)"/)
  assert.match(card, /class="book-row book"[\s\S]*?@click="handleBookRowClick\(book\)"/)
})

test('Home uses the upstream loading owner and blank result instead of synthetic cards', () => {
  assert.match(home, /v-loading="shelfLoading"[\s\S]*class="books-wrapper"/)
  assert.match(home, /:element-loading-text="shelfLoadingText"/)
  assert.match(home, /正在刷新书籍信息/)
  assert.match(home, /正在获取书籍信息/)
  assert.match(home, /<div class="book-list wrapper">[\s\S]*v-for="book in displayedBooks"/)
  assert.doesNotMatch(home, /skeleton-row|<el-skeleton|<el-empty|emptyText|empty-panel/)
})

test('Home locks the desktop and mobile fixed-baseline geometry', () => {
  assert.match(shelfCss, /\.shelf-page\s*\{[\s\S]*padding:\s*48px;/)
  assert.match(shelfCss, /\.shelf-title\s*\{[\s\S]*font-size:\s*20px;[\s\S]*font-weight:\s*600;/)
  assert.match(shelfCss, /\.shelf-title\s*\{[\s\S]*margin-bottom:\s*5px;/)
  assert.match(shelfCss, /\.book-group-wrapper\s*\{[\s\S]*margin-bottom:\s*10px;[\s\S]*padding:\s*5px 0;/)
  assert.match(shelfCss, /grid-template-columns:\s*repeat\(auto-fill, 380px\)/)
  assert.match(shelfCss, /\.book-row\s*\{[\s\S]*display:\s*flex;[\s\S]*width:\s*360px;[\s\S]*margin-bottom:\s*18px;[\s\S]*padding:\s*24px;/)
  assert.match(shelfCss, /\.cover-img\s*\{[\s\S]*width:\s*84px;[\s\S]*height:\s*112px;/)
  assert.match(shelfCss, /\.info\s*\{[\s\S]*height:\s*112px;[\s\S]*margin-left:\s*20px;/)
  assert.match(shelfCss, /@media \(max-width:\s*750px\)[\s\S]*\.book-row\s*\{[\s\S]*width:\s*100%;[\s\S]*box-sizing:\s*border-box;[\s\S]*margin-bottom:\s*0;[\s\S]*padding:\s*10px 20px;/)
  assert.doesNotMatch(shelfCss, /@media \(max-width:\s*520px\)/)
})

test('Home restores the fixed-upstream night shelf surface', () => {
  assert.match(shelfCss, /:global\(html\.dark-reader \.shelf-page\)[\s\S]*background:\s*#222;/)
  assert.match(shelfCss, /:global\(html\.dark-reader \.shelf-title\)[\s\S]*color:\s*#bbb;/)
  assert.match(shelfCss, /:global\(html\.dark-reader \.shelf-page \.name\)[\s\S]*color:\s*#bbb;/)
  assert.match(shelfCss, /:global\(html\.dark-reader \.shelf-page \.sub\)[\s\S]*color:\s*#6b6b6b;/)
  assert.match(shelfCss, /:global\(html\.dark-reader \.shelf-page \.dur-chapter\)[\s\S]*color:\s*#969ba3;/)
})
