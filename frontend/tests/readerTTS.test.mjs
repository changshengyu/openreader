import assert from 'node:assert/strict'
import test from 'node:test'
import {
  normalizeTTSPitch,
  normalizeTTSRate,
  normalizeTTSSleepMinutes,
  readerTTSProgressLabel,
  readerTTSSleepExpired,
  sortTTSVoices,
} from '../src/utils/readerTTS.js'

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

test('formats reader TTS paragraph progress', () => {
  assert.equal(readerTTSProgressLabel({ playing: false, currentIndex: 0, total: 4 }), '段落 - / -')
  assert.equal(readerTTSProgressLabel({ playing: true, currentIndex: 1, total: 4 }), '段落 2 / 4')
})

test('detects expiration only for an active reader TTS deadline', () => {
  assert.equal(readerTTSSleepExpired(0, 1000), false)
  assert.equal(readerTTSSleepExpired(900, 1000), true)
  assert.equal(readerTTSSleepExpired(1100, 1000), false)
})
