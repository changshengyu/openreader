import assert from 'node:assert/strict'
import test from 'node:test'
import { reactive, ref } from 'vue'
import { useReaderLayout } from '../src/composables/useReaderLayout.js'

function createController() {
  const reader = reactive({ mode: 'flip' })
  const contentEl = ref({
    clientWidth: 640,
    clientHeight: 480,
    scrollHeight: 1500,
    scrollTop: 500,
  })
  const contentBody = ref({ scrollWidth: 1900 })
  const state = {
    reader,
    contentEl,
    contentBody,
    page: ref(4),
    pageCount: ref(1),
    pageWidth: ref(0),
    pageHeight: ref(0),
    windowWidth: ref(800),
  }
  const windowTarget = {
    innerWidth: 1200,
    innerHeight: 900,
    getComputedStyle: () => ({
      paddingLeft: '20px',
      paddingRight: '20px',
      paddingTop: '10px',
      paddingBottom: '10px',
    }),
  }
  const controller = useReaderLayout({
    ...state,
    windowTarget,
    getScrollStep: () => 400,
    getViewportWidth: () => 1024,
  })
  return { controller, state }
}

test('calculates mobile flip pages from the upstream readable stride minus 16px', () => {
  const fixture = createController()
  fixture.controller.update()
  assert.equal(fixture.state.pageWidth.value, 584)
  assert.equal(fixture.state.pageHeight.value, 460)
  assert.equal(fixture.state.pageCount.value, 4)
  assert.equal(fixture.state.page.value, 3)
})

test('calculates vertical pages for paged and native continuous modes', () => {
  const fixture = createController()
  fixture.state.reader.mode = 'page'
  fixture.controller.update()
  assert.equal(fixture.state.pageHeight.value, 400)
  assert.equal(fixture.state.pageCount.value, 4)
  assert.equal(fixture.state.page.value, 1)

  fixture.state.reader.mode = 'scroll2'
  fixture.controller.update()
  assert.equal(fixture.state.pageHeight.value, 400)
  assert.equal(fixture.state.pageCount.value, 4)
  assert.equal(fixture.state.page.value, 1)

  fixture.state.reader.mode = 'scroll'
  fixture.controller.update()
  assert.equal(fixture.state.pageCount.value, 4)
  assert.equal(fixture.state.page.value, 1)
})

test('updates responsive width before recalculating layout on resize', () => {
  const fixture = createController()
  fixture.controller.resize()
  assert.equal(fixture.state.windowWidth.value, 1024)
  assert.equal(fixture.state.pageWidth.value, 584)
})
