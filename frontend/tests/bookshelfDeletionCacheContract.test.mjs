import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const bookshelf = readFileSync(resolve(__dirname, '../src/stores/bookshelf.js'), 'utf8')

test('routes direct, batch, and sync deletion through one consumer convergence transaction', () => {
  assert.match(bookshelf, /import \{ clearBookBrowserChapterCache \} from '\.\.\/utils\/bookChapterCache'/)
  assert.match(bookshelf, /import \{ dispatchBooksDeleted, normalizeDeletedBookIds \} from '\.\.\/utils\/bookDeletion'/)
  assert.match(bookshelf, /async function clearDeletedBookBrowserCache\(book, bookId, scope\)/)
  assert.match(bookshelf, /async removeBook\(bookId\)[\s\S]*?await deleteBook\(bookId\)[\s\S]*?return this\.reconcileDeletedBooks\(\[bookId\], \[book\]\)/)
  assert.match(bookshelf, /removeBookLocal\(bookId\)[\s\S]*?return this\.reconcileDeletedBooks\(\[bookId\], \[book\]\)/)
  assert.match(bookshelf, /async batchDeleteBooks\(bookIds\)[\s\S]*?return this\.reconcileDeletedBooks\(/)
  assert.match(bookshelf, /async reconcileDeletedBooks\(bookIds, knownBooks = \[\]\)[\s\S]*?const scope = this\.ensureShelfScope\(\)[\s\S]*?reader\.clearProgress\(id\)[\s\S]*?dispatchBooksDeleted\(deletedIds\)[\s\S]*?clearDeletedBookBrowserCache\(booksByID\.get\(id\), id, scope\)[\s\S]*?syncCachedBookRemoval\(id, scope\)/)
})
