import assert from 'node:assert/strict'
import test from 'node:test'
import {
  didReaderTouchMove,
  isReaderTouchTap,
  normalizedReaderWheelDelta,
  readerTapPointAction,
  readerTapZoneAction,
  shouldHandleReaderHorizontalSwipe,
  shouldPreventReaderTouchMove,
} from '../src/utils/readerInteraction.js'

test('keeps reader touch movement and swipe thresholds stable', () => {
  assert.equal(didReaderTouchMove({ x: 14, y: 0 }), false)
  assert.equal(didReaderTouchMove({ x: 14.01, y: 0 }), true)
  assert.equal(isReaderTouchTap({ move: { x: 8, y: 8 }, elapsed: 649, hasTouch: true }), true)
  assert.equal(isReaderTouchTap({ move: { x: 0, y: 0 }, elapsed: 650, hasTouch: true }), false)

  assert.equal(shouldPreventReaderTouchMove({ mode: 'flip', moveX: 21, moveY: 0 }), true)
  assert.equal(shouldPreventReaderTouchMove({ mode: 'flip', moveX: 20, moveY: 12 }), false)
  assert.equal(shouldPreventReaderTouchMove({ mode: 'scroll', moveX: 100, moveY: 0 }), false)

  assert.equal(shouldHandleReaderHorizontalSwipe({ mode: 'flip', move: { x: 42, y: 30 } }), true)
  assert.equal(shouldHandleReaderHorizontalSwipe({ mode: 'flip', move: { x: 42, y: 35 } }), false)
  assert.equal(shouldHandleReaderHorizontalSwipe({ mode: 'page', move: { x: 100, y: 0 } }), false)
})

test('maps mobile and desktop reader tap points without changing click modes', () => {
  const base = {
    pointX: 50,
    pointY: 50,
    viewportWidth: 100,
    viewportHeight: 100,
    clickMethod: 'default',
    mode: 'flip',
    autoReading: false,
  }

  assert.equal(readerTapPointAction({ ...base, mobile: true }), 'toggle-chrome')
  assert.equal(readerTapPointAction({ ...base, mobile: false }), null)
  assert.equal(readerTapPointAction({ ...base, mobile: true, pointX: 80 }), 'next')
  assert.equal(readerTapPointAction({ ...base, mobile: true, pointX: 20 }), 'previous')
  assert.equal(readerTapPointAction({ ...base, mobile: true, pointX: 10, clickMethod: 'next' }), 'next')
  assert.equal(readerTapPointAction({ ...base, mobile: true, pointX: 10, clickMethod: 'none' }), 'toggle-chrome')
  assert.equal(readerTapPointAction({ ...base, mobile: false, pointX: 10, clickMethod: 'none' }), null)
  assert.equal(readerTapPointAction({ ...base, mobile: true, pointX: 10, autoReading: true }), 'toggle-chrome')
  assert.equal(readerTapPointAction({
    ...base,
    mobile: true,
    mode: 'page',
    pointX: 50,
    pointY: 80,
  }), 'next')
})

test('maps explicit reader click zones to the existing actions', () => {
  assert.equal(readerTapZoneAction({
    zone: 'center',
    clickMethod: 'default',
    mode: 'flip',
    autoReading: false,
  }), 'toggle-chrome')
  assert.equal(readerTapZoneAction({
    zone: 'left',
    clickMethod: 'default',
    mode: 'flip',
    autoReading: false,
  }), 'previous')
  assert.equal(readerTapZoneAction({
    zone: 'lower',
    clickMethod: 'default',
    mode: 'page',
    autoReading: false,
  }), 'next')
  assert.equal(readerTapZoneAction({
    zone: 'upper',
    clickMethod: 'next',
    mode: 'page',
    autoReading: false,
  }), 'next')
})

test('normalizes pixel, line, and page wheel deltas', () => {
  assert.equal(normalizedReaderWheelDelta({
    deltaX: 2,
    deltaY: -8,
    deltaMode: 0,
  }), -8)
  assert.equal(normalizedReaderWheelDelta({
    deltaX: 0,
    deltaY: 2,
    deltaMode: 1,
    fontSize: 20,
    lineHeight: 1.5,
  }), 60)
  assert.equal(normalizedReaderWheelDelta({
    deltaX: -1,
    deltaY: 0,
    deltaMode: 2,
    pageHeight: 720,
  }), -720)
})
