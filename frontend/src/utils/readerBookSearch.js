export function bookContentSearchPagingParams(book) {
  if (Number(book?.sourceId || 0) > 0) {
    return { chapterLimit: 10, scanLimit: 10, matchLimit: 120, perChapterLimit: 20 }
  }
  return { chapterLimit: 160, scanLimit: 480, matchLimit: 1000, perChapterLimit: 100, localFull: 1 }
}

export function bookContentSearchMaxRounds({ append = false, scanAll = false, remote = false } = {}) {
  if (scanAll) return 80
  if (append) return 1
  return remote ? 4 : 1
}

export function bookContentSearchStatus({
  searched,
  lastIndex,
  total,
  chapterCount,
  resultCount,
}) {
  if (!searched) return ''
  const scanned = Number(lastIndex) >= 0 ? Number(lastIndex) + 1 : 0
  const availableTotal = Number(total) || Number(chapterCount) || 0
  if (!availableTotal) return `${Number(resultCount) || 0} 条结果`
  return `已搜索 ${Math.min(scanned, availableTotal)} / ${availableTotal} 章，${Number(resultCount) || 0} 条结果`
}
