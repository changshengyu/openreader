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
  assert.match(readerStoreSource, /autoTheme:\s*true/)
})

test('page mode participates in the same upstream configuration payload and presets', () => {
  assert.match(readerStoreSource, /function readerSettingsPayload[\s\S]*?pageMode:\s*state\.pageMode/)
  assert.match(readerStoreSource, /function defaultReaderSettings[\s\S]*?pageMode:\s*'auto'/)
  assert.match(readerStoreSource, /function sanitizeReaderSettings[\s\S]*?payload\.pageMode/)
  assert.match(readerStoreSource, /function defaultCustomConfigList[\s\S]*?pageMode:\s*'auto'/)
  assert.doesNotMatch(
    readerStoreSource,
    /resetReaderSettingsState[\s\S]*?const pageMode = this\.pageMode/,
    'switching users must not retain the previous user page mode',
  )
})

test('Kindle mode preserves the upstream full-screen click choice', () => {
  const start = readerStoreSource.indexOf('setPageType(pageType)')
  const end = readerStoreSource.indexOf('setPageMode(pageMode)', start)
  const setPageTypeSource = readerStoreSource.slice(start, end)
  assert(start >= 0 && end > start, 'setPageType action missing')
  assert.doesNotMatch(setPageTypeSource, /this\.clickMethod\s*=\s*'none'/)
})

test('existing persisted auto-reading choices are preserved during settings normalization', () => {
  assert.match(
    readerStoreSource,
    /autoReadingPixel\s*=\s*numberAtLeast\(payload\.autoReadingPixel\s*\?\?\s*payload\.autoReadSpeed,\s*1,\s*1\)/,
  )
  assert.match(
    readerStoreSource,
    /autoReadingLineTime\s*=\s*numberAtLeast\(payload\.autoReadingLineTime,\s*10,\s*1000\)/,
  )
  assert.match(readerStoreSource, /fontSize\s*=\s*numberAtLeast\(payload\.fontSize,\s*8,\s*18\)/)
  assert.doesNotMatch(readerStoreSource, /fontSize[^\n]*36/)
  assert.doesNotMatch(readerStoreSource, /autoReadingPixel[^\n]*80/)
  assert.doesNotMatch(readerStoreSource, /autoReadingLineTime[^\n]*3000/)
})

test('reader settings persist separate normal and Kindle recent configurations', () => {
  assert.match(readerStoreSource, /normalPageConfig/)
  assert.match(readerStoreSource, /kindlePageConfig/)
  assert.match(readerStoreSource, /normalPageConfig:\s*sanitizePageConfigSnapshot/)
  assert.match(readerStoreSource, /kindlePageConfig:\s*sanitizePageConfigSnapshot/)
  assert.doesNotMatch(readerStoreSource, /this\.pageType === 'kindle' \? 0 : clampNumber\(duration/)
})
