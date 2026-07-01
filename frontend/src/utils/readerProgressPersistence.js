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

export function readerProgressPayload({
  bookId,
  visibleSnapshot,
  currentChapter,
  currentChapterIndex,
  currentOffset,
  currentChapterPercent,
  totalChapters,
}) {
  const progressChapter = visibleSnapshot?.chapter || currentChapter
  const progressChapterIndex = Number.isInteger(visibleSnapshot?.chapterIndex)
    ? visibleSnapshot.chapterIndex
    : currentChapterIndex
  const progressChapterPercent = visibleSnapshot
    ? visibleSnapshot.chapterPercent
    : currentChapterPercent
  const total = Math.max(Number(totalChapters) || 0, 1)
  return {
    bookId,
    chapterId: progressChapter?.id,
    chapterIndex: progressChapterIndex,
    offset: visibleSnapshot ? visibleSnapshot.offset : currentOffset,
    percent: Math.min(
      1,
      Math.max(0, (Number(progressChapterIndex) + Number(progressChapterPercent || 0)) / total),
    ),
    chapterPercent: progressChapterPercent,
    chapterTitle: progressChapter?.title || '',
  }
}
