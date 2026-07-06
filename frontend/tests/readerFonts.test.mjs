import assert from 'node:assert/strict'
import test from 'node:test'
import {
  readerFontOptions,
  readerFontStack,
} from '../src/utils/readerFonts.js'

test('exposes the five upstream reader font families in order', () => {
  assert.deepEqual(
    readerFontOptions.map(font => font.label),
    ['系统', '黑体', '楷体', '宋体', '仿宋'],
  )
})

test('keeps legacy monospace settings readable without exposing them as a preset', () => {
  assert.match(readerFontStack('mono'), /ui-monospace/)
  assert.match(
    readerFontStack('serif', { serif: '/fonts/song.woff2' }),
    /OpenReaderCustomSong/,
  )
})
