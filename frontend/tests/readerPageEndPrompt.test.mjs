import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const readerSource = readFileSync(
  new URL('../src/views/Reader.vue', import.meta.url),
  'utf8',
)

test('page mode renders the upstream chapter-end action inside the reading flow', () => {
  assert.match(readerSource, /v-if="showChapterEndPrompt"/)
  assert.match(readerSource, /class="reader-chapter-end"/)
  assert.match(readerSource, />\s*加载下一章\s*</)
  assert.match(readerSource, /@click\.stop="handleChapterEndNext"/)
  assert.match(readerSource, /@touchstart\.stop/)
  assert.match(readerSource, /@touchend\.stop/)
  assert.match(
    readerSource,
    /showChapterEndPrompt\s*=\s*computed\([\s\S]*effectiveReaderMode\.value\s*===\s*'page'[\s\S]*chapterLoaded\.value[\s\S]*!chapterLoadError\.value[\s\S]*!isAudioChapter\.value/,
  )
})

test('chapter-end action navigates explicitly and preserves the upstream last-chapter error', () => {
  assert.match(
    readerSource,
    /function handleChapterEndNext\(\)[\s\S]*currentIndex\.value\s*>=\s*chapters\.value\.length\s*-\s*1[\s\S]*showReaderToast\('本章是最后一章'\)[\s\S]*return[\s\S]*goChapter\(currentIndex\.value\s*\+\s*1\)/,
  )
})
