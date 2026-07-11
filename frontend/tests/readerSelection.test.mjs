import assert from 'node:assert/strict'
import test from 'node:test'
import {
  normalizeReaderSelectionText,
  readerSelectionBelongsToRoot,
} from '../src/utils/readerSelection.js'

test('preserves original selected reader text while rejecting whitespace-only selections', () => {
  assert.equal(normalizeReaderSelectionText('  第一段\n\n第二段  '), '  第一段\n\n第二段  ')
  assert.equal(normalizeReaderSelectionText('abcdef', 4), 'abcd')
  assert.equal(normalizeReaderSelectionText(' \n '), '')
})

test('accepts selection containers only inside the reader root', () => {
  const inside = {}
  const root = { contains: value => value === inside }
  assert.equal(readerSelectionBelongsToRoot(root, inside), true)
  assert.equal(readerSelectionBelongsToRoot(root, {}), false)
})
