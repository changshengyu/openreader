import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderProgressControls } from '../src/composables/useReaderProgressControls.js'

function createControls(overrides = {}) {
  const saved = []
  const scheduled = []
  const navigated = []
  const local = []
  const animations = []
  const options = {
    contentEl: ref(null),
    contentBody: ref(null),
    chapters: ref([{ id: 1 }, { id: 2 }, { id: 3 }, { id: 4 }]),
    currentIndex: ref(1),
    page: ref(1),
    pageCount: ref(5),
    progressVersion: ref(0),
    isContinuousScrollRead: ref(false),
    getMode: () => 'flip',
    getAnimateDuration: () => 300,
    getCurrentChapterPercent: () => 0.25,
    navigate: async query => navigated.push(query),
    applyLocalProgress: () => local.push(true),
    saveProgress: () => saved.push(true),
    scheduleProgressSave: delay => scheduled.push(delay),
    scrollAnimator: {
      cancel: () => {},
      scrollTo: (element, top, duration, onFinish) => {
        animations.push({ element, top, duration })
        element.scrollTop = top
        onFinish?.()
        return true
      },
    },
    ...overrides,
  }
  return {
    controls: useReaderProgressControls(options),
    animations,
    local,
    navigated,
    options,
    saved,
    scheduled,
  }
}

test('derives whole-book label plus upstream 1-based mobile page controls', () => {
  const { controls } = createControls()
  assert.equal(controls.bookProgress.value, 0.3125)
  assert.equal(controls.bookProgressLabel.value, '31%')
  assert.equal(controls.mobilePageSliderValue.value, 2)
  assert.equal(controls.mobilePageSliderMax.value, 5)
  assert.equal(controls.mobilePageProgressLabel.value, '第 2/5 页')
  assert.equal(controls.desktopChapterSliderValue.value, 250)
  assert.equal(controls.desktopChapterProgressLabel.value, '25%')
})

test('seeks flip chapter progress and preserves input versus change saving', () => {
  const fixture = createControls()
  fixture.controls.handleDesktopProgressInput({ target: { value: '750' } })
  assert.equal(fixture.options.page.value, 3)
  assert.deepEqual(fixture.saved, [])

  fixture.controls.handleDesktopProgressChange({ target: { value: '500' } })
  assert.equal(fixture.options.page.value, 2)
  assert.equal(fixture.saved.length, 1)
})

test('keeps mobile page input as a draft and commits within the rendered flip document', () => {
  const fixture = createControls()
  fixture.controls.handleMobilePageProgressInput({
    target: { value: '4' },
  })
  assert.equal(fixture.options.page.value, 1)
  assert.equal(fixture.controls.mobilePageSliderValue.value, 4)
  assert.equal(fixture.controls.mobilePageProgressLabel.value, '第 4/5 页')
  assert.deepEqual(fixture.saved, [])

  fixture.controls.handleMobilePageProgressChange({
    target: { value: '4' },
  })
  assert.equal(fixture.options.page.value, 3)
  assert.equal(fixture.controls.mobilePageSliderValue.value, 4)
  assert.deepEqual(fixture.navigated, [])
  assert.equal(fixture.saved.length, 1)
})

test('seeks a mobile vertical page without navigating to another chapter', () => {
  const fixture = createControls({
    contentEl: ref({
      scrollTop: 0,
      scrollHeight: 2400,
      clientHeight: 800,
    }),
    page: ref(0),
    pageCount: ref(4),
    getMode: () => 'scroll',
  })
  fixture.controls.handleMobilePageProgressChange({ target: { value: '3' } })
  assert.equal(fixture.options.contentEl.value.scrollTop, 1067)
  assert.deepEqual(fixture.animations.map(({ top, duration }) => ({ top, duration })), [
    { top: 1067, duration: 300 },
  ])
  assert.deepEqual(fixture.navigated, [])
  assert.equal(fixture.local.length, 1)
  assert.equal(fixture.saved.length, 1)
})

test('seeks vertical content and schedules local progress for input', () => {
  const fixture = createControls({
    contentEl: ref({
      scrollTop: 0,
      scrollHeight: 2400,
      clientHeight: 800,
    }),
    getMode: () => 'page',
  })
  fixture.controls.handleDesktopProgressInput({ target: { value: '500' } })
  assert.equal(fixture.options.contentEl.value.scrollTop, 800)
  assert.equal(fixture.local.length, 1)
  assert.deepEqual(fixture.scheduled, [500])
})
