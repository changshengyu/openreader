import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

import { sortByShelfOrder } from '../src/utils/bookOrder.js'

const homeSource = readFileSync(new URL('../src/views/Home.vue', import.meta.url), 'utf8')

test('shelf latest-chapter label uses only the upstream lastCheckTime field', () => {
  const helper = homeSource.match(/function latestChapterLabel\(book\) \{([\s\S]*?)\n\}/)?.[1] || ''
  assert.match(helper, /book\.lastCheckTime/)
  assert.doesNotMatch(helper, /shelfOrderAt/, 'last-reading order time must not be rendered as book-update time')
  assert.doesNotMatch(helper, /updatedAt/, 'generic metadata row time must not be rendered as book-update time')
})

test('shelf ordering remains independently driven by newest reading progress', () => {
  const orderSource = readFileSync(new URL('../src/utils/bookOrder.js', import.meta.url), 'utf8')
  assert.match(orderSource, /progressAt\s*=\s*toTime\(progressFor\(book, progressByBook\)\?\.updatedAt\)/)
  assert.match(orderSource, /Math\.max\(explicitShelfAt, progressAt, shelfAt\)/)
})

test('generic metadata edits do not move a book ahead of a newer shelf insertion', () => {
  const editedOldBook = {
    id: 1,
    createdAt: '2025-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
  }
  const newerShelfBook = {
    id: 2,
    createdAt: '2025-06-01T00:00:00.000Z',
    updatedAt: '2025-06-01T00:00:00.000Z',
  }
  assert.deepEqual(sortByShelfOrder([editedOldBook, newerShelfBook], {}).map(book => book.id), [2, 1])
})
