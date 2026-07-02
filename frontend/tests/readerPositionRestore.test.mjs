import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive, ref } from 'vue'
import { useReaderPositionRestore } from '../src/composables/useReaderPositionRestore.js'

function createController(overrides = {}) {
  const calls = []
  const reader = reactive({ mode: 'page' })
  const contentEl = ref({
    scrollTop: 0,
    scrollHeight: 1000,
    clientHeight: 200,
  })
  const activeChapter = {
    offsetTop: 300,
    offsetHeight: 600,
  }
  const contentBody = ref({
    querySelector: () => activeChapter,
  })
  const state = {
    reader,
    contentEl,
    contentBody,
    currentIndex: ref(2),
    page: ref(0),
    pageCount: ref(5),
    isContinuousScrollRead: ref(false),
  }
  const controller = useReaderPositionRestore({
    ...state,
    paragraphByChapterPosition: (_root, position) => (
      position === 240 ? { dataset: { pos: '200' } } : null
    ),
    jumpToParagraph: (target, options) => calls.push(['jump', target.dataset.pos, options]),
    updateLayout: () => calls.push(['layout']),
    nextFrame: async () => calls.push(['frame']),
    ...overrides,
  })
  return { activeChapter, calls, controller, state }
}

test('restores flip pages after the layout frame', async () => {
  const fixture = createController()
  fixture.state.reader.mode = 'flip'
  await fixture.controller.restore(0, { restorePercent: 0.5 })
  assert.equal(fixture.state.page.value, 2)
  assert.deepEqual(fixture.calls, [['frame'], ['layout']])
})

test('restores single chapter positions by paragraph before scroll offset', async () => {
  const fixture = createController()
  await fixture.controller.restore(240)
  assert.equal(fixture.state.contentEl.value.scrollTop, 0)
  assert.deepEqual(fixture.calls, [
    ['frame'],
    ['layout'],
    ['jump', '200', { save: false, flash: false }],
  ])
})

test('applies single chapter percentage again after the second frame', async () => {
  let frames = 0
  const fixture = createController({
    nextFrame: async () => {
      frames += 1
      if (frames === 2) fixture.state.contentEl.value.scrollHeight = 1200
    },
  })
  await fixture.controller.restore(0, { restorePercent: 0.5 })
  assert.equal(fixture.state.contentEl.value.scrollTop, 500)
  assert.equal(frames, 2)
})

test('restores continuous chapter percentages relative to the active block', async () => {
  const fixture = createController()
  fixture.state.isContinuousScrollRead.value = true
  await fixture.controller.restore(0, { restorePercent: 0.5 })
  assert.equal(fixture.state.contentEl.value.scrollTop, 500)
  assert.deepEqual(fixture.calls, [['frame'], ['layout']])
})
