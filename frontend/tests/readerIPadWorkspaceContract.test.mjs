import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import {
  isMobileLikeViewport,
  shouldUseMiniInterface,
} from '../src/utils/responsive.js'

const readerSource = readFileSync(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')
const mobileChromeSource = readFileSync(new URL('../src/components/reader/ReaderMobileChrome.vue', import.meta.url), 'utf8')
const responsiveSource = readFileSync(new URL('../src/utils/responsive.js', import.meta.url), 'utf8')
const sharedResponsiveConsumers = [
  '../src/views/Reader.vue',
  '../src/views/Home.vue',
  '../src/layouts/AppLayout.vue',
  '../src/components/BookInfoDialog.vue',
  '../src/components/BookEditDialog.vue',
  '../src/components/GlobalOverlayHost.vue',
  '../src/components/workspace/SourceManager.vue',
]

function withIPadNavigator(run) {
  const previous = Object.getOwnPropertyDescriptor(globalThis, 'navigator')
  Object.defineProperty(globalThis, 'navigator', {
    configurable: true,
    value: {
      maxTouchPoints: 5,
      platform: 'MacIntel',
      userAgent: 'Mozilla/5.0 (iPad; CPU OS 18_5 like Mac OS X) AppleWebKit/605.1.15 Version/18.5 Mobile/15E148 Safari/604.1',
    },
  })
  try {
    run()
  } finally {
    if (previous) Object.defineProperty(globalThis, 'navigator', previous)
    else delete globalThis.navigator
  }
}

test('adaptive interface follows the upstream width boundary even on a touch iPad', () => {
  withIPadNavigator(() => {
    assert.equal(isMobileLikeViewport(1366), false)
    assert.equal(isMobileLikeViewport(1024), false)
    assert.equal(isMobileLikeViewport(751), false)
    assert.equal(isMobileLikeViewport(750), true)
    assert.equal(isMobileLikeViewport(390), true)
    assert.equal(isMobileLikeViewport(360), true)

    assert.equal(shouldUseMiniInterface('auto', 1024), false)
    assert.equal(shouldUseMiniInterface('auto', 1366), false)
    assert.equal(shouldUseMiniInterface('mobile', 1024), true)
    assert.equal(shouldUseMiniInterface('mobile', 1366), true)
  })

  assert.doesNotMatch(
    responsiveSource,
    /isMobileLikeViewport[\s\S]*?isMobileBrowser\(\)/,
    'adaptive mode must not turn every mobile UA or touch iPad into a phone-width interface',
  )
})

test('Reader mounts exactly one coherent desktop or mobile scene from the shared decision', () => {
  assert.match(
    readerSource,
    /class="reader-shell"[\s\S]*?'mini-interface':\s*isMobileReader/,
    'the Reader root must expose the computed mini-interface state to layout CSS',
  )
  assert.match(readerSource, /const isMobileReader = computed\(\(\) => shouldUseMiniInterface/)
  assert.match(readerSource, /<ReaderDesktopTools\s+v-if="!isMobileReader"/)
  assert.match(readerSource, /<ReaderMobileChrome\s+v-if="isMobileReader"/)
  assert.match(readerSource, /<ReaderDesktopProgress\s+v-if="!isMobileReader"/)
  assert.doesNotMatch(
    readerSource,
    /@media\s*\(max-width:\s*750px\)\s*\{[\s\S]*?\.reader-mobile-primary-dismiss/,
    'explicit wide phone mode must retain the same coherent mobile panel CSS',
  )
})

test('Reader and workspace dialogs share one width-based responsive predicate', () => {
  for (const file of sharedResponsiveConsumers) {
    const source = readFileSync(new URL(file, import.meta.url), 'utf8')
    assert.match(source, /shouldUseMiniInterface/, `${file} must use the shared responsive decision`)
  }

  assert.match(mobileChromeSource, /\.reader-mobile-top\.visible\s*\{/)
  assert.doesNotMatch(
    mobileChromeSource,
    /@media\s*\(max-width:\s*750px\)/,
    'mounted mobile chrome must still work when explicit phone mode is selected on a wide tablet',
  )
})
