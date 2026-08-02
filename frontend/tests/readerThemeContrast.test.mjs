import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import {
  isBlackNightReaderSurface,
  readerColorContrast,
  readerTextShadow,
  resolveReaderSurface,
  resolveReaderTextColor,
} from '../src/utils/readerThemeContrast.js'

const readerViewSource = readFileSync(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')
const readerStoreSource = readFileSync(new URL('../src/stores/reader.js', import.meta.url), 'utf8')
const readerEpubSource = readFileSync(new URL('../src/components/reader/ReaderEpubContent.vue', import.meta.url), 'utf8')
const readerChapterSource = readFileSync(new URL('../src/components/reader/ReaderChapterContent.vue', import.meta.url), 'utf8')
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

  assert.equal(resolved, '#ffffff')
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

test('built-in night resolves to a texture-free black page and white default text', () => {
  assert.match(
    readerStoreSource,
    /dark:\s*\{[^}]*bg:\s*'#000000'[^}]*text:\s*'#ffffff'[^}]*body:\s*'#000000'/,
  )
  assert.match(
    readerStoreSource,
    /black:\s*\{[^}]*bg:\s*'#000000'[^}]*text:\s*'#ffffff'[^}]*body:\s*'#000000'/,
  )

  assert.deepEqual(resolveReaderSurface({
    theme: 'dark',
    themeType: 'night',
    themeBackground: '#2d2d2d',
    themeBody: '#121212',
    themePopup: '#171717',
  }), {
    bodyColor: '#000000',
    bodyImage: 'none',
    pageColor: '#000000',
    pageImage: 'none',
    popupColor: '#171717',
    popupImage: 'none',
    pageBorder: 'transparent',
    pageShadow: 'none',
  })

  assert.equal(resolveReaderTextColor({
    requestedColor: '#d8d4c8',
    themeTextColor: '#ffffff',
    backgroundColor: '#000000',
    themeType: 'night',
    builtInNight: true,
  }), '#ffffff')
  assert.equal(resolveReaderTextColor({
    requestedColor: '#262626',
    themeTextColor: '#ffffff',
    backgroundColor: '#000000',
    themeType: 'night',
    builtInNight: true,
  }), '#ffffff')
  assert.equal(readerColorContrast('#ffffff', '#000000'), 21)
})

test('black content ownership follows the rendered surface even when persisted day/night semantics are stale', () => {
  assert.equal(isBlackNightReaderSurface({
    themeType: 'night',
    pageColor: '#000000',
    pageImage: 'none',
  }), true)
  assert.equal(isBlackNightReaderSurface({
    themeType: 'night',
    pageColor: 'rgb(0, 0, 0)',
    pageImage: 'none',
  }), true)
  assert.equal(isBlackNightReaderSurface({
    themeType: 'day',
    pageColor: '#000000',
    pageImage: 'none',
  }), true)
  assert.equal(isBlackNightReaderSurface({
    themeType: 'night',
    pageColor: '#171717',
    pageImage: 'none',
  }), false)
  assert.equal(isBlackNightReaderSurface({
    themeType: 'night',
    pageColor: '#000000',
    pageImage: 'url("/night.png")',
  }), false)
  assert.equal(isBlackNightReaderSurface({
    themeType: 'night',
    pageColor: 'rgba(0, 0, 0, 0)',
    pageImage: 'none',
  }), false)
})

test('day and custom surfaces do not leak their textures into built-in night', () => {
  assert.deepEqual(resolveReaderSurface({
    theme: 'parchment',
    themeType: 'day',
    themeBackground: '#f4e9bd',
    themeBody: '#d9c27f',
    themePopup: '#fffcef',
  }), {
    bodyColor: '#d9c27f',
    bodyImage: 'var(--reader-body-texture)',
    pageColor: '#f4e9bd',
    pageImage: 'var(--paper-texture)',
    popupColor: '#fffcef',
    popupImage: 'none',
    pageBorder: 'rgba(109, 95, 55, 0.28)',
    pageShadow: 'inset 24px 0 44px rgba(90, 71, 28, 0.05), inset -24px 0 44px rgba(90, 71, 28, 0.05)',
  })
  assert.deepEqual(resolveReaderSurface({
    theme: 'custom',
    themeType: 'night',
    themeBackground: '#020202',
    themeBody: '#030303',
    themePopup: '#040404',
    customBackgroundImage: '/uploads/users/1/backgrounds/night.png',
  }), {
    bodyColor: '#030303',
    bodyImage: 'none',
    pageColor: '#020202',
    pageImage: 'url("/uploads/users/1/backgrounds/night.png")',
    popupColor: '#040404',
    popupImage: 'none',
    pageBorder: 'transparent',
    pageShadow: 'none',
  })
})

test('fixed-baseline day themes own distinct body, content, and popup textures', () => {
  assert.deepEqual(resolveReaderSurface({
    theme: 'parchment',
    themeType: 'day',
    themeBackground: '#ffffff',
    themeBody: '#eadfca',
    themePopup: '#ede7da',
    themeBodyImage: '/themes/body_0.png',
    themePageImage: '/themes/content_0.png',
    themePopupImage: '/themes/popup_0.png',
  }), {
    bodyColor: '#eadfca',
    bodyImage: 'url("/themes/body_0.png")',
    pageColor: '#ffffff',
    pageImage: 'url("/themes/content_0.png")',
    popupColor: '#ede7da',
    popupImage: 'url("/themes/popup_0.png")',
    pageBorder: 'rgba(109, 95, 55, 0.28)',
    pageShadow: 'inset 24px 0 44px rgba(90, 71, 28, 0.05), inset -24px 0 44px rgba(90, 71, 28, 0.05)',
  })
})

test('Reader applies semantic surface variables and EPUB paints the actual reader background', () => {
  assert.match(readerViewSource, /const effectiveReaderSurface = computed\(\(\) => resolveReaderSurface\(/)
  assert.match(readerViewSource, /'--reader-body-bg-image': effectiveReaderSurface\.value\.bodyImage/)
  assert.match(readerViewSource, /'--reader-bg-image': effectiveReaderSurface\.value\.pageImage/)
  assert.match(readerViewSource, /'--reader-popup-bg-image': effectiveReaderSurface\.value\.popupImage/)
  assert.match(readerViewSource, /'--reader-page-border': effectiveReaderSurface\.value\.pageBorder/)
  assert.match(readerViewSource, /'--reader-page-shadow': effectiveReaderSurface\.value\.pageShadow/)
  assert.match(readerViewSource, /background-image:\s*var\(--reader-body-bg-image\)/)
  assert.match(readerViewSource, /background-image:\s*var\(--reader-bg-image\)/)
  assert.doesNotMatch(readerViewSource, /var\(--reader-bg-image,\s*var\(--paper-texture\)\)/)
  assert.match(readerViewSource, /html,\s*\n\s*body\s*\{[\s\S]*?background-color:\s*\$\{effectiveReaderBackgroundColor\.value\} !important;/)
  assert.match(readerViewSource, /background-image:\s*none !important;/)
  assert.match(readerViewSource, /body :where\(p,\s*h1,\s*h2,\s*h3,\s*h4,\s*h5,\s*h6,\s*li,\s*blockquote,\s*figcaption,\s*td,\s*th\)/)
  assert.match(readerEpubSource, /background:\s*var\(--reader-bg,\s*transparent\)/)
})

test('built-in night clears author backgrounds on the actual text-bearing descendants', () => {
  assert.match(
    readerViewSource,
    /'black-night-surface':\s*usesBlackNightContentSurface/,
    'Reader root must expose a rendered black-night content-surface state',
  )
  assert.match(
    readerViewSource,
    /\.reader-shell\.black-night-surface[\s\S]*?:deep\(\[data-reader-block\][^)]*\)[\s\S]*?color:\s*#ffffff !important;[\s\S]*?background-color:\s*#000000 !important;[\s\S]*?background-image:\s*none !important;/,
    'ordinary text-bearing descendants must own an explicit black surface instead of relying on transparent composition',
  )
  assert.match(
    readerViewSource,
    /\.reader-shell\.black-night-surface \.reader-content,[\s\S]*?\.reader-shell\.black-night-surface \.reader-body,[\s\S]*?\.reader-shell\.black-night-surface \.reader-body :deep\(\.chapter-content\)[\s\S]*?background-color:\s*#000000 !important;/,
    'the actual ordinary text-bearing structural surfaces must be explicitly black instead of relying on transparent composition',
  )
  assert.match(
    readerViewSource,
    /usesBlackNightContentSurface\.value[\s\S]*?body :where\(\*\)[\s\S]*?color:\s*inherit !important;[\s\S]*?background-color:\s*#000000 !important;[\s\S]*?background-image:\s*none !important;/,
    'EPUB descendants must own an explicit black surface for every rendered pure-black night surface',
  )
  assert.match(
    readerViewSource,
    /html::before,[\s\S]*?html::after,[\s\S]*?body::before,[\s\S]*?body::after[\s\S]*?background:\s*transparent !important;[\s\S]*?background-image:\s*none !important;/,
    'EPUB root pseudo-elements must not retain a light authored backdrop over the black body',
  )
  assert.match(
    readerViewSource,
    /:built-in-night="usesBlackNightContentSurface"/,
    'EPUB bridge ownership must follow the rendered black-night surface state',
  )
  assert.match(
    readerChapterSource,
    /<p v-else-if="line\.html"[^>]*data-reader-block/,
    'the ordinary HTML fixture must exercise the actual text-bearing block',
  )
})
