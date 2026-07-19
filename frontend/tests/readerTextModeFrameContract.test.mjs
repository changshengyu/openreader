import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const readerSource = readFileSync(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')
const layoutSource = readFileSync(new URL('../src/composables/useReaderLayout.js', import.meta.url), 'utf8')

test('desktop text frame preserves reader-dev 670px content inside the configured 800px width', () => {
  assert.match(
    readerSource,
    /\.reader-page\s*\{[\s\S]*?box-sizing:\s*content-box;/,
    'desktop reader page must leave its two border pixels outside the configured 800px content frame',
  )
  assert.match(
    readerSource,
    /\.reader-page\s*\{[\s\S]*?width:\s*var\(--reader-frame-width\)/,
    'desktop reader page must keep the upstream configured reading width',
  )
  assert.match(
    readerSource,
    /\.reader-content\s*\{[\s\S]*?padding:\s*44px 65px var\(--reader-content-bottom-space\)/,
    'desktop text must retain the upstream 65px horizontal inset',
  )
  assert.match(
    readerSource,
    /@media \(min-width: 751px\)[\s\S]*?\.reader-body\s*\{[\s\S]*?text-align:\s*left;/,
    'desktop text alignment must be explicit rather than inheriting direction-dependent start',
  )
})

test('mobile flip mode owns its upstream viewport, inner clip, and page stride', () => {
  assert.match(
    readerSource,
    /\.reader-shell\.mini-interface\.flip \.reader-page\s*\{[\s\S]*?padding:\s*0;/,
    'mobile flip must not inherit normal reader page padding',
  )
  assert.match(
    readerSource,
    /\.reader-shell\.mini-interface\.flip \.reader-content\s*\{[\s\S]*?position:\s*absolute;[\s\S]*?top:\s*calc\(30px \+ env\(safe-area-inset-top\)\);[\s\S]*?bottom:\s*24px;[\s\S]*?width:\s*100%;[\s\S]*?height:\s*auto;/,
    'mobile flip content must use the upstream top/bottom viewport instead of a normal full-height scroller',
  )
  assert.match(
    readerSource,
    /\.reader-shell\.mini-interface\.flip \.reader-body\s*\{[\s\S]*?margin:\s*0 16px;[\s\S]*?padding:\s*0;[\s\S]*?column-width:\s*calc\(100vw - 16px\);[\s\S]*?column-gap:\s*16px;/,
    'mobile flip must keep upstream 16px inner edges and its width-minus-16 page relationship',
  )
  assert.match(
    layoutSource,
    /pageStride:\s*Math\.max\(1, viewport\.width - 16\)/,
    'flip pagination must move by the upstream viewport width minus 16px',
  )
})
