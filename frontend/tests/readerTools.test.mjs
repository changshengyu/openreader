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
      top: () => calls.push(['top']),
      bottom: () => calls.push(['bottom']),
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

  fixture.controller.handleMobileChromeAction('bottom')
  fixture.controller.handleMobileChromeAction('top')
  assert.equal(fixture.mobileChromeVisible.value, true)
  assert.deepEqual(fixture.calls.slice(-2), [['bottom'], ['top']])
})

// Desktop panel content controls contract P7 (section 13): the progress block's 上一章/下一章 run with every primary
// panel closed first, as a press outside reader-dev's click popovers closes them, at the book boundaries too.
test('desktop chapter steps close every primary panel before they step', () => {
  const calls = []
  const currentIndex = ref(0)
  const controller = useReaderTools({
    currentIndex,
    chapterCount: () => 3,
    mobileChromeVisible: ref(false),
    goChapter: index => calls.push(['chapter', index]),
    toggleChrome: () => calls.push(['toggle']),
    notify: message => calls.push(['notify', message]),
    closePrimaryPanels: () => calls.push(['close']),
    actions: {},
  })
  controller.handleDesktopToolAction('next')
  controller.handleDesktopToolAction('previous')
  currentIndex.value = 2
  controller.handleDesktopToolAction('next')
  assert.deepEqual(calls, [
    ['close'],
    ['chapter', 1],
    ['close'],
    ['notify', '本章是第一章'],
    ['close'],
    ['notify', '本章是最后一章'],
  ])
})
