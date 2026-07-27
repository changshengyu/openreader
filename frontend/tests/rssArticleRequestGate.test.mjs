import assert from 'node:assert/strict'
import test from 'node:test'

import { createRSSArticleRequestGate } from '../src/utils/rssArticleRequestGate.js'

function query(overrides = {}) {
  return {
    rootVisible: true,
    listVisible: true,
    sourceId: 1,
    sort: '默认',
    filter: 'all',
    page: 1,
    ...overrides,
  }
}

test('only the latest RSS article request can commit to its captured query', () => {
  const gate = createRSSArticleRequestGate()
  const sourceA = query({ sourceId: 1 })
  const requestA = gate.begin(sourceA)
  const sourceB = query({ sourceId: 2 })
  const requestB = gate.begin(sourceB)

  assert.equal(gate.isCurrent(requestA, sourceA), false)
  assert.equal(gate.isCurrent(requestB, sourceB), true)
  assert.equal(gate.isCurrent(requestB, sourceA), false)
})

test('closing or resetting the RSS article scene invalidates pending requests', () => {
  const gate = createRSSArticleRequestGate()
  const request = gate.begin(query())

  gate.invalidate()

  assert.equal(gate.isCurrent(request, query()), false)
})

test('sort, filter and page are part of RSS article request ownership', () => {
  const gate = createRSSArticleRequestGate()
  const request = gate.begin(query())

  assert.equal(gate.isCurrent(request, query({ sort: '科技' })), false)
  assert.equal(gate.isCurrent(request, query({ filter: 'unread' })), false)
  assert.equal(gate.isCurrent(request, query({ page: 2 })), false)
})
