import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

test('Reader initializes currentIndex before wiring bookmark actions', async () => {
  const source = await readFile(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')
  const currentIndexDeclaration = source.indexOf('const currentIndex = ref(')
  const bookmarkActionsSetup = source.indexOf('useReaderBookmarkActions({')

  assert.notEqual(currentIndexDeclaration, -1)
  assert.notEqual(bookmarkActionsSetup, -1)
  assert.ok(
    currentIndexDeclaration < bookmarkActionsSetup,
    'currentIndex must exist before it is passed to useReaderBookmarkActions',
  )
})

test('Reader defers TOC callbacks that are declared by later controllers', async () => {
  const source = await readFile(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')
  const tocSetupStart = source.indexOf('useReaderToc({')
  const tocSetupEnd = source.indexOf('\n})', tocSetupStart)
  assert.notEqual(tocSetupStart, -1)
  assert.notEqual(tocSetupEnd, -1)
  const tocSetup = source.slice(tocSetupStart, tocSetupEnd)

  assert.match(
    tocSetup,
    /refreshCachedChapters: \(\.\.\.args\) => computeBrowserCachedChapters\(\.\.\.args\)/,
  )
  assert.match(
    tocSetup,
    /syncCurrentChapter: \(\.\.\.args\) => updateCurrentChapterFromScroll\(\.\.\.args\)/,
  )
})
