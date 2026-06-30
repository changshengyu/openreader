export function readerProgressSaveKey(payload, mode = '') {
  if (!payload) return ''
  return [
    payload.bookId,
    payload.chapterId,
    payload.chapterIndex,
    payload.offset,
    Math.round(Number(payload.percent || 0) * 10000),
    Math.round(Number(payload.chapterPercent || 0) * 10000),
    mode,
  ].join(':')
}

export function readerProgressBaseUpdatedAt(progress) {
  if (!progress) return ''
  if (progress.pendingSync) return progress.baseUpdatedAt || ''
  return progress.updatedAt || ''
}

export function readerProgressThrottleDelay(lastRequestAt, now, minimumInterval) {
  const elapsed = Math.max(0, Number(now) - Number(lastRequestAt || 0))
  return Math.max(0, Number(minimumInterval || 0) - elapsed)
}
