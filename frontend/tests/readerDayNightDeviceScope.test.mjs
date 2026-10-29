import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test, { after } from 'node:test'
import { createServer } from 'vite'

// Contract: docs/compat/reader-day-night-device-scope-and-palette-fixed-baseline-second-audit-p1-contract.md
// reader-dev keeps day/night per browser (App.vue#autoSetTheme only reads the
// local prefers-color-scheme); its built-in night is theme 6 with body_6 /
// content_6 textures, a #121212 popup and #666 text (config.js, Content.vue).

class MemoryStorage {
  constructor() {
    this.values = new Map()
  }

  get length() {
    return this.values.size
  }

  clear() {
    this.values.clear()
  }

  getItem(key) {
    return this.values.has(String(key)) ? this.values.get(String(key)) : null
  }

  key(index) {
    return [...this.values.keys()][index] ?? null
  }

  removeItem(key) {
    this.values.delete(String(key))
  }

  setItem(key, value) {
    this.values.set(String(key), String(value))
  }
}

function tokenFor(userId) {
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url')
  const payload = Buffer.from(JSON.stringify({ userId })).toString('base64url')
  return `${header}.${payload}.day-night-${userId}`
}

const delay = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds))

const storage = new MemoryStorage()
let systemPrefersDark = false
globalThis.localStorage = storage
globalThis.window = {
  localStorage: storage,
  location: { protocol: 'http:', host: 'openreader.test' },
  addEventListener() {},
  removeEventListener() {},
  dispatchEvent() {},
  matchMedia(query) {
    return {
      media: query,
      matches: query === '(prefers-color-scheme: dark)' && systemPrefersDark,
      addEventListener() {},
      removeEventListener() {},
    }
  },
}

const vite = await createServer({
  root: new URL('..', import.meta.url).pathname,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
})
after(() => vite.close())

const { createPinia, setActivePinia } = await import('pinia')
const { default: api } = await vite.ssrLoadModule('/src/api/client.js')
const { themePresets, useReaderStore } = await vite.ssrLoadModule('/src/stores/reader.js')
const {
  isBlackNightReaderSurface,
  readerColorContrast,
  resolveReaderSurface,
  resolveReaderTextColor,
} = await vite.ssrLoadModule('/src/utils/readerThemeContrast.js')

const appSource = readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8')
const readerViewSource = readFileSync(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')

function freshReader({ dark = false } = {}) {
  storage.clear()
  systemPrefersDark = dark
  storage.setItem('openreader_token', tokenFor(1))
  setActivePinia(createPinia())
  const reader = useReaderStore()
  reader.ensureReaderSettingsScope()
  return reader
}

async function withAPI(method, replacement, callback) {
  const original = api[method]
  api[method] = replacement
  try {
    return await callback()
  } finally {
    api[method] = original
  }
}

// The account settings another device saved while it was on `schemeName`.
function accountSettings(reader, schemeName, overrides = {}) {
  const scheme = reader.customConfigList.find(item => item.name === schemeName)
  const { name: _name, configDefaultType: _type, builtin: _builtin, ...fields } = scheme
  return {
    ...fields,
    pageType: 'normal',
    customConfigName: schemeName,
    customConfigList: reader.customConfigList.map(item => ({ ...item })),
    autoTheme: true,
    settingsVersion: 15,
    ...overrides,
  }
}

test('automatic day/night stays on this device and is never saved to the account', { concurrency: false }, async () => {
  const reader = freshReader()
  const saved = []
  await withAPI('put', async (_path, body) => {
    saved.push(body.value)
    return { data: { value: body.value, updatedAt: '2026-10-29T00:00:01Z' }, headers: {} }
  }, async () => {
    const updatedAt = reader.settingsUpdatedAt
    assert.equal(reader.applyAutoTheme(true), true)
    assert.equal(reader.customConfigName, '内置黑夜')
    assert.equal(reader.themeType, 'night')
    assert.equal(reader.settingsUpdatedAt, updatedAt, 'an automatic switch is not a settings change')
    await delay(800)
    assert.deepEqual(saved, [], 'automatic day/night must not be written to the account')

    // Control: the moon/sun button is the user's own choice and is saved.
    assert.equal(reader.setNightTheme(false), true)
    await delay(800)
    assert.equal(saved.length, 1)
    assert.equal(saved[0].customConfigName, '内置白天')
  })
})

test('account settings from another device do not switch this device out of its automatic day/night', { concurrency: false }, async () => {
  for (const { dark, incoming, expected, themeType } of [
    { dark: false, incoming: '内置黑夜', expected: '内置白天', themeType: 'day' },
    { dark: true, incoming: '内置白天', expected: '内置黑夜', themeType: 'night' },
  ]) {
    const reader = freshReader({ dark })
    reader.applyAutoTheme(dark)
    const payload = accountSettings(reader, incoming)
    let writes = 0
    await withAPI('get', async () => ({ data: { value: payload, updatedAt: '2026-10-29T00:00:05Z' } }), async () => {
      await withAPI('put', async () => {
        writes += 1
        throw new Error('re-applying the device color scheme must stay local')
      }, async () => {
        await reader.reconcileReaderSettingsUpdate('2026-10-29T00:00:05Z')
        await delay(800)
      })
    })
    assert.equal(reader.settingsSyncBaseUpdatedAt, '2026-10-29T00:00:05Z')
    assert.equal(reader.customConfigName, expected, `a ${dark ? 'dark' : 'light'} device received ${incoming}`)
    assert.equal(reader.themeType, themeType)
    assert.equal(writes, 0)
  }

  // Control: with automatic switching off, the account choice applies here too.
  const reader = freshReader({ dark: false })
  reader.applyReaderSettings(accountSettings(reader, '内置黑夜', { autoTheme: false }), '2026-10-29T00:00:06Z')
  assert.equal(reader.autoTheme, false)
  assert.equal(reader.customConfigName, '内置黑夜')
  assert.equal(reader.themeType, 'night')
})

test('a signed-in start follows the system color scheme before waiting for account settings', () => {
  const mounted = appSource.slice(appSource.indexOf('onMounted(() => {'), appSource.indexOf('onBeforeUnmount('))
  const signedIn = mounted.slice(mounted.indexOf('if (userStore.token) {\n    bookshelf.ensureShelfScope()'))
  assert.match(
    signedIn,
    /readerStore\.ensureReaderSettingsScope\(\)[\s\S]*?applyAutoThemeFromSystem\(\)[\s\S]*?loadReaderSettingsAndApplyTheme\(\)/,
    'the first paint must already follow this device before the account settings request resolves',
  )
})

test('the built-in night preset and scheme use the reader-dev palette', { concurrency: false }, () => {
  assert.equal(themePresets.dark.bg, '#171717')
  assert.equal(themePresets.dark.text, '#666666')
  assert.equal(themePresets.dark.body, '#121212')
  assert.equal(themePresets.dark.popup, '#121212')
  assert.equal(themePresets.dark.bodyImage, '/themes/body_6.png')
  assert.equal(themePresets.dark.pageImage, '/themes/content_6.png')
  assert.equal(themePresets.dark.popupImage, 'none')
  assert.equal(themePresets.dark.pageBorder, '#444')

  const reader = freshReader()
  const night = reader.customConfigList.find(item => item.name === '内置黑夜')
  assert.equal(night.theme, 'dark')
  assert.equal(night.fontColor, '#666666')
  assert.equal(night.customBodyColor, '#121212')
  assert.equal(night.customPopupColor, '#121212')
  assert.equal(night.customBgColor, '#171717')

  assert.equal(reader.setNightTheme(true), true)
  assert.equal(reader.fontColor, '#666666')
  assert.equal(reader.currentTheme.bg, '#171717')
})

test('built-in night renders the textured surface with #666 text and no black takeover', () => {
  const surface = resolveReaderSurface({
    theme: 'dark',
    themeType: 'night',
    themeBackground: themePresets.dark.bg,
    themeBody: themePresets.dark.body,
    themePopup: themePresets.dark.popup,
    themeBodyImage: themePresets.dark.bodyImage,
    themePageImage: themePresets.dark.pageImage,
    themePopupImage: themePresets.dark.popupImage,
    themePageBorder: themePresets.dark.pageBorder,
  })
  assert.deepEqual(surface, {
    bodyColor: '#121212',
    bodyImage: 'url("/themes/body_6.png")',
    pageColor: '#171717',
    pageImage: 'url("/themes/content_6.png")',
    popupColor: '#121212',
    popupImage: 'none',
    pageBorder: '#444',
    pageShadow: 'none',
  })
  assert.equal(isBlackNightReaderSurface({ pageColor: surface.pageColor, pageImage: surface.pageImage }), false)

  for (const requestedColor of ['#666666', '']) {
    assert.equal(resolveReaderTextColor({
      requestedColor,
      themeTextColor: themePresets.dark.text,
      backgroundColor: surface.pageColor,
      themeType: 'night',
      theme: 'dark',
    }), '#666666', `built-in night must not replace ${requestedColor || 'an empty'} font color`)
  }
  assert.match(
    readerViewSource,
    /const effectiveReaderTextColor = computed\(\(\) => resolveReaderTextColor\(\{[\s\S]*?theme: reader\.theme,[\s\S]*?\}\)\)/,
    'Reader must tell the text-color resolver which theme is rendered',
  )
  assert.match(
    readerViewSource,
    /resolveReaderSurface\(\{[\s\S]*?themePageBorder: reader\.currentTheme\.pageBorder,[\s\S]*?\}\)/,
    'Reader must pass the preset page border to the surface resolver',
  )

  // Controls: custom schemes keep the contrast guard and the pure-black takeover.
  const guarded = resolveReaderTextColor({
    requestedColor: '#333333',
    themeTextColor: '#333333',
    backgroundColor: '#171717',
    themeType: 'night',
    theme: 'custom',
  })
  assert.notEqual(guarded, '#333333')
  assert.ok(readerColorContrast(guarded, '#171717') >= 4.5)
  assert.equal(isBlackNightReaderSurface({ pageColor: '#000000', pageImage: 'none' }), true)
  assert.equal(resolveReaderTextColor({
    requestedColor: '#262626',
    backgroundColor: '#000000',
    themeType: 'night',
    theme: 'custom',
    builtInNight: true,
  }), '#ffffff')
})

test('settings saved before version 15 move the old built-in night white text to #666666', { concurrency: false }, () => {
  const legacyList = reader => reader.customConfigList.map(item => (
    item.name === '内置黑夜' ? { ...item, fontColor: '#ffffff' } : { ...item }
  )).concat([{ ...reader.customConfigList[1], name: '我的夜间', builtin: false, configDefaultType: '', fontColor: '#ffffff' }])
  const legacyNightSnapshot = { mode: 'page', theme: 'dark', themeType: 'night', fontColor: '#ffffff', customConfigName: '内置黑夜' }

  // Locally persisted state from an older build.
  const local = freshReader()
  Object.assign(local, {
    customConfigList: legacyList(local),
    customConfigName: '内置黑夜',
    theme: 'dark',
    themeType: 'night',
    fontColor: '#ffffff',
    normalPageConfig: { ...legacyNightSnapshot },
    settingsVersion: 14,
  })
  local.normalizeSettings()
  assert.equal(local.customConfigList.find(item => item.name === '内置黑夜').fontColor, '#666666')
  assert.equal(local.customConfigList.find(item => item.name === '我的夜间').fontColor, '#ffffff', 'custom schemes keep their color')
  assert.equal(local.fontColor, '#666666')
  assert.equal(local.normalPageConfig.fontColor, '#666666')

  // Account settings saved by an older build.
  const remote = freshReader()
  remote.applyReaderSettings({
    ...accountSettings(remote, '内置黑夜'),
    customConfigList: legacyList(remote),
    fontColor: '#ffffff',
    kindlePageConfig: { ...legacyNightSnapshot },
    autoTheme: false,
    settingsVersion: 14,
  }, '2026-10-29T00:00:07Z')
  assert.equal(remote.customConfigList.find(item => item.name === '内置黑夜').fontColor, '#666666')
  assert.equal(remote.fontColor, '#666666')
  assert.equal(remote.kindlePageConfig.fontColor, '#666666')

  // An empty color already falls back to #666; a version-15 white is the user's choice.
  const current = freshReader()
  current.applyReaderSettings({
    ...accountSettings(current, '内置黑夜'),
    customConfigList: legacyList(current),
    fontColor: '#ffffff',
    autoTheme: false,
    settingsVersion: 15,
  }, '2026-10-29T00:00:08Z')
  assert.equal(current.customConfigList.find(item => item.name === '内置黑夜').fontColor, '#ffffff')
  assert.equal(current.fontColor, '#ffffff')

  const empty = freshReader()
  empty.applyReaderSettings({
    ...accountSettings(empty, '内置黑夜'),
    customConfigList: empty.customConfigList.map(item => (item.name === '内置黑夜' ? { ...item, fontColor: '' } : { ...item })),
    fontColor: '',
    autoTheme: false,
    settingsVersion: 14,
  }, '2026-10-29T00:00:09Z')
  assert.equal(empty.customConfigList.find(item => item.name === '内置黑夜').fontColor, '')
  assert.equal(empty.fontColor, '')
})

// reader-dev's manual moon/sun stays in this browser until the page reloads,
// the system scheme changes or automatic switching is turned on again
// (App.vue#autoSetTheme only runs on created, matchMedia change and the
// autoTheme watcher). Another device saving settings must not undo it.
test('a manual switch on this device survives another device\'s settings until the system scheme changes', { concurrency: false }, async () => {
  const reader = freshReader({ dark: false })
  reader.applyAutoTheme(false)
  await withAPI('put', async (_path, body) => ({ data: { value: body.value, updatedAt: '2026-10-29T00:00:10Z' }, headers: {} }), async () => {
    assert.equal(reader.setNightTheme(true), true)
    await delay(800)
  })
  assert.equal(reader.customConfigName, '内置黑夜')

  const otherDeviceSave = accountSettings(reader, '内置白天', { fontSize: 24 })
  await withAPI('get', async () => ({ data: { value: otherDeviceSave, updatedAt: '2026-10-29T00:00:11Z' } }), async () => {
    await reader.reconcileReaderSettingsUpdate('2026-10-29T00:00:11Z')
  })
  assert.equal(reader.settingsSyncBaseUpdatedAt, '2026-10-29T00:00:11Z')
  assert.equal(reader.customConfigName, '内置黑夜', 'another device\'s save must not undo this device\'s manual night')
  assert.equal(reader.themeType, 'night')

  assert.equal(reader.followSystemColorSchemeChange(), true)
  assert.equal(reader.customConfigName, '内置白天', 'a system color-scheme change ends the manual choice')

  assert.equal(reader.setNightTheme(true), true)
  reader.setAutoTheme(false)
  reader.setAutoTheme(true)
  assert.equal(reader.followDeviceColorScheme(), true)
  assert.equal(reader.customConfigName, '内置白天', 'turning automatic switching on again follows the system')
})

test('the system color-scheme listener ends a manual day/night choice', () => {
  assert.match(appSource, /function handleSystemThemeChange\(\) \{\s*readerStore\.followSystemColorSchemeChange\(\)\s*\}/)
  assert.match(appSource, /systemThemeMedia\.addEventListener\('change', handleSystemThemeChange\)/)
  assert.match(appSource, /systemThemeMedia\.removeEventListener\('change', handleSystemThemeChange\)/)
})
