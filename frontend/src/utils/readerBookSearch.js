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

export function countBookContentMatches(text, keyword) {
  const haystack = String(text || '').toLowerCase()
  const needle = String(keyword || '').toLowerCase()
  if (!haystack || !needle) return 0
  let count = 0
  for (let offset = 0; offset < haystack.length;) {
    const position = haystack.indexOf(needle, offset)
    if (position < 0) break
    count += 1
    offset = position + Math.max(needle.length, 1)
  }
  return count
}

export function normalizeBookContentSearchText(value) {
  return String(value || '').toLowerCase().replace(/[\s\p{P}\p{S}]+/gu, '')
}

export function bookContentSearchParagraphIndex(texts, keyword, matchIndex = 0) {
  const rows = Array.isArray(texts) ? texts : []
  const query = String(keyword || '').trim()
  if (!query) return -1
  const expectedIndex = Number.isFinite(Number(matchIndex))
    ? Math.max(0, Math.floor(Number(matchIndex)))
    : 0
  const exact = paragraphIndexForMatch(rows, query, expectedIndex)
  if (exact >= 0) return exact
  const normalizedKeyword = normalizeBookContentSearchText(query)
  if (!normalizedKeyword) return -1
  return paragraphIndexForMatch(
    rows.map(normalizeBookContentSearchText),
    normalizedKeyword,
    expectedIndex,
  )
}

function paragraphIndexForMatch(texts, keyword, expectedIndex) {
  let matchCount = 0
  for (let index = 0; index < texts.length; index += 1) {
    const matches = countBookContentMatches(texts[index], keyword)
    if (matchCount + matches > expectedIndex) return index
    matchCount += matches
  }
  return -1
}
