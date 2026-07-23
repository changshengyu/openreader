import assert from 'node:assert/strict'
import test from 'node:test'
import { effectScope, nextTick, reactive, ref } from 'vue'
import { useReaderTypographySync } from '../src/composables/useReaderTypographySync.js'

async function flushTypographySync() {
  await nextTick()
  await Promise.resolve()
  await nextTick()
}

function createController(overrides = {}) {
  const reader = reactive({
    fontFamily: 'system',
    fontSize: 18,
    customFontsMap: {},
  })
  const calls = []
  const progressVersion = ref(0)
  const scope = effectScope()
  const controller = scope.run(() => useReaderTypographySync({
    reader,
    progressVersion,
    getCurrentOffset: () => 320,
    getCurrentPercent: () => 0.42,
    setRestoring: value => calls.push(['restoring', value]),
    updateLayout: () => calls.push(['layout']),
    restorePosition: async (...args) => calls.push(['restore', ...args]),
    scheduleProgressSave: delay => calls.push(['schedule', delay]),
    syncFonts: fonts => calls.push(['fonts', fonts]),
    ...overrides,
  }))
  return { calls, controller, progressVersion, reader, scope }
}

test('restores the same reading position after typography changes', async () => {
  const controller = createController()
  controller.reader.fontSize = 20
  await flushTypographySync()
  assert.deepEqual(controller.calls, [
    ['restoring', true],
    ['layout'],
    ['restore', 320, { restorePercent: 0.42, saveAfterLoad: false }],
    ['restoring', false],
    ['schedule', 300],
  ])
  assert.equal(controller.progressVersion.value, 1)
  controller.scope.stop()
})

test('clears the restoring guard when position restoration fails', async () => {
  const controller = createController({
    restorePosition: async () => {
      throw new Error('restore failed')
    },
  })
  await assert.rejects(() => controller.controller.syncPosition(), /restore failed/)
  assert.deepEqual(controller.calls, [
    ['restoring', true],
    ['layout'],
    ['restoring', false],
  ])
  assert.equal(controller.progressVersion.value, 0)
  controller.scope.stop()
})

test('synchronizes deeply changed custom font maps', async () => {
  const controller = createController()
  controller.reader.customFontsMap.reader = '/uploads/reader.woff2'
  await flushTypographySync()
  assert.deepEqual(controller.calls, [
    ['fonts', { reader: '/uploads/reader.woff2' }],
  ])
  controller.scope.stop()
})

test('can delegate layout positioning to the unified mode transaction without duplicating restores', async () => {
  const controller = createController({ watchPosition: false })
  controller.reader.fontSize = 22
  await flushTypographySync()
  assert.deepEqual(controller.calls, [])
  controller.scope.stop()
})
