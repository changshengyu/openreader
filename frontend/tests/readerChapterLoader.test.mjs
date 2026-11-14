import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderChapterLoader } from '../src/composables/useReaderChapterLoader.js'

function createController(overrides = {}) {
  const calls = []
  const state = {
    chapters: ref([{ id: 1 }, { id: 2 }, { id: 3 }]),
    currentIndex: ref(0),
    mobileChromeVisible: ref(true),
    restoringPosition: ref(false),
    chapterLoaded: ref(true),
    chapterLoadError: ref('old error'),
    chapterLoading: ref(false),
    chapter: ref(null),
    content: ref(''),
    cachedImages: ref({}),
    chapterFormat: ref('text'),
    epubResource: ref(null),
    audioResource: ref(null),
    page: ref(4),
    chapterBlocks: ref([]),
    progressVersion: ref(0),
    isContinuousScrollRead: ref(false),
  }
  const currentProgress = { bookId: 7, chapterIndex: 2, offset: 80 }
  const controller = useReaderChapterLoader({
    ...state,
    cancelProgressSave: () => calls.push(['cancel']),
    getMemoryContent: () => null,
    loadContent: async (index, options) => {
      const { signal: _signal, ...recordedOptions } = options
      calls.push(['load', index, recordedOptions])
      return {
        chapter: { id: index + 1, title: `第 ${index + 1} 章` },
        content: `正文 ${index}`,
        format: 'text',
      }
    },
    makeChapterBlock: (index, chapter, content) => ({ index, id: chapter.id, content }),
    updateLayout: () => calls.push(['layout']),
    restorePosition: async (...args) => calls.push(['restore', ...args]),
    preloadNearby: index => calls.push(['preload', index]),
    saveProgress: async options => calls.push(['save', options]),
    markProgressSaved: progress => calls.push(['mark', progress]),
    getCurrentProgress: () => currentProgress,
    computeChapterWindow: async options => calls.push(['window', options]),
    formatError: error => `加载失败：${error.message}`,
    nextFrame: async () => calls.push(['frame']),
    ...overrides,
  })
  return { calls, controller, currentProgress, state }
}

test('loads a clamped chapter and marks its restored progress', async () => {
  const fixture = createController()
  await fixture.controller.load(9, 120, { restorePercent: 0.5 })
  assert.equal(fixture.state.mobileChromeVisible.value, true)
  assert.equal(fixture.state.currentIndex.value, 2)
  assert.equal(fixture.state.chapter.value.id, 3)
  assert.equal(fixture.state.content.value, '正文 2')
  assert.deepEqual(fixture.state.chapterBlocks.value, [{ index: 2, id: 3, content: '正文 2' }])
  assert.equal(fixture.state.page.value, 0)
  assert.equal(fixture.state.progressVersion.value, 1)
  assert.equal(fixture.state.chapterLoaded.value, true)
  assert.equal(fixture.state.restoringPosition.value, false)
  assert.deepEqual(fixture.calls, [
    ['cancel'],
    ['load', 2, { refresh: false }],
    ['layout'],
    ['restore', 120, { restorePercent: 0.5 }],
    ['preload', 2],
    ['mark', fixture.currentProgress],
    ['frame'],
  ])
})

test('hides mobile chrome only when the caller explicitly requests it', async () => {
  const fixture = createController()
  await fixture.controller.load(1, 0, { hideChrome: true })
  assert.equal(fixture.state.mobileChromeVisible.value, false)
})

test('saves forced progress and expands continuous chapter windows', async () => {
  const fixture = createController()
  fixture.state.isContinuousScrollRead.value = true
  await fixture.controller.load(1, 0, { refresh: true, saveAfterLoad: true })
  assert.deepEqual(fixture.calls, [
    ['cancel'],
    ['load', 1, { refresh: true }],
    ['layout'],
    ['restore', 0, { refresh: true, saveAfterLoad: true }],
    ['preload', 1],
    ['save', { force: true }],
    ['window', { anchorIndex: 1 }],
    ['frame'],
  ])
})

// The calls used to be [cancel, frame]: a failure kept the previous chapter's page count. Error surface contract R8
// (section 13 item 9) rewrites the sequence: the error page itself is measured once before the guards are released.
test('records load failures, measures the error page and always releases loading guards', async () => {
  const fixture = createController({
    loadContent: async () => {
      throw new Error('network error')
    },
  })
  await fixture.controller.load(1)
  assert.equal(fixture.state.chapterLoadError.value, '加载失败：network error')
  assert.equal(fixture.state.chapterLoaded.value, false)
  assert.equal(fixture.state.chapterLoading.value, false)
  assert.equal(fixture.state.restoringPosition.value, false)
  assert.equal(fixture.state.page.value, 0, 'the error page starts on its first page')
  assert.deepEqual(fixture.calls, [
    ['cancel'],
    ['layout'],
    ['frame'],
  ])
})

test('reports every failed current load once, including a failed retry', async () => {
  const reported = []
  const fixture = createController({
    loadContent: async () => {
      throw new Error('network error')
    },
    notifyError: error => reported.push(error.message),
  })
  await fixture.controller.load(1)
  assert.deepEqual(reported, ['network error'])
  await fixture.controller.load(1, 0, { refresh: true })
  assert.deepEqual(reported, ['network error', 'network error'])
})

test('does not report aborted or superseded loads', async () => {
  const reported = []
  const pending = new Map()
  const fixture = createController({
    loadContent: (index, options) => new Promise((resolve, reject) => {
      pending.set(index, { resolve, reject, signal: options.signal })
    }),
    notifyError: error => reported.push(error.message),
  })
  const first = fixture.controller.load(0)
  const second = fixture.controller.load(1)
  const aborted = new Error('chapter request cancelled')
  aborted.name = 'AbortError'
  pending.get(0).reject(aborted)
  assert.equal(await first, false)
  const third = fixture.controller.load(2)
  pending.get(1).reject(new Error('superseded failure'))
  assert.equal(await second, false)
  assert.deepEqual(reported, [], 'aborted and superseded loads must stay silent')
  pending.get(2).reject(new Error('current failure'))
  assert.equal(await third, false)
  assert.deepEqual(reported, ['current failure'])
  assert.equal(fixture.state.chapterLoadError.value, '加载失败：current failure')
})

test('keeps a failed continuous target chapter in the chapter window', async () => {
  const fixture = createController({
    loadContent: async () => {
      throw new Error('network error')
    },
    showFailedChapterWindow: (index, error) => fixture.calls.push(['failed-window', index, error.message]),
  })
  fixture.state.isContinuousScrollRead.value = true
  await fixture.controller.load(2)
  assert.equal(fixture.state.chapterLoadError.value, '加载失败：network error')
  assert.deepEqual(
    fixture.calls.filter(call => call[0] === 'failed-window'),
    [['failed-window', 2, 'network error']],
  )

  const single = createController({
    loadContent: async () => {
      throw new Error('network error')
    },
    showFailedChapterWindow: (index, error) => single.calls.push(['failed-window', index, error.message]),
  })
  await single.controller.load(2)
  assert.deepEqual(single.calls.filter(call => call[0] === 'failed-window'), [], 'single-chapter modes show the error page')
})

test('discards an older main load that finishes after the active chapter', async () => {
  const pending = new Map()
  const fixture = createController({
    loadContent: index => new Promise(resolve => {
      pending.set(index, resolve)
    }),
  })

  const first = fixture.controller.load(0)
  const second = fixture.controller.load(1)
  pending.get(1)(validLoaderContent(1))
  await second
  pending.get(0)(validLoaderContent(0))
  await first

  assert.equal(fixture.state.currentIndex.value, 1)
  assert.equal(fixture.state.chapter.value.id, 2)
  assert.equal(fixture.state.content.value, '正文 1')
  assert.deepEqual(
    fixture.calls.filter(call => ['layout', 'restore', 'preload', 'mark'].includes(call[0])),
    [
      ['layout'],
      ['restore', 0, {}],
      ['preload', 1],
      ['mark', fixture.currentProgress],
    ],
  )
})

test('aborts the previous main request when another chapter starts loading', async () => {
  const signals = []
  const pending = new Map()
  const fixture = createController({
    loadContent: (index, options) => {
      signals.push(options.signal)
      return new Promise(resolve => {
        pending.set(index, resolve)
      })
    },
  })

  const first = fixture.controller.load(0)
  const second = fixture.controller.load(1)
  assert.ok(signals[0] instanceof AbortSignal)
  assert.equal(signals[0].aborted, true)
  assert.equal(signals[1].aborted, false)
  pending.get(1)(validLoaderContent(1))
  assert.equal(await second, true)
  pending.get(0)(validLoaderContent(0))
  assert.equal(await first, false)
})

test('keeps EPUB document metadata out of the ordinary paragraph renderer', async () => {
  const fixture = createController({
    loadContent: async () => ({
      chapter: { id: 2, title: 'EPUB 第二章', resourcePath: 'OPS/two.xhtml' },
      content: '可搜索纯文本',
      format: 'epub',
      resourceUrl: '/api/epub-resource/token/OPS/two.xhtml',
      resourceExpiresAt: '2026-07-06T12:00:00Z',
    }),
  })
  await fixture.controller.load(1, 88, { restorePercent: 0.4 })
  assert.equal(fixture.state.chapterFormat.value, 'epub')
  assert.deepEqual(fixture.state.epubResource.value, {
    url: '/api/epub-resource/token/OPS/two.xhtml',
    expiresAt: '2026-07-06T12:00:00Z',
  })
  assert.deepEqual(fixture.state.chapterBlocks.value, [])
  assert.equal(fixture.state.content.value, '可搜索纯文本')
})

test('keeps audio chapter responses out of the ordinary paragraph renderer', async () => {
  const fixture = createController({
    loadContent: async () => ({
      chapter: { id: 4, title: '第一集' },
      content: 'https://audio.example.test/001.mp3',
      format: 'audio',
      resourceUrl: 'https://audio.example.test/001.mp3',
      resourceExpiresAt: '2026-07-06T12:00:00Z',
    }),
    onAudioPrepared: pending => fixture.calls.push(['audio', pending]),
  })
  await fixture.controller.load(0, 37, { restorePercent: 0.25 })
  assert.equal(fixture.state.chapterFormat.value, 'audio')
  assert.deepEqual(fixture.state.audioResource.value, {
    url: 'https://audio.example.test/001.mp3',
    expiresAt: '2026-07-06T12:00:00Z',
    title: '第一集',
  })
  assert.deepEqual(fixture.state.chapterBlocks.value, [])
  assert.equal(fixture.state.content.value, 'https://audio.example.test/001.mp3')
  assert.deepEqual(fixture.calls.find(call => call[0] === 'audio'), [
    'audio',
    {
      chapterIndex: 0,
      offset: 37,
      restoreOptions: { restorePercent: 0.25 },
    },
  ])
})

test('renders CBZ image chapter responses through the ordinary image block path', async () => {
  const fixture = createController({
    loadContent: async () => ({
      chapter: { id: 3, title: 'pages/001.jpg', resourcePath: 'pages/001.jpg' },
      content: '<img src="/api/cbz-resource/token/pages/001.jpg" />',
      format: 'cbz',
      resourceUrl: '/api/cbz-resource/token/pages/001.jpg',
      resourceExpiresAt: '2026-07-06T12:00:00Z',
    }),
  })
  await fixture.controller.load(0)
  assert.equal(fixture.state.chapterFormat.value, 'text')
  assert.equal(fixture.state.epubResource.value, null)
  assert.deepEqual(fixture.state.chapterBlocks.value, [{
    index: 0,
    id: 3,
    content: '<img src="/api/cbz-resource/token/pages/001.jpg" />',
  }])
})

test('stores cached image mappings and passes them to presentation after loading text', async () => {
  const remote = 'https://cdn.example.test/one.png'
  const capability = '/api/chapter-image/signed.capability'
  let presented
  const fixture = createController({
    loadContent: async () => ({
      chapter: { id: 1, title: '第一章' },
      content: `<img src="${remote}">`,
      format: 'text',
      cachedImages: { [remote]: capability },
      cachedImagesExpiresAt: '2026-07-20T00:00:00Z',
    }),
    makeChapterBlock: (index, chapter, content, cachedImages) => {
      presented = { index, chapter, content, cachedImages }
      return presented
    },
  })

  await fixture.controller.load(0)
  assert.deepEqual(fixture.state.cachedImages.value, { [remote]: capability })
  assert.deepEqual(presented.cachedImages, { [remote]: capability })
})

function validLoaderContent(index) {
  return {
    chapter: { id: index + 1, title: `第 ${index + 1} 章` },
    content: `正文 ${index}`,
    format: 'text',
  }
}

// docs/compat/reader-failure-progress-payload-fixed-baseline-second-audit-p0-contract.md P2 (D1-A): reader-dev saves the
// requested chapter on the server before fetching its content, so a failed navigation still moves the progress there.
test('saves the failed target chapter once after a failed navigation (P2)', async () => {
  const fixture = createController({
    loadContent: async () => {
      throw new Error('source down')
    },
  })
  assert.equal(await fixture.controller.load(1, 0, { saveAfterLoad: true }), false)
  assert.deepEqual(fixture.calls.map(call => call[0]), ['cancel', 'layout', 'save', 'frame'])
  assert.deepEqual(fixture.calls.filter(call => call[0] === 'save'), [['save', { force: true }]])
})

test('saves a failed continuous target only after its chapter window settles (P2)', async () => {
  let settle = () => {}
  const fixture = createController({
    loadContent: async () => {
      throw new Error('source down')
    },
    showFailedChapterWindow: () => new Promise((resolve) => {
      settle = () => {
        fixture.calls.push(['window-settled'])
        resolve()
      }
    }),
  })
  fixture.state.isContinuousScrollRead.value = true
  await fixture.controller.load(2, 0, { saveAfterLoad: true })
  assert.deepEqual(fixture.calls.filter(call => call[0] === 'save'), [], 'the window transaction blocks progress until it settles')
  settle()
  for (let tick = 0; tick < 5; tick += 1) await Promise.resolve()
  const order = fixture.calls.map(call => call[0]).filter(name => name === 'window-settled' || name === 'save')
  assert.deepEqual(order, ['window-settled', 'save'])
})

test('does not save after a failed initial load, a failed reload or an aborted or superseded navigation (P2 control)', async () => {
  const failing = createController({
    loadContent: async () => {
      throw new Error('source down')
    },
  })
  await failing.controller.load(1)
  await failing.controller.load(1, 0, { refresh: true })
  assert.deepEqual(failing.calls.filter(call => call[0] === 'save'), [])

  const pending = new Map()
  const raced = createController({
    loadContent: (index, options) => new Promise((resolve, reject) => {
      pending.set(index, { resolve, reject, signal: options.signal })
    }),
  })
  const first = raced.controller.load(0, 0, { saveAfterLoad: true })
  const second = raced.controller.load(1, 0, { saveAfterLoad: true })
  const aborted = new Error('chapter request cancelled')
  aborted.name = 'AbortError'
  pending.get(0).reject(aborted)
  assert.equal(await first, false)
  const third = raced.controller.load(2, 0, { saveAfterLoad: true })
  pending.get(1).reject(new Error('superseded failure'))
  assert.equal(await second, false)
  pending.get(2).reject(aborted)
  assert.equal(await third, false)
  assert.deepEqual(raced.calls.filter(call => call[0] === 'save'), [])
})

// Contract P7: a progress save is best effort. A rejected save after a successful navigation must not turn the loaded
// chapter into an error page.
test('keeps a successfully loaded chapter when its progress save is rejected (P7)', async () => {
  const fixture = createController({
    saveProgress: async (options) => {
      fixture.calls.push(['save', options])
      throw new Error('invalid progress payload')
    },
  })
  fixture.state.isContinuousScrollRead.value = true
  assert.equal(await fixture.controller.load(1, 0, { saveAfterLoad: true }), true)
  assert.equal(fixture.state.chapterLoadError.value, '')
  assert.equal(fixture.state.chapterLoaded.value, true)
  assert.ok(fixture.calls.some(call => call[0] === 'window'), 'the continuous window is still computed')
})
