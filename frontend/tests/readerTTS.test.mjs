import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import {
  normalizeTTSPitch,
  normalizeTTSRate,
  normalizeTTSSleepMinutes,
  readerTTSBarVisible,
  readerTTSCurrentParagraphIndex,
  readerTTSParagraphElements,
  readerTTSParagraphText,
  readerTTSProgressLabel,
  readerTTSSleepExpired,
  sortTTSVoices,
} from '../src/utils/readerTTS.js'

const readerView = readFileSync(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')

test('normalizes reader TTS rate and pitch to upstream ranges', () => {
  assert.equal(normalizeTTSRate(0), 0.5)
  assert.equal(normalizeTTSRate('1.6'), 1.6)
  assert.equal(normalizeTTSRate(3), 2)
  assert.equal(normalizeTTSPitch(-1), 0)
  assert.equal(normalizeTTSPitch('0.2'), 0.2)
  assert.equal(normalizeTTSPitch(3), 2)
})

test('normalizes reader TTS sleep minutes to the supported range', () => {
  assert.equal(normalizeTTSSleepMinutes(-1), 0)
  assert.equal(normalizeTTSSleepMinutes('25.9'), 25)
  assert.equal(normalizeTTSSleepMinutes(999), 180)
})

test('sorts reader TTS voices with Chinese voices first then by language', () => {
  const voices = [
    { name: 'English', lang: 'en-US', voiceURI: 'en' },
    { name: 'Japanese', lang: 'ja-JP', voiceURI: 'ja' },
    { name: 'Chinese Taiwan', lang: 'zh-TW', voiceURI: 'tw' },
    { name: 'Chinese Mainland', lang: 'zh-CN', voiceURI: 'cn' },
    { name: 'French', lang: 'fr-FR', voiceURI: 'fr' },
  ]

  assert.deepEqual(sortTTSVoices(voices).map(voice => voice.lang), [
    'zh-CN',
    'zh-TW',
    'en-US',
    'fr-FR',
    'ja-JP',
  ])
  assert.deepEqual(voices.map(voice => voice.lang), [
    'en-US',
    'ja-JP',
    'zh-TW',
    'zh-CN',
    'fr-FR',
  ])
})

test('keeps reader TTS bar visibility independent from playback state', () => {
  assert.equal(readerTTSBarVisible({
    requested: true,
    supported: true,
    chapterFormat: 'text',
    audio: false,
  }), true)
  assert.equal(readerTTSBarVisible({
    requested: true,
    supported: true,
    chapterFormat: 'epub',
    audio: false,
  }), false)
  assert.equal(readerTTSBarVisible({
    requested: true,
    supported: true,
    chapterFormat: 'text',
    audio: true,
  }), false)
  assert.equal(readerTTSBarVisible({
    requested: false,
    supported: true,
    chapterFormat: 'text',
    audio: false,
  }), false)
  assert.equal(readerTTSBarVisible({
    requested: true,
    supported: true,
    chapterFormat: 'text',
    audio: false,
    comic: true,
  }), false)
})

test('opening the TTS bar applies the upstream mobile chrome exception without reopening it on close', () => {
  assert.match(readerView, /ttsBarRequested\.value\s*=\s*!ttsBarRequested\.value[\s\S]*?mobileChromeVisible\.value\s*=\s*false/, 'opening the TTS bar must hide mobile chrome')
  assert.doesNotMatch(readerView, /function closeTTSBar\(\)\s*\{[\s\S]*?mobileChromeVisible\.value\s*=\s*true/, 'closing the TTS bar must not invent a chrome reopen transition')
  assert.match(readerView, /--reader-tts-bottom-space.*?280.*?80/s, 'TTS bar must reserve upstream-like expanded and collapsed content space')
  assert.match(readerView, /ttsSupportedForChapter[\s\S]*?readerTTSSupported\([\s\S]*?isOrdinaryImageComic:\s*isOrdinaryImageComicChapter\.value/s, 'ordinary image comics must use the shared upstream-disabled TTS capability contract')
})

function fakeParagraph({ text, bottom = 100, right = 100, active = false }) {
  return {
    innerText: text,
    textContent: text,
    classList: {
      contains: className => active && ['reading', 'tts-active'].includes(className),
    },
    getBoundingClientRect: () => ({ bottom, right }),
  }
}

test('selects reader TTS paragraphs from the upstream h3 and paragraph DOM contract', () => {
  const elements = [
    fakeParagraph({ text: '标题' }),
    fakeParagraph({ text: '' }),
    fakeParagraph({ text: '第一段' }),
  ]
  const root = {
    querySelectorAll: selector => (selector === 'h3,p' ? elements : []),
  }

  assert.equal(readerTTSParagraphText(elements[0]), '标题')
  assert.deepEqual(readerTTSParagraphElements(root).map(readerTTSParagraphText), ['标题', '第一段'])
})

test('selects current reader TTS paragraph from active class or visible geometry', () => {
  const activeList = [
    fakeParagraph({ text: '第一段', bottom: 20 }),
    fakeParagraph({ text: '第二段', bottom: 80, active: true }),
  ]
  assert.equal(readerTTSCurrentParagraphIndex(activeList), 1)

  const visibleList = [
    fakeParagraph({ text: '第一段', bottom: 20 }),
    fakeParagraph({ text: '第二段', bottom: 80 }),
    fakeParagraph({ text: '第三段', bottom: 120 }),
  ]
  assert.equal(readerTTSCurrentParagraphIndex(visibleList, { topOffset: 50 }), 1)
  assert.equal(readerTTSCurrentParagraphIndex(visibleList, { slide: true }), 0)
  assert.equal(readerTTSCurrentParagraphIndex([]), -1)
})

test('formats reader TTS paragraph progress', () => {
  assert.equal(readerTTSProgressLabel({ playing: false, currentIndex: 0, total: 4 }), '段落 - / -')
  assert.equal(readerTTSProgressLabel({ playing: true, currentIndex: 1, total: 4 }), '段落 2 / 4')
})

test('detects expiration only for an active reader TTS deadline', () => {
  assert.equal(readerTTSSleepExpired(0, 1000), false)
  assert.equal(readerTTSSleepExpired(900, 1000), true)
  assert.equal(readerTTSSleepExpired(1100, 1000), false)
})
