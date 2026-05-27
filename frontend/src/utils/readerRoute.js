export function readerRouteQueryFromBook(book, progressOverride = null, totalChaptersOverride = null) {
  const progress = progressOverride || book?.progress
  if (!progress) return {}
  const query = {}
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
  if (!progress || !Number.isFinite(Number(progress.percent))) return null
  const chapterIndex = Number(progress.chapterIndex)
  if (!Number.isFinite(chapterIndex)) return null
  const totalValue = Number(totalChapters || 0)
  if (!Number.isFinite(totalValue) || totalValue <= 0) return null
  const total = Math.max(totalValue, 1)
  const raw = Number(progress.percent) * total - chapterIndex
  if (!Number.isFinite(raw) || raw <= 0) return null
  return Math.max(0, Math.min(1, raw))
}
