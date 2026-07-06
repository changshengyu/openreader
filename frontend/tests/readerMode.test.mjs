import assert from 'node:assert/strict'
import test from 'node:test'
import { computed, effectScope, nextTick, reactive, ref } from 'vue'
import { readerEffectiveMode, useReaderMode } from '../src/composables/useReaderMode.js'

async function flushModeChange() {
  await nextTick()
  await Promise.resolve()
  await nextTick()
}

test('forces EPUB documents through the upstream vertical page branch', () => {
  assert.equal(readerEffectiveMode('flip', true), 'page')
  assert.equal(readerEffectiveMode('scroll2', true), 'page')
  assert.equal(readerEffectiveMode('scroll2', false), 'scroll2')
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
