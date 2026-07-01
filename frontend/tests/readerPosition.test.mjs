import assert from 'node:assert/strict'
import test from 'node:test'
import {
  readerChapterBoundaryScrollTop,
  readerParagraphAtPosition,
  READER_CHAPTER_END_OFFSET,
  restoredReaderContinuousScrollTop,
  restoredReaderFlipPage,
  restoredReaderSingleChapterScrollTop,
} from '../src/utils/readerPosition.js'

test('restores flip pages from offsets, percentages, and chapter end', () => {
  assert.equal(restoredReaderFlipPage({
    offset: READER_CHAPTER_END_OFFSET,
    percent: null,
    pageCount: 8,
  }), 7)
  assert.equal(restoredReaderFlipPage({
    offset: 0,
    percent: 0.5,
    pageCount: 8,
  }), 4)
  assert.equal(restoredReaderFlipPage({
    offset: 20,
    percent: null,
    pageCount: 8,
  }), 7)
  assert.equal(restoredReaderFlipPage({
    offset: 3,
    percent: null,
    pageCount: 8,
  }), 3)
})

test('restores single chapter scrolling within available content', () => {
  assert.equal(restoredReaderSingleChapterScrollTop({
    offset: READER_CHAPTER_END_OFFSET,
    percent: null,
    scrollHeight: 2400,
    clientHeight: 800,
  }), 1600)
  assert.equal(restoredReaderSingleChapterScrollTop({
    offset: 0,
    percent: 0.25,
    scrollHeight: 2400,
    clientHeight: 800,
  }), 400)
  assert.equal(restoredReaderSingleChapterScrollTop({
    offset: 320,
    percent: null,
    scrollHeight: 2400,
    clientHeight: 800,
  }), 320)
})

test('restores continuous chapter boundaries while preserving text offsets', () => {
  assert.equal(restoredReaderContinuousScrollTop({
    offset: READER_CHAPTER_END_OFFSET,
    percent: null,
    chapterTop: 1200,
    chapterHeight: 1600,
    clientHeight: 800,
  }), 2000)
  assert.equal(restoredReaderContinuousScrollTop({
    offset: 0,
    percent: 0.5,
    chapterTop: 1200,
    chapterHeight: 1600,
    clientHeight: 800,
  }), 1600)
  assert.equal(restoredReaderContinuousScrollTop({
    offset: 240,
    percent: null,
    chapterTop: 1200,
    chapterHeight: 1600,
    clientHeight: 800,
  }), null)
  assert.equal(readerChapterBoundaryScrollTop({
    chapterTop: 1200,
    chapterHeight: 1600,
    clientHeight: 800,
    end: false,
  }), 1200)
})

test('selects the nearest paragraph at or before a text position', () => {
  const rows = [0, 120, 260].map(pos => ({ dataset: { pos: String(pos) } }))
  assert.equal(readerParagraphAtPosition(rows, 180), rows[1])
  assert.equal(readerParagraphAtPosition(rows, 20), rows[0])
  assert.equal(readerParagraphAtPosition(rows, 500), rows[2])
  assert.equal(readerParagraphAtPosition([], 100), null)
})
