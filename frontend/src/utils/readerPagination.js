export function clampReaderPercent(value) {
  return Math.max(0, Math.min(1, Number(value) || 0))
}

export function readerBookProgress({
  chapterIndex,
  chapterPercent,
  totalChapters,
}) {
  const total = Math.max(Number(totalChapters) || 0, 1)
  return clampReaderPercent((Number(chapterIndex) + clampReaderPercent(chapterPercent)) / total)
}

// reader-dev Reader.vue#readingProgress: the share of chapters reached, as a whole percentage.
export function readerChapterProgressLabel({ chapterIndex, totalChapters }) {
  const total = Number(totalChapters) || 0
  if (total <= 0) return ''
  return `${Math.trunc(((Number(chapterIndex) || 0) + 1) * 100 / total)}%`
}

// reader-dev Reader.vue#scrollOffset: two lines plus two paragraph gaps.
function readerScrollOffset({ fontSize, lineHeight, paragraphSpace }) {
  const size = Number(fontSize || 18)
  return size * Number(lineHeight || 1.8) * 2 + size * Number(paragraphSpace || 0) * 2
}

export function readerScrollStep({
  viewportHeight,
  fontSize,
  lineHeight,
  paragraphSpace,
}) {
  const offset = readerScrollOffset({ fontSize, lineHeight, paragraphSpace })
  return Math.max(1, Math.floor(Number(viewportHeight || 0) - offset))
}

// reader-dev Reader.vue#computePages outside slide reading: the content height in pages of
// the window height minus the scroll offset.
export function readerVerticalPageCount({
  contentHeight,
  viewportHeight,
  fontSize,
  lineHeight,
  paragraphSpace,
}) {
  const pageHeight = Number(viewportHeight || 0) - readerScrollOffset({ fontSize, lineHeight, paragraphSpace })
  if (!(pageHeight > 0)) return 1
  return Math.max(1, Math.ceil((Number(contentHeight) || 0) / pageHeight))
}

// reader-dev Reader.vue#scrollHandler; the page slider holds it within 1…pageCount.
export function readerVerticalCurrentPage({
  scrollTop,
  viewportHeight,
  fontSize,
  lineHeight,
  paragraphSpace,
  pageCount,
}) {
  const height = Number(viewportHeight || 0)
  const pageHeight = height - readerScrollOffset({ fontSize, lineHeight, paragraphSpace })
  const count = Math.max(1, Number(pageCount) || 1)
  if (!(pageHeight > 0)) return 1
  const page = Math.round(((Number(scrollTop) || 0) + height) / pageHeight)
  return Math.max(1, Math.min(count, page))
}

// reader-dev Reader.vue#showPage outside slide reading.
export function readerVerticalPageScrollTop({ page, viewportHeight }) {
  return Math.max(0, (Number(viewportHeight || 0) - 10) * ((Number(page) || 1) - 1))
}

// reader-dev Reader.vue#readWidthConfig: the reading width steps down by 20px until the
// window keeps 140px for the side bars.
export function readerReadWidthConfig({ readWidth, windowWidth }) {
  let width = Number(readWidth) || 800
  const limit = (Number(windowWidth) || 0) - 140
  while (width > limit && width > 20) width -= 20
  return width
}

export function readerScrollBehaviorForDuration(animateDuration) {
  return Number(animateDuration) > 0 ? 'smooth' : 'auto'
}

export function readerFlipPageLayout({
  viewportWidth,
  pageStride,
  viewportHeight,
  scrollWidth,
  currentPage,
}) {
  const requestedStride = Number(pageStride)
  const width = Math.max(
    1,
    Number.isFinite(requestedStride) && requestedStride > 0
      ? requestedStride
      : Number(viewportWidth) || 0,
  )
  const height = Math.max(1, Number(viewportHeight) || 0)
  const pageCount = Math.max(1, Math.ceil((Number(scrollWidth) || 0) / width))
  return {
    pageWidth: width,
    pageHeight: height,
    pageCount,
    page: Math.min(Math.max(0, Number(currentPage) || 0), pageCount - 1),
  }
}

export function readerFlipChapterPercent(currentPage, pageCount) {
  const count = Math.max(1, Number(pageCount) || 0)
  if (count <= 1) return 0
  return clampReaderPercent((Number(currentPage) || 0) / (count - 1))
}
