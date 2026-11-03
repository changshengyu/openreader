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

// The tools carry reader-dev's `.icon-text` labels.
function toolOrder(source, labels) {
  const template = source.slice(0, source.indexOf('</template>'))
  return labels.map(label => template.indexOf(`<span class="icon-text">${label}</span>`))
}

test('mobile Reader uses the upstream visible top-tool order', () => {
  const indexes = toolOrder(mobileChrome, ['首页', '书架', '书源', '目录', '设置'])
  assert(indexes.every(index => index >= 0), `missing mobile top-tool label: ${indexes.join(', ')}`)
  assert.deepEqual([...indexes].sort((left, right) => left - right), indexes)
})

test('desktop Reader keeps the upstream rail order independently from mobile flex ordering', () => {
  const indexes = toolOrder(desktopTools, ['书架', '书源', '目录', '设置', '首页', '顶部', '底部'])
  assert(indexes.every(index => index >= 0), `missing desktop rail label: ${indexes.join(', ')}`)
  assert.deepEqual([...indexes].sort((left, right) => left - right), indexes)
})

test('mobile Reader exposes the upstream top and bottom float controls', () => {
  const template = mobileChrome.slice(0, mobileChrome.indexOf('</template>'))
  const floatLeft = template.match(/<aside class="reader-mobile-float-tools reader-mobile-float-left"[\s\S]*?<\/aside>/)?.[0] || ''
  // reader-dev float buttons have no tooltip; the accessible name stays.
  assert.match(floatLeft, /aria-label="顶部"/, 'mobile top control should be in the left float zone')
  assert.match(floatLeft, /aria-label="底部"/, 'mobile bottom control should be in the left float zone')
  assert.match(floatLeft, /aria-label="顶部"[^>]*@click="\$emit\('action', 'top'\)"/)
  assert.match(floatLeft, /aria-label="底部"[^>]*@click="\$emit\('action', 'bottom'\)"/)
  assert.doesNotMatch(floatLeft, /\stitle=/)
})

test('Reader source entry stays available instead of being disabled for local books', () => {
  assert.doesNotMatch(mobileChrome, /:disabled="!remoteBook"/)
  assert.doesNotMatch(desktopTools, /:disabled="!remoteBook"/)
  const openSource = panels.match(/function openSource\(\)\s*\{[\s\S]*?\n  \}/)?.[0] || ''
  assert.match(openSource, /options\.sourceVisible\.value\s*=\s*true/)
  assert.doesNotMatch(openSource, /isRemoteBook/)
})
