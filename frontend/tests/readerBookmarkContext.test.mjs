import assert from 'node:assert/strict'
import test from 'node:test'
import {
  captureReaderBookmarkExcerpt,
  findReaderBookmarkParagraph,
  selectedTextBookmarkContext,
} from '../src/utils/readerBookmarkContext.js'

const paragraphs = [
  { text: '第一段，春风过处，纸页微明。' },
  { text: '第二段写下后续内容。' },
  { text: '第三段继续展开故事。' },
  { text: '第四段仍在继续。' },
  { text: '第五段到达上限。' },
  { text: '第六段不应收入书签。' },
]

test('locates one or two selected upstream paragraphs despite punctuation and whitespace', () => {
  const match = findReaderBookmarkParagraph({
    selectedText: '第一段 春风过处 纸页微明\n第二段写下后续内容',
    paragraphs,
  })
  assert.equal(match?.index, 0)
  assert.equal(match?.similarity, 1)
})

test('captures the upstream paragraph context window for selected text bookmarks', () => {
  assert.deepEqual(selectedTextBookmarkContext({
    selectedText: '第二段写下后续内容。',
    paragraphs,
  }), {
    index: 1,
    excerpt: [
      '第二段写下后续内容。',
      '第三段继续展开故事。',
      '第四段仍在继续。',
      '第五段到达上限。',
      '第六段不应收入书签。',
    ].join('\n'),
  })
  assert.equal(captureReaderBookmarkExcerpt(paragraphs, 0, { maxCharacters: 12 }), '第一段，春风过处，纸页微明。')
})

test('rejects selections that cannot be located as one or two reader paragraphs', () => {
  assert.equal(selectedTextBookmarkContext({
    selectedText: '不存在的文本',
    paragraphs,
  }), null)
  assert.equal(selectedTextBookmarkContext({
    selectedText: '第一段\n第二段\n第三段',
    paragraphs,
  }), null)
})
