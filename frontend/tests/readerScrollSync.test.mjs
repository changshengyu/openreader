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
})
