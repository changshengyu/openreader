import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

test('audio reader uses upstream-style custom controls instead of native controls', async () => {
  const [audioSource, contentSource, readerSource] = await Promise.all([
    readFile(new URL('../src/components/reader/ReaderAudioContent.vue', import.meta.url), 'utf8'),
    readFile(new URL('../src/components/reader/ReaderChapterContent.vue', import.meta.url), 'utf8'),
    readFile(new URL('../src/views/Reader.vue', import.meta.url), 'utf8'),
  ])

  assert.match(audioSource, /<audio[\s\S]*preload="metadata"[\s\S]*@play="handlePlay"[\s\S]*@pause="handlePause"/)
  assert.doesNotMatch(audioSource, /\scontrols(?:\s|>|=)/)
  assert.match(audioSource, /class="reader-audio-cover"/)
  assert.match(audioSource, /aria-label="音频播放进度"/)
  assert.match(audioSource, /class="reader-audio-play"/)
  assert.match(audioSource, /@click="togglePlay"/)
  assert.match(audioSource, /@click="seekBy\(-15\)"/)
  assert.match(audioSource, /@click="seekBy\(15\)"/)
  assert.match(audioSource, /aria-label="音频音量"/)
  assert.match(audioSource, /@click="toggleMute"/)
  assert.match(audioSource, /audio\.volume = Math\.max\(0, Math\.min\(1, volume\.value \/ 100\)\)/)

  assert.match(contentSource, /:cover-url="audioCoverUrl"/)
  assert.match(readerSource, /:audio-cover-url="book\?\.customCoverUrl \|\| book\?\.coverUrl \|\| ''"/)
})
