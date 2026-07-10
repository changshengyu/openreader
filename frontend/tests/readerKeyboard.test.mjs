import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive, ref } from 'vue'
import { useReaderKeyboard } from '../src/composables/useReaderKeyboard.js'

function createController(overrides = {}) {
  const calls = []
  let handlers
  const reader = reactive({ mode: 'scroll' })
  const state = {
    currentIndex: ref(1),
    chapters: ref([{}, {}, {}]),
    isScrollRead: ref(true),
    isAudio: ref(false),
    mobileChromeVisible: ref(true),
    primaryPanelOpen: ref(false),
    tocVisible: ref(false),
    settingsVisible: ref(false),
  }
  useReaderKeyboard({
    reader,
    ...state,
    previousPage: () => calls.push(['previous']),
    nextPage: () => calls.push(['next']),
    goChapter: (...args) => calls.push(['chapter', ...args]),
    scrollToTop: () => calls.push(['top']),
    scrollToBottom: () => calls.push(['bottom']),
    goShelf: () => calls.push(['shelf']),
    registerKeyboard: value => {
      handlers = value
    },
    ...overrides,
  })
  return { calls, handlers, reader, state }
}

test('uses pages in flip mode and chapters in vertical modes', () => {
  const fixture = createController()
  fixture.handlers.onArrowLeft()
  fixture.handlers.onArrowRight()
  assert.deepEqual(fixture.calls, [
    ['chapter', 0, -1],
    ['chapter', 2],
  ])
  assert.equal(fixture.state.mobileChromeVisible.value, false)

  fixture.calls.length = 0
  fixture.reader.mode = 'flip'
  fixture.handlers.onArrowLeft()
  fixture.handlers.onArrowRight()
  assert.deepEqual(fixture.calls, [['previous'], ['next']])
})

test('keeps chapter arrows inside book boundaries', () => {
  const fixture = createController()
  fixture.state.currentIndex.value = 0
  fixture.handlers.onArrowLeft()
  fixture.state.currentIndex.value = 2
  fixture.handlers.onArrowRight()
  assert.deepEqual(fixture.calls, [])
})

test('maps vertical, paging, home, end, and space keys without changing semantics', () => {
  const fixture = createController()
  fixture.handlers.onArrowUp()
  fixture.handlers.onArrowDown()
  fixture.handlers.onPageUp()
  fixture.handlers.onPageDown()
  fixture.handlers.onHome()
  fixture.handlers.onEnd()
  fixture.handlers.onSpace()
  assert.deepEqual(fixture.calls, [
    ['previous'],
    ['next'],
    ['previous'],
    ['next'],
    ['top'],
    ['bottom'],
    ['next'],
  ])
})

test('primary popovers consume all keyboard navigation until their trigger closes them', () => {
  const fixture = createController()
  fixture.state.primaryPanelOpen.value = true
  fixture.handlers.onArrowLeft()
  fixture.handlers.onArrowDown()
  fixture.handlers.onPageDown()
  fixture.handlers.onEscape()
  assert.equal(fixture.state.mobileChromeVisible.value, true)
  assert.deepEqual(fixture.calls, [])

  fixture.state.primaryPanelOpen.value = false
  fixture.handlers.onEscape()
  assert.equal(fixture.state.mobileChromeVisible.value, false)
  assert.deepEqual(fixture.calls, [['shelf']])
})

test('ignores text paging keyboard shortcuts for audio chapters', () => {
  const fixture = createController()
  fixture.state.isAudio.value = true
  fixture.handlers.onArrowLeft()
  fixture.handlers.onArrowRight()
  fixture.handlers.onArrowUp()
  fixture.handlers.onArrowDown()
  fixture.handlers.onPageUp()
  fixture.handlers.onPageDown()
  fixture.handlers.onHome()
  fixture.handlers.onEnd()
  fixture.handlers.onSpace()
  assert.deepEqual(fixture.calls, [])
  assert.equal(fixture.state.mobileChromeVisible.value, true)

  fixture.handlers.onEscape()
  assert.deepEqual(fixture.calls, [['shelf']])
})
