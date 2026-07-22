import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive, ref } from 'vue'
import { createReaderPageLifecycle } from '../src/composables/useReaderPageLifecycle.js'

class FakeEventTarget {
  constructor() {
    this.listeners = new Map()
    this.added = []
    this.removed = []
  }

  addEventListener(type, handler, options) {
    this.listeners.set(type, handler)
    this.added.push([type, options])
  }

  removeEventListener(type, handler) {
    if (this.listeners.get(type) === handler) this.listeners.delete(type)
    this.removed.push(type)
  }

  emit(type) {
    this.listeners.get(type)?.({ type })
  }
}

function createLifecycle(overrides = {}) {
  const calls = []
  const windowTarget = new FakeEventTarget()
  const documentTarget = new FakeEventTarget()
  const reader = reactive({
    customBgColor: '#f5ecd2',
    customFontsMap: { reader: '/fonts/reader.woff2' },
    lineHeight: 2,
    normalizeSettings: () => calls.push(['normalize']),
  })
  const handlers = Object.fromEntries([
    'Resize',
    'Wheel',
    'PageHide',
    'VisibilityChange',
    'ProgressUpdated',
    'BookDataUpdated',
    'ReplaceRulesUpdated',
    'BookmarksUpdated',
    'BooksDeleted',
  ].map(name => [`on${name}`, () => calls.push([name])]))
  const customBg = ref('')
  const sliderLineHeight = ref(0)
  const lifecycle = createReaderPageLifecycle({
    reader,
    customBg,
    sliderLineHeight,
    windowTarget,
    documentTarget,
    syncFonts: fonts => calls.push(['fonts', fonts]),
    loadBook: async () => calls.push(['load']),
    onBookLoadError: error => calls.push(['error', error.message]),
    cancelProgressSave: () => calls.push(['cancel']),
    clearChapterLoadingTimer: () => calls.push(['clear-timer']),
    stopAutoReading: () => calls.push(['stop-auto']),
    saveProgress: options => calls.push(['save', options]),
    ...handlers,
    ...overrides,
  })
  return {
    calls,
    customBg,
    documentTarget,
    lifecycle,
    sliderLineHeight,
    windowTarget,
  }
}

test('registers deletion and page listeners before awaiting the initial book load', async () => {
  let finishLoad
  const controller = createLifecycle({
    loadBook: () => new Promise(resolve => {
      finishLoad = () => {
        controller.calls.push(['load'])
        resolve()
      }
    }),
  })
  const mounting = controller.lifecycle.mount()
  await Promise.resolve()
  controller.windowTarget.emit('openreader:books-deleted')
  assert.equal(controller.calls.at(-1)?.[0], 'BooksDeleted')
  finishLoad()
  await mounting
  assert.deepEqual(controller.calls, [
    ['normalize'],
    ['fonts', { reader: '/fonts/reader.woff2' }],
    ['BooksDeleted'],
    ['load'],
  ])
  assert.equal(controller.customBg.value, '#f5ecd2')
  assert.equal(controller.sliderLineHeight.value, 2)
  assert.deepEqual(controller.windowTarget.added, [
    ['resize', undefined],
    ['wheel', { passive: false }],
    ['pagehide', undefined],
    ['openreader:progress-updated', undefined],
    ['openreader:reader-book-data-updated', undefined],
    ['openreader:replace-rules-updated', undefined],
    ['openreader:bookmarks-updated', undefined],
    ['openreader:books-deleted', undefined],
  ])
  assert.deepEqual(controller.documentTarget.added, [
    ['visibilitychange', undefined],
  ])
})

test('reports initial load errors and still activates page events', async () => {
  const controller = createLifecycle({
    loadBook: async () => {
      throw new Error('load failed')
    },
  })
  await controller.lifecycle.mount()
  assert.deepEqual(controller.calls.slice(0, 3), [
    ['normalize'],
    ['fonts', { reader: '/fonts/reader.woff2' }],
    ['error', 'load failed'],
  ])
  controller.windowTarget.emit('resize')
  assert.deepEqual(controller.calls.at(-1), ['Resize'])
})

test('saves progress and removes every listener during teardown', async () => {
  const controller = createLifecycle()
  await controller.lifecycle.mount()
  controller.calls.length = 0
  controller.lifecycle.unmount()
  assert.deepEqual(controller.calls, [
    ['cancel'],
    ['clear-timer'],
    ['stop-auto'],
    ['save', { force: true, background: true }],
  ])
  assert.deepEqual(controller.windowTarget.removed, [
    'resize',
    'wheel',
    'pagehide',
    'openreader:progress-updated',
    'openreader:reader-book-data-updated',
    'openreader:replace-rules-updated',
    'openreader:bookmarks-updated',
    'openreader:books-deleted',
  ])
  assert.deepEqual(controller.documentTarget.removed, ['visibilitychange'])
  controller.windowTarget.emit('resize')
  assert.equal(controller.calls.length, 4)
})
