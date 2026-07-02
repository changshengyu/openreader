import assert from 'node:assert/strict'
import test from 'node:test'
import { computed, reactive, ref } from 'vue'
import { useReaderAutoReading } from '../src/composables/useReaderAutoReading.js'

function createController(overrides = {}) {
  const calls = []
  let adapterOptions
  const currentIndex = ref(2)
  const page = ref(3)
  const progressVersion = ref(5)
  const reader = reactive({
    autoReadingMethod: '像素滚动',
    autoReadingPixel: 4,
    autoReadingLineTime: 900,
    fontSize: 18,
    lineHeight: 1.8,
  })
  const controller = useReaderAutoReading({
    reader,
    contentEl: ref(null),
    contentBody: ref(null),
    isVerticalRead: ref(true),
    isOverlayOpen: ref(false),
    mobileChromeVisible: ref(false),
    currentIndex,
    page,
    progressVersion,
    currentVisibleParagraph: () => null,
    scrollBehavior: () => 'smooth',
    nextPage: async () => calls.push(['next']),
    saveProgress: () => calls.push(['save']),
    notify: (...args) => calls.push(['notify', ...args]),
    createAutoReading: options => {
      adapterOptions = options
      return { active: ref(false), stop() {}, toggle() {} }
    },
    ...overrides,
  })
  return {
    adapterOptions,
    calls,
    controller,
    currentIndex,
    page,
    progressVersion,
    reader,
  }
}

test('maps reader settings and pause state into the generic auto-reader', () => {
  const fixture = createController()
  assert.deepEqual(fixture.adapterOptions.settings(), {
    method: '像素滚动',
    pixel: 4,
    interval: 900,
    fontSize: 18,
    lineHeight: 1.8,
  })
  assert.equal(fixture.adapterOptions.shouldPause(), false)

  const overlay = ref(true)
  const paused = createController({
    isOverlayOpen: overlay,
    mobileChromeVisible: computed(() => false),
  })
  assert.equal(paused.adapterOptions.shouldPause(), true)
})

test('reports whether the generic next-page action advanced reader state', async () => {
  const fixture = createController()
  assert.equal(await fixture.adapterOptions.advancePage(), false)

  const advanced = createController({
    nextPage: async () => {
      advanced.page.value += 1
      advanced.calls.push(['next'])
    },
  })
  assert.equal(await advanced.adapterOptions.advancePage(), true)
  assert.deepEqual(advanced.calls, [['next']])
})

test('records local progress and keeps the reader toast duration', () => {
  const fixture = createController()
  fixture.adapterOptions.onProgress()
  fixture.adapterOptions.onNotify('自动阅读已开始')
  assert.equal(fixture.progressVersion.value, 6)
  assert.deepEqual(fixture.calls, [
    ['save'],
    ['notify', '自动阅读已开始', 1200],
  ])
})
