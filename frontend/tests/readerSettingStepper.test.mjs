import assert from 'node:assert/strict'
import test from 'node:test'
import {
  readerSettingStepLabel,
  steppedReaderSettingValue,
} from '../src/utils/readerSettingStepper.js'

test('steps typography settings without floating-point drift', () => {
  assert.equal(steppedReaderSettingValue({
    value: 1.8,
    direction: 1,
    min: 1,
    max: 5,
    step: 0.2,
  }), 2)
  assert.equal(steppedReaderSettingValue({
    value: 0.2,
    direction: -1,
    min: 0,
    max: 5,
    step: 0.2,
  }), 0)
})

test('clamps stepper values and formats compact labels', () => {
  assert.equal(steppedReaderSettingValue({
    value: 36,
    direction: 1,
    min: 8,
    max: 36,
    step: 1,
  }), 36)
  assert.equal(steppedReaderSettingValue({
    value: 100,
    direction: -1,
    min: 100,
    max: 900,
    step: 100,
  }), 100)
  assert.equal(readerSettingStepLabel(2, 0.2), '2')
  assert.equal(readerSettingStepLabel(400, 100), '400')
})
