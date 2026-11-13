import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test, { after } from 'node:test'
import { effectScope, nextTick, reactive, ref } from 'vue'
import { createServer } from 'vite'
import * as readerModeModule from '../src/composables/useReaderMode.js'

const { readerEffectiveMode, useReaderMode } = readerModeModule

// Contract: docs/compat/reader-desktop-flip-account-fixed-baseline-second-audit-p1-contract.md
// reader-dev derives isSlideRead (miniInterface && readMethod === '左右滑动') and never rewrites readMethod when
// the interface changes. A desktop Reader renders the account's flip as page and leaves the setting alone; the
// code that read the raw setting reads the interface mode instead.

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

const readerSource = readFileSync(new URL('../src/views/Reader.vue', import.meta.url), 'utf8')

const delay = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds))

async function flushModeChange() {
  await nextTick()
  await Promise.resolve()
  await nextTick()
}

function tokenFor(userId) {
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url')
  const payload = Buffer.from(JSON.stringify({ userId })).toString('base64url')
  return `${header}.${payload}.flip-account-${userId}`
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

function modeOptions(reader, isMobileReader, calls) {
  return {
    reader,
    isMobileReader,
    isContinuousScrollRead: ref(false),
    getEffectiveMode: () => readerEffectiveMode(reader.mode, false, false, false, false, false, !isMobileReader.value),
    page: ref(0),
    chapterLoading: ref(false),
    chapterBlocks: ref([]),
    currentIndex: ref(0),
    chapter: ref({ id: 1, title: '第一章' }),
    content: ref('正文'),
    capturePosition: state => {
      calls.push(['capture', state])
      return { chapterIndex: 0, paragraphPos: 240 }
    },
    restoreCapturedPosition: async (_snapshot, transition) => {
      calls.push(['restore', transition])
      return true
    },
    getCurrentOffset: () => 0,
    computeChapterWindow: async () => {},
    makeChapterBlock: () => ({ index: 0 }),
    updateLayout: () => calls.push(['layout']),
    restorePosition: async () => calls.push(['legacy-restore']),
    saveProgress: () => calls.push(['save']),
  }
}

// A Reader whose setting starts as `mode`, in the mini interface (`mobile`) or on the desktop.
function mountMode({ mobile, mode = 'flip' }) {
  const calls = []
  const isMobileReader = ref(mobile)
  const reader = reactive({
    mode,
    fontFamily: 'system',
    chineseFont: '简体',
    fontSize: 18,
    fontWeight: 400,
    lineHeight: 1.8,
    paragraphSpace: 0.2,
    columnWidth: 800,
    setMode(next, options) {
      calls.push(['set-mode', next, options ?? null])
      this.mode = next
    },
  })
  const options = modeOptions(reader, isMobileReader, calls)
  const scope = effectScope()
  scope.run(() => useReaderMode(options))
  return { calls, effectiveMode: options.getEffectiveMode, isMobileReader, reader, scope }
}

test('the interface mode renders a desktop flip as page and leaves every other setting alone', () => {
  const { readerInterfaceMode } = readerModeModule
  assert.equal(typeof readerInterfaceMode, 'function', 'useReaderMode.js must export the interface mode')
  assert.equal(readerInterfaceMode('flip', true), 'page')
  assert.equal(readerInterfaceMode('flip', false), 'flip')
  assert.equal(readerInterfaceMode('flip'), 'flip', 'callers that do not name the interface keep flip')
  for (const mode of ['page', 'scroll', 'scroll2']) {
    assert.equal(readerInterfaceMode(mode, true), mode)
    assert.equal(readerInterfaceMode(mode, false), mode)
  }
})

test('a desktop Reader that sets up on a flip setting renders page and leaves the setting alone', async () => {
  const { calls, effectiveMode, reader, scope } = mountMode({ mobile: false })
  await flushModeChange()

  assert.equal(reader.mode, 'flip', 'the desktop must not rewrite the setting')
  assert.equal(effectiveMode(), 'page')
  assert.deepEqual(calls, [], 'setting up needs no setMode and no position transaction')
  scope.stop()
})

test('mini -> desktop -> mini keeps the flip setting and slides again', async () => {
  const { calls, effectiveMode, isMobileReader, reader, scope } = mountMode({ mobile: true })
  await flushModeChange()
  assert.equal(effectiveMode(), 'flip', 'precondition: the mini interface slides')
  assert.deepEqual(calls, [])

  // iPad 手机模式 -> 自适应.
  isMobileReader.value = false
  await flushModeChange()
  await flushModeChange()
  assert.equal(reader.mode, 'flip', 'switching to the desktop must not rewrite the setting')
  assert.equal(effectiveMode(), 'page')

  // 自适应 -> 手机模式.
  isMobileReader.value = true
  await flushModeChange()
  await flushModeChange()
  assert.equal(reader.mode, 'flip')
  assert.equal(effectiveMode(), 'flip', 'back in the mini interface the book slides again')
  assert.equal(calls.some(call => call[0] === 'set-mode'), false, 'no interface switch may call setMode')
  assert.deepEqual(calls.filter(call => call[0] === 'capture'), [
    ['capture', { mode: 'flip', mobile: true }],
    ['capture', { mode: 'page', mobile: false }],
  ])
  assert.deepEqual(calls.filter(call => call[0] === 'restore').map(call => call[1]), [
    { fromMode: 'flip', toMode: 'page', fromMobile: true, toMobile: false },
    { fromMode: 'page', toMode: 'flip', fromMobile: false, toMobile: true },
  ])
  scope.stop()
})

test('the account keeps flip through desktop setup, interface switches and a later desktop save', { concurrency: false }, async () => {
  const reader = freshReader()
  const saved = []
  await withAPI('put', async (path, body) => {
    saved.push({ path, mode: body.value?.mode, fontSize: body.value?.fontSize })
    return { data: { value: body.value, updatedAt: '2026-10-30T00:00:01Z' }, headers: {} }
  }, async () => {
    reader.applyReaderSettings({ ...reader.$state, mode: 'flip', autoTheme: false }, '2026-10-30T00:00:00Z')
    assert.equal(reader.mode, 'flip', 'precondition: the account settings say flip')
    const updatedAt = reader.settingsUpdatedAt
    const schemes = JSON.stringify(reader.customConfigList)

    const isMobileReader = ref(false)
    const scope = effectScope()
    scope.run(() => useReaderMode(modeOptions(reader, isMobileReader, [])))
    await flushModeChange()
    isMobileReader.value = true
    await flushModeChange()
    isMobileReader.value = false
    await flushModeChange()

    assert.equal(reader.mode, 'flip', 'the desktop must not rewrite the account setting')
    assert.equal(reader.settingsUpdatedAt, updatedAt, 'the desktop must not look newer than the account')
    assert.equal(JSON.stringify(reader.customConfigList), schemes, 'the desktop must not rewrite the active scheme')
    await delay(800)
    assert.deepEqual(saved, [], 'opening the Reader and switching the interface must not save the settings')

    // The desktop's own edit is saved with the account's flip.
    reader.setFontSize(reader.fontSize + 1)
    await delay(800)
    assert.deepEqual(saved, [{ path: '/settings/reader', mode: 'flip', fontSize: 19 }])
    scope.stop()
  })
})

test('a progress save records the mode its payload names and the setting otherwise', { concurrency: false }, async () => {
  const reader = freshReader()
  const writes = []
  await withAPI('put', async (path, body) => {
    writes.push({ path, mode: body.mode })
    return { data: { ...body, updatedAt: `2026-10-30T00:00:0${writes.length}Z` }, headers: {} }
  }, async () => {
    reader.applyReaderSettings({ ...reader.$state, mode: 'flip', autoTheme: false }, '2026-10-30T00:00:00Z')
    const payload = {
      bookId: 7,
      chapterId: 12,
      chapterIndex: 2,
      offset: 120,
      percent: 0.24,
      chapterPercent: 0.4,
      chapterTitle: '第三章',
    }

    // The desktop measured this position on its page layout.
    await reader.saveProgress({ ...payload, mode: 'page' })
    assert.deepEqual(writes, [{ path: '/progress', mode: 'page' }])
    assert.equal(reader.progressByBook[7]?.mode, 'page')

    // Control: a payload without a mode keeps the setting.
    await reader.saveProgress({ ...payload, offset: 160 })
    assert.deepEqual(writes[1], { path: '/progress', mode: 'flip' })
  })
})

test('the Reader feeds the interface mode to the code that read the raw setting', () => {
  // assert.ok keeps a failure from printing the whole Reader source.
  assert.ok(
    /const interfaceReaderMode = computed\(\(\) => readerInterfaceMode\(reader\.mode, !isMobileReader\.value\)\)/.test(readerSource),
    'Reader.vue must derive interfaceReaderMode from the setting and the interface',
  )
  for (const composable of ['useReaderLocalProgress', 'useReaderChapterWindow', 'useReaderPositionRestore']) {
    const call = readerSource.match(new RegExp(`\\} = ${composable}\\(\\{([\\s\\S]*?)\\n\\}\\)`))
    assert.ok(call, `${composable} call is missing`)
    assert.ok(/\n {2}getMode: \(\) => interfaceReaderMode\.value,/.test(call[1]), `${composable} must get the interface mode`)
  }
  assert.ok(
    /saveRemote: payload => reader\.saveProgress\(\{ \.\.\.payload, mode: interfaceReaderMode\.value \}\)/.test(readerSource),
    'progress saves must record the interface mode',
  )
  const closeTTSBar = readerSource.match(/function closeTTSBar\(\) \{([\s\S]*?)\n\}/)
  assert.ok(closeTTSBar, 'closeTTSBar is missing')
  assert.ok(/interfaceReaderMode\.value !== 'flip'/.test(closeTTSBar[1]), 'closeTTSBar must check the interface mode')
  assert.deepEqual(readerSource.match(/reader\.mode\s*[!=]==/g) || [], [], 'no Reader code may compare the raw setting')

  for (const file of ['useReaderChapterWindow.js', 'useReaderPositionRestore.js', 'useReaderLocalProgress.js']) {
    const source = readFileSync(new URL(`../src/composables/${file}`, import.meta.url), 'utf8')
    assert.deepEqual(source.match(/options\.reader\.mode/g) || [], [], `${file} must read the mode it is given, not the raw setting`)
  }
})
