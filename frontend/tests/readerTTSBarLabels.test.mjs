import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

// docs/compat/reader-audio-tts-fixed-baseline-p0-contract.md 栏位结构 (and section 11): the expanded read-aloud bar
// follows reader-dev web/src/views/Reader.vue L193-L201, whose rows are labelled 语速 and 语调 (speechRate,
// speechPitch); OpenReader keeps its 定时 row and the user-requested minus/value/plus steppers.

async function ttsBarTemplate() {
  const source = await readFile(new URL('../src/components/reader/ReaderTTSBar.vue', import.meta.url), 'utf8')
  const match = source.match(/<template>([\s\S]*)<\/template>/)
  assert(match, 'ReaderTTSBar.vue has no template')
  return match[1]
}

test('the read-aloud bar labels its rows 语速, 语调 and 定时 in upstream order', async () => {
  const template = await ttsBarTemplate()
  const labels = [...template.matchAll(/<span class="tts-label">([^<]*)<\/span>/g)].map(match => match[1].trim())
  assert.deepEqual(labels.filter(label => ['语速', '语调', '音调', '定时'].includes(label)), ['语速', '语调', '定时'])
})

test('the pitch stepper names the pitch 语调 in every accessible label', async () => {
  const template = await ttsBarTemplate()
  const pitchRow = template.match(/<div class="tts-row">\s*<span class="tts-label">[^<]*<\/span>\s*<ReaderSettingStepper\b[^>]*:model-value="pitch"[^>]*\/>/)
  assert(pitchRow, 'the pitch row is missing')
  for (const label of ['降低朗读语调', '提高朗读语调', '自定义朗读语调']) {
    assert(pitchRow[0].includes(label), `the pitch stepper must be labelled ${label}`)
  }
  assert(!/音调/.test(template), 'the read-aloud bar must not say 音调')
})
