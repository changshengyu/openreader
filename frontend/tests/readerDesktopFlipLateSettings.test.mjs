import assert from 'node:assert/strict'
import test, { after } from 'node:test'
import { effectScope, nextTick, reactive, ref } from 'vue'
import { createServer } from 'vite'
import {
  readerEffectiveMode,
  useReaderMode,
} from '../src/composables/useReaderMode.js'

// Contract: docs/compat/reader-desktop-flip-late-settings-fixed-baseline-second-audit-p0-contract.md
// reader-dev only slides in its mini interface (vuex isSlideRead). A flip
// setting that reaches a desktop Reader after setup (the account settings,
// another device, a scheme) renders as page without a position transaction,
// and the desktop writes nothing back to the account.

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

const storage = new MemoryStorage()
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
      matches: false,
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
const { useReaderStore } = await vite.ssrLoadModule('/src/stores/reader.js')

const delay = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds))

async function flushModeChange() {
  await nextTick()
  await Promise.resolve()
  await nextTick()
}

function tokenFor(userId) {
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url')
  const payload = Buffer.from(JSON.stringify({ userId })).toString('base64url')
  return `${header}.${payload}.late-flip-${userId}`
}

function freshReader() {
  storage.clear()
  storage.setItem('openreader_token', tokenFor(1))
  setActivePinia(createPinia())
  const reader = useReaderStore()
  reader.resetReaderSettingsState()
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

// A Reader in the mini interface (`mobile`) or on the desktop whose position transaction records every step.
function mountMode({ mobile }) {
  const calls = []
  const isMobileReader = ref(mobile)
  const reader = reactive({
    mode: 'page',
    fontFamily: 'system',
    chineseFont: '简体',
    fontSize: 18,
    fontWeight: 400,
    lineHeight: 1.8,
    paragraphSpace: 0.2,
    columnWidth: 800,
    setMode(mode, options) {
      calls.push(['set-mode', mode, options ?? null])
      this.mode = mode
    },
  })
  const scope = effectScope()
  scope.run(() => useReaderMode({
    reader,
    isMobileReader,
    isContinuousScrollRead: ref(false),
    getEffectiveMode: () => readerEffectiveMode(reader.mode, false, false, false, false, false, !isMobileReader.value),
    page: ref(2),
    chapterLoading: ref(false),
    chapterBlocks: ref([]),
    currentIndex: ref(0),
    chapter: ref({ id: 1, title: '第一章' }),
    content: ref('正文'),
    capturePosition: state => {
      calls.push(['capture', state])
      return { chapterIndex: 0, paragraphPos: 240 }
    },
    restoreCapturedPosition: async () => {
      calls.push(['restore'])
      return true
    },
    getCurrentOffset: () => 0,
    computeChapterWindow: async () => {},
    makeChapterBlock: () => ({ index: 0 }),
    updateLayout: () => calls.push(['layout']),
    restorePosition: async () => calls.push(['legacy-restore']),
    saveProgress: () => calls.push(['save']),
  }))
  return { calls, isMobileReader, reader, scope }
}

test('the desktop renders a flip setting as the page branch, the mini interface keeps it', () => {
  assert.equal(readerEffectiveMode('flip', false, false, false, false, false, true), 'page')
  assert.equal(readerEffectiveMode('flip', false, false, false, false, false, false), 'flip')
  assert.equal(readerEffectiveMode('flip', false), 'flip', 'callers that do not name the interface keep flip')
  for (const mode of ['page', 'scroll', 'scroll2']) {
    assert.equal(readerEffectiveMode(mode, false, false, false, false, false, true), mode)
  }
})

// The desktop leaves the setting as it is (docs/compat/reader-desktop-flip-account-fixed-baseline-second-audit-p1-contract.md).
test('a flip setting that reaches the desktop after setup renders as page without touching the setting or running a position transaction', async () => {
  const { calls, reader, scope } = mountMode({ mobile: false })
  await flushModeChange()
  assert.deepEqual(calls, [], 'a page setting needs nothing at setup')

  // The account settings, another device's settings_update or a scheme apply the whole setting at once.
  Object.assign(reader, { mode: 'flip' })
  await flushModeChange()

  assert.equal(reader.mode, 'flip', 'the desktop must not rewrite the setting')
  assert.deepEqual(calls, [], 'a late flip needs no setMode and no position transaction')
  scope.stop()
})

test('control: the mini interface keeps a late flip setting and runs its one position transaction', async () => {
  const { calls, reader, scope } = mountMode({ mobile: true })
  await flushModeChange()

  Object.assign(reader, { mode: 'flip' })
  await flushModeChange()

  assert.equal(reader.mode, 'flip')
  assert.equal(calls.some(call => call[0] === 'set-mode'), false)
  assert.deepEqual(calls.filter(call => call[0] === 'capture'), [['capture', { mode: 'page', mobile: true }]])
  assert.equal(calls.filter(call => call[0] === 'restore').length, 1)
  scope.stop()
})

test('control: a late flip that arrives with the mini interface in the same update stays flip', async () => {
  const { calls, isMobileReader, reader, scope } = mountMode({ mobile: false })
  await flushModeChange()

  // Settings that also switch 页面模式 to 手机模式 assign mode before pageMode.
  reader.mode = 'flip'
  isMobileReader.value = true
  await flushModeChange()

  assert.equal(reader.mode, 'flip')
  assert.equal(calls.some(call => call[0] === 'set-mode'), false)
  scope.stop()
})

test('a device-local mode change is not a settings change and is never written to the account', { concurrency: false }, async () => {
  const reader = freshReader()
  const saved = []
  await withAPI('put', async (_path, body) => {
    saved.push(body.value.mode)
    return { data: { value: body.value, updatedAt: '2026-10-30T00:00:01Z' }, headers: {} }
  }, async () => {
    reader.applyReaderSettings({ ...reader.$state, mode: 'flip', autoTheme: false }, '2026-10-30T00:00:00Z')
    assert.equal(reader.mode, 'flip', 'precondition: the account settings say flip')
    const updatedAt = reader.settingsUpdatedAt
    const schemes = JSON.stringify(reader.customConfigList)

    reader.setMode('page', { deviceLocal: true })
    assert.equal(reader.mode, 'page')
    assert.equal(reader.settingsUpdatedAt, updatedAt, 'a device-local mode change must not look newer than the account')
    assert.equal(JSON.stringify(reader.customConfigList), schemes, 'a device-local mode change must not rewrite the active scheme')
    await delay(800)
    assert.deepEqual(saved, [], 'a device-local mode change must not be written to the account')

    // Control: the user's own mode choice is saved.
    reader.setMode('scroll')
    assert.notEqual(reader.settingsUpdatedAt, updatedAt)
    await delay(800)
    assert.deepEqual(saved, ['scroll'])
  })
})
