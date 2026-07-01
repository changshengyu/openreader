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
