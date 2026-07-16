import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const mobileChrome = readFileSync(
  new URL('../src/components/reader/ReaderMobileChrome.vue', import.meta.url),
  'utf8',
)
const desktopTools = readFileSync(
  new URL('../src/components/reader/ReaderDesktopTools.vue', import.meta.url),
  'utf8',
)
const panels = readFileSync(
  new URL('../src/composables/useReaderPanels.js', import.meta.url),
  'utf8',
)

function visibleTopToolOrder(source) {
  const template = source.slice(0, source.indexOf('</template>'))
  return ['书架', '书源', '目录', '设置', '首页'].map(label => template.indexOf(`<span>${label}</span>`))
}

test('mobile Reader uses the upstream visible top-tool order', () => {
  const indexes = visibleTopToolOrder(mobileChrome)
  assert(indexes.every(index => index >= 0), `missing mobile top-tool label: ${indexes.join(', ')}`)
  assert.deepEqual([...indexes].sort((left, right) => left - right), indexes)
})

test('Reader source entry stays available instead of being disabled for local books', () => {
  assert.doesNotMatch(mobileChrome, /:disabled="!remoteBook"/)
  assert.doesNotMatch(desktopTools, /:disabled="!remoteBook"/)
  const openSource = panels.match(/function openSource\(\)\s*\{[\s\S]*?\n  \}/)?.[0] || ''
  assert.match(openSource, /options\.sourceVisible\.value\s*=\s*true/)
  assert.doesNotMatch(openSource, /isRemoteBook/)
})
