import assert from 'node:assert/strict'
import test from 'node:test'
import { sourceCandidateChangePayload } from '../src/utils/sourceCandidate.js'

test('normalizes an upstream source candidate into a change-source payload', () => {
  assert.deepEqual(sourceCandidateChangePayload({
    bookSourceId: 9,
    url: 'https://example.test/book',
    bookName: '候选书名',
    bookAuthor: '作者',
    bookCover: '/cover.jpg',
    description: '简介',
    category: '分类',
    words: '12万字',
  }, '原书名'), {
    sourceId: 9,
    bookUrl: 'https://example.test/book',
    title: '候选书名',
    author: '作者',
    coverUrl: '/cover.jpg',
    intro: '简介',
    kind: '分类',
    wordCount: '12万字',
  })
})

test('falls back to the current title when a candidate has no title', () => {
  assert.equal(sourceCandidateChangePayload({ sourceId: 2 }, '当前书名').title, '当前书名')
})
