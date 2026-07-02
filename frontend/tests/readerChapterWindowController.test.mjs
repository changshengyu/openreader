import assert from 'node:assert/strict'
import test from 'node:test'
import { nextTick, reactive, ref } from 'vue'
import { useReaderChapterWindow } from '../src/composables/useReaderChapterWindow.js'

function createController(overrides = {}) {
  const calls = []
  const reader = reactive({ mode: 'scroll2' })
  const chapters = ref(Array.from({ length: 7 }, (_, index) => ({
    id: index + 1,
    title: `第 ${index + 1} 章`,
  })))
  const currentIndex = ref(2)
  const chapter = ref(chapters.value[2])
  const content = ref('正文 2')
  const chapterBlocks = ref([])
  const contentEl = ref({
    scrollTop: 100,
    scrollHeight: 500,
    clientHeight: 200,
  })
  const contentBody = ref({
    querySelector: () => null,
  })
  const controller = useReaderChapterWindow({
    reader,
    contentEl,
    contentBody,
    chapters,
    currentIndex,
    chapter,
    content,
    chapterBlocks,
    isContinuousScrollRead: ref(true),
    loadContent: async index => {
      calls.push(['load', index])
      return { chapter: chapters.value[index], content: `正文 ${index}` }
    },
    makeChapterBlock: (index, row, text) => ({
      index,
      id: row.id,
      title: row.title,
      content: text,
    }),
    captureScrollAnchor: () => {
      calls.push(['capture'])
      return { index: 2, offset: 30 }
    },
    restoreScrollAnchor: async anchor => calls.push(['restore', anchor]),
    visibleProgressSnapshot: () => ({ chapterIndex: 2, chapter: chapters.value[2] }),
    nextFrame: async () => calls.push(['frame']),
    previousSize: 1,
    nextSize: 2,
    ...overrides,
  })
  return {
    calls,
    chapter,
    chapterBlocks,
    chapters,
    content,
    contentBody,
    contentEl,
    controller,
    currentIndex,
    reader,
  }
}

test('builds a continuous chapter window around the active chapter', async () => {
  const fixture = createController()
  await fixture.controller.compute()
  assert.deepEqual(
    fixture.chapterBlocks.value.map(block => block.index),
    [1, 2, 3, 4],
  )
  assert.deepEqual(fixture.calls, [
    ['load', 1],
    ['load', 2],
    ['load', 3],
    ['load', 4],
    ['capture'],
    ['restore', { index: 2, offset: 30 }],
  ])
})

test('prepends the previous chapter while preserving the viewport position', async () => {
  const fixture = createController()
  fixture.currentIndex.value = 3
  fixture.chapterBlocks.value = [3, 4].map(index => ({
    index,
    id: index + 1,
    content: `正文 ${index}`,
  }))
  fixture.contentEl.value.scrollTop = 120
  fixture.controller = useReaderChapterWindow({
    reader: fixture.reader,
    contentEl: fixture.contentEl,
    contentBody: fixture.contentBody,
    chapters: fixture.chapters,
    currentIndex: fixture.currentIndex,
    chapter: fixture.chapter,
    content: fixture.content,
    chapterBlocks: fixture.chapterBlocks,
    isContinuousScrollRead: ref(true),
    loadContent: async index => ({
      chapter: fixture.chapters.value[index],
      content: `正文 ${index}`,
    }),
    makeChapterBlock: (index, row, text) => ({ index, id: row.id, content: text }),
    captureScrollAnchor: () => null,
    restoreScrollAnchor: async () => {},
    visibleProgressSnapshot: () => null,
    nextFrame: async () => {
      fixture.contentEl.value.scrollHeight = 650
    },
    previousSize: 1,
    nextSize: 2,
  })
  await fixture.controller.prependPrevious()
  assert.deepEqual(fixture.chapterBlocks.value.map(block => block.index), [2, 3, 4])
  assert.equal(fixture.contentEl.value.scrollTop, 270)
})

test('syncs the visible chapter and prunes old scroll2 blocks with compensation', async () => {
  const fixture = createController({
    visibleProgressSnapshot: () => ({
      chapterIndex: 3,
      chapter: { id: 4, title: '第四章' },
    }),
  })
  fixture.chapterBlocks.value = [0, 1, 2, 3, 4, 5, 6].map(index => ({
    index,
    id: index + 1,
    title: `第 ${index + 1} 章`,
    content: `正文 ${index}`,
  }))
  fixture.contentBody.value.querySelector = selector => (
    selector.includes('"0"') || selector.includes('"1"')
      ? { getBoundingClientRect: () => ({ height: 50 }) }
      : null
  )
  fixture.contentEl.value.scrollTop = 300
  fixture.controller.syncCurrentChapter()
  await nextTick()
  assert.equal(fixture.currentIndex.value, 3)
  assert.equal(fixture.chapter.value.id, 4)
  assert.equal(fixture.content.value, '正文 3')
  assert.deepEqual(fixture.chapterBlocks.value.map(block => block.index), [2, 3, 4, 5])
  assert.equal(fixture.contentEl.value.scrollTop, 200)
})
