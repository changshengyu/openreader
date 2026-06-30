export function nextReaderBlock(container, current) {
  const blocks = [...(container?.querySelectorAll?.('[data-reader-block]') || [])]
  if (!current) return blocks[0] || null
  const index = blocks.indexOf(current)
  return index >= 0 ? blocks[index + 1] || null : blocks[0] || null
}

export function paragraphAutoReadDelay({
  paragraphHeight,
  fontSize,
  lineHeight,
  baseDelay,
}) {
  const estimatedLineHeight = Math.max(1, Number(fontSize || 18) * Number(lineHeight || 1.8))
  const lineCount = Number(paragraphHeight) > 0
    ? Math.max(1, Math.ceil(Number(paragraphHeight) / estimatedLineHeight))
    : 1
  return Math.max(0, Number(baseDelay) || 0) * lineCount
}
