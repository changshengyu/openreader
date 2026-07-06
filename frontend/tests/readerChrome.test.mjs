import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderChrome } from '../src/composables/useReaderChrome.js'

function createController({ mobile = false, toc = false } = {}) {
  const calls = []
  const state = {
    isMobileReader: ref(mobile),
    mobileChromeVisible: ref(true),
    tocVisible: ref(toc),
    settingsVisible: ref(true),
  }
  const controller = useReaderChrome({
    ...state,
    openToc: () => {
      calls.push(['open-toc'])
      state.tocVisible.value = true
    },
  })
  return { calls, controller, state }
}

test('toggles only the mobile reader chrome on compact screens', () => {
  const fixture = createController({ mobile: true })
  fixture.controller.toggle()
  assert.equal(fixture.state.mobileChromeVisible.value, false)
  assert.equal(fixture.state.settingsVisible.value, true)
  assert.deepEqual(fixture.calls, [])
})

test('opens or closes the TOC and always closes desktop settings', () => {
  const closed = createController()
  closed.controller.toggle()
  assert.equal(closed.state.tocVisible.value, true)
  assert.equal(closed.state.settingsVisible.value, false)
  assert.deepEqual(closed.calls, [['open-toc']])

  const opened = createController({ toc: true })
  opened.controller.toggle()
  assert.equal(opened.state.tocVisible.value, false)
  assert.equal(opened.state.settingsVisible.value, false)
  assert.deepEqual(opened.calls, [])
})
