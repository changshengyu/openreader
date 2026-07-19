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
      calls.push(['load', index, options])
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

test('records load failures and always releases loading guards', async () => {
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
  assert.deepEqual(fixture.calls, [
    ['cancel'],
    ['frame'],
  ])
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
