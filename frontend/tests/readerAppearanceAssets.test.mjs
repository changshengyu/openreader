import assert from 'node:assert/strict'
import test from 'node:test'
import { useReaderAppearanceAssets } from '../src/composables/useReaderAppearanceAssets.js'
import { themeTypeForTheme } from '../src/utils/readerThemeType.js'

function createAppearance(overrides = {}) {
  const calls = []
  const reader = {
    theme: 'parchment',
    themeType: 'day',
    fontFamily: 'system',
    customBgImage: '',
    customBgImageList: [],
    customFontsMap: {},
    setTheme: theme => {
      reader.theme = theme
      reader.themeType = themeTypeForTheme(theme, reader.themeType)
      calls.push(['theme', theme])
    },
    addCustomBgImage: url => {
      reader.customBgImageList = reader.customBgImageList.includes(url)
        ? reader.customBgImageList
        : [...reader.customBgImageList, url]
      reader.customBgImage = url
      calls.push(['add-bg', url])
    },
    removeCustomBgImage: url => {
      reader.customBgImageList = reader.customBgImageList.filter(item => item !== url)
      if (reader.customBgImage === url) reader.customBgImage = ''
      calls.push(['remove-bg', url])
    },
    setCustomFont: (font, url) => {
      reader.customFontsMap[font] = url
      calls.push(['set-font', font, url])
    },
    clearCustomFont: font => {
      delete reader.customFontsMap[font]
      calls.push(['clear-font', font])
    },
    setFontFamily: font => {
      reader.fontFamily = font
      calls.push(['font-family', font])
    },
  }
  const options = {
    reader,
    upload: async payload => {
      calls.push(['upload', payload.type])
      if (overrides.upload) return overrides.upload(payload, reader, calls)
      const directory = payload.type === 'background' ? 'backgrounds' : `${payload.type}s`
      return { data: { url: `/uploads/users/1/${directory}/${payload.type}.asset` } }
    },
    removeAsset: async url => {
      calls.push(['delete', url])
      return overrides.removeAsset?.(url, reader, calls)
    },
    saveSettings: async () => {
      calls.push(['save-settings'])
      if (overrides.saveSettings) return overrides.saveSettings(reader, calls)
      return readerSettingsSnapshot(reader)
    },
    syncFonts: fonts => calls.push(['sync-fonts', { ...fonts }]),
    onSuccess: message => calls.push(['success', message]),
    onWarning: message => calls.push(['warning', message]),
    onError: (error, fallback) => calls.push(['error', error?.message, fallback]),
  }
  return {
    appearance: useReaderAppearanceAssets(options),
    calls,
    reader,
  }
}

function readerSettingsSnapshot(reader) {
  return {
    customBgImage: reader.customBgImage,
    customBgImageList: [...reader.customBgImageList],
    customFontsMap: { ...reader.customFontsMap },
    fontFamily: reader.fontFamily,
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
  const url = '/uploads/users/1/backgrounds/background.asset'
  assert.equal(await appearance.pickBgImage({ raw: { name: 'bg.png' } }), url)
  assert.equal(await appearance.clearBgImage(url), true)
  assert.deepEqual(calls.filter(call => call[0] === 'add-bg' || call[0] === 'remove-bg'), [
    ['add-bg', url],
    ['remove-bg', url],
  ])
  assert.ok(calls.findIndex(call => call[0] === 'save-settings') < calls.findIndex(call => call[0] === 'success'))
  assert.ok(calls.findLastIndex(call => call[0] === 'save-settings') < calls.findIndex(call => call[0] === 'delete'))
})

test('uploads, selects, synchronizes, and clears custom fonts', async () => {
  const { appearance, calls, reader } = createAppearance()
  const font = { value: 'serif', label: '宋体' }
  const url = '/uploads/users/1/fonts/font.asset'
  assert.equal(await appearance.pickFontFile({
    file: { raw: { name: 'font.woff2' } },
    font,
  }), url)
  assert.equal(reader.customFontsMap.serif, url)
  assert.equal(await appearance.clearFontFile(font), true)
  assert.equal(reader.customFontsMap.serif, undefined)
  assert.equal(calls.filter(call => call[0] === 'sync-fonts').length, 2)
  assert.ok(calls.findIndex(call => call[0] === 'save-settings') < calls.findIndex(call => call[0] === 'delete'))
})

test('rolls back an uploaded background and removes the orphan when settings cannot be persisted', async () => {
  const { appearance, calls, reader } = createAppearance({
    saveSettings: () => null,
  })
  reader.customBgImage = '/uploads/users/1/old-background.asset'
  reader.customBgImageList = [reader.customBgImage]
  assert.equal(await appearance.pickBgImage({ raw: { name: 'new.png' } }), null)
  assert.equal(reader.customBgImage, '/uploads/users/1/old-background.asset')
  assert.deepEqual(reader.customBgImageList, ['/uploads/users/1/old-background.asset'])
  assert.deepEqual(calls.find(call => call[0] === 'delete'), [
    'delete',
    '/uploads/users/1/backgrounds/background.asset',
  ])
  assert.equal(calls.some(call => call[0] === 'success'), false)
})

test('does not overwrite server-winning appearance state after a settings conflict', async () => {
  const { appearance, calls, reader } = createAppearance({
    saveSettings: current => {
      current.customBgImage = '/uploads/users/1/server-background.asset'
      current.customBgImageList = [current.customBgImage]
      return readerSettingsSnapshot(current)
    },
  })
  reader.customBgImage = '/uploads/users/1/old-background.asset'
  reader.customBgImageList = [reader.customBgImage]
  assert.equal(await appearance.pickBgImage({ raw: { name: 'new.png' } }), null)
  assert.equal(reader.customBgImage, '/uploads/users/1/server-background.asset')
  assert.deepEqual(reader.customBgImageList, ['/uploads/users/1/server-background.asset'])
  assert.deepEqual(calls.find(call => call[0] === 'delete'), [
    'delete',
    '/uploads/users/1/backgrounds/background.asset',
  ])
})

test('persists a replacement font before cleaning the old managed asset', async () => {
  const { appearance, calls, reader } = createAppearance()
  const oldURL = '/uploads/users/1/fonts/old.woff2'
  reader.customFontsMap.serif = oldURL
  assert.equal(await appearance.pickFontFile({
    file: { raw: { name: 'new.woff2' } },
    font: { value: 'serif', label: '宋体' },
  }), '/uploads/users/1/fonts/font.asset')
  assert.ok(calls.findIndex(call => call[0] === 'save-settings') < calls.findIndex(call => call[0] === 'delete'))
  assert.deepEqual(calls.find(call => call[0] === 'delete'), ['delete', oldURL])
})

test('treats a referenced-file cleanup conflict as a successful setting change', async () => {
  const conflict = new Error('upload is still in use')
  conflict.response = { status: 409 }
  const { appearance, calls, reader } = createAppearance({
    removeAsset: async () => {
      throw conflict
    },
  })
  reader.customFontsMap.system = '/uploads/users/1/fonts/shared.ttf'
  const font = { value: 'system', label: '系统' }
  assert.equal(await appearance.clearFontFile(font), true)
  assert.equal(reader.customFontsMap.system, undefined)
  assert.equal(calls.some(call => call[0] === 'error'), false)
  assert.equal(calls.some(call => call[0] === 'success'), true)
})

test('keeps the asset and restores the font when reader settings cannot be persisted', async () => {
  const { appearance, calls, reader } = createAppearance({
    saveSettings: () => null,
  })
  reader.customFontsMap.system = '/uploads/users/1/fonts/font.ttf'
  const font = { value: 'system', label: '系统' }
  assert.equal(await appearance.clearFontFile(font), false)
  assert.equal(reader.customFontsMap.system, '/uploads/users/1/fonts/font.ttf')
  assert.equal(calls.some(call => call[0] === 'delete'), false)
  assert.deepEqual(calls.find(call => call[0] === 'error'), [
    'error',
    '阅读设置同步失败',
    '恢复默认字体失败',
  ])
})

test('reports upload responses without an asset URL', async () => {
  const { appearance, calls } = createAppearance({
    upload: () => ({ data: {} }),
  })
  assert.equal(await appearance.pickBgImage({ file: { name: 'bad.png' } }), null)
  assert.deepEqual(calls.find(call => call[0] === 'error'), [
    'error',
    '上传结果缺少背景图地址',
    '上传背景图失败',
  ])
})
