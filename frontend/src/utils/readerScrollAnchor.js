export function restoredReaderScrollTop(options = {}) {
  const scrollTop = finiteNumber(options.scrollTop)
  const previousOffset = finiteNumber(options.previousOffset)
  const currentOffset = finiteNumber(options.currentOffset)
  const maxScroll = Math.max(0, finiteNumber(options.maxScroll))
  return Math.max(0, Math.min(maxScroll, scrollTop + currentOffset - previousOffset))
}

function finiteNumber(value) {
  const number = Number(value)
  return Number.isFinite(number) ? number : 0
}
