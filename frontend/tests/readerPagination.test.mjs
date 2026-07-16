import assert from 'node:assert/strict'
import test from 'node:test'
import {
  readerBookProgress,
  readerBookSeekTarget,
  readerFlipChapterPercent,
  readerFlipPageLayout,
  readerScrollBehaviorForDuration,
  readerScrollStep,
  readerVerticalPageLayout,
} from '../src/utils/readerPagination.js'

test('maps book progress to chapter targets at boundaries', () => {
  assert.equal(readerBookProgress({
    chapterIndex: 2,
    chapterPercent: 0.5,
    totalChapters: 10,
  }), 0.25)
  assert.deepEqual(readerBookSeekTarget(0.25, 10), {
    chapterIndex: 2,
    chapterPercent: 0.5,
  })
  assert.deepEqual(readerBookSeekTarget(1, 10), {
    chapterIndex: 9,
    chapterPercent: 1,
  })
  assert.deepEqual(readerBookSeekTarget(-1, 0), {
    chapterIndex: 0,
    chapterPercent: 0,
  })
})

test('calculates reader scroll steps and animation behavior', () => {
  assert.equal(readerScrollStep({
    viewportHeight: 800,
    fontSize: 20,
    lineHeight: 1.5,
    paragraphSpace: 0.5,
  }), 720)
  assert.equal(readerScrollStep({
    viewportHeight: 20,
    fontSize: 20,
    lineHeight: 2,
    paragraphSpace: 1,
  }), 1)
  assert.equal(readerScrollBehaviorForDuration(200), 'smooth')
  assert.equal(readerScrollBehaviorForDuration(0), 'auto')
})

test('clamps flip pagination when content or viewport changes', () => {
  assert.deepEqual(readerFlipPageLayout({
    viewportWidth: 400,
    pageStride: 384,
    viewportHeight: 700,
    scrollWidth: 1152,
    currentPage: 5,
  }), {
    pageWidth: 384,
    pageHeight: 700,
    pageCount: 3,
    page: 2,
  })
  assert.equal(readerFlipChapterPercent(2, 3), 1)
  assert.equal(readerFlipChapterPercent(0, 1), 0)
})

test('derives vertical page count and current page from scroll position', () => {
  assert.deepEqual(readerVerticalPageLayout({
    scrollHeight: 2400,
    clientHeight: 800,
    scrollTop: 800,
    pageHeight: 720,
  }), {
    pageHeight: 720,
    pageCount: 4,
    page: 2,
  })
  assert.deepEqual(readerVerticalPageLayout({
    scrollHeight: 0,
    clientHeight: 800,
    scrollTop: 100,
    pageHeight: 0,
  }), {
    pageHeight: 1,
    pageCount: 1,
    page: 0,
  })
})
