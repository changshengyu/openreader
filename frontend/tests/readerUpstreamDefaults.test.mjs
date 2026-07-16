import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const readerStoreSource = readFileSync(
  new URL('../src/stores/reader.js', import.meta.url),
  'utf8',
)

test('new reader settings use reader-dev auto-reading defaults', () => {
  assert.match(readerStoreSource, /autoReadingMethod:\s*'像素滚动'/)
  assert.match(readerStoreSource, /autoReadingPixel:\s*1/)
  assert.match(readerStoreSource, /autoReadingLineTime:\s*1000/)
  assert.match(readerStoreSource, /animateDuration:\s*300/)
})

test('existing persisted auto-reading choices are preserved during settings normalization', () => {
  assert.match(
    readerStoreSource,
    /autoReadingPixel\s*=\s*clampNumber\(payload\.autoReadingPixel\s*\?\?\s*payload\.autoReadSpeed,\s*1,\s*80,\s*1\)/,
  )
  assert.match(
    readerStoreSource,
    /autoReadingLineTime\s*=\s*clampNumber\(payload\.autoReadingLineTime,\s*10,\s*3000,\s*1000\)/,
  )
})
