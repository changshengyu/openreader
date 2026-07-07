import assert from 'node:assert/strict'
import test from 'node:test'
import { useAppMobileNavigation } from '../src/composables/useAppMobileNavigation.js'

function createController(overrides = {}) {
  let viewportWidth = 390
  let now = 1000
  const controller = useAppMobileNavigation({
    currentViewportWidth: () => viewportWidth,
    getViewportWidth: () => viewportWidth,
    getViewportHeight: () => 844,
    getPageMode: () => 'auto',
    shouldUseMiniInterface: (_mode, width) => width <= 750,
    now: () => now,
    ...overrides,
  })
  return {
    controller,
    setNow: value => {
      now = value
    },
    setViewportWidth: value => {
      viewportWidth = value
    },
  }
}

function touchEvent(x, y) {
  const calls = []
  return {
    touches: [{ clientX: x, clientY: y }],
    preventDefault: () => calls.push('prevent'),
    stopPropagation: () => calls.push('stop'),
    calls,
  }
}

test('derives mobile mode and updates the viewport width', () => {
  const fixture = createController()
  assert.equal(fixture.controller.isMobile.value, true)
  assert.deepEqual(fixture.controller.navigationStyle.value, {
    '--mobile-nav-width': '260px',
  })

  fixture.setViewportWidth(1024)
  fixture.controller.updateViewport()
  assert.equal(fixture.controller.isMobile.value, false)
  fixture.controller.toggle()
  assert.equal(fixture.controller.visible.value, false)
})

test('opens from a horizontal drag and suppresses its trailing workspace click', () => {
  const fixture = createController()
  fixture.controller.handleTouchStart(touchEvent(100, 200))
  const move = touchEvent(180, 204)
  fixture.controller.handleTouchMove(move)

  assert.deepEqual(move.calls, ['prevent', 'stop'])
  assert.equal(fixture.controller.touchAxis.value, 'x')
  assert.deepEqual(fixture.controller.navigationStyle.value, {
    '--mobile-nav-width': '260px',
    '--mobile-nav-drag-offset': '-190px',
    marginLeft: '-190px',
    transition: 'none',
  })

  fixture.controller.handleTouchEnd()
  assert.equal(fixture.controller.visible.value, true)
  fixture.controller.close()
  assert.equal(fixture.controller.visible.value, true)
  fixture.setNow(1351)
  fixture.controller.close()
  assert.equal(fixture.controller.visible.value, false)
})

test('uses the upstream 270px drag window while keeping a 260px sidebar width', () => {
  const fixture = createController()
  assert.equal(fixture.controller.navigationWidth.value, 260)
  assert.equal(fixture.controller.dragLimit.value, 270)

  fixture.controller.handleTouchStart(touchEvent(60, 220))
  const fullOpenMove = touchEvent(330, 222)
  fixture.controller.handleTouchMove(fullOpenMove)

  assert.deepEqual(fullOpenMove.calls, ['prevent', 'stop'])
  assert.equal(fixture.controller.touchMoveX.value, 270)
  assert.deepEqual(fixture.controller.navigationStyle.value, {
    '--mobile-nav-width': '260px',
    '--mobile-nav-drag-offset': '0px',
    marginLeft: '0px',
    transition: 'none',
  })

  const beyondLimitMove = touchEvent(331, 222)
  fixture.controller.handleTouchMove(beyondLimitMove)
  assert.deepEqual(beyondLimitMove.calls, [])
  assert.equal(fixture.controller.touchMoveX.value, 270)

  fixture.controller.handleTouchEnd()
  assert.equal(fixture.controller.visible.value, true)

  fixture.controller.handleTouchStart(touchEvent(330, 220))
  const fullCloseMove = touchEvent(60, 222)
  fixture.controller.handleTouchMove(fullCloseMove)
  assert.deepEqual(fullCloseMove.calls, ['prevent', 'stop'])
  assert.equal(fixture.controller.touchMoveX.value, -270)
  assert.deepEqual(fixture.controller.navigationStyle.value, {
    '--mobile-nav-width': '260px',
    '--mobile-nav-drag-offset': '-270px',
    marginLeft: '-270px',
    transition: 'none',
  })
})

test('closes an open sidebar with a left drag', () => {
  const fixture = createController()
  fixture.controller.visible.value = true
  fixture.controller.handleTouchStart(touchEvent(220, 240))
  const move = touchEvent(120, 242)
  fixture.controller.handleTouchMove(move)

  assert.deepEqual(fixture.controller.navigationStyle.value, {
    '--mobile-nav-width': '260px',
    '--mobile-nav-drag-offset': '-100px',
    marginLeft: '-100px',
    transition: 'none',
  })
  fixture.controller.handleTouchEnd()
  assert.equal(fixture.controller.visible.value, false)
})

test('ignores edge touches and vertical scrolling gestures', () => {
  const fixture = createController()
  fixture.controller.handleTouchStart(touchEvent(10, 200))
  assert.equal(fixture.controller.touchStart.value, null)

  fixture.controller.handleTouchStart(touchEvent(100, 200))
  const move = touchEvent(104, 250)
  fixture.controller.handleTouchMove(move)
  assert.equal(fixture.controller.touchAxis.value, 'y')
  assert.equal(fixture.controller.touchMoveX.value, 0)
  assert.deepEqual(move.calls, [])
  fixture.controller.handleTouchCancel()
  assert.equal(fixture.controller.touchAxis.value, '')
})
