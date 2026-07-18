import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderScrollSync } from '../src/composables/useReaderScrollSync.js'

function createController(overrides = {}) {
  const calls = []
  const progressVersion = ref(3)
  const controller = useReaderScrollSync({
    isVerticalRead: ref(true),
    restoringPosition: ref(false),
    chapterLoading: ref(false),
    progressVersion,
    syncCurrentChapter: () => calls.push(['sync-chapter']),
    maybeExtendChapterWindow: () => calls.push(['extend-window']),
    updateLayout: () => calls.push(['layout']),
    applyLocalProgress: () => calls.push(['local-progress']),
    scheduleProgressSave: delay => calls.push(['schedule', delay]),
    ...overrides,
  })
  return { calls, controller, progressVersion }
}

test('synchronizes chapter, layout, and progress after vertical scrolling', () => {
  const fixture = createController()
  fixture.controller.handle()
  assert.equal(fixture.progressVersion.value, 4)
  assert.deepEqual(fixture.calls, [
    ['sync-chapter'],
    ['extend-window'],
    ['layout'],
    ['local-progress'],
    ['schedule', 500],
  ])
})

test('ignores scroll events outside vertical mode or during restoration', () => {
  const horizontal = createController({ isVerticalRead: ref(false) })
  horizontal.controller.handle()
  assert.deepEqual(horizontal.calls, [])
  assert.equal(horizontal.progressVersion.value, 3)

  const restoring = createController({ restoringPosition: ref(true) })
  restoring.controller.handle()
  assert.deepEqual(restoring.calls, [])

  const loading = createController({ chapterLoading: ref(true) })
  loading.controller.handle()
  assert.deepEqual(loading.calls, [])

  const replacingWindow = createController({ windowBusy: ref(true) })
  replacingWindow.controller.handle()
  assert.deepEqual(replacingWindow.calls, [])
  assert.equal(replacingWindow.progressVersion.value, 3)
})

test('does not persist the scroll event that starts a window transaction', () => {
  const windowBusy = ref(false)
  const fixture = createController({
    windowBusy,
    maybeExtendChapterWindow: () => {
      fixture.calls.push(['extend-window'])
      windowBusy.value = true
    },
  })

  fixture.controller.handle()
  assert.deepEqual(fixture.calls, [
    ['sync-chapter'],
    ['extend-window'],
    ['layout'],
  ])
  assert.equal(fixture.progressVersion.value, 3)
})

test('defers heavy scroll synchronization during page animation and settles it once', () => {
  const pageAnimationActive = ref(true)
  const scrollTop = ref(772)
  const fixture = createController({
    pageAnimationActive,
    scrollPosition: () => scrollTop.value,
  })

  fixture.controller.handle()
  fixture.controller.handle()
  assert.deepEqual(fixture.calls, [])
  assert.equal(fixture.progressVersion.value, 3)

  pageAnimationActive.value = false
  assert.equal(fixture.controller.flush(), true)
  assert.equal(fixture.controller.flush(), false)
  fixture.controller.handle()
  assert.equal(fixture.progressVersion.value, 4)
  assert.deepEqual(fixture.calls, [
    ['sync-chapter'],
    ['extend-window'],
    ['layout'],
    ['local-progress'],
    ['schedule', 500],
  ])
})

test('settles a completed page animation even before its browser scroll event arrives', () => {
  const fixture = createController({
    pageAnimationActive: ref(false),
    scrollPosition: () => 600,
  })

  assert.equal(fixture.controller.flush(), true)
  assert.equal(fixture.controller.flush(), false)
  assert.equal(fixture.progressVersion.value, 4)
})
