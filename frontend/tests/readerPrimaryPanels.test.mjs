import assert from 'node:assert/strict'
import test from 'node:test'
import { ref } from 'vue'
import { useReaderPrimaryPanels } from '../src/composables/useReaderPrimaryPanels.js'

function createController() {
  const calls = []
  const state = {
    shelf: ref(false),
    source: ref(false),
    toc: ref(false),
    settings: ref(false),
    mobileChromeVisible: ref(true),
  }
  const controller = useReaderPrimaryPanels({
    panels: {
      shelf: state.shelf,
      source: state.source,
      toc: state.toc,
      settings: state.settings,
    },
  })
  const open = name => controller.toggle(name, () => {
    calls.push(['open', name])
    state[name].value = true
  })
  return { calls, controller, open, state }
}

test('toggles the same mobile primary tool without changing reader chrome', () => {
  const fixture = createController()

  assert.equal(fixture.open('shelf'), true)
  assert.equal(fixture.state.shelf.value, true)
  assert.equal(fixture.state.mobileChromeVisible.value, true)
  assert.deepEqual(fixture.calls, [['open', 'shelf']])

  assert.equal(fixture.open('shelf'), false)
  assert.equal(fixture.state.shelf.value, false)
  assert.equal(fixture.state.mobileChromeVisible.value, true)
  assert.deepEqual(fixture.calls, [['open', 'shelf']])
})

test('switches mobile primary tools atomically so only one workspace remains open', () => {
  const fixture = createController()

  fixture.open('shelf')
  fixture.open('source')
  assert.equal(fixture.state.shelf.value, false)
  assert.equal(fixture.state.source.value, true)
  assert.equal(fixture.state.toc.value, false)
  assert.equal(fixture.state.settings.value, false)
  assert.equal(fixture.controller.isOpen(), true)
  assert.deepEqual(fixture.calls, [['open', 'shelf'], ['open', 'source']])

  fixture.controller.close()
  assert.equal(fixture.controller.isOpen(), false)
  assert.equal(fixture.state.mobileChromeVisible.value, true)
})

test('rejects unknown primary-panel names without mutating visible panels', () => {
  const fixture = createController()
  fixture.state.toc.value = true

  assert.equal(fixture.open('bookmarks'), false)
  assert.equal(fixture.state.toc.value, true)
  assert.deepEqual(fixture.calls, [])
})
