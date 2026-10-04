import { newestProgress } from './bookOrder.js'

export function parseReaderRoutePercent(value) {
  if (value === undefined || value === null || value === '') return null
  const percent = Number(value)
  return Number.isFinite(percent) ? Math.max(0, Math.min(1, percent)) : null
}

export function readerRouteQueryFromBook(book, progressOverride = null, totalChaptersOverride = null) {
  const progress = newestProgress(book?.progress || null, progressOverride || null)
  const query = { resume: '1' }
  if (!progress) return query
  const chapterIndex = Number(progress.chapterIndex)
  if (Number.isFinite(chapterIndex)) query.chapter = Math.max(0, Math.floor(chapterIndex))
  const offset = Number(progress.offset)
  if (Number.isFinite(offset) && offset > 0) query.offset = Math.floor(offset)
  const totalChapters = totalChaptersOverride ?? book?.chapterCount
  const chapterPercent = savedBookChapterPercent(progress, totalChapters)
  if (chapterPercent !== null) query.percent = Number(chapterPercent.toFixed(6))
  return query
}

export function savedBookChapterPercent(progress, totalChapters) {
  const explicitPercent = parseReaderRoutePercent(progress?.chapterPercent)
  if (explicitPercent !== null) {
    // Old/WebDAV records have no measured chapter fraction. Their default zero
    // must not erase a positive character position; explicit route zero still wins.
    return explicitPercent === 0 && Number(progress?.offset) > 0 ? null : explicitPercent
  }
  if (!progress || !Number.isFinite(Number(progress.percent))) return null
  const chapterIndex = Number(progress.chapterIndex)
  if (!Number.isFinite(chapterIndex)) return null
  const totalValue = Number(totalChapters || 0)
  if (!Number.isFinite(totalValue) || totalValue <= 0) return null
  const total = Math.max(totalValue, 1)
  const raw = Number(progress.percent) * total - chapterIndex
  if (!Number.isFinite(raw) || raw <= 0 || raw > 1) return null
  return raw
}
