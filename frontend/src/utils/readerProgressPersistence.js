import { readerCatalogRow } from './readerCatalogRow.js'

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

// docs/compat/reader-failure-progress-payload-fixed-baseline-second-audit-p0-contract.md P1, P3, P4. reader-dev names
// progress by catalog index alone and the server titles it from that row. Given the catalog, a payload therefore names
// exactly one row of this book: the visible snapshot's row, or the row at the current index. A current chapter whose
// content is not on the page (still loading, or failed) is saved at that row's start, or at the saved position when the
// saved progress already names that row (reader-dev leaves its in-chapter position alone while a load fails). Without
// a catalog row of this book there is nothing to save.
export function readerProgressPayload({
  bookId,
  chapters,
  visibleSnapshot,
  currentChapter,
  currentChapterIndex,
  currentOffset,
  currentChapterPercent,
  totalChapters,
  getStoredProgress,
}) {
  if (Array.isArray(chapters)) {
    return readerCatalogProgressPayload({
      bookId,
      chapters,
      visibleSnapshot,
      currentChapter,
      currentChapterIndex,
      currentOffset,
      currentChapterPercent,
      totalChapters,
      getStoredProgress,
    })
  }
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

// True when a ready-made payload (the audio player's) names the catalog row of this book at its chapter index.
export function readerProgressNamesCatalogRow(payload, chapters) {
  const row = readerCatalogRow({ id: payload?.bookId }, chapters, payload?.chapterIndex)
  return Boolean(row) && persistedChapterId(row.id) === persistedChapterId(payload.chapterId)
}

function readerCatalogProgressPayload({
  bookId,
  chapters,
  visibleSnapshot,
  currentChapter,
  currentChapterIndex,
  currentOffset,
  currentChapterPercent,
  totalChapters,
  getStoredProgress,
}) {
  const book = { id: bookId }
  if (visibleSnapshot && Number.isInteger(visibleSnapshot.chapterIndex)) {
    const row = readerCatalogRow(book, chapters, visibleSnapshot.chapterIndex)
    if (!row) return null
    return catalogRowPayload({
      bookId,
      chapter: visibleSnapshot.chapter || row,
      row,
      index: visibleSnapshot.chapterIndex,
      offset: visibleSnapshot.offset,
      chapterPercent: visibleSnapshot.chapterPercent,
      totalChapters,
    })
  }
  const index = Number(currentChapterIndex)
  const row = readerCatalogRow(book, chapters, index)
  if (!row) return null
  if (readerChapterIsRow(currentChapter, row, index)) {
    return catalogRowPayload({
      bookId,
      chapter: currentChapter,
      row,
      index,
      offset: currentOffset,
      chapterPercent: currentChapterPercent,
      totalChapters,
    })
  }
  const stored = getStoredProgress?.()
  const keepsStored = Boolean(stored)
    && Number(stored.chapterIndex) === index
    && persistedChapterId(stored.chapterId) === persistedChapterId(row.id)
  const storedPercent = Number(stored?.chapterPercent)
  return catalogRowPayload({
    bookId,
    chapter: row,
    row,
    index,
    offset: keepsStored ? Math.max(0, Math.floor(Number(stored.offset) || 0)) : 0,
    chapterPercent: keepsStored && Number.isFinite(storedPercent) ? storedPercent : 0,
    totalChapters,
  })
}

function catalogRowPayload({ bookId, chapter, row, index, offset, chapterPercent, totalChapters }) {
  const total = Math.max(Number(totalChapters) || 0, 1)
  return {
    bookId,
    chapterId: row.id,
    chapterIndex: index,
    offset,
    percent: Math.min(1, Math.max(0, (Number(index) + Number(chapterPercent || 0)) / total)),
    chapterPercent,
    chapterTitle: chapter?.title || row.title || '',
  }
}

// The loaded chapter object is this row's content: same index (when it carries one) and same id (when both have one).
function readerChapterIsRow(chapter, row, index) {
  if (!chapter) return false
  const hasIndex = chapter.index !== undefined && chapter.index !== null
  if (hasIndex && Number(chapter.index) !== Number(index)) return false
  const chapterId = persistedChapterId(chapter.id)
  const rowId = persistedChapterId(row.id)
  if (chapterId && rowId) return chapterId === rowId
  return hasIndex
}

function persistedChapterId(value) {
  const id = Number(value)
  return Number.isFinite(id) && id > 0 ? id : 0
}
