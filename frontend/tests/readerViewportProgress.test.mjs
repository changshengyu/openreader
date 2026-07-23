import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderViewportProgress } from '../src/composables/useReaderViewportProgress.js'

function createController(overrides = {}) {
  const viewport = { top: 100, bottom: 900, left: 0, right: 600, height: 800 }
  const chapterEl = {
    dataset: { index: '1' },
    querySelector: () => null,
  }
  const paragraph = {
    dataset: { pos: '100' },
    textContent: 'x'.repeat(200),
    closest: selector => selector === '.chapter-content' ? chapterEl : null,
    getBoundingClientRect: () => ({
      top: 180,
      bottom: 380,
      left: 10,
      right: 590,
      height: 200,
    }),
  }
  const contentEl = ref({
    scrollTop: 800,
    scrollHeight: 2400,
    clientHeight: 800,
    getBoundingClientRect: () => viewport,
  })
  const contentBody = ref({
    querySelectorAll: () => [paragraph],
    querySelector: () => chapterEl,
  })
  const options = {
    contentEl,
    contentBody,
    chapterBlocks: ref([{ index: 1, id: 11, title: '第二章', content: '正文' }]),
    displayedChapterBlocks: ref([{ index: 1, id: 11, title: '第二章', content: '正文' }]),
    chapters: ref([{ id: 10, title: '第一章' }, { id: 11, title: '第二章' }]),
    currentIndex: ref(1),
    chapter: ref({ id: 11, title: '第二章' }),
    content: ref('正文'),
    chapterTextLength: ref(1000),
    progressVersion: ref(0),
    page: ref(0),
    pageCount: ref(1),
    isContinuousScrollRead: ref(true),
    getMode: () => 'scroll2',
    makeChapterBlock: (index, chapter, content) => ({ index, ...chapter, content }),
    chapterBlockTextLength: () => 1000,
    nextFrame: async () => {},
    ...overrides,
  }
  return {
    controller: useReaderViewportProgress(options),
    options,
    paragraph,
  }
}

test('assembles the visible chapter progress snapshot', () => {
  const { controller, options, paragraph } = createController()
  const snapshot = controller.visibleChapterProgressSnapshot()

  assert.equal(controller.currentVisibleParagraph(), paragraph)
  assert.deepEqual(snapshot, {
    chapterIndex: 1,
    chapter: options.chapters.value[1],
    offset: 200,
    chapterPercent: 0.2,
  })
  assert.equal(controller.currentChapterPosition(), 200)
  assert.equal(controller.currentOffset(), 200)
  assert.equal(controller.currentChapterPercent(), 0.2)
})

test('uses a visible chapter heading as upstream position zero before a middle paragraph', () => {
  const firstChapter = {
    dataset: { index: '0' },
    querySelector: () => null,
  }
  const secondChapter = {
    dataset: { index: '1' },
    querySelector: () => null,
  }
  const heading = {
    dataset: { pos: '0' },
    textContent: '第一章',
    closest: selector => selector === '.chapter-content' ? firstChapter : null,
    getBoundingClientRect: () => ({
      top: 110,
      bottom: 170,
      left: 10,
      right: 590,
      height: 60,
    }),
  }
  const paragraph = {
    dataset: { pos: '100' },
    textContent: '第二章正文',
    closest: selector => selector === '.chapter-content' ? secondChapter : null,
    getBoundingClientRect: () => ({
      top: 240,
      bottom: 420,
      left: 10,
      right: 590,
      height: 180,
    }),
  }
  const { controller } = createController({
    contentBody: ref({
      querySelectorAll: () => [heading, paragraph],
      querySelector: () => firstChapter,
    }),
    chapterBlocks: ref([
      { index: 0, id: 10, title: '第一章', content: '正文 0' },
      { index: 1, id: 11, title: '第二章', content: '正文 1' },
    ]),
    displayedChapterBlocks: ref([
      { index: 0, id: 10, title: '第一章', content: '正文 0' },
      { index: 1, id: 11, title: '第二章', content: '正文 1' },
    ]),
    currentIndex: ref(0),
    chapter: ref({ id: 10, title: '第一章' }),
  })

  assert.deepEqual(controller.visibleChapterProgressSnapshot(), {
    chapterIndex: 0,
    chapter: { id: 10, title: '第一章' },
    offset: 0,
    chapterPercent: 0,
  })
})

test('stops measuring paragraph geometry as soon as the upstream visible block is found', () => {
  let geometryReads = 0
  const chapterEl = {
    dataset: { index: '1' },
    querySelector: () => null,
  }
  const nodes = Array.from({ length: 720 }, (_, index) => ({
    dataset: { pos: String(index * 100) },
    textContent: `第 ${index + 1} 段`,
    closest: selector => selector === '.chapter-content' ? chapterEl : null,
    getBoundingClientRect: () => {
      geometryReads += 1
      return index === 0
        ? { top: 120, bottom: 220, left: 10, right: 590, height: 100 }
        : { top: 920 + index * 100, bottom: 1000 + index * 100, left: 10, right: 590, height: 80 }
    },
  }))
  const { controller } = createController({
    contentBody: ref({
      querySelectorAll: () => nodes,
      querySelector: () => chapterEl,
    }),
  })

  const snapshot = controller.visibleChapterProgressSnapshot()
  assert.equal(snapshot.chapterIndex, 1)
  assert(
    geometryReads <= 3,
    `visible paragraph lookup measured the whole long chapter (${geometryReads} reads)`,
  )
})

test('uses flip page state without requiring rendered paragraphs', () => {
  const { controller } = createController({
    contentEl: ref(null),
    contentBody: ref(null),
    page: ref(2),
    pageCount: ref(5),
    isContinuousScrollRead: ref(false),
    getMode: () => 'flip',
  })

  assert.equal(controller.currentOffset(), 2)
  assert.equal(controller.currentChapterPercent(), 0.5)
  assert.equal(controller.visibleChapterProgressSnapshot(), null)
})

test('stores EPUB positions as document scroll pixels like upstream', () => {
  const contentEl = ref({
    scrollTop: 800,
    scrollHeight: 2400,
    clientHeight: 800,
    getBoundingClientRect: () => ({ top: 0, bottom: 800 }),
  })
  const { controller } = createController({
    contentEl,
    contentBody: ref({ querySelectorAll: () => [], querySelector: () => null }),
    isEPUB: ref(true),
    isContinuousScrollRead: ref(false),
    getMode: () => 'page',
  })

  assert.equal(controller.currentChapterPosition(), 800)
  assert.equal(controller.currentOffset(), 800)
  assert.equal(controller.currentChapterPercent(), 0.5)
})

test('captures a mode transition as one paragraph anchor instead of a mode-specific offset', () => {
  const { controller, options, paragraph } = createController()
  const snapshot = controller.captureReaderLayoutPosition({
    viewport: options.contentEl.value,
    mode: 'scroll2',
  })

  assert.equal(snapshot.chapterIndex, 1)
  assert.equal(snapshot.paragraph, paragraph)
  assert.equal(snapshot.paragraphPos, 100)
  assert.equal(snapshot.paragraphIndex, 0)
  assert.equal(snapshot.offset, 200)
  assert.equal(snapshot.percent, 0.2)
})

test('keeps the source flip page only as a fallback beside the paragraph anchor', () => {
  const { controller, options, paragraph } = createController({
    page: ref(2),
    pageCount: ref(5),
    isContinuousScrollRead: ref(false),
    getMode: () => 'flip',
  })
  const snapshot = controller.captureReaderLayoutPosition({
    viewport: options.contentEl.value,
    mode: 'flip',
  })

  assert.equal(snapshot.paragraph, paragraph)
  assert.equal(snapshot.paragraphPos, 100)
  assert.equal(snapshot.offset, 2)
  assert.equal(snapshot.percent, 0.5)
})
