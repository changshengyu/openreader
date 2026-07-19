import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

test('reader image loads trigger pagination and progress recomputation', async () => {
  const [contentSource, readerSource] = await Promise.all([
    readFile(new URL('../src/components/reader/ReaderChapterContent.vue', import.meta.url), 'utf8'),
    readFile(new URL('../src/views/Reader.vue', import.meta.url), 'utf8'),
  ])

  assert.match(
    contentSource,
    /@load="emit\('image-load', \{ blockIndex: block\.index, pos: line\.pos, src: line\.src \}\)"/,
  )
  assert.match(contentSource, /class="reader-content-image"[\s\S]*?@click\.stop/)
  assert.match(contentSource, /@error="handleImageError\(line\)"/)
  assert.match(contentSource, /line\.fallbackSrc/)
  assert.match(contentSource, /'image-load'/)
  assert.match(readerSource, /@image-load="handleReaderImageLoad"/)
  assert.match(
    readerSource,
    /function handleReaderImageLoad\(\) \{\s*updateFlipLayout\(\)\s*progressVersion\.value \+= 1\s*\}/,
  )
})
