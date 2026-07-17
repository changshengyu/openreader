import assert from 'node:assert/strict'
import test from 'node:test'
import { createShelfRequestRevisionGate } from '../src/utils/shelfRequestRevision.js'

test('rejects an older shelf response after a newer force refresh starts', () => {
  const gate = createShelfRequestRevisionGate()
  const oldRequest = gate.begin('user:1')
  const newRequest = gate.begin('user:1')

  assert.equal(gate.canCommit(oldRequest, 'user:1'), false)
  assert.equal(gate.canCommit(newRequest, 'user:1'), true)
})

test('rejects an in-flight shelf response after a local import upsert', () => {
  const gate = createShelfRequestRevisionGate()
  const request = gate.begin('user:1')
  gate.mutate('user:1')

  assert.equal(gate.canCommit(request, 'user:1'), false)
})

test('rejects a previous user response after the authenticated scope changes', () => {
  const gate = createShelfRequestRevisionGate()
  const request = gate.begin('user:1')
  gate.reset('user:2')

  assert.equal(gate.canCommit(request, 'user:2'), false)
})
