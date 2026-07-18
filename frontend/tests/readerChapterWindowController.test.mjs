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
  const scopeKey = ref('book:1')
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
    formatError: error => `加载失败：${error.message}`,
    getScopeKey: () => scopeKey.value,
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
    scopeKey,
  }
}

test('builds a continuous chapter window around the active chapter', async () => {
  const fixture = createController()
  await fixture.controller.compute()
  assert.deepEqual(
    fixture.chapterBlocks.value.map(block => block.index),
    [2, 3],
  )
  assert.deepEqual(fixture.calls, [
    ['load', 3],
    ['capture'],
    ['restore', { index: 2, offset: 30 }],
  ])
})

test('syncs the visible chapter without replacing the chapter window mid-scroll', async () => {
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
  assert.deepEqual(fixture.chapterBlocks.value.map(block => block.index), [0, 1, 2, 3, 4, 5, 6])
  assert.equal(fixture.contentEl.value.scrollTop, 300)
})

test('appends one chapter and removes read scroll2 chapters in one anchored transaction', async () => {
  const fixture = createController({
    visibleProgressSnapshot: () => ({
      chapterIndex: 3,
      chapter: { id: 4, title: '第四章' },
    }),
  })
  fixture.chapterBlocks.value = [2, 3].map(index => ({
    index,
    id: index + 1,
    title: `第 ${index + 1} 章`,
    content: `正文 ${index}`,
  }))
  fixture.controller.syncCurrentChapter()
  fixture.calls.length = 0
  await fixture.controller.appendNext()
  assert.deepEqual(fixture.chapterBlocks.value.map(block => block.index), [3, 4])
  assert.deepEqual(fixture.calls, [
    ['load', 4],
    ['capture'],
    ['restore', { index: 2, offset: 30 }],
  ])
})

test('renders adjacent chapter failures and can retry them without losing current content', async () => {
  let attempts = 0
  const loadedIndexes = []
  const fixture = createController({
    loadContent: async index => {
      loadedIndexes.push(index)
      if (index === 4 && attempts++ === 0) throw new Error('network error')
      return { chapter: fixture.chapters.value[index], content: `正文 ${index}` }
    },
  })
  fixture.currentIndex.value = 3
  fixture.chapterBlocks.value = [3].map(index => ({
    index,
    id: index + 1,
    title: `第 ${index + 1} 章`,
    content: `正文 ${index}`,
  }))

  await fixture.controller.appendNext()
  assert.equal(fixture.chapterBlocks.value[0].content, '正文 3')
  assert.equal(fixture.chapterBlocks.value[1].index, 4)
  assert.equal(fixture.chapterBlocks.value[1].error, '加载失败：network error')
  fixture.controller.maybeExtend()
  await new Promise(resolve => setImmediate(resolve))
  assert.deepEqual(loadedIndexes, [4])

  await fixture.controller.retry(4)
  assert.equal(fixture.chapterBlocks.value[1].error, undefined)
  assert.equal(fixture.chapterBlocks.value[1].content, '正文 4')
  assert.deepEqual(loadedIndexes, [4, 4])
})

test('serializes boundary extension and releases its lock after completion', async () => {
  const attempts = []
  let resolveFirst
  const fixture = createController({
    loadContent: index => {
      attempts.push(index)
      if (index === 4) {
        return new Promise(resolve => {
          resolveFirst = resolve
        })
      }
      return Promise.resolve({
        chapter: fixture.chapters.value[index],
        content: `正文 ${index}`,
      })
    },
  })
  fixture.currentIndex.value = 3
  fixture.chapterBlocks.value = [{
    index: 3,
    id: 4,
    title: '第四章',
    content: '正文 3',
  }]

  fixture.controller.maybeExtend()
  fixture.controller.maybeExtend()
  assert.deepEqual(attempts, [4])

  resolveFirst({
    chapter: fixture.chapters.value[4],
    content: '正文 4',
  })
  await new Promise(resolve => setImmediate(resolve))
  fixture.controller.maybeExtend()
  await new Promise(resolve => setImmediate(resolve))
  assert.deepEqual(attempts, [4, 5])
})

test('keeps the window transaction busy through anchor restoration', async () => {
  let resolveLoad
  let resolveRestore
  const fixture = createController({
    loadContent: index => new Promise(resolve => {
      resolveLoad = () => resolve({
        chapter: fixture.chapters.value[index],
        content: `正文 ${index}`,
      })
    }),
    restoreScrollAnchor: () => new Promise(resolve => {
      resolveRestore = resolve
    }),
  })
  fixture.currentIndex.value = 3
  fixture.chapterBlocks.value = [{
    index: 3,
    id: 4,
    title: '第四章',
    content: '正文 3',
  }]

  const pending = fixture.controller.appendNext()
  assert.equal(fixture.controller.busy.value, true)
  resolveLoad()
  await new Promise(resolve => setImmediate(resolve))
  assert.equal(fixture.controller.busy.value, true)
  resolveRestore()
  await pending
  assert.equal(fixture.controller.busy.value, false)
})

test('drops a delayed append after an explicit chapter-window rebuild', async () => {
  let resolveAppend
  const fixture = createController({
    loadContent: index => {
      if (index === 4) {
        return new Promise(resolve => {
          resolveAppend = () => resolve({
            chapter: fixture.chapters.value[index],
            content: `旧事务正文 ${index}`,
          })
        })
      }
      return Promise.resolve({
        chapter: fixture.chapters.value[index],
        content: `正文 ${index}`,
      })
    },
  })
  fixture.currentIndex.value = 3
  fixture.chapter.value = fixture.chapters.value[3]
  fixture.content.value = '正文 3'
  fixture.chapterBlocks.value = [{
    index: 3,
    id: 4,
    title: '第四章',
    content: '正文 3',
  }]

  const staleAppend = fixture.controller.appendNext()
  fixture.currentIndex.value = 0
  fixture.chapter.value = fixture.chapters.value[0]
  fixture.content.value = '正文 0'
  await fixture.controller.compute({ anchorIndex: 0, activate: true })
  resolveAppend()
  await staleAppend

  assert.deepEqual(fixture.chapterBlocks.value.map(block => block.index), [0, 1])
  assert.equal(fixture.chapterBlocks.value.some(block => block.content.includes('旧事务正文')), false)
})

test('drops delayed append and retry results after the book scope changes', async () => {
  let resolveAppend
  let resolveRetry
  const fixture = createController({
    loadContent: (index, options = {}) => new Promise(resolve => {
      const finish = () => resolve({
        chapter: fixture.chapters.value[index],
        content: `${options.refresh ? '重试' : '追加'}正文 ${index}`,
      })
      if (options.refresh) resolveRetry = finish
      else resolveAppend = finish
    }),
  })
  fixture.currentIndex.value = 3
  fixture.chapterBlocks.value = [{
    index: 3,
    id: 4,
    title: '第四章',
    content: '正文 3',
  }]
  const append = fixture.controller.appendNext()
  fixture.scopeKey.value = 'book:2'
  resolveAppend()
  await append
  assert.deepEqual(fixture.chapterBlocks.value.map(block => block.index), [3])

  fixture.chapterBlocks.value = [{
    index: 3,
    id: 4,
    title: '第四章',
    content: '旧错误',
    error: '旧错误',
  }]
  fixture.scopeKey.value = 'book:2'
  const retry = fixture.controller.retry(3)
  fixture.scopeKey.value = 'book:3'
  resolveRetry()
  await retry
  assert.equal(fixture.chapterBlocks.value[0].error, '旧错误')
  assert.equal(fixture.chapterBlocks.value[0].content, '旧错误')
})
