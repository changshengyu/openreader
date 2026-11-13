import { readerCatalogRow } from './readerCatalogRow.js'

// The running title `第 N 章 章节名` (docs/compat/reader-running-title-load-failure-fixed-baseline-second-audit-p0-contract.md
// R1-R2, R5): the 1-based ordinal and the trimmed title of the catalog row at currentIndex, from the current book's
// catalog only, the same row the tab title uses. An unknown row gives an empty label instead of a guessed ordinal.
export function formatRunningChapterLabel(index, title) {
  const ordinal = `第 ${Math.max(0, Number(index) || 0) + 1} 章`
  const normalizedTitle = String(title || '').trim()
  return normalizedTitle ? `${ordinal} ${normalizedTitle}` : ordinal
}

export function runningChapterLabelFor({ book, chapters, index }) {
  const row = readerCatalogRow(book, chapters, index)
  return row ? formatRunningChapterLabel(index, row.title) : ''
}
