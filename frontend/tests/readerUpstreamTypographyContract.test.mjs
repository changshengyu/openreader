import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const chapterContent = readFileSync(
  new URL('../src/components/reader/ReaderChapterContent.vue', import.meta.url),
  'utf8',
)
const readerView = readFileSync(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')
const tts = readFileSync(new URL('../src/utils/readerTTS.js', import.meta.url), 'utf8')
const bookmarkSearch = readFileSync(
  new URL('../src/composables/useReaderSearchNavigation.js', import.meta.url),
  'utf8',
)

test('text Reader restores the upstream h3 chapter-title contract across render and navigation', () => {
  assert.match(chapterContent, /<h3\b[^>]*data-pos="0"/)
  assert.doesNotMatch(chapterContent, /<h1\b/)
  assert.match(chapterContent, /h3\s*\{[\s\S]*?font-size:\s*28px;[\s\S]*?line-height:\s*1\.2;[\s\S]*?margin:\s*1em\s+0;[\s\S]*?text-align:\s*center;/)
  assert.match(readerView, /querySelectorAll\('h3, p'\)/)
  assert.match(bookmarkSearch, /querySelectorAll\('h3, p'\)/)
  assert.match(tts, /querySelectorAll\('h3,p'\)/)
})

test('text Reader preserves upstream paragraph wrapping semantics', () => {
  assert.match(chapterContent, /p\s*\{[\s\S]*?word-wrap:\s*break-word;[\s\S]*?word-break:\s*break-all;[\s\S]*?text-indent:\s*2em;/)
})
