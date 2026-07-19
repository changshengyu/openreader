import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'

const readerSource = readFileSync(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')
const mobileChromeSource = readFileSync(new URL('../src/components/reader/ReaderMobileChrome.vue', import.meta.url), 'utf8')

test('Reader uses one semantic mini-interface state for wide touch devices', () => {
  assert.match(
    readerSource,
    /class="reader-shell"[\s\S]*?'mini-interface':\s*isMobileReader/,
    'the Reader root must expose the computed mini-interface state to layout CSS',
  )
  assert.match(
    readerSource,
    /<ReaderDesktopTools\s+v-if="!isMobileReader"/,
    'desktop rails must not remain mounted when a wide iPad is using mini mode',
  )
  assert.match(
    readerSource,
    /<ReaderMobileChrome\s+v-if="isMobileReader"/,
    'mobile chrome must be mounted from the same state that selects mobile panels',
  )
  assert.match(
    readerSource,
    /<ReaderDesktopProgress\s+v-if="!isMobileReader"/,
    'desktop progress must not remain mounted when a wide iPad is using mini mode',
  )
  assert.match(
    readerSource,
    /\.reader-shell\.mini-interface\s*\{/,
    'Reader mobile geometry must be keyed by the semantic mini-interface class',
  )
  assert.doesNotMatch(
    readerSource,
    /@media\s*\(max-width:\s*750px\)\s*\{[\s\S]*?\.reader-mobile-primary-dismiss/,
    'primary panel close geometry must not disappear solely because an iPad is wider than 750px',
  )
})

test('mobile chrome visibility is not gated by a second width breakpoint', () => {
  assert.match(mobileChromeSource, /\.reader-mobile-top\.visible\s*\{/, 'visible mobile toolbar styling must exist')
  assert.doesNotMatch(
    mobileChromeSource,
    /@media\s*\(max-width:\s*750px\)/,
    'a mounted mobile chrome must stay usable at iPad Pro widths',
  )
  assert.match(mobileChromeSource, /\.reader-mobile-top\.visible\s*\{[\s\S]*?z-index:\s*11/, 'the toolbar must remain above primary popovers')
})
