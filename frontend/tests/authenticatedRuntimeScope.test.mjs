import assert from 'node:assert/strict'
import test, { after } from 'node:test'
import { createServer } from 'vite'

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

function deferred() {
  let resolve
  let reject
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function tokenFor(userId, nonce = '') {
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url')
  const payload = Buffer.from(JSON.stringify({ userId })).toString('base64url')
  return `${header}.${payload}.scope-${userId}-${nonce}`
}

const storage = new MemoryStorage()
const timerCallbacks = new Map()
const dispatchedEvents = []
let timerSerial = 0
globalThis.localStorage = storage
globalThis.window = {
  localStorage: storage,
  location: { protocol: 'http:', host: 'openreader.test' },
  addEventListener() {},
  removeEventListener() {},
  dispatchEvent(event) {
    dispatchedEvents.push({
      type: event?.type || '',
      detail: event?.detail,
      tokenAtDispatch: storage.getItem('openreader_token'),
    })
  },
  setTimeout(callback) {
    timerSerial += 1
    timerCallbacks.set(timerSerial, callback)
    return timerSerial
  },
  clearTimeout(id) {
    timerCallbacks.delete(id)
  },
}
globalThis.CustomEvent = class CustomEvent {
  constructor(type, options = {}) {
    this.type = type
    this.detail = options.detail
  }
}

const vite = await createServer({
  root: new URL('..', import.meta.url).pathname,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
})
after(() => vite.close())

const { createPinia, setActivePinia } = await import('pinia')
// Load the shared API module before stores so Vite's SSR graph gives every
// production import the same mutable client instance used by these contracts.
const { default: api } = await vite.ssrLoadModule('/src/api/client.js')
const { currentUserScope } = await vite.ssrLoadModule('/src/utils/authScope.js')
const { useBookshelfStore } = await vite.ssrLoadModule('/src/stores/bookshelf.js')
const { usePreferencesStore } = await vite.ssrLoadModule('/src/stores/preferences.js')
const { useReaderStore } = await vite.ssrLoadModule('/src/stores/reader.js')
const { useUserStore } = await vite.ssrLoadModule('/src/stores/user.js')
const { useOverlayStore } = await vite.ssrLoadModule('/src/stores/overlay.js')
const { useIndexWorkspaceStore } = await vite.ssrLoadModule('/src/stores/indexWorkspace.js')
const { useSync } = await vite.ssrLoadModule('/src/composables/useSync.js')

function activateUser(userId, nonce = '') {
  const token = tokenFor(userId, nonce)
  storage.setItem('openreader_token', token)
  return token
}

function freshStores(userId = 1) {
  storage.clear()
  timerCallbacks.clear()
  dispatchedEvents.length = 0
  activateUser(userId, 'initial')
  setActivePinia(createPinia())
  return {
    bookshelf: useBookshelfStore(),
    preferences: usePreferencesStore(),
    reader: useReaderStore(),
    user: useUserStore(),
    overlay: useOverlayStore(),
    workspace: useIndexWorkspaceStore(),
  }
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

test('reader settings preserve all five upstream font slots plus the legacy monospace slot', { concurrency: false }, () => {
  const { reader } = freshStores(1)
  reader.setFontFamily('hei')
  reader.setCustomFont('hei', '/uploads/users/1/fonts/hei.ttf')
  reader.setCustomFont('fangsong', '/uploads/users/1/fonts/fangsong.ttf')
  assert.equal(reader.fontFamily, 'hei')
  assert.deepEqual(reader.customFontsMap, {
    hei: '/uploads/users/1/fonts/hei.ttf',
    fangsong: '/uploads/users/1/fonts/fangsong.ttf',
  })

  reader.applyReaderSettings({
    fontFamily: 'fangsong',
    customFontsMap: {
      system: '/uploads/users/1/fonts/system.ttf',
      hei: '/uploads/users/1/fonts/hei.ttf',
      kai: '/uploads/users/1/fonts/kai.ttf',
      serif: '/uploads/users/1/fonts/serif.ttf',
      fangsong: '/uploads/users/1/fonts/fangsong.ttf',
      mono: '/uploads/users/1/fonts/legacy-mono.ttf',
      unsupported: '/uploads/users/1/fonts/unsupported.ttf',
    },
  })
  assert.equal(reader.fontFamily, 'fangsong')
  assert.deepEqual(Object.keys(reader.customFontsMap).sort(), [
    'fangsong',
    'hei',
    'kai',
    'mono',
    'serif',
    'system',
  ])
})

test('manual night switching applies a complete scheme without corrupting the saved day scheme', { concurrency: false }, () => {
  const { reader } = freshStores(1)
  const originalDay = {
    ...reader.customConfigList.find(item => item.name === '内置白天'),
  }
  reader.fontColor = '#222222'
  reader.customBgImage = '/uploads/users/1/backgrounds/day.png'
  reader.syncActiveCustomConfig()

  assert.equal(reader.setNightTheme(true), true)
  assert.equal(reader.customConfigName, '内置黑夜')
  assert.equal(reader.theme, 'dark')
  assert.equal(reader.themeType, 'night')
  assert.equal(reader.currentTheme.bg, '#000000')
  assert.equal(reader.currentTheme.text, '#ffffff')
  assert.equal(reader.currentTheme.body, '#000000')
  assert.equal(reader.customBgImage, '')

  const savedDay = reader.customConfigList.find(item => item.name === '内置白天')
  assert.equal(savedDay.name, originalDay.name)
  assert.equal(savedDay.configDefaultType, '白天默认')
  assert.equal(savedDay.theme, 'parchment')
  assert.equal(savedDay.themeType, 'day')
})

test('browser color-scheme switching uses the same complete default-scheme transition', { concurrency: false }, () => {
  const { reader } = freshStores(1)
  reader.setAutoTheme(true)

  assert.equal(reader.applyAutoTheme(true), true)
  assert.equal(reader.customConfigName, '内置黑夜')
  assert.equal(reader.themeType, 'night')
  assert.equal(reader.currentTheme.bg, '#000000')
  assert.equal(reader.currentTheme.text, '#ffffff')

  assert.equal(reader.applyAutoTheme(false), true)
  assert.equal(reader.customConfigName, '内置白天')
  assert.equal(reader.themeType, 'day')
})

test('custom schemes copy built-in day without activating or capturing global settings', { concurrency: false }, () => {
  const { reader } = freshStores(1)
  const builtInDay = { ...reader.customConfigList[0] }
  reader.customConfigList = [
    ...reader.customConfigList,
    { ...builtInDay, name: '当前方案', builtin: false, configDefaultType: '', theme: 'blue', fontSize: 29 },
  ]
  assert.equal(reader.setCustomConfig('当前方案'), true)
  reader.pageType = 'kindle'
  reader.autoTheme = false
  reader.ttsRate = 1.7
  reader.customFontsMap = { hei: '/uploads/users/1/fonts/hei.ttf' }
  reader.customBgImageList = ['/uploads/users/1/backgrounds/paper.png']

  assert.deepEqual(reader.createCustomConfig(' 新方案 '), { ok: true })
  assert.equal(reader.customConfigName, '当前方案')
  const created = reader.customConfigList.find(item => item.name === '新方案')
  assert.equal(created.theme, builtInDay.theme)
  assert.equal(created.fontSize, builtInDay.fontSize)
  for (const forbidden of ['pageType', 'autoTheme', 'ttsRate', 'ttsPitch', 'ttsVoiceURI', 'customFontsMap', 'customBgImageList']) {
    assert.equal(Object.hasOwn(created, forbidden), false, `scheme captured global field ${forbidden}`)
  }
})

test('legacy polluted schemes apply only scheme fields and preserve global mode, TTS, and assets', { concurrency: false }, () => {
  const { reader } = freshStores(1)
  reader.customConfigList = [
    ...reader.customConfigList,
    {
      ...reader.customConfigList[0],
      name: '旧方案',
      builtin: false,
      configDefaultType: '',
      theme: 'blue',
      fontSize: 24,
      brightness: 83,
      pageType: 'normal',
      autoTheme: true,
      ttsRate: 0.5,
      ttsPitch: 0,
      ttsVoiceURI: 'wrong-voice',
      customFontsMap: {},
      customBgImageList: [],
    },
  ]
  reader.pageType = 'kindle'
  reader.autoTheme = false
  reader.ttsRate = 1.8
  reader.ttsPitch = 1.2
  reader.ttsVoiceURI = 'voice-a'
  reader.customFontsMap = { hei: '/uploads/users/1/fonts/hei.ttf' }
  reader.customBgImageList = ['/uploads/users/1/backgrounds/paper.png']

  assert.equal(reader.setCustomConfig('旧方案'), true)
  assert.equal(reader.theme, 'blue')
  assert.equal(reader.fontSize, 24)
  assert.equal(reader.brightness, 83)
  assert.equal(reader.pageType, 'kindle')
  assert.equal(reader.autoTheme, false)
  assert.equal(reader.ttsRate, 1.8)
  assert.equal(reader.ttsPitch, 1.2)
  assert.equal(reader.ttsVoiceURI, 'voice-a')
  assert.deepEqual(reader.customFontsMap, { hei: '/uploads/users/1/fonts/hei.ttf' })
  assert.deepEqual(reader.customBgImageList, ['/uploads/users/1/backgrounds/paper.png'])
})

test('reset restores the day configuration without deleting schemes, TTS, or uploaded assets', { concurrency: false }, () => {
  const { reader } = freshStores(1)
  reader.customConfigList = [
    ...reader.customConfigList,
    { ...reader.customConfigList[0], name: '保留方案', builtin: false, configDefaultType: '' },
  ]
  reader.customFontsMap = { serif: '/uploads/users/1/fonts/song.woff2' }
  reader.customBgImageList = ['/uploads/users/1/backgrounds/paper.png']
  reader.customBgImage = reader.customBgImageList[0]
  reader.ttsRate = 1.6
  reader.ttsPitch = 0.8
  reader.ttsVoiceURI = 'voice-b'
  reader.theme = 'custom'
  reader.brightness = 72

  reader.resetReaderSettings()

  assert(reader.customConfigList.some(item => item.name === '保留方案'))
  assert.deepEqual(reader.customFontsMap, { serif: '/uploads/users/1/fonts/song.woff2' })
  assert.deepEqual(reader.customBgImageList, ['/uploads/users/1/backgrounds/paper.png'])
  assert.equal(reader.customBgImage, '')
  assert.equal(reader.ttsRate, 1.6)
  assert.equal(reader.ttsPitch, 0.8)
  assert.equal(reader.ttsVoiceURI, 'voice-b')
  assert.equal(reader.customConfigName, '内置白天')
  assert.equal(reader.theme, 'parchment')
  assert.equal(reader.themeType, 'day')
  assert.equal(reader.fontColor, '#262626')
  assert.equal(reader.brightness, 100)
})

test('normal and Kindle configurations survive round trips and reader-settings reload', { concurrency: false }, async () => {
  const { reader } = freshStores(1)
  reader.setMode('scroll')
  reader.setPageMode('auto')
  reader.setAnimateDuration(450)
  reader.setFontSize(27)
  reader.setClickMethod('next')

  reader.setPageType('kindle')
  assert.equal(reader.mode, 'flip')
  assert.equal(reader.animateDuration, 0)
  assert.equal(reader.fontSize, 20)
  assert.equal(reader.clickMethod, 'next')
  reader.setAnimateDuration(150)
  reader.setFontSize(19)
  reader.setTheme('green')

  reader.setPageType('normal')
  assert.equal(reader.mode, 'scroll')
  assert.equal(reader.animateDuration, 450)
  assert.equal(reader.fontSize, 27)
  reader.setPageType('kindle')
  assert.equal(reader.mode, 'flip')
  assert.equal(reader.animateDuration, 150)
  assert.equal(reader.fontSize, 19)
  assert.equal(reader.theme, 'green')

  let persisted
  await withAPI('put', async (_path, body) => {
    persisted = body.value
    return { data: { value: body.value, updatedAt: '2026-08-02T12:00:00Z' }, headers: {} }
  }, async () => {
    await reader.saveReaderSettings()
  })
  assert.equal(persisted.pageType, 'kindle')
  assert.equal(persisted.normalPageConfig.mode, 'scroll')
  assert.equal(persisted.kindlePageConfig.animateDuration, 150)

  const { reader: restored } = freshStores(1)
  restored.applyReaderSettings(persisted)
  restored.setPageType('normal')
  assert.equal(restored.mode, 'scroll')
  assert.equal(restored.animateDuration, 450)
  assert.equal(restored.fontSize, 27)
  restored.setPageType('kindle')
  assert.equal(restored.animateDuration, 150)
  assert.equal(restored.fontSize, 19)
  assert.equal(restored.theme, 'green')
})

test('a reader settings broadcast received before its own save response does not supersede that save', { concurrency: false }, async () => {
  const request = deferred()
  const { reader } = freshStores(1)
  reader.theme = 'custom'
  reader.settingsSyncBaseUpdatedAt = '2026-07-23T06:00:00Z'
  let reads = 0

  await withAPI('get', async () => {
    reads += 1
    return {
      data: {
        value: { theme: 'blue', themeType: 'day' },
        updatedAt: '2026-07-23T06:00:01Z',
      },
    }
  }, async () => {
    await withAPI('put', () => request.promise, async () => {
      const saving = reader.saveReaderSettings()
      const queued = reader.reconcileReaderSettingsUpdate('2026-07-23T06:00:01Z')
      assert.equal(queued, null)

      request.resolve({
        data: {
          value: { theme: 'custom', themeType: 'day' },
          updatedAt: '2026-07-23T06:00:01Z',
        },
        headers: {},
      })
      const saved = await saving
      await Promise.resolve()

      assert.deepEqual(saved, { theme: 'custom', themeType: 'day' })
      assert.equal(reader.theme, 'custom')
      assert.equal(reader.settingsSyncBaseUpdatedAt, '2026-07-23T06:00:01Z')
      assert.equal(reads, 0)
    })
  })
})

test('a genuinely newer reader settings broadcast is loaded once after the local save settles', { concurrency: false }, async () => {
  const request = deferred()
  const loaded = deferred()
  const { reader } = freshStores(1)
  reader.theme = 'custom'
  reader.settingsSyncBaseUpdatedAt = '2026-07-23T06:00:00Z'
  let reads = 0

  await withAPI('get', async () => {
    reads += 1
    loaded.resolve()
    return {
      data: {
        value: { theme: 'blue', themeType: 'day' },
        updatedAt: '2026-07-23T06:00:02Z',
      },
    }
  }, async () => {
    await withAPI('put', () => request.promise, async () => {
      const saving = reader.saveReaderSettings()
      reader.reconcileReaderSettingsUpdate('2026-07-23T06:00:02Z')
      request.resolve({
        data: {
          value: { theme: 'custom', themeType: 'day' },
          updatedAt: '2026-07-23T06:00:01Z',
        },
        headers: {},
      })
      assert.deepEqual(await saving, { theme: 'custom', themeType: 'day' })
      await loaded.promise
      await Promise.resolve()

      assert.equal(reads, 1)
      assert.equal(reader.theme, 'blue')
      assert.equal(reader.settingsSyncBaseUpdatedAt, '2026-07-23T06:00:02Z')
    })
  })
})

test('a delayed reader-settings load cannot commit into a later authenticated scope', { concurrency: false }, async () => {
  const request = deferred()
  const { reader } = freshStores(1)

  await withAPI('get', () => request.promise, async () => {
    const loading = reader.loadReaderSettings()
    activateUser(2, 'next')
    reader.resetReaderSettingsState()
    reader.theme = 'blue'
    reader.settingsSyncBaseUpdatedAt = 'user-b-base'
    reader.settingsUpdatedAt = 'user-b-updated'
    reader.settingsSyncError = 'user-b-error'

    request.resolve({
      data: {
        value: { theme: 'dark', themeType: 'night', fontSize: 31 },
        updatedAt: 'user-a-updated',
      },
    })
    await loading

    assert.equal(reader.theme, 'blue')
    assert.equal(reader.fontSize, 18)
    assert.equal(reader.settingsSyncBaseUpdatedAt, 'user-b-base')
    assert.equal(reader.settingsUpdatedAt, 'user-b-updated')
    assert.equal(reader.settingsSyncError, 'user-b-error')
  })
})

test('a delayed preference load cannot mix user A into user B persisted preferences', { concurrency: false }, async () => {
  const request = deferred()
  const { preferences } = freshStores(1)

  await withAPI('get', () => request.promise, async () => {
    const loading = preferences.loadPreference('shelf')
    activateUser(2, 'next')
    preferences.resetPreferenceState()
    preferences.shelf = { view: 'list', layoutVersion: 2 }
    preferences.syncBaseUpdatedAt.shelf = 'user-b-base'
    preferences.syncError.shelf = 'user-b-error'

    request.resolve({
      data: {
        value: { view: 'grid', layoutVersion: 2 },
        updatedAt: 'user-a-updated',
      },
    })
    await loading

    assert.equal(preferences.shelf.view, 'list')
    assert.equal(preferences.syncBaseUpdatedAt.shelf, 'user-b-base')
    assert.equal(preferences.syncError.shelf, 'user-b-error')
  })
})

test('explicit reader configuration sync reports a missing backup without creating one', { concurrency: false }, async () => {
  const { reader } = freshStores(1)
  let writes = 0
  await withAPI('get', async () => ({ data: { key: 'reader', value: null, updatedAt: '' } }), async () => {
    await withAPI('put', async () => {
      writes += 1
      return { data: {} }
    }, async () => {
      const restored = await reader.loadReaderSettings({ createIfMissing: false })
      assert.equal(restored, null)
      assert.equal(reader.settingsSyncError, '没有备份文件')
      assert.equal(writes, 0)
    })
  })
})

test('explicit preference sync reports missing snapshots without writing them back', { concurrency: false }, async () => {
  const { preferences } = freshStores(1)
  let writes = 0
  await withAPI('get', async path => ({ data: { key: path.split('/').at(-1), value: null, updatedAt: '' } }), async () => {
    await withAPI('put', async () => {
      writes += 1
      return { data: {} }
    }, async () => {
      const restored = await preferences.loadPreferences({ createIfMissing: false })
      assert.deepEqual(restored, [null, null])
      assert.equal(preferences.syncError.shelf, '没有备份文件')
      assert.equal(preferences.syncError.search, '没有备份文件')
      assert.equal(writes, 0)
    })
  })
})

test('an older preference save cannot settle a newer same-key operation in another scope', { concurrency: false }, async () => {
  const requestA = deferred()
  const requestB = deferred()
  let requestCount = 0
  const { preferences } = freshStores(1)
  preferences.shelf = { view: 'grid', layoutVersion: 2 }
  preferences.syncBaseUpdatedAt.shelf = 'user-a-base'

  await withAPI('put', () => {
    requestCount += 1
    return requestCount === 1 ? requestA.promise : requestB.promise
  }, async () => {
    const savingA = preferences.savePreference('shelf')
    activateUser(2, 'next')
    preferences.resetPreferenceState()
    preferences.shelf = { view: 'list', layoutVersion: 2 }
    const savingB = preferences.savePreference('shelf')

    requestA.resolve({
      data: { value: { view: 'grid', layoutVersion: 2 }, updatedAt: 'user-a-updated' },
      headers: { 'x-openreader-setting-conflict': '1' },
    })
    await savingA
    assert.equal(preferences.shelf.view, 'grid')
    assert.equal(preferences.syncing.shelf, true)
    assert.notEqual(preferences.syncBaseUpdatedAt.shelf, 'user-a-updated')

    requestB.resolve({
      data: { value: { view: 'list', layoutVersion: 2 }, updatedAt: 'user-b-updated' },
      headers: {},
    })
    await savingB
    assert.equal(preferences.shelf.view, 'grid')
    assert.equal(preferences.syncBaseUpdatedAt.shelf, 'user-b-updated')
    assert.equal(preferences.syncing.shelf, false)
  })
})

test('a delayed progress response cannot create a local progress key in the next user scope', { concurrency: false }, async () => {
  const request = deferred()
  const { reader } = freshStores(1)

  await withAPI('get', () => request.promise, async () => {
    const loading = reader.loadProgress(101)
    activateUser(2, 'next')
    reader.ensureProgressScope()
    reader.applyProgress({
      bookId: 202,
      chapterIndex: 2,
      offset: 20,
      percent: 0.2,
      updatedAt: '2026-07-18T00:00:00Z',
    })

    request.resolve({
      data: {
        bookId: 101,
        chapterIndex: 8,
        offset: 88,
        percent: 0.8,
        updatedAt: '2026-07-18T01:00:00Z',
      },
    })
    const result = await loading

    assert.equal(result, null)
    assert.equal(reader.progressByBook[101], undefined)
    assert.equal(storage.getItem('openreader_chapter_progress@user:2@101'), null)
    assert.equal(reader.progressByBook[202]?.offset, 20)
  })
})

test('a forced network shelf refresh replaces future-dated confirmed client progress', { concurrency: false }, async () => {
  const { bookshelf, reader } = freshStores(1)
  const stale = {
    bookId: 7,
    chapterIndex: 2,
    offset: 20,
    percent: 0.2,
    chapterTitle: '旧客户端章节',
    updatedAt: '2099-07-22T00:00:00Z',
  }
  const authoritative = {
    bookId: 7,
    chapterIndex: 8,
    offset: 80,
    percent: 0.8,
    chapterTitle: '服务器最新章节',
    updatedAt: '2026-07-22T00:00:00Z',
  }
  reader.applyProgress(stale)
  bookshelf.books = [{ id: 7, title: '进度测试书', progress: stale }]

  await withAPI('get', async (path) => {
    assert.equal(path, '/books')
    return { data: [{ id: 7, title: '进度测试书', progress: authoritative }] }
  }, async () => {
    await bookshelf.loadBooks({ force: true, all: true })
  })

  assert.equal(bookshelf.books[0].progress.chapterIndex, 8)
  assert.equal(reader.progressByBook[7].chapterIndex, 8)
  assert.equal(
    JSON.parse(storage.getItem('openreader_chapter_progress@user:1@7')).chapterIndex,
    8,
  )
})

test('a forced network shelf refresh removes confirmed progress absent from the server', { concurrency: false }, async () => {
  const { bookshelf, reader } = freshStores(1)
  const stale = {
    bookId: 7,
    chapterIndex: 2,
    offset: 20,
    percent: 0.2,
    updatedAt: '2099-07-22T00:00:00Z',
  }
  reader.applyProgress(stale)
  bookshelf.books = [{ id: 7, title: '已清空进度', progress: stale }]

  await withAPI('get', async () => ({ data: [{ id: 7, title: '已清空进度' }] }), async () => {
    await bookshelf.loadBooks({ force: true, all: true })
  })

  assert.equal(bookshelf.books[0].progress, undefined)
  assert.equal(reader.progressByBook[7], undefined)
  assert.equal(storage.getItem('openreader_chapter_progress@user:1@7'), null)
})

test('a delayed category response cannot replace the next user shelf state', { concurrency: false }, async () => {
  const request = deferred()
  const { bookshelf } = freshStores(1)

  await withAPI('get', () => request.promise, async () => {
    const loading = bookshelf.loadCategories({ force: true })
    activateUser(2, 'next')
    bookshelf.resetShelfState()
    bookshelf.categories = [{ id: 202, name: '用户 B 分组' }]

    request.resolve({ data: [{ id: 101, name: '用户 A 分组' }] })
    await loading

    assert.deepEqual(bookshelf.categories, [{ id: 202, name: '用户 B 分组' }])
  })
})

test('a delayed unified book-group response cannot replace the next user shelf state', { concurrency: false }, async () => {
  const request = deferred()
  const { bookshelf } = freshStores(1)

  await withAPI('get', () => request.promise, async () => {
    const loading = bookshelf.loadBookGroups({ force: true })
    activateUser(2, 'next')
    bookshelf.resetShelfState()
    bookshelf.bookGroups = [{ key: 'builtin:all', name: '用户 B 全部' }]

    request.resolve({ data: [{ key: 'builtin:all', name: '用户 A 全部' }] })
    await loading

    assert.deepEqual(bookshelf.bookGroups, [{ key: 'builtin:all', name: '用户 B 全部' }])
  })
})

test('shelf preferences persist the stable selected book-group token', { concurrency: false }, () => {
  const { preferences } = freshStores(1)
  preferences.applyPreference('shelf', { view: 'list', layoutVersion: 2, groupKey: 'category:9' })
  assert.deepEqual(preferences.shelf, { view: 'grid', layoutVersion: 3, groupKey: 'category:9' })

  preferences.setShelfGroup('builtin:audio')
  assert.equal(preferences.shelf.groupKey, 'builtin:audio')
})

test('a delayed profile response cannot overwrite a later login profile', { concurrency: false }, async () => {
  const request = deferred()
  const tokenB = tokenFor(2, 'next')
  const { user } = freshStores(1)

  await withAPI('get', () => request.promise, async () => {
    const loading = user.loadMe()
    storage.setItem('openreader_token', tokenB)
    user.token = tokenB
    user.profile = { id: 2, username: 'user-b', canAccessStore: false }

    request.resolve({ data: { id: 1, username: 'user-a', canAccessStore: true } })
    await loading

    assert.deepEqual(user.profile, { id: 2, username: 'user-b', canAccessStore: false })
  })
})

test('an explicit rejected token can identify its user scope after storage removal', { concurrency: false }, () => {
  const rejectedToken = tokenFor(19, 'expired')
  storage.removeItem('openreader_token')

  assert.equal(currentUserScope(rejectedToken), 'user:19')
  assert.equal(currentUserScope('invalid-token'), 'anonymous')
})

test('session clearing invalidates the mounted reader before removing its token and resets account overlays', { concurrency: false }, async () => {
  const { overlay, user, workspace } = freshStores(1)
  workspace.beginSearch({ keyword: '用户 A 搜索', sourceId: 8, searchType: 'single' })
  workspace.replaceResultRows([{ title: '用户 A 结果', bookUrl: 'https://private.example/a' }])
  const bookmarkResult = overlay.openBookmarkForm(
    { id: 101, title: '用户 A 的书' },
    { chapterIndex: 3 },
  )
  const categoryResult = overlay.selectBookAddCategories([7])
  overlay.openBookInfo({ id: 101, title: '用户 A 的书' })
  overlay.openSearchBookContent({ id: 101, title: '用户 A 的书' })
  overlay.openStorageImport('local-store', ['user-a.epub'])

  const initialGeneration = user.sessionGeneration
  user.clearSession()

  const invalidation = dispatchedEvents.find(event => event.type === 'openreader:session-invalidated')
  assert.ok(invalidation)
  assert.match(invalidation.tokenAtDispatch, /^ey/)
  assert.equal(user.token, '')
  assert.equal(user.invalidatedScope, 'user:1')
  assert.equal(user.readerSessionBlocked, true)
  assert.equal(user.sessionGeneration, initialGeneration + 1)
  assert.equal(overlay.bookInfoVisible, false)
  assert.equal(overlay.bookInfoBook, null)
  assert.equal(overlay.searchBookContentVisible, false)
  assert.equal(overlay.searchBook, null)
  assert.equal(overlay.storageImportVisible, false)
  assert.equal(overlay.storageImportRequest, null)
  assert.equal(workspace.mode, 'shelf')
  assert.deepEqual(workspace.resultRows, [])
  assert.deepEqual(workspace.suspendedSession, {
    mode: 'search',
    search: {
      keyword: '用户 A 搜索',
      mode: 'remote',
      searchType: 'single',
      group: '',
      sourceId: 8,
      concurrent: 24,
    },
  })
  assert.deepEqual(await bookmarkResult, { saved: false, reason: 'session-invalidated' })
  assert.equal(await categoryResult, null)
})

test('the real interceptor order preserves same-account reauthentication after storage removal', { concurrency: false }, async () => {
  const { user } = freshStores(13)
  const rejectedToken = user.token
  storage.removeItem('openreader_token')

  user.requireLogin('session', rejectedToken)
  assert.equal(user.invalidatedScope, 'user:13')

  await withAPI('post', async () => ({
    data: {
      token: tokenFor(13, 'renewed'),
      user: { id: 13, username: 'user-thirteen' },
    },
  }), async () => {
    const result = await user.login('user-thirteen', 'password')
    assert.deepEqual(result, {
      previousScope: 'user:13',
      currentScope: 'user:13',
      sameAuthenticatedScope: true,
    })
    assert.equal(JSON.stringify(result).includes(rejectedToken), false)
    assert.equal(JSON.stringify(result).includes(tokenFor(13, 'renewed')), false)
  })
})

test('a pending startup 401 opens reauthentication once after localStorage has already dropped the token', { concurrency: false }, () => {
  const rejectedToken = tokenFor(5, 'expired')
  const { user } = freshStores(5)
  user.token = ''
  storage.removeItem('openreader_token')
  window.__openreaderAuthRequired = { reason: 'session', rejectedToken }

  user.requireLogin('session', rejectedToken)
  const generation = user.sessionGeneration
  assert.equal(user.authDialogVisible, true)
  assert.equal(user.readerSessionBlocked, true)
  assert.equal(user.invalidatedScope, 'user:5')

  user.requireLogin('session', rejectedToken)
  assert.equal(user.sessionGeneration, generation)
  delete window.__openreaderAuthRequired
})

test('reauthentication keeps the reader blocked until same-account or account-switch routing is settled', { concurrency: false }, async () => {
  const { user } = freshStores(1)
  user.clearSession()
  const invalidatedGeneration = user.sessionGeneration

  await withAPI('post', async () => ({
    data: {
      token: tokenFor(2, 'login'),
      user: { id: 2, username: 'user-b' },
    },
  }), async () => {
    const result = await user.login('user-b', 'password')
    assert.equal(result.sameAuthenticatedScope, false)
    assert.equal(result.previousScope, 'user:1')
    assert.equal(result.currentScope, 'user:2')
    assert.equal(user.readerSessionBlocked, true)
    assert.equal(user.sessionGeneration, invalidatedGeneration + 1)
  })

  user.completeReauthentication()
  assert.equal(user.readerSessionBlocked, false)
})

test('same-account reauthentication is identified without exposing either token', { concurrency: false }, async () => {
  const { user, workspace } = freshStores(7)
  workspace.beginSearch({ keyword: '同账号恢复', sourceId: 3, searchType: 'single' })
  workspace.replaceResultRows([{ title: '必须丢弃的旧结果' }])
  user.requireLogin('session')

  await withAPI('post', async () => ({
    data: {
      token: tokenFor(7, 'renewed'),
      user: { id: 7, username: 'user-seven' },
    },
  }), async () => {
    const result = await user.login('user-seven', 'password')
    assert.deepEqual({
      sameAuthenticatedScope: result.sameAuthenticatedScope,
      previousScope: result.previousScope,
      currentScope: result.currentScope,
    }, {
      sameAuthenticatedScope: true,
      previousScope: 'user:7',
      currentScope: 'user:7',
    })
    assert.equal(JSON.stringify(result).includes(tokenFor(7, 'initial')), false)
    assert.equal(user.readerSessionBlocked, true)
    assert.equal(workspace.mode, 'search')
    assert.equal(workspace.search.keyword, '同账号恢复')
    assert.deepEqual(workspace.resultRows, [])
    assert.equal(workspace.suspendedSession, null)
  })
})

test('different-account reauthentication discards the suspended Index scene', { concurrency: false }, async () => {
  const { user, workspace } = freshStores(7)
  workspace.showExploreResults([{ title: '用户 A 探索结果' }], {
    sourceId: 77,
    sourceName: '用户 A 来源',
    url: 'https://private.example/explore',
    name: '用户 A 入口',
  })
  user.requireLogin('session')

  await withAPI('post', async () => ({
    data: {
      token: tokenFor(8, 'renewed'),
      user: { id: 8, username: 'user-eight' },
    },
  }), async () => {
    const result = await user.login('user-eight', 'password')
    assert.equal(result.sameAuthenticatedScope, false)
    assert.equal(workspace.mode, 'shelf')
    assert.deepEqual(workspace.resultRows, [])
    assert.deepEqual(workspace.explore, {
      sourceId: '',
      sourceGroup: '',
      url: '',
      name: '',
      sourceName: '',
    })
    assert.equal(workspace.suspendedSession, null)
  })
})

test('explicit logout never restores an old Index scene after the same user logs in again', { concurrency: false }, async () => {
  const { user, workspace } = freshStores(11)
  workspace.beginSearch({ keyword: '退出前搜索', sourceId: 5, searchType: 'single' })
  user.logout()

  await withAPI('post', async () => ({
    data: {
      token: tokenFor(11, 'again'),
      user: { id: 11, username: 'user-eleven' },
    },
  }), async () => {
    const result = await user.login('user-eleven', 'password')
    assert.equal(result.sameAuthenticatedScope, true)
    assert.equal(workspace.mode, 'shelf')
    assert.equal(workspace.search.keyword, '')
    assert.equal(workspace.suspendedSession, null)
  })
})

test('callbacks from a superseded websocket cannot close, clear, reconnect, or dispatch into the new session', { concurrency: false }, async () => {
  class FakeWebSocket {
    static OPEN = 1
    static instances = []

    constructor(url) {
      this.url = url
      this.readyState = 0
      this.closeCalls = 0
      this.listeners = new Map()
      FakeWebSocket.instances.push(this)
    }

    addEventListener(type, listener) {
      const listeners = this.listeners.get(type) || []
      listeners.push(listener)
      this.listeners.set(type, listeners)
    }

    close() {
      this.closeCalls += 1
      this.readyState = 3
    }

    send() {}

    emit(type, value = {}) {
      for (const listener of this.listeners.get(type) || []) listener(value)
    }
  }

  globalThis.WebSocket = FakeWebSocket
  window.WebSocket = FakeWebSocket
  const { bookshelf } = freshStores(1)
  bookshelf.loadBooks = async () => []
  bookshelf.loadCategories = async () => []
  bookshelf.loadBookGroups = async () => []
  const sync = useSync()

  assert.equal(sync.send, undefined, 'the sync transport must not expose an unaudited client-to-server event path')

  sync.connect()
  const socketA = FakeWebSocket.instances[0]
  socketA.readyState = FakeWebSocket.OPEN
  socketA.emit('open')
  sync.disconnect()

  activateUser(2, 'next')
  bookshelf.resetShelfState()
  bookshelf.categories = [{ id: 202, name: '用户 B 分组' }]
  sync.connect()
  const socketB = FakeWebSocket.instances[1]
  socketB.readyState = FakeWebSocket.OPEN
  socketB.emit('open')

  socketA.emit('error')
  socketA.emit('close')
  socketA.emit('message', {
    data: JSON.stringify({ type: 'category_update', payload: { id: 101, name: '用户 A 分组' } }),
  })
  for (const callback of [...timerCallbacks.values()]) callback()

  assert.deepEqual({
    socketBCloseCalls: socketB.closeCalls,
    connected: sync.connected.value,
    socketCount: FakeWebSocket.instances.length,
    categories: bookshelf.categories,
  }, {
    socketBCloseCalls: 0,
    connected: true,
    socketCount: 2,
    categories: [{ id: 202, name: '用户 B 分组' }],
  })

  sync.disconnect()
})
