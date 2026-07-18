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
  assert.match(audioSource, /class="reader-audio-cover primary"/)
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

test('audio reader keeps the fixed upstream cover-controls-book-info structure', async () => {
  const [audioSource, contentSource, readerSource] = await Promise.all([
    readFile(new URL('../src/components/reader/ReaderAudioContent.vue', import.meta.url), 'utf8'),
    readFile(new URL('../src/components/reader/ReaderChapterContent.vue', import.meta.url), 'utf8'),
    readFile(new URL('../src/views/Reader.vue', import.meta.url), 'utf8'),
  ])

  assert.doesNotMatch(audioSource, /reader-audio-card|reader-audio-kicker/, 'the rewritten card/kicker is not part of reader-dev audio')
  assert.match(audioSource, /class="reader-audio-info book-info"/)
  assert.match(audioSource, /class="reader-audio-book-title title"/)
  assert.match(audioSource, /class="reader-audio-author subtitle"/)
  assert.match(contentSource, /:book-title="audioBookTitle"/)
  assert.match(contentSource, /:author="audioAuthor"/)
  assert.match(readerSource, /:audio-book-title="book\?\.title \|\| ''"/)
  assert.match(readerSource, /:audio-author="book\?\.author \|\| ''"/)

  const coverIndex = audioSource.indexOf('reader-audio-cover primary')
  const progressIndex = audioSource.indexOf('reader-audio-progress')
  const controlsIndex = audioSource.indexOf('reader-audio-actions primary')
  const volumeIndex = audioSource.indexOf('reader-audio-volume')
  const infoIndex = audioSource.indexOf('reader-audio-info book-info')
  assert(coverIndex >= 0 && coverIndex < progressIndex)
  assert(progressIndex < controlsIndex)
  assert(controlsIndex < volumeIndex)
  assert(volumeIndex < infoIndex)
})

test('audio chapter boundaries stay actionable and autoplay waits for a real play outcome', async () => {
  const [audioSource, contentSource, readerSource] = await Promise.all([
    readFile(new URL('../src/components/reader/ReaderAudioContent.vue', import.meta.url), 'utf8'),
    readFile(new URL('../src/components/reader/ReaderChapterContent.vue', import.meta.url), 'utf8'),
    readFile(new URL('../src/views/Reader.vue', import.meta.url), 'utf8'),
  ])

  assert.doesNotMatch(audioSource, /:disabled="previousDisabled"|:disabled="nextDisabled"/)
  assert.doesNotMatch(contentSource, /previousDisabled|nextDisabled/)
  assert.match(audioSource, /handleLoadedMetadata[\s\S]*attemptAutoplay\(\)/)
  assert.match(audioSource, /function attemptAutoplay\([\s\S]*audio\.play\(\)/)
  assert.match(audioSource, /function handlePlay\([\s\S]*emit\('play'\)/)
  assert.match(contentSource, /@play="emit\('audio-play'\)"/)
  assert.match(readerSource, /@audio-play="handleAudioPlay"/)
  const handleLoadedSource = readerSource.match(/function handleAudioLoaded\([\s\S]*?\n\}/)?.[0] || ''
  assert.doesNotMatch(handleLoadedSource, /audioAutoplay\.value\s*=\s*false/)
  assert.match(readerSource, /function goAudioChapter\([\s\S]*本章是第一章[\s\S]*本章是最后一章/)
})
