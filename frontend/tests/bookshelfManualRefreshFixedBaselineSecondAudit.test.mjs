import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const bookshelfSource = readFileSync(new URL('../src/stores/bookshelf.js', import.meta.url), 'utf8')
const homeSource = readFileSync(new URL('../src/views/Home.vue', import.meta.url), 'utf8')
const readerShelfSource = readFileSync(new URL('../src/composables/useReaderShelf.js', import.meta.url), 'utf8')

test('one Pinia action owns the upstream manual remote refresh transaction', () => {
  assert.match(bookshelfSource, /import\s*\{[^}]*checkBookUpdates[^}]*\}\s*from\s*['"]\.\.\/api\/books['"]/)
  assert.match(bookshelfSource, /let manualRefreshRequest = null/)
  assert.match(
    bookshelfSource,
    /async refreshFromSources\([^)]*\)[\s\S]*?checkBookUpdates\([\s\S]*?replacedBookIds[\s\S]*?clearBookBrowserChapterCache[\s\S]*?loadBooks\(\{ force: true, all: true, settleProgress: true \}\)/,
  )
  assert.match(bookshelfSource, /if \(manualRefreshRequest\) return manualRefreshRequest/)
})

test('Home and Reader shelf reuse the store action instead of implementing two refresh paths', () => {
  assert.match(homeSource, /bookshelf\.refreshFromSources\(\)/)
  assert.match(readerShelfSource, /options\.bookshelf\.refreshFromSources\(\)/)
  assert.doesNotMatch(homeSource, /async function refreshShelf\(\)[\s\S]*?bookshelf\.loadBooks\(\{ force: true, all: true, settleProgress: true \}\)/)
  assert.doesNotMatch(readerShelfSource, /async function refresh\(\)[\s\S]*?bookshelf\.loadBooks\(\{ force: true, all: true, settleProgress: true \}\)/)
})

test('partial remote failures remain visible while the authoritative shelf still commits', () => {
  assert.match(homeSource, /failed[\s\S]*?书架已刷新，\$\{[^}]+\} 本书检查失败/)
  assert.match(readerShelfSource, /failed[\s\S]*?本书检查失败/)
  assert.match(bookshelfSource, /catch \(checkError\)[\s\S]*?loadBooks\(\{ force: true, all: true, settleProgress: true \}\)[\s\S]*?throw checkError/)
})

test('ordinary initial, focus and sync shelf loads do not trigger remote source checks', () => {
  const directCalls = [...bookshelfSource.matchAll(/checkBookUpdates\(/g)]
  assert.equal(directCalls.length, 1)
  assert.doesNotMatch(homeSource, /checkBookUpdates\(/)
  assert.doesNotMatch(readerShelfSource, /checkBookUpdates\(/)
})
