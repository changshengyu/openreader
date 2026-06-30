export function normalizeReaderSelectionText(value, maxLength = 1000) {
  return String(value || '')
    .replace(/\s+/g, ' ')
    .trim()
    .slice(0, Math.max(0, Number(maxLength) || 0))
}

export function readerSelectionBelongsToRoot(root, container) {
  return Boolean(root && container && root.contains?.(container))
}
