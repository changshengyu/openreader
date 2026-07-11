const punctuationPattern = /[\p{P}\p{S}\s]+/gu

export function readerBookmarkText(value) {
  return String(value?.text ?? value?.textContent ?? value?.innerText ?? value ?? '').trim()
}

export function normalizeReaderBookmarkText(value) {
  return readerBookmarkText(value).replace(punctuationPattern, '')
}

export function readerBookmarkSimilarity(left, right) {
  const a = normalizeReaderBookmarkText(left)
  const b = normalizeReaderBookmarkText(right)
  if (!a || !b) return 0
  if (a === b) return 1
  const width = b.length + 1
  let previous = Array.from({ length: width }, (_, index) => index)
  for (let row = 1; row <= a.length; row += 1) {
    const next = [row]
    for (let column = 1; column <= b.length; column += 1) {
      next[column] = Math.min(
        previous[column] + 1,
        next[column - 1] + 1,
        previous[column - 1] + (a[row - 1] === b[column - 1] ? 0 : 1),
      )
    }
    previous = next
  }
  return 1 - (previous[b.length] / Math.max(a.length, b.length))
}

export function findReaderBookmarkParagraph({
  selectedText,
  paragraphs,
  minSimilarity = 0.6,
  maxSelectedParagraphs = Number.POSITIVE_INFINITY,
} = {}) {
  const selected = String(selectedText || '')
    .replace(/\n+/g, '\n')
    .split(/\n+/)
    .map(readerBookmarkText)
    .filter(Boolean)
  if (!selected.length || selected.length > maxSelectedParagraphs) return null
  const rows = Array.isArray(paragraphs) ? paragraphs : []
  for (let index = 0; index < rows.length; index += 1) {
    let cursor = index
    let matched = true
    let similarity = 1
    for (const selectedParagraph of selected) {
      while (cursor < rows.length && !readerBookmarkText(rows[cursor])) cursor += 1
      if (cursor >= rows.length) {
        matched = false
        break
      }
      const score = readerBookmarkSimilarity(rows[cursor], selectedParagraph)
      if (score < minSimilarity) {
        matched = false
        break
      }
      similarity = Math.min(similarity, score)
      cursor += 1
    }
    if (matched) return { index, similarity }
  }
  return null
}

export function captureReaderBookmarkExcerpt(paragraphs, startIndex, {
  maxParagraphs = 5,
  maxCharacters = 150,
} = {}) {
  const rows = Array.isArray(paragraphs) ? paragraphs : []
  const lines = []
  let characterCount = 0
  for (let index = Math.max(0, Number(startIndex) || 0); index < rows.length; index += 1) {
    const text = readerBookmarkText(rows[index])
    if (!text) continue
    lines.push(text)
    characterCount += text.length + (lines.length > 1 ? 1 : 0)
    if (lines.length >= maxParagraphs || characterCount >= maxCharacters) break
  }
  return lines.join('\n')
}

export function selectedTextBookmarkContext({ selectedText, paragraphs } = {}) {
  const match = findReaderBookmarkParagraph({
    selectedText,
    paragraphs,
    minSimilarity: 0.7,
    maxSelectedParagraphs: 2,
  })
  if (!match) return null
  const excerpt = captureReaderBookmarkExcerpt(paragraphs, match.index)
  return excerpt ? { index: match.index, excerpt } : null
}
