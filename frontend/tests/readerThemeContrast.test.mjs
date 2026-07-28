import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import {
  readerColorContrast,
  readerTextShadow,
  resolveReaderTextColor,
} from '../src/utils/readerThemeContrast.js'

const readerViewSource = readFileSync(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')
const readerSettingsSource = readFileSync(new URL('../src/components/reader/ReaderSettingsPanel.vue', import.meta.url), 'utf8')
const readerStepperSource = readFileSync(new URL('../src/components/reader/ReaderSettingStepper.vue', import.meta.url), 'utf8')

test('replaces an unreadable persisted day color on a night background at render time', () => {
  assert.ok(readerColorContrast('#262626', '#171717') < 4.5)

  const resolved = resolveReaderTextColor({
    requestedColor: '#262626',
    themeTextColor: '#d8d4c8',
    backgroundColor: '#171717',
    themeType: 'night',
  })

  assert.notEqual(resolved, '#262626')
  assert.ok(readerColorContrast(resolved, '#171717') >= 4.5)
})

test('preserves user colors that already satisfy the reader contrast contract', () => {
  assert.equal(resolveReaderTextColor({
    requestedColor: '#d8d4c8',
    themeTextColor: '#d8d4c8',
    backgroundColor: '#2d2d2d',
    themeType: 'night',
  }), '#d8d4c8')

  assert.equal(resolveReaderTextColor({
    requestedColor: '#262626',
    themeTextColor: '#262626',
    backgroundColor: '#ffffff',
    themeType: 'day',
  }), '#262626')
})

test('protects custom image themes without overwriting the stored text color', () => {
  const resolved = resolveReaderTextColor({
    requestedColor: '#333333',
    themeTextColor: '#333333',
    backgroundColor: '#f4e9bd',
    themeType: 'night',
    hasBackgroundImage: true,
  })

  assert.equal(resolved, '#f2eee4')
  assert.match(readerTextShadow({
    textColor: resolved,
    hasBackgroundImage: true,
  }), /rgba\(0,\s*0,\s*0/)
  assert.equal(readerTextShadow({
    textColor: resolved,
    hasBackgroundImage: false,
  }), 'none')
})

test('ordinary content and EPUB share one effective reader text color', () => {
  assert.match(readerViewSource, /const effectiveReaderTextColor = computed\(/)
  assert.match(readerViewSource, /'--reader-text': effectiveReaderTextColor\.value/)
  assert.match(readerViewSource, /color: \$\{effectiveReaderTextColor\.value\}/)
  assert.match(readerViewSource, /'--reader-text-shadow': effectiveReaderTextShadow\.value/)
  assert.match(
    readerViewSource,
    /reader\.theme === 'custom' && reader\.customBgImage/,
    'custom background images must only apply to the custom theme',
  )
})

test('night settings controls use semantic surfaces instead of translucent day controls', () => {
  assert.match(readerViewSource, /'--reader-control-bg':/)
  assert.match(readerViewSource, /'--reader-control-border':/)
  assert.match(readerViewSource, /'--reader-accent':\s*reader\.themeType === 'night' \? '#ff7589'/)
  assert.match(readerSettingsSource, /background:\s*var\(--reader-control-bg\)/)
  assert.match(readerSettingsSource, /border:\s*1px solid var\(--reader-control-border\)/)
  assert.match(readerSettingsSource, /color:\s*var\(--reader-accent,\s*#ed4259\)/)
  assert.match(readerStepperSource, /background:\s*var\(--reader-control-bg\)/)
  assert.match(readerStepperSource, /border:\s*1px solid var\(--reader-control-border\)/)
  assert.ok(readerColorContrast('#ff7589', '#303030') >= 4.5)
})
