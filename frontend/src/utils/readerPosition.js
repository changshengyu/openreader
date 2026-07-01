export const READER_CHAPTER_END_OFFSET = -1

function clampedPercent(value) {
  return Math.max(0, Math.min(1, Number(value) || 0))
}

function hasReaderPercent(value) {
  return value !== null
    && value !== undefined
    && value !== ''
    && Number.isFinite(Number(value))
}

export function restoredReaderFlipPage({
  offset,
  percent,
  pageCount,
}) {
  const lastPage = Math.max(0, (Number(pageCount) || 0) - 1)
  if (Number(offset) === READER_CHAPTER_END_OFFSET) return lastPage
  if (hasReaderPercent(percent)) {
    return Math.round(clampedPercent(percent) * lastPage)
  }
  return Math.min(Math.max(Number(offset) || 0, 0), lastPage)
}

export function restoredReaderSingleChapterScrollTop({
  offset,
  percent,
  scrollHeight,
  clientHeight,
}) {
  const bottom = Math.max(0, (Number(scrollHeight) || 0) - (Number(clientHeight) || 0))
  if (Number(offset) === READER_CHAPTER_END_OFFSET) return bottom
  if (hasReaderPercent(percent)) {
    return Math.round(clampedPercent(percent) * bottom)
  }
  return Math.max(Number(offset) || 0, 0)
}

export function restoredReaderContinuousScrollTop({
  offset,
  percent,
  chapterTop,
  chapterHeight,
  clientHeight,
}) {
  const top = Math.max(0, Number(chapterTop) || 0)
  const height = Math.max(0, Number(chapterHeight) || 0)
  const viewport = Math.max(0, Number(clientHeight) || 0)
  if (Number(offset) === READER_CHAPTER_END_OFFSET) {
    return Math.max(0, top + height - viewport)
  }
  if (hasReaderPercent(percent)) {
    const room = Math.max(height - viewport, 0)
    return Math.max(0, top + Math.round(clampedPercent(percent) * room))
  }
  if (Number(offset) > 0) return null
  return top
}

export function readerChapterBoundaryScrollTop({
  chapterTop,
  chapterHeight,
  clientHeight,
  end,
}) {
  const top = Math.max(0, Number(chapterTop) || 0)
  if (!end) return top
  return Math.max(
    0,
    top + Math.max(0, Number(chapterHeight) || 0) - Math.max(0, Number(clientHeight) || 0),
  )
}

export function readerParagraphAtPosition(nodes, position) {
  const targetPosition = Number(position)
  const rows = Array.isArray(nodes) ? nodes : [...(nodes || [])]
  if (!rows.length || !Number.isFinite(targetPosition) || targetPosition <= 0) return null
  return [...rows].reverse().find(node => Number(node?.dataset?.pos) <= targetPosition) || rows[0]
}
