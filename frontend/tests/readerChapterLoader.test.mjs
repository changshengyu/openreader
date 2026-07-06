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
