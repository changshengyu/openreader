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

export const MAX_READER_PROGRESS_CLIENT_ID_BYTES = 128

const READER_PROGRESS_CLIENT_ID_KEY = 'openreader_reader_client_id'

export function isReaderProgressClientId(value) {
  return typeof value === 'string'
    && value.length > 0
    && utf8ByteLength(value) <= MAX_READER_PROGRESS_CLIENT_ID_BYTES
}

export function readOrCreateReaderClientId(storage, createClientId) {
  let current = ''
  try {
    current = storage?.getItem(READER_PROGRESS_CLIENT_ID_KEY) || ''
  } catch {
    // Restricted storage falls back to a per-page client ID.
  }
  if (isReaderProgressClientId(current)) return current

  const next = createClientId()
  try {
    storage?.setItem(READER_PROGRESS_CLIENT_ID_KEY, next)
  } catch {
    // The generated ID remains valid for this page when storage is unavailable.
  }
  return next
}

function utf8ByteLength(value) {
  let bytes = 0
  for (const character of value) {
    const codePoint = character.codePointAt(0)
    bytes += codePoint <= 0x7f ? 1 : codePoint <= 0x7ff ? 2 : codePoint <= 0xffff ? 3 : 4
  }
  return bytes
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
