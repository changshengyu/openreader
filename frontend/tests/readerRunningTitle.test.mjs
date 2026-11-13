import assert from 'node:assert/strict'
import test from 'node:test'

// docs/compat/reader-running-title-load-failure-fixed-baseline-second-audit-p0-contract.md R1, R2, R5 (section 8.1 item 2):
// the running title is `第 N 章` plus the trimmed title of the catalog row at currentIndex, from the current book's
// catalog only; an unknown row gives an empty label instead of a guessed ordinal.

const MODULE = '../src/utils/readerRunningTitle.js'

async function runningTitle() {
  try {
    return await import(MODULE)
  } catch (error) {
    assert.fail(`running title helper must exist at frontend/src/utils/readerRunningTitle.js: ${error.message}`)
  }
}

const BOOK = { id: 7, title: '运行标题契约' }
const CATALOG = ['第一章 起点', '第二章 延迟', '  第三章 缓存  ', '', '   '].map((title, index) => ({
  id: 70 + index,
  bookId: 7,
  index,
  title,
}))

test('labels the catalog row at currentIndex with its 1-based ordinal', async () => {
  const { runningChapterLabelFor } = await runningTitle()
  assert.equal(runningChapterLabelFor({ book: BOOK, chapters: CATALOG, index: 0 }), '第 1 章 第一章 起点')
  assert.equal(runningChapterLabelFor({ book: BOOK, chapters: CATALOG, index: 1 }), '第 2 章 第二章 延迟')
})

test('trims the catalog title and falls back to the ordinal for an empty title', async () => {
  const { runningChapterLabelFor } = await runningTitle()
  assert.equal(runningChapterLabelFor({ book: BOOK, chapters: CATALOG, index: 2 }), '第 3 章 第三章 缓存')
  assert.equal(runningChapterLabelFor({ book: BOOK, chapters: CATALOG, index: 3 }), '第 4 章')
  assert.equal(runningChapterLabelFor({ book: BOOK, chapters: CATALOG, index: 4 }), '第 5 章')
})

test('an unknown catalog row gives an empty label, never a guessed ordinal', async () => {
  const { runningChapterLabelFor } = await runningTitle()
  assert.equal(runningChapterLabelFor({ book: BOOK, chapters: [], index: 0 }), '')
  assert.equal(runningChapterLabelFor({ book: BOOK, chapters: CATALOG, index: 5 }), '')
  assert.equal(runningChapterLabelFor({ book: BOOK, chapters: CATALOG, index: -1 }), '')
  assert.equal(runningChapterLabelFor({ book: null, chapters: CATALOG, index: 0 }), '')
  assert.equal(runningChapterLabelFor({ book: BOOK, chapters: null, index: 0 }), '')
})

test('never pairs an ordinal with another book catalog row', async () => {
  const { runningChapterLabelFor } = await runningTitle()
  assert.equal(runningChapterLabelFor({ book: { id: 8, title: '另一本书' }, chapters: CATALOG, index: 1 }), '')
  const remoteCatalog = [{ id: 0, bookId: 0, index: 0, title: '远程第一章' }]
  assert.equal(
    runningChapterLabelFor({ book: { id: 0, title: '临时阅读' }, chapters: remoteCatalog, index: 0 }),
    '第 1 章 远程第一章',
    'a temporary remote session serializes book and row ids as 0',
  )
})

test('uses the raw catalog title without the book name, Chinese conversion or replace rules', async () => {
  const { runningChapterLabelFor } = await runningTitle()
  const catalog = [{ id: 1, bookId: 7, index: 0, title: '第一章 這裡' }]
  const label = runningChapterLabelFor({ book: BOOK, chapters: catalog, index: 0 })
  assert.equal(label, '第 1 章 第一章 這裡')
  assert(!label.includes(BOOK.title), 'the running title never carries the book name')
})
