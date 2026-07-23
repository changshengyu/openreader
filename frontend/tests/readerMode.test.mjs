import assert from 'node:assert/strict'
import test from 'node:test'
import { computed, effectScope, nextTick, reactive, ref } from 'vue'
import {
  readerAutoReadingSupported,
  readerEffectiveMode,
  readerTTSSupported,
  useReaderMode,
} from '../src/composables/useReaderMode.js'

async function flushModeChange() {
  await nextTick()
  await Promise.resolve()
  await nextTick()
}

test('forces EPUB, audio, and ordinary image-comic documents through the upstream vertical page branch', () => {
  assert.equal(readerEffectiveMode('flip', true), 'page')
  assert.equal(readerEffectiveMode('scroll2', true), 'page')
  assert.equal(readerEffectiveMode('scroll2', false), 'scroll2')
  assert.equal(readerEffectiveMode('scroll2', false, true), 'page')
  assert.equal(readerEffectiveMode('flip', false, false, false, true), 'page')
  assert.equal(readerEffectiveMode('flip', false, false, false, false), 'flip')
  assert.equal(readerEffectiveMode('flip', false, false, true), 'page')
  assert.equal(readerEffectiveMode('scroll', false, false, true), 'scroll')
  assert.equal(readerEffectiveMode('flip', false, false, false, false, true), 'page')
  assert.equal(readerEffectiveMode('scroll', false, false, false, false, true), 'scroll')
})

test('separates CBZ comic presentation from upstream ordinary-image control exclusions', () => {
  assert.equal(readerAutoReadingSupported({}), true, 'plain text keeps auto reading')
  assert.equal(readerAutoReadingSupported({ isCBZ: true }), true, 'CBZ keeps auto reading')
  assert.equal(readerAutoReadingSupported({ isEPUB: true }), false)
  assert.equal(readerAutoReadingSupported({ isAudio: true }), false)
  assert.equal(readerAutoReadingSupported({ isOrdinaryImageComic: true }), false)

  assert.equal(readerTTSSupported({ speechSupported: true, isCBZ: true }), true, 'CBZ keeps TTS')
  assert.equal(readerTTSSupported({ speechSupported: true, isEPUB: true }), false)
  assert.equal(readerTTSSupported({ speechSupported: true, isAudio: true }), false)
  assert.equal(readerTTSSupported({ speechSupported: true, isOrdinaryImageComic: true }), false)
  assert.equal(readerTTSSupported({ speechSupported: false, isCBZ: true }), false)
})

test('rebuilds continuous chapter windows and restores reading position', async () => {
  const calls = []
  const reader = reactive({
    mode: 'page',
    setMode(mode) {
      this.mode = mode
    },
  })
  const chapterLoading = ref(false)
  const page = ref(3)
  const scope = effectScope()
  const controller = scope.run(() => useReaderMode({
    reader,
    isMobileReader: ref(true),
    isContinuousScrollRead: computed(() => reader.mode === 'scroll2'),
    page,
    chapterLoading,
    chapterBlocks: ref([]),
    currentIndex: ref(2),
    chapter: ref({ id: 3, title: '第三章' }),
    content: ref('正文'),
    getCurrentOffset: () => 240,
    computeChapterWindow: async () => calls.push(['window', chapterLoading.value]),
    makeChapterBlock: () => null,
    updateLayout: () => calls.push(['layout']),
    restorePosition: async (offset, options) => calls.push(['restore', offset, options]),
    saveProgress: () => calls.push(['save']),
  }))

  controller.change('scroll2')
  await flushModeChange()
  assert.equal(page.value, 0)
  assert.equal(chapterLoading.value, false)
  assert.deepEqual(calls, [
    ['window', false],
    ['layout'],
    ['restore', 240, { saveAfterLoad: false }],
    ['save'],
  ])
  scope.stop()
})

test('rebuilds a single block for non-continuous modes', async () => {
  const reader = reactive({
    mode: 'scroll',
    setMode(mode) {
      this.mode = mode
    },
  })
  const chapterBlocks = ref([])
  const chapter = ref({ id: 5, title: '第五章' })
  const scope = effectScope()
  const controller = scope.run(() => useReaderMode({
    reader,
    isMobileReader: ref(true),
    isContinuousScrollRead: computed(() => reader.mode === 'scroll'),
    page: ref(0),
    chapterLoading: ref(false),
    chapterBlocks,
    currentIndex: ref(4),
    chapter,
    content: ref('正文'),
    getCurrentOffset: () => 0,
    computeChapterWindow: async () => {},
    makeChapterBlock: (index, row, content) => ({ index, id: row.id, content }),
    updateLayout: () => {},
    restorePosition: async () => {},
    saveProgress: () => {},
  }))

  controller.change('page')
  await flushModeChange()
  assert.deepEqual(chapterBlocks.value, [{ index: 4, id: 5, content: '正文' }])
  scope.stop()
})

test('keeps audio chapters out of text block rebuilds when modes change', async () => {
  const reader = reactive({
    mode: 'scroll',
    setMode(mode) {
      this.mode = mode
    },
  })
  const chapterBlocks = ref([{ index: 0, content: '旧正文' }])
  const scope = effectScope()
  const calls = []
  const controller = scope.run(() => useReaderMode({
    reader,
    isMobileReader: ref(true),
    isContinuousScrollRead: ref(false),
    isEPUB: ref(false),
    isAudio: ref(true),
    page: ref(0),
    chapterLoading: ref(false),
    chapterBlocks,
    currentIndex: ref(0),
    chapter: ref({ id: 1, title: '第一集' }),
    content: ref('https://audio.example.test/001.mp3'),
    getCurrentOffset: () => 18,
    computeChapterWindow: async () => calls.push(['window']),
    makeChapterBlock: () => ({ should: 'not build' }),
    updateLayout: () => calls.push(['layout']),
    restorePosition: async (...args) => calls.push(['restore', ...args]),
    saveProgress: () => calls.push(['save']),
  }))

  controller.change('page')
  await flushModeChange()
  assert.deepEqual(chapterBlocks.value, [])
  assert.deepEqual(calls, [
    ['layout'],
    ['restore', 18, { saveAfterLoad: false }],
    ['save'],
  ])
  scope.stop()
})

test('forces flip mode back to page on desktop', async () => {
  const reader = reactive({
    mode: 'flip',
    setMode(mode) {
      this.mode = mode
    },
  })
  const scope = effectScope()
  scope.run(() => useReaderMode({
    reader,
    isMobileReader: ref(false),
    isContinuousScrollRead: ref(false),
    page: ref(0),
    chapterLoading: ref(false),
    chapterBlocks: ref([]),
    currentIndex: ref(0),
    chapter: ref(null),
    content: ref(''),
    getCurrentOffset: () => 0,
    computeChapterWindow: async () => {},
    makeChapterBlock: () => null,
    updateLayout: () => {},
    restorePosition: async () => {},
    saveProgress: () => {},
  }))
  await flushModeChange()
  assert.equal(reader.mode, 'page')
  scope.stop()
})

test('captures the source paragraph before a direct mode mutation and restores that anchor', async () => {
  const calls = []
  const reader = reactive({
    mode: 'page',
    fontFamily: 'system',
    chineseFont: '简体',
    fontSize: 18,
    fontWeight: 400,
    lineHeight: 1.8,
    paragraphSpace: 0.2,
    columnWidth: 800,
    setMode(mode) {
      calls.push(['set-mode', this.mode, mode])
      this.mode = mode
    },
  })
  const scope = effectScope()
  const controller = scope.run(() => useReaderMode({
    reader,
    isMobileReader: ref(true),
    isContinuousScrollRead: ref(false),
    getEffectiveMode: () => reader.mode,
    page: ref(3),
    chapterLoading: ref(false),
    chapterBlocks: ref([]),
    currentIndex: ref(2),
    chapter: ref({ id: 3, title: '第三章' }),
    content: ref('正文'),
    capturePosition: state => {
      calls.push(['capture', state])
      return { chapterIndex: 2, paragraphPos: 480 }
    },
    restoreCapturedPosition: async (snapshot, state) => {
      calls.push(['restore-anchor', snapshot, state])
      return true
    },
    getCurrentOffset: () => {
      calls.push(['legacy-offset', reader.mode])
      return reader.mode === 'page' ? 480 : 0
    },
    computeChapterWindow: async () => {},
    makeChapterBlock: () => ({ index: 2 }),
    updateLayout: () => calls.push(['layout']),
    restorePosition: async (...args) => calls.push(['legacy-restore', ...args]),
    saveProgress: () => calls.push(['save']),
  }))

  controller.change('flip')
  await flushModeChange()

  assert.deepEqual(calls, [
    ['set-mode', 'page', 'flip'],
    ['capture', { mode: 'page', mobile: true }],
    ['layout'],
    [
      'restore-anchor',
      { chapterIndex: 2, paragraphPos: 480 },
      { fromMode: 'page', toMode: 'flip', fromMobile: true, toMobile: true },
    ],
    ['save'],
  ])
  scope.stop()
})

test('coalesces a scheme mode and typography update into one position transaction', async () => {
  const reader = reactive({
    mode: 'page',
    fontFamily: 'system',
    chineseFont: '简体',
    fontSize: 18,
    fontWeight: 400,
    lineHeight: 1.8,
    paragraphSpace: 0.2,
    columnWidth: 800,
    setMode(mode) {
      this.mode = mode
    },
  })
  const calls = []
  const scope = effectScope()
  scope.run(() => useReaderMode({
    reader,
    isMobileReader: ref(true),
    isContinuousScrollRead: ref(false),
    getEffectiveMode: () => reader.mode,
    page: ref(1),
    chapterLoading: ref(false),
    chapterBlocks: ref([]),
    currentIndex: ref(4),
    chapter: ref({ id: 5, title: '第五章' }),
    content: ref('正文'),
    capturePosition: state => {
      calls.push(['capture', state])
      return { chapterIndex: 4, paragraphPos: 900 }
    },
    restoreCapturedPosition: async snapshot => {
      calls.push(['restore', snapshot])
      return true
    },
    getCurrentOffset: () => 0,
    computeChapterWindow: async () => {},
    makeChapterBlock: () => ({ index: 4 }),
    updateLayout: () => calls.push(['layout']),
    restorePosition: async () => calls.push(['legacy-restore']),
    saveProgress: () => calls.push(['save']),
  }))

  Object.assign(reader, {
    mode: 'flip',
    fontSize: 22,
    lineHeight: 2.2,
    paragraphSpace: 0.6,
  })
  await flushModeChange()

  assert.equal(calls.filter(call => call[0] === 'capture').length, 1)
  assert.equal(calls.filter(call => call[0] === 'restore').length, 1)
  assert.equal(calls.filter(call => call[0] === 'legacy-restore').length, 0)
  scope.stop()
})

test('does not rebuild a fixed-format chapter when only its raw configured mode changes', async () => {
  const reader = reactive({
    mode: 'page',
    fontFamily: 'system',
    chineseFont: '简体',
    fontSize: 18,
    fontWeight: 400,
    lineHeight: 1.8,
    paragraphSpace: 0.2,
    columnWidth: 800,
    setMode(mode) {
      this.mode = mode
    },
  })
  const calls = []
  const scope = effectScope()
  scope.run(() => useReaderMode({
    reader,
    isMobileReader: ref(true),
    isContinuousScrollRead: ref(false),
    getEffectiveMode: () => readerEffectiveMode(reader.mode, true),
    page: ref(1),
    chapterLoading: ref(false),
    chapterBlocks: ref([{ index: 0, content: 'EPUB' }]),
    currentIndex: ref(0),
    chapter: ref({ id: 1, title: 'EPUB' }),
    content: ref('chapter.xhtml'),
    capturePosition: () => {
      calls.push(['capture'])
      return {}
    },
    restoreCapturedPosition: async () => calls.push(['restore']),
    getCurrentOffset: () => 0,
    computeChapterWindow: async () => calls.push(['window']),
    makeChapterBlock: () => ({ index: 0 }),
    updateLayout: () => calls.push(['layout']),
    restorePosition: async () => calls.push(['legacy-restore']),
    saveProgress: () => calls.push(['save']),
  }))

  reader.mode = 'flip'
  await flushModeChange()

  assert.deepEqual(calls, [])
  scope.stop()
})

test('invalidates an older asynchronous position transaction before it can save', async () => {
  let releaseFirstRestore
  const firstRestore = new Promise(resolve => {
    releaseFirstRestore = resolve
  })
  const reader = reactive({
    mode: 'page',
    fontFamily: 'system',
    chineseFont: '简体',
    fontSize: 18,
    fontWeight: 400,
    lineHeight: 1.8,
    paragraphSpace: 0.2,
    columnWidth: 800,
    setMode(mode) {
      this.mode = mode
    },
  })
  const calls = []
  const scope = effectScope()
  const controller = scope.run(() => useReaderMode({
    reader,
    isMobileReader: ref(true),
    isContinuousScrollRead: ref(false),
    getEffectiveMode: () => reader.mode,
    page: ref(1),
    chapterLoading: ref(false),
    chapterBlocks: ref([]),
    currentIndex: ref(0),
    chapter: ref({ id: 1, title: '第一章' }),
    content: ref('正文'),
    capturePosition: ({ mode }) => ({ chapterIndex: 0, paragraphPos: 100, mode }),
    restoreCapturedPosition: async (_snapshot, transition, isCurrent) => {
      calls.push(['restore-start', transition.toMode])
      if (transition.toMode === 'flip') await firstRestore
      calls.push(['restore-end', transition.toMode, isCurrent()])
      return true
    },
    getCurrentOffset: () => 0,
    computeChapterWindow: async () => {},
    makeChapterBlock: () => ({ index: 0 }),
    updateLayout: () => {},
    restorePosition: async () => {},
    saveProgress: () => calls.push(['save', reader.mode]),
  }))

  controller.change('flip')
  await flushModeChange()
  controller.change('scroll')
  await flushModeChange()
  releaseFirstRestore()
  await flushModeChange()

  assert.deepEqual(calls, [
    ['restore-start', 'flip'],
    ['restore-start', 'scroll'],
    ['restore-end', 'scroll', true],
    ['save', 'scroll'],
    ['restore-end', 'flip', false],
  ])
  scope.stop()
})
