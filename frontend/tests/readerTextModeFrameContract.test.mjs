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

test('keeps the scrolling text outside a CSS filter while preserving brightness with a passive overlay', () => {
  assert.doesNotMatch(
    readerSource,
    /\.reader-page\s*\{[\s\S]*?filter:\s*brightness\(/,
    'the fixed upstream scroll path has no filtered scrolling ancestor',
  )
  assert.match(
    readerSource,
    /['"]--reader-dim-opacity['"]:\s*Math\.max\(0,\s*1 - reader\.brightness \/ 100\)/,
    'reader brightness must be converted to an equivalent black-overlay alpha',
  )
  assert.match(
    readerSource,
    /\.reader-page::after\s*\{[\s\S]*?background:\s*rgba\(0,\s*0,\s*0,\s*var\(--reader-dim-opacity\)\);[\s\S]*?pointer-events:\s*none;[\s\S]*?\.reader-shell\.mini-interface\.document-scroll \.reader-page::after\s*\{[\s\S]*?position:\s*fixed;/,
    'brightness overlay must not intercept paging and must become viewport-sized for mobile document scrolling',
  )
})

test('mobile document scrolling uses the upstream tiled texture without a chapter-height shadow surface', () => {
  assert.match(
    readerSource,
    /--paper-texture:\s*url\(["']data:image\/png;base64,[A-Za-z0-9+/=]+["']\)/,
    'the default paper surface must use the fixed upstream small PNG texture',
  )
  assert.doesNotMatch(
    readerSource,
    /--paper-texture:\s*[\s\S]*?radial-gradient/,
    'the mobile root-scroll surface must not retain the multi-layer CSS gradient texture',
  )
  assert.match(
    readerSource,
    /\.reader-shell\.mini-interface \.reader-page\s*\{[\s\S]*?box-shadow:\s*none;/,
    'mobile text must not raster an inset shadow along the full chapter height',
  )
  assert.match(
    readerSource,
    /\.reader-shell\.mini-interface\.document-scroll\.has-custom-background \.reader-page\s*\{[\s\S]*?background-attachment:\s*fixed;/,
    'custom mobile backgrounds must be limited to the viewport rather than rastered as one chapter-height cover',
  )
})

test('mobile vertical text restores the upstream document scroll surface', () => {
  assert.match(
    readerSource,
    /'document-scroll':\s*usesDocumentScroll/,
    'Reader must expose the effective document-scroll state on its shell',
  )
  assert.match(
    readerSource,
    /\.reader-shell\.mini-interface\.document-scroll \.reader-content\s*\{[\s\S]*?height:\s*auto;[\s\S]*?overflow:\s*visible;/,
    'mobile document-scroll text must not keep an independently scrolling reader-content',
  )
  assert.match(
    readerSource,
    /\.reader-shell\.mini-interface\.document-scroll \.reader-page\s*\{[\s\S]*?height:\s*auto;[\s\S]*?overflow:\s*visible;/,
    'the reader page must grow with the root document in vertical text modes',
  )
})

test('reader exposes the current chapter number and title in the upstream running header', () => {
  assert.match(
    readerSource,
    /<header\s+v-if="showRunningChapterHeader"\s+class="reader-page-head">\s*<span class="reader-running-chapter-title">\{\{ runningChapterLabel \}\}<\/span>\s*<\/header>/,
    'the running header shows only the running chapter label on every viewport',
  )
  assert.doesNotMatch(
    readerSource,
    /\{\{\s*chapterLabel\s*\}\}/,
    'fixed upstream reader renders no chapter N / total counter anywhere in the template',
  )
  assert.doesNotMatch(
    readerSource,
    /const chapterLabel = computed/,
    'the unused chapter counter computed must be removed so it cannot be rendered again',
  )
  // Rewritten for the running title contract R1 (section 8.1 item 1): the label used to come from the committed
  // chapter object, which shows the new ordinal with the previous chapter name while loading and after a failure.
  assert.match(
    readerSource,
    /const runningChapterLabel = computed\(\(\) => runningChapterLabelFor\(\{[^}]*\bchapters:\s*chapters\.value\b[^}]*\bindex:\s*currentIndex\.value\b[^}]*\}\)\)/,
    'the label must derive from the catalog row at the current 1-based chapter position',
  )
  assert.doesNotMatch(
    readerSource,
    /formatRunningChapterLabel\([^)]*chapter\.value\?\.title/,
    'the running title must not read the committed chapter object',
  )
  assert.match(
    readerSource,
    /const showRunningChapterHeader = computed\(\(\) => !isAudioChapter\.value && !isCBZBook\(book\.value\)\)/,
    'audio and full-width CBZ reading must preserve their header exceptions',
  )
  assert.doesNotMatch(
    readerSource,
    /\.reader-shell\.mini-interface \.reader-page-head\s*\{\s*display:\s*none;/,
    'mini interface must not hide the upstream chapter header',
  )
  assert.match(
    readerSource,
    /\.reader-shell\.mini-interface \.reader-page-head\s*\{[\s\S]*?position:\s*fixed;[\s\S]*?height:\s*calc\(30px \+ env\(safe-area-inset-top\)\);[\s\S]*?padding:\s*calc\(6px \+ env\(safe-area-inset-top\)\) 16px 6px;[\s\S]*?font-size:\s*12px;/,
    'mobile header must retain the upstream fixed safe-area geometry',
  )
  assert.match(
    readerSource,
    /\.reader-running-chapter-title\s*\{[\s\S]*?overflow:\s*hidden;[\s\S]*?text-overflow:\s*ellipsis;[\s\S]*?white-space:\s*nowrap;/,
    'long chapter titles must stay on one non-overlapping line',
  )
})
