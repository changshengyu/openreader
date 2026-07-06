import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderTools } from '../src/composables/useReaderTools.js'

function createController() {
  const calls = []
  const currentIndex = ref(3)
  const mobileChromeVisible = ref(true)
  const controller = useReaderTools({
    currentIndex,
    mobileChromeVisible,
    goChapter: index => calls.push(['chapter', index]),
    toggleChrome: () => calls.push(['toggle']),
    actions: {
      toc: () => calls.push(['toc']),
      settings: () => calls.push(['settings']),
    },
  })
  return {
    calls,
    controller,
    currentIndex,
    mobileChromeVisible,
  }
}

test('dispatches desktop tools through the shared action map', () => {
  const fixture = createController()
  fixture.controller.handleDesktopToolAction('toc')
  fixture.controller.handleDesktopToolAction('missing')
  assert.deepEqual(fixture.calls, [['toc']])
  assert.equal(typeof fixture.controller.resolve('settings'), 'function')
})

test('keeps mobile chrome navigation and direct tool actions distinct', () => {
  const fixture = createController()
  fixture.controller.handleMobileChromeAction('previous')
  fixture.controller.handleMobileChromeAction('next')
  fixture.controller.handleMobileChromeAction('toggle')
  assert.deepEqual(fixture.calls, [
    ['chapter', 2],
    ['chapter', 4],
    ['toggle'],
  ])

  fixture.mobileChromeVisible.value = true
  fixture.controller.handleMobileChromeAction('toc')
  assert.equal(fixture.mobileChromeVisible.value, true)
  assert.deepEqual(fixture.calls.at(-1), ['toc'])
})
