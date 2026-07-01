export function readerTocTargetIndex(index, chapterCount) {
  const maxIndex = Math.max(0, Number(chapterCount || 0) - 1)
  return Math.max(0, Math.min(Math.floor(Number(index) || 0), maxIndex))
}

export function toggleReaderTocReverse(reverse) {
  return !Boolean(reverse)
}
