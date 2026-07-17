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

export function readerScrollStep({
  viewportHeight,
  fontSize,
  lineHeight,
  paragraphSpace,
}) {
  const size = Number(fontSize || 18)
  const offset = (
    size * Number(lineHeight || 1.8) * 2
    + size * Number(paragraphSpace || 0) * 2
  )
  return Math.max(1, Math.floor(Number(viewportHeight || 0) - offset))
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

export function readerVerticalPageLayout({
  scrollHeight,
  clientHeight,
  scrollTop,
  pageHeight,
}) {
  const height = Math.max(1, Number(pageHeight) || 0)
  const totalHeight = Math.max(0, Number(scrollHeight) || 0)
  const scrollBottom = Math.max(totalHeight - (Number(clientHeight) || 0), 1)
  const pageCount = Math.max(1, Math.ceil(totalHeight / height))
  const page = Math.round(((Number(scrollTop) || 0) / scrollBottom) * Math.max(pageCount - 1, 0))
  return {
    pageHeight: height,
    pageCount,
    page: Math.max(0, Math.min(pageCount - 1, page)),
  }
}

export function readerFlipChapterPercent(currentPage, pageCount) {
  const count = Math.max(1, Number(pageCount) || 0)
  if (count <= 1) return 0
  return clampReaderPercent((Number(currentPage) || 0) / (count - 1))
}
