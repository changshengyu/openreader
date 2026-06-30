import assert from 'node:assert/strict'
import test from 'node:test'
import { nextReaderBlock, paragraphAutoReadDelay } from '../src/utils/readerAutoReading.js'

test('finds the next rendered reader block', () => {
  const first = { id: 1 }
  const second = { id: 2 }
  const container = {
    querySelectorAll: () => [first, second],
  }

  assert.equal(nextReaderBlock(container, null), first)
  assert.equal(nextReaderBlock(container, first), second)
  assert.equal(nextReaderBlock(container, second), null)
})

test('scales paragraph auto-reading delay by estimated line count', () => {
  assert.equal(paragraphAutoReadDelay({
    paragraphHeight: 65,
    fontSize: 20,
    lineHeight: 1.5,
    baseDelay: 800,
  }), 2400)
  assert.equal(paragraphAutoReadDelay({
    paragraphHeight: 0,
    fontSize: 20,
    lineHeight: 1.5,
    baseDelay: 800,
  }), 800)
})
