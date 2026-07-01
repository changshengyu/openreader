import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderProgressControls } from '../src/composables/useReaderProgressControls.js'

function createControls(overrides = {}) {
  const saved = []
  const scheduled = []
  const navigated = []
  const local = []
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
    getCurrentChapterPercent: () => 0.25,
    navigate: async query => navigated.push(query),
    applyLocalProgress: () => local.push(true),
    saveProgress: () => saved.push(true),
    scheduleProgressSave: delay => scheduled.push(delay),
    ...overrides,
  }
  return {
    controls: useReaderProgressControls(options),
    local,
    navigated,
    options,
    saved,
    scheduled,
  }
}

test('derives whole-book and chapter slider values from current progress', () => {
  const { controls } = createControls()
  assert.equal(controls.bookProgress.value, 0.3125)
  assert.equal(controls.bookProgressLabel.value, '31%')
  assert.equal(controls.mobileBookSliderValue.value, 313)
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

test('routes whole-book seeks across chapters and clears mobile draft state', async () => {
  const fixture = createControls()
  await fixture.controls.handleMobileBookProgressChange({
    target: { value: '900' },
  })
  assert.deepEqual(fixture.navigated, [{
    chapter: 3,
    percent: 0.6000000000000001,
  }])
  assert.equal(fixture.controls.mobileBookSliderValue.value, 313)
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
