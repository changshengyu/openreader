import assert from 'node:assert/strict'
import test from 'node:test'
import { useReaderAppearanceAssets } from '../src/composables/useReaderAppearanceAssets.js'
import { themeTypeForTheme } from '../src/utils/readerThemeType.js'

function createAppearance(overrides = {}) {
  const calls = []
  const reader = {
    theme: 'parchment',
    themeType: 'day',
    customFontsMap: {},
    setTheme: theme => {
      reader.theme = theme
      reader.themeType = themeTypeForTheme(theme, reader.themeType)
      calls.push(['theme', theme])
    },
    addCustomBgImage: url => calls.push(['add-bg', url]),
    removeCustomBgImage: url => calls.push(['remove-bg', url]),
    setCustomFont: (font, url) => {
      reader.customFontsMap[font] = url
      calls.push(['set-font', font, url])
    },
    clearCustomFont: font => {
      delete reader.customFontsMap[font]
      calls.push(['clear-font', font])
    },
    setFontFamily: font => calls.push(['font-family', font]),
  }
  const options = {
    reader,
    upload: async ({ type }) => ({ data: { url: `/${type}.asset` } }),
    removeAsset: async url => calls.push(['delete', url]),
    saveSettings: async () => {
      calls.push(['save-settings'])
      return { ok: true }
    },
    syncFonts: fonts => calls.push(['sync-fonts', { ...fonts }]),
    onSuccess: message => calls.push(['success', message]),
    onError: (error, fallback) => calls.push(['error', error?.message, fallback]),
    ...overrides,
  }
  return {
    appearance: useReaderAppearanceAssets(options),
    calls,
    reader,
  }
}

test('toggles night themes while allowing explicit theme selection', () => {
  const { appearance, reader } = createAppearance()
  appearance.toggleNight()
  assert.equal(reader.theme, 'dark')
  appearance.toggleNight()
  assert.equal(reader.theme, 'parchment')
  appearance.setTheme('green')
  assert.equal(reader.theme, 'green')
})

test('uploads and removes reader background images', async () => {
  const { appearance, calls } = createAppearance()
  assert.equal(await appearance.pickBgImage({ raw: { name: 'bg.png' } }), '/background.asset')
  assert.equal(await appearance.clearBgImage('/background.asset'), true)
  assert.deepEqual(calls.filter(call => call[0] === 'add-bg' || call[0] === 'remove-bg'), [
    ['add-bg', '/background.asset'],
    ['remove-bg', '/background.asset'],
  ])
  assert.ok(calls.findIndex(call => call[0] === 'save-settings') < calls.findIndex(call => call[0] === 'delete'))
})

test('uploads, selects, synchronizes, and clears custom fonts', async () => {
  const { appearance, calls, reader } = createAppearance()
  const font = { value: 'custom-song', label: '宋体' }
  assert.equal(await appearance.pickFontFile({
    file: { raw: { name: 'font.woff2' } },
    font,
  }), '/font.asset')
  assert.equal(reader.customFontsMap['custom-song'], '/font.asset')
  assert.equal(await appearance.clearFontFile(font), true)
  assert.equal(reader.customFontsMap['custom-song'], undefined)
  assert.equal(calls.filter(call => call[0] === 'sync-fonts').length, 2)
  assert.ok(calls.findIndex(call => call[0] === 'save-settings') < calls.findIndex(call => call[0] === 'delete'))
})

test('keeps the asset and restores the font when reader settings cannot be persisted', async () => {
  const { appearance, calls, reader } = createAppearance({
    saveSettings: async () => null,
  })
  reader.customFontsMap.system = '/font.asset'
  const font = { value: 'system', label: '系统' }
  assert.equal(await appearance.clearFontFile(font), false)
  assert.equal(reader.customFontsMap.system, '/font.asset')
  assert.equal(calls.some(call => call[0] === 'delete'), false)
  assert.deepEqual(calls.find(call => call[0] === 'error'), [
    'error',
    '阅读设置同步失败',
    '恢复默认字体失败',
  ])
})

test('reports upload responses without an asset URL', async () => {
  const { appearance, calls } = createAppearance({
    upload: async () => ({ data: {} }),
  })
  assert.equal(await appearance.pickBgImage({ file: { name: 'bad.png' } }), null)
  assert.deepEqual(calls.find(call => call[0] === 'error'), [
    'error',
    '上传结果缺少背景图地址',
    '上传背景图失败',
  ])
})
