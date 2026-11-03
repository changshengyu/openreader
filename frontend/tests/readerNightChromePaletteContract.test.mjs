import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

// 2026-10-29 user decision (docs/compat/reader-night-chrome-palette-fixed-baseline-second-audit-p1-contract.md):
// Reader night chrome uses reader-dev's `.night` palette for every night scheme; day keeps its colors.
const source = path => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8')
const readerViewSource = source('views/Reader.vue')
const desktopToolsSource = source('components/reader/ReaderDesktopTools.vue')
const desktopProgressSource = source('components/reader/ReaderDesktopProgress.vue')
const cachePanelSource = source('components/reader/ReaderCachePanel.vue')
const mobileChromeSource = source('components/reader/ReaderMobileChrome.vue')
const settingsSource = source('components/reader/ReaderSettingsPanel.vue')
const stepperSource = source('components/reader/ReaderSettingStepper.vue')

const POPUP_TEXT = 'var\\(--reader-popup-text, var\\(--reader-text\\)\\)'
const mixed = percent => `color-mix\\(in srgb, ${POPUP_TEXT} ${percent}%, transparent\\)`
// 2026-11-03 reader-menu contract: the chrome falls back to reader-dev's day colors.
const chromeText = day => new RegExp(`color: var\\(--reader-chrome-text, ${day.replace(/[().]/g, '\\$&')}\\);`)
const chromeLabel = /color: var\(--reader-chrome-label, var\(--reader-chrome-text, rgba\(0, 0, 0, 0\.4\)\)\);/
const settingsText = /color: var\(--reader-settings-text, #606266\);/

// Bodies of every top-level rule whose selector list is exactly `selector`.
function rule(src, selector) {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const bodies = [...src.matchAll(new RegExp(`(?:^|\\n)${escaped} \\{([^}]*)\\}`, 'g'))].map(match => match[1])
  assert.ok(bodies.length, `missing CSS rule ${selector}`)
  return bodies.join('\n')
}

test('reader night chrome variables carry the reader-dev night palette for every night scheme', () => {
  const night = readerViewSource.match(/const NIGHT_CHROME_STYLE = Object\.freeze\(\{([\s\S]*?)\}\)/)
  assert.ok(night, 'Reader.vue must define the reader-dev night chrome variables in one place')
  const entries = Object.fromEntries(
    [...night[1].matchAll(/'(--reader-[a-z-]+)':\s*'([^']+)'/g)].map(match => [match[1], match[2]]),
  )
  assert.deepEqual(entries, {
    '--reader-chrome-text': '#666',
    '--reader-chrome-border': '#444',
    '--reader-slider-accent': '#185798',
    '--reader-slider-runway': '#282828',
    '--reader-settings-text': '#606266',
    '--reader-settings-value-text': '#a6a6a6',
    '--reader-theme-selected-border': '#666',
    '--reader-moon-icon': '#ed4259',
    // 2026-10-30 D3 (reader-night-list-typography contract): the OpenReader-only
    // font preview takes ReadSettings' night `.span-item` surface.
    '--reader-font-preview-bg': 'rgba(45, 45, 45, 0.5)',
    '--reader-font-preview-border': '#666',
  })
  assert.match(readerViewSource, /\.\.\.\(reader\.themeType === 'night' \? NIGHT_CHROME_STYLE : \{\}\)/)
})

test('reader night controls and accent use reader-dev values', () => {
  assert.match(readerViewSource, /'--reader-control-bg': reader\.themeType === 'night' \? 'rgba\(45, 45, 45, 0\.5\)' : 'rgba\(255, 255, 255, 0\.5\)'/)
  assert.match(readerViewSource, /'--reader-control-border': reader\.themeType === 'night' \? '#666' : 'rgba\(0, 0, 0, 0\.1\)'/)
  assert.match(readerViewSource, /'--reader-accent': '#ed4259',/)
  for (const stale of ['#ff7589', '#303030', '#565656']) {
    assert.doesNotMatch(readerViewSource, new RegExp(stale, 'i'), `Reader.vue must not keep readability color ${stale}`)
  }
})

test('desktop tools, progress and cache take night text and borders from the palette', () => {
  const item = rule(desktopToolsSource, '.rail-item')
  assert.match(item, chromeText('#000'))
  assert.match(item, /border: 1px solid var\(--reader-chrome-border, rgba\(0, 0, 0, 0\.1\)\);/)
  assert.match(rule(desktopToolsSource, '.rail-item .icon-text'), chromeLabel)
  assert.match(rule(desktopToolsSource, '.round-tool'), chromeText('#121212'))

  const step = rule(desktopProgressSource, '.progress-box,\n.page-step')
  assert.match(step, chromeText('#000'))
  assert.match(step, /border: 1px solid var\(--reader-chrome-border, rgba\(0, 0, 0, 0\.1\)\);/)
  assert.match(rule(cachePanelSource, '.reader-cache-zone'), chromeLabel)
})

test('mobile chrome takes night text and slider colors from the palette', () => {
  assert.match(rule(mobileChromeSource, '.mobile-tool-button'), chromeText('#000'))
  assert.match(rule(mobileChromeSource, '.mobile-tool-button .icon-text'), chromeLabel)
  const float = rule(mobileChromeSource, '.reader-mobile-float-tools button')
  assert.match(float, chromeText('#121212'))
  assert.match(float, /border: 0;/)
  assert.match(rule(mobileChromeSource, '.mobile-progress-slider-row .progress-tip'), chromeLabel)
  assert.match(rule(mobileChromeSource, '.mobile-chapter-step,\n.mobile-chapter-progress'), chromeText('#000'))
})

test('mobile page slider draws reader-dev el-slider in day and night', () => {
  // A native range with the dark #185798 accent-color gets a light unfilled track in Chromium, so the
  // runway and bar are drawn behind a transparent native range; only its thumb shows.
  const template = mobileChromeSource.slice(0, mobileChromeSource.indexOf('</template>'))
  assert.match(template, /:style="\{ '--page-slider-progress': pageSliderProgress \}"/)
  assert.match(mobileChromeSource, /const pageSliderProgress = computed\(/)
  const slider = rule(mobileChromeSource, '.mobile-progress-slider')
  assert.match(slider, /-webkit-appearance: none;/)
  assert.match(slider, /appearance: none;/)
  assert.match(slider, /background: transparent;/)
  assert.match(
    rule(mobileChromeSource, '.mobile-progress-slider-row .progress-bar::before'),
    /var\(--reader-slider-accent, #409eff\) var\(--page-slider-progress, 0%\),\s*var\(--reader-slider-runway, #fff\) var\(--page-slider-progress, 0%\)/,
  )
  for (const thumb of ['::-webkit-slider-thumb', '::-moz-range-thumb']) {
    const body = rule(mobileChromeSource, `.mobile-progress-slider${thumb}`)
    assert.match(body, /background: var\(--reader-slider-runway, #fff\);/)
    assert.match(body, /border: 2px solid var\(--reader-slider-accent, #409eff\);/)
  }
})

test('settings text, swatches and stepper follow ReadSettings night', () => {
  assert.match(rule(settingsSource, '.settings-body'), settingsText)
  assert.match(rule(settingsSource, '.setting-label'), chromeText('#666'))
  assert.match(rule(settingsSource, '.setting-help'), new RegExp(`color: var\\(--reader-settings-text, ${mixed(72)}\\);`))
  assert.match(rule(settingsSource, '.font-family-option'), settingsText)
  assert.match(rule(settingsSource, '.font-size-preset'), settingsText)
  assert.match(rule(settingsSource, '.font-action-btn'), new RegExp(`color: var\\(--reader-settings-text, ${mixed(72)}\\);`))
  assert.match(rule(settingsSource, '.theme-item'), /border: 1px solid var\(--reader-control-border\);/)
  assert.match(rule(settingsSource, '.theme-item.active'), /border-color: var\(--reader-theme-selected-border, var\(--reader-accent, #ed4259\)\);/)
  assert.match(rule(settingsSource, '.moon-icon'), /color: var\(--reader-moon-icon, rgba\(255, 255, 255, 0\.2\)\);/)

  // The user-requested stepper keeps its own day text color.
  assert.match(rule(stepperSource, '.reader-setting-stepper'), new RegExp(`color: var\\(--reader-settings-text, ${POPUP_TEXT}\\);`))
  assert.match(
    rule(stepperSource, '.reader-setting-stepper button:disabled'),
    new RegExp(`color: color-mix\\(in srgb, var\\(--reader-settings-text, ${POPUP_TEXT}\\) 48%, transparent\\);`),
  )
  assert.match(
    rule(stepperSource, '.reader-setting-stepper .reader-setting-stepper-value,\n.reader-setting-stepper .reader-setting-stepper-input'),
    new RegExp(`color: var\\(--reader-settings-value-text, ${POPUP_TEXT}\\);`),
  )
})

test('day chrome falls back to the reader-dev day colors', () => {
  assert.doesNotMatch(readerViewSource, /--reader-active-text|--reader-float-border/)
  assert.match(readerViewSource, /'--reader-control-bg': [^\n]*'rgba\(255, 255, 255, 0\.5\)'/)
  assert.match(readerViewSource, /'--reader-control-border': [^\n]*'rgba\(0, 0, 0, 0\.1\)'/)
  assert.match(rule(desktopToolsSource, '.rail-item'), /#000/)
  assert.match(rule(desktopToolsSource, '.round-tool'), /#121212/)
  assert.match(rule(mobileChromeSource, '.reader-mobile-float-tools button'), /#121212/)
  assert.match(rule(settingsSource, '.moon-icon'), /rgba\(255, 255, 255, 0\.2\)/)
  assert.match(rule(settingsSource, '.theme-item.active .moon-icon'), /var\(--reader-accent, #ed4259\)/)
})
