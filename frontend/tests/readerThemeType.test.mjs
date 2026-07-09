import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import {
  normalizeReaderThemeType,
  themeTypeForTheme,
} from '../src/utils/readerThemeType.js'

const storeSource = readFileSync(new URL('../src/stores/reader.js', import.meta.url), 'utf8')
const readerViewSource = readFileSync(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')
const appSource = readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8')
const appLayoutSource = readFileSync(new URL('../src/layouts/AppLayout.vue', import.meta.url), 'utf8')

test('normalizes old and explicit reader theme types without losing compatibility', () => {
  assert.equal(normalizeReaderThemeType(undefined, 'parchment'), 'day')
  assert.equal(normalizeReaderThemeType(undefined, 'dark'), 'night')
  assert.equal(normalizeReaderThemeType('invalid', 'black'), 'night')
  assert.equal(normalizeReaderThemeType('day', 'dark'), 'night')
  assert.equal(normalizeReaderThemeType('night', 'parchment'), 'day')
  assert.equal(normalizeReaderThemeType('night', 'custom'), 'night')
})

test('preset themes derive day/night while custom themes preserve the explicit type', () => {
  assert.equal(themeTypeForTheme('parchment', 'night'), 'day')
  assert.equal(themeTypeForTheme('dark', 'day'), 'night')
  assert.equal(themeTypeForTheme('black', 'day'), 'night')
  assert.equal(themeTypeForTheme('custom', 'night'), 'night')
  assert.equal(themeTypeForTheme('custom', 'day'), 'day')
})

test('reader store persists themeType through settings and custom-config contracts', () => {
  assert.match(storeSource, /themeType:\s*'day'/, 'reader defaults must expose a day theme type')
  assert.match(storeSource, /themeType:\s*normalizeReaderThemeType\(state\.themeType,\s*state\.theme\)/, 'reader settings payload must persist a normalized themeType')
  assert.match(storeSource, /settings\.themeType\s*=\s*normalizeReaderThemeType\(payload\.themeType,\s*payload\.theme\)/, 'old synchronized settings must use the compatibility shim')
  assert.match(storeSource, /setThemeType\(themeType\)/, 'reader store must expose an explicit custom theme type action')
  assert.match(storeSource, /this\.themeType\s*=\s*themeTypeForTheme\(theme,\s*this\.themeType\)/, 'theme selection must apply the upstream preset/custom transition')
  assert.match(storeSource, /themeType:\s*'night'[\s\S]*?name:\s*'内置黑夜'/, 'built-in night config must persist its semantic type')
})

test('reader rendering and shared shell use semantic themeType night state', () => {
  assert.match(readerViewSource, /reader\.themeType\s*===\s*'night'/, 'Reader must render night state from themeType')
  assert.match(appLayoutSource, /reader\.themeType\s*===\s*'night'/, 'workspace shell must render night state from themeType')
  assert.match(appSource, /\(\)\s*=>\s*readerStore\.themeType/, 'document dark-reader class must watch themeType')
  for (const source of [readerViewSource, appLayoutSource, appSource]) {
    assert.doesNotMatch(source, /theme\s*===\s*'dark'\s*\|\|\s*theme\s*===\s*'black'/, 'night rendering must not keep deriving state from theme names')
  }
})
