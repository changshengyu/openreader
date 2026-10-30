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
const chromeText = new RegExp(`color: var\\(--reader-chrome-text, ${POPUP_TEXT}\\);`)
const settingsText = new RegExp(`color: var\\(--reader-settings-text, ${POPUP_TEXT}\\);`)

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
    '--reader-float-border': 'transparent',
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
  const rail = rule(desktopToolsSource, '.reader-left-rail')
  assert.match(rail, /border-right: 1px solid var\(--reader-chrome-border, rgba\(148, 132, 87, 0\.38\)\);/)
  assert.match(rail, /border-left: 1px solid var\(--reader-chrome-border, rgba\(148, 132, 87, 0\.26\)\);/)
  const item = rule(desktopToolsSource, '.rail-item')
  assert.match(item, new RegExp(`color: var\\(--reader-chrome-text, ${mixed(72)}\\);`))
  assert.match(item, /border-bottom: 1px solid var\(--reader-chrome-border, rgba\(148, 132, 87, 0\.35\)\);/)
  assert.match(rule(desktopToolsSource, '.rail-home'), chromeText)
  const round = rule(desktopToolsSource, '.round-tool')
  assert.match(round, chromeText)
  assert.match(round, /border: 1px solid var\(--reader-float-border, rgba\(255, 255, 255, 0\.7\)\);/)

  assert.match(rule(desktopProgressSource, '.reader-page-control'), /border: 1px solid var\(--reader-chrome-border, rgba\(148, 132, 87, 0\.38\)\);/)
  const step = rule(desktopProgressSource, '.progress-box,\n.page-step')
  assert.match(step, chromeText)
  assert.match(step, /border-bottom: 1px solid var\(--reader-chrome-border, rgba\(148, 132, 87, 0\.32\)\);/)
  assert.match(rule(cachePanelSource, '.reader-cache-zone'), chromeText)
  assert.match(rule(cachePanelSource, '.reader-cache-actions button,\n.reader-cache-status button'), chromeText)
})

test('mobile chrome takes night text, float ring and slider color from the palette', () => {
  assert.match(rule(mobileChromeSource, '.mobile-tool-button'), chromeText)
  const float = rule(mobileChromeSource, '.reader-mobile-float-tools button')
  assert.match(float, chromeText)
  assert.match(float, /border: 1px solid var\(--reader-float-border, rgba\(255, 255, 255, 0\.72\)\);/)
  assert.match(rule(mobileChromeSource, '.mobile-progress-slider-row'), new RegExp(`color: var\\(--reader-chrome-text, ${mixed(76)}\\);`))
  assert.match(rule(mobileChromeSource, '.mobile-progress-slider'), /accent-color: var\(--reader-slider-accent, #409eff\);/)
  assert.match(rule(mobileChromeSource, '.mobile-chapter-step'), chromeText)
  assert.match(rule(mobileChromeSource, '.mobile-chapter-progress span'), chromeText)
})

test('mobile page slider draws reader-dev el-slider night instead of a native light track', () => {
  // A native range with the dark #185798 accent-color gets a light unfilled
  // track in Chromium, so night draws reader-dev's #185798 bar on #282828.
  const template = mobileChromeSource.slice(0, mobileChromeSource.indexOf('</template>'))
  assert.match(template, /class="mobile-progress-slider"[\s\S]*?:class="\{ 'night-slider': isNight \}"/)
  assert.match(template, /:style="\{ '--page-slider-progress': pageSliderProgress \}"/)
  assert.match(mobileChromeSource, /const pageSliderProgress = computed\(/)
  const slider = rule(mobileChromeSource, '.mobile-progress-slider.night-slider')
  assert.match(slider, /-webkit-appearance: none;/)
  assert.match(slider, /appearance: none;/)
  assert.match(
    rule(mobileChromeSource, '.mobile-progress-slider.night-slider::-webkit-slider-runnable-track'),
    /background: linear-gradient\(to right, var\(--reader-slider-accent\) var\(--page-slider-progress, 0%\), var\(--reader-slider-runway\) var\(--page-slider-progress, 0%\)\);/,
  )
  for (const thumb of ['::-webkit-slider-thumb', '::-moz-range-thumb']) {
    const body = rule(mobileChromeSource, `.mobile-progress-slider.night-slider${thumb}`)
    assert.match(body, /background: var\(--reader-slider-runway\);/)
    assert.match(body, /border: 2px solid var\(--reader-slider-accent\);/)
  }
  assert.match(rule(mobileChromeSource, '.mobile-progress-slider.night-slider::-moz-range-track'), /background: var\(--reader-slider-runway\);/)
  assert.match(rule(mobileChromeSource, '.mobile-progress-slider.night-slider::-moz-range-progress'), /background: var\(--reader-slider-accent\);/)
})

test('settings text, swatches and stepper follow ReadSettings night', () => {
  assert.match(rule(settingsSource, '.settings-body'), settingsText)
  assert.match(rule(settingsSource, '.setting-label'), chromeText)
  assert.match(rule(settingsSource, '.setting-help'), new RegExp(`color: var\\(--reader-settings-text, ${mixed(72)}\\);`))
  assert.match(rule(settingsSource, '.font-family-option'), settingsText)
  assert.match(rule(settingsSource, '.font-size-preset'), settingsText)
  assert.match(rule(settingsSource, '.font-action-btn'), new RegExp(`color: var\\(--reader-settings-text, ${mixed(72)}\\);`))
  assert.match(rule(settingsSource, '.theme-item'), /border: 1px solid var\(--reader-control-border\);/)
  assert.match(rule(settingsSource, '.theme-item.active'), /border-color: var\(--reader-theme-selected-border, var\(--reader-accent, #ed4259\)\);/)
  assert.match(rule(settingsSource, '.moon-icon'), /color: var\(--reader-moon-icon, rgba\(255, 255, 255, 0\.2\)\);/)

  assert.match(rule(stepperSource, '.reader-setting-stepper'), settingsText)
  assert.match(
    rule(stepperSource, '.reader-setting-stepper button:disabled'),
    new RegExp(`color: color-mix\\(in srgb, var\\(--reader-settings-text, ${POPUP_TEXT}\\) 48%, transparent\\);`),
  )
  assert.match(
    rule(stepperSource, '.reader-setting-stepper .reader-setting-stepper-value,\n.reader-setting-stepper .reader-setting-stepper-input'),
    new RegExp(`color: var\\(--reader-settings-value-text, ${POPUP_TEXT}\\);`),
  )
})

test('day chrome keeps its current colors as the fallbacks', () => {
  assert.match(readerViewSource, /'--reader-active-text': reader\.themeType === 'night' \? '#7ed8d4' : '#0f5451'/)
  assert.match(readerViewSource, /'--reader-control-bg': [^\n]*'rgba\(255, 255, 255, 0\.5\)'/)
  assert.match(readerViewSource, /'--reader-control-border': [^\n]*'rgba\(0, 0, 0, 0\.1\)'/)
  assert.match(rule(desktopToolsSource, '.rail-item'), new RegExp(mixed(72)))
  assert.match(rule(desktopToolsSource, '.round-tool'), /rgba\(255, 255, 255, 0\.7\)/)
  assert.match(rule(desktopToolsSource, '.round-tool:hover,\n.round-tool.active'), /var\(--reader-active-text, #0f5451\)/)
  assert.match(rule(mobileChromeSource, '.reader-mobile-float-tools button'), /rgba\(255, 255, 255, 0\.72\)/)
  assert.match(rule(mobileChromeSource, '.mobile-progress-slider'), /#409eff/)
  assert.match(rule(settingsSource, '.moon-icon'), /rgba\(255, 255, 255, 0\.2\)/)
  assert.match(rule(settingsSource, '.theme-item.active .moon-icon'), /var\(--reader-accent, #ed4259\)/)
})
