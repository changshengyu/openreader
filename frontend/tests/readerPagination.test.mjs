import assert from 'node:assert/strict'
import test from 'node:test'
import {
  readerBookProgress,
  readerFlipChapterPercent,
  readerFlipPageLayout,
  readerScrollBehaviorForDuration,
  readerScrollStep,
  readerVerticalCurrentPage,
  readerVerticalPageCount,
} from '../src/utils/readerPagination.js'

test('derives smooth whole-book display progress without owning mobile navigation', () => {
  assert.equal(readerBookProgress({
    chapterIndex: 2,
    chapterPercent: 0.5,
    totalChapters: 10,
  }), 0.25)
  assert.equal(readerBookProgress({ chapterIndex: 9, chapterPercent: 1, totalChapters: 10 }), 1)
  assert.equal(readerBookProgress({ chapterIndex: -1, chapterPercent: -1, totalChapters: 0 }), 0)
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

test('derives vertical page count and current page like reader-dev computePages and scrollHandler', () => {
  const typography = { fontSize: 20, lineHeight: 1.5, paragraphSpace: 0.5 }
  // 800px window minus two lines and two paragraph gaps (80px) gives 720px pages.
  assert.equal(readerVerticalPageCount({ contentHeight: 2400, viewportHeight: 800, ...typography }), 4)
  assert.equal(readerVerticalCurrentPage({ scrollTop: 800, viewportHeight: 800, pageCount: 4, ...typography }), 2)
  assert.equal(readerVerticalPageCount({ contentHeight: 0, viewportHeight: 0, ...typography }), 1)
  assert.equal(readerVerticalCurrentPage({ scrollTop: 100, viewportHeight: 0, pageCount: 1, ...typography }), 1)
})
