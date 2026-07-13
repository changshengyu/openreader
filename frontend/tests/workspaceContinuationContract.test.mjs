import assert from 'node:assert/strict'
import test from 'node:test'

import {
  captureWorkspaceRequest,
  createAsyncRequestGate,
  isWorkspaceRequestCurrent,
  mergeRemoteSearchResults,
} from '../src/utils/workspaceContinuation.js'

test('merges remote search pages by reader-dev bookUrl without collapsing unrelated blank URLs', () => {
  const first = mergeRemoteSearchResults([], [
    { sourceId: 1, bookUrl: 'https://source.example/shared', title: '同一本', author: '甲' },
    { sourceId: 1, bookUrl: '', title: '缺失链接甲', author: '甲' },
  ])
  const second = mergeRemoteSearchResults(first.rows, [
    { sourceId: 2, bookUrl: 'https://source.example/shared', title: '同一本副本', author: '乙' },
    { sourceId: 1, bookUrl: '', title: '缺失链接乙', author: '乙' },
    { sourceId: 2, bookUrl: '', title: '缺失链接甲', author: '甲' },
  ])

  assert.equal(first.added, 2)
  assert.equal(second.added, 2)
  assert.deepEqual(second.rows.map(item => item.title), ['同一本', '缺失链接甲', '缺失链接乙', '缺失链接甲'])
})

test('rejects stale result mutations after a new workspace request or scene transition', () => {
  const gate = createAsyncRequestGate()
  const workspace = { mode: 'search', searchRevision: 4, exploreRevision: 1 }
  const first = gate.begin()
  const firstStamp = captureWorkspaceRequest(workspace, 'search')
  const second = gate.begin()

  assert.equal(gate.isCurrent(first), false)
  assert.equal(gate.isCurrent(second), true)
  assert.equal(isWorkspaceRequestCurrent(workspace, firstStamp), true)

  workspace.mode = 'explore'
  assert.equal(isWorkspaceRequestCurrent(workspace, firstStamp), false)
  workspace.mode = 'search'
  workspace.searchRevision += 1
  assert.equal(isWorkspaceRequestCurrent(workspace, firstStamp), false)
})
