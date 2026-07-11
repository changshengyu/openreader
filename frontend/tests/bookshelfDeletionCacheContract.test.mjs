import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const bookshelf = readFileSync(resolve(__dirname, '../src/stores/bookshelf.js'), 'utf8')

test('clears browser chapter caches after direct, batch, and sync-driven shelf deletion', () => {
  assert.match(bookshelf, /import \{ clearBookBrowserChapterCache \} from '\.\.\/utils\/bookChapterCache'/)
  assert.match(bookshelf, /async function clearDeletedBookBrowserCache\(book, bookId\)/)
  assert.match(bookshelf, /async removeBook\(bookId\)[\s\S]*?await deleteBook\(bookId\)[\s\S]*?await clearDeletedBookBrowserCache\(book, bookId\)/)
  assert.match(bookshelf, /removeBookLocal\(bookId\)[\s\S]*?clearDeletedBookBrowserCache\(book, bookId\)/)
  assert.match(bookshelf, /async batchDeleteBooks\(bookIds\)[\s\S]*?await Promise\.all\(deletedIds\.map\(bookId => \([\s\S]*?clearDeletedBookBrowserCache\(booksByID\.get\(Number\(bookId\)\), bookId\)/)
})
