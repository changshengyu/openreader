import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import { bookCoverUrl, hasBookCover } from '../src/utils/bookCover.js'

test('cover display prefers custom then projected resource then legacy raw URL', () => {
  const book = {
    customCoverUrl: '/uploads/users/1/covers/custom.png',
    coverResourceUrl: '/api/cover/opaque',
    coverUrl: 'https://third-party.example/raw.png',
  }
  assert.equal(bookCoverUrl(book), book.customCoverUrl)
  assert.equal(bookCoverUrl({ ...book, customCoverUrl: '' }), book.coverResourceUrl)
  assert.equal(bookCoverUrl({ ...book, customCoverUrl: '', coverResourceUrl: '' }), '')
  assert.equal(bookCoverUrl({ coverResourceUrl: '', coverUrl: 'http://127.0.0.1/private.png' }), '')
  assert.equal(bookCoverUrl({ coverUrl: 'https://legacy.example/raw.png' }), 'https://legacy.example/raw.png')
  assert.equal(bookCoverUrl({}), '')
  assert.equal(hasBookCover({ coverResourceUrl: '/api/cover/opaque' }), true)
})

test('shared cover owns observable image failure fallback', () => {
  const component = fs.readFileSync(new URL('../src/components/BookCover.vue', import.meta.url), 'utf8')
  assert.match(component, /<img/)
  assert.match(component, /@load=/)
  assert.match(component, /@error=/)
  assert.doesNotMatch(component, /backgroundImage:\s*`url\(/)
  assert.match(component, /暂无封面/)
})

test('shelf reuses BookCover while upstream table-only BookManage adds no CSS cover surface', () => {
  const home = fs.readFileSync(new URL('../src/views/Home.vue', import.meta.url), 'utf8')
  const managerTable = fs.readFileSync(new URL('../src/components/overlays/BookManagementTable.vue', import.meta.url), 'utf8')
  assert.match(home, /<BookCover/)
  assert.match(home, /import BookCover from/)
  assert.doesNotMatch(home, /function coverStyle\(/)
  assert.doesNotMatch(managerTable, /BookCover|backgroundImage|function coverStyle\(/)
})
