import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const mobileChrome = readFileSync(
  new URL('../src/components/reader/ReaderMobileChrome.vue', import.meta.url),
  'utf8',
)
const readerView = readFileSync(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')

test('mobile Reader exposes upstream page progress rather than a whole-book seek slider', () => {
  const template = mobileChrome.slice(0, mobileChrome.indexOf('</template>'))
  assert.match(template, /v-if="pageSliderVisible"[\s\S]*?class="mobile-progress-slider"/)
  assert.match(template, /class="mobile-progress-slider"[\s\S]*?min="1"[\s\S]*?:max="pageSliderMax"/)
  assert.match(template, /:value="pageSliderValue"/)
  assert.match(template, /\{\{ pageSliderLabel \}\}/)
  assert.doesNotMatch(template, /max="1000"/)
  assert.doesNotMatch(template, /bookSliderValue|bookSliderLabel/)
})

test('mobile Reader hides text page progress for audio and keeps one-line book progress cache action', () => {
  assert.match(readerView, /:page-slider-visible="!isAudioChapter"/)
  const template = mobileChrome.slice(0, mobileChrome.indexOf('</template>'))
  assert.match(template, /'without-page-slider': !pageSliderVisible/)
  const progressButton = template.match(/<button class="mobile-chapter-progress"[\s\S]*?<\/button>/)?.[0] || ''
  assert.match(progressButton, /阅读进度: \{\{ bookProgressLabel \}\}/)
  assert.doesNotMatch(progressButton, /chapterLabel/)
})
