export function normalizeReaderSelectionText(value, maxLength = 0) {
  const text = String(value || '')
  if (!text.trim()) return ''
  const limit = Math.max(0, Number(maxLength) || 0)
  return limit > 0 ? text.slice(0, limit) : text
}

export function readerSelectionBelongsToRoot(root, container) {
  return Boolean(root && container && root.contains?.(container))
}
