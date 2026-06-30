import assert from 'node:assert/strict'
import test from 'node:test'
import {
  normalizeTTSSleepMinutes,
  readerTTSProgressLabel,
  readerTTSSleepExpired,
} from '../src/utils/readerTTS.js'

test('normalizes reader TTS sleep minutes to the supported range', () => {
  assert.equal(normalizeTTSSleepMinutes(-1), 0)
  assert.equal(normalizeTTSSleepMinutes('25.9'), 25)
  assert.equal(normalizeTTSSleepMinutes(999), 180)
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
