function normalizedChapterCount(totalChapters) {
  return Math.max(0, Math.floor(Number(totalChapters) || 0))
}

export function readerChapterWindowIndexes({
  mode,
  anchorIndex,
  startIndex = anchorIndex,
  totalChapters,
  nextSize = 1,
}) {
  const total = normalizedChapterCount(totalChapters)
  if (!total) return []
  const anchor = Math.max(0, Math.min(Math.floor(Number(anchorIndex) || 0), total - 1))
  const requestedStart = Math.max(
    0,
    Math.min(Math.floor(Number(startIndex) || 0), total - 1),
  )
  const start = mode === 'scroll2' ? anchor : Math.min(requestedStart, anchor)
  const end = Math.min(total - 1, anchor + Math.max(0, Number(nextSize) || 0))
  return Array.from({ length: end - start + 1 }, (_, offset) => start + offset)
}

export function adjacentReaderChapterIndex({
  blocks,
  direction,
  totalChapters,
}) {
  const rows = Array.isArray(blocks) ? blocks : []
  if (!rows.length) return null
  if (direction === 'previous') {
    const index = Number(rows[0]?.index) - 1
    return index >= 0 ? index : null
  }
  const index = Number(rows[rows.length - 1]?.index) + 1
  return index < normalizedChapterCount(totalChapters) ? index : null
}

export function nearbyReaderChapterIndexes({
  chapterIndex,
  totalChapters,
  radius = 2,
}) {
  const total = normalizedChapterCount(totalChapters)
  const anchor = Math.floor(Number(chapterIndex) || 0)
  const distanceLimit = Math.max(0, Math.floor(Number(radius) || 0))
  const indexes = []
  for (let distance = 1; distance <= distanceLimit; distance += 1) {
    const next = anchor + distance
    const previous = anchor - distance
    if (next >= 0 && next < total) indexes.push(next)
    if (previous >= 0 && previous < total) indexes.push(previous)
  }
  return indexes
}

export function readerChapterWindowExtension({
  scrollTop,
  clientHeight,
  scrollHeight,
}) {
  const top = Number(scrollTop) || 0
  const viewport = Math.max(0, Number(clientHeight) || 0)
  const height = Math.max(0, Number(scrollHeight) || 0)
  return {
    next: viewport > 0 && top > height - viewport * 4,
  }
}

export function readerChapterWindowPrunePlan({
  blocks,
  mode,
  currentIndex,
  totalChapters,
}) {
  const rows = Array.isArray(blocks) ? blocks : []
  const total = normalizedChapterCount(totalChapters)
  if (!rows.length || !total) {
    return {
      blocks: [],
      removedBeforeIndexes: [],
      changed: rows.length > 0,
    }
  }
  if (mode !== 'scroll2') {
    return {
      blocks: rows,
      removedBeforeIndexes: [],
      changed: false,
    }
  }
  const anchor = Math.max(0, Math.min(Math.floor(Number(currentIndex) || 0), total - 1))
  const kept = rows.filter(block => Number(block?.index) >= anchor)
  return {
    blocks: kept,
    removedBeforeIndexes: rows
      .filter(block => Number(block?.index) < anchor)
      .map(block => Number(block.index)),
    changed: kept.length !== rows.length,
  }
}
