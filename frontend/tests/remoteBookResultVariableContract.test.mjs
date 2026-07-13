import assert from 'node:assert/strict'
import test from 'node:test'

import { remoteBookCreatePayload, remoteBookVariable } from '../src/utils/remoteBookResult.js'

test('search-result variable remains opaque but survives add-to-shelf payload construction', () => {
  const book = {
    title: '变量书籍',
    sourceId: 7,
    bookUrl: 'https://source.example/book/7',
    variable: '{"searchToken":"opaque"}',
  }

  assert.equal(remoteBookVariable(book), '{"searchToken":"opaque"}')
  assert.equal(remoteBookVariable({ variable: { searchToken: 'invalid' } }), '')
  assert.deepEqual(remoteBookCreatePayload(book, [3]), {
    title: '变量书籍',
    author: '',
    coverUrl: '',
    intro: '',
    kind: '',
    wordCount: '',
    bookUrl: 'https://source.example/book/7',
    sourceId: 7,
    sourceName: '未知书源',
    variable: '{"searchToken":"opaque"}',
    categoryIds: [3],
  })
})
