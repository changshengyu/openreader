export function remoteBookSourceId(book = {}, fallback = '') {
  return book.sourceId || book.bookSourceId || fallback || ''
}

export function remoteBookUrl(book = {}) {
  return book.bookUrl || book.url || book.bookURL || ''
}

export function remoteBookTitle(book = {}, fallback = '') {
  return book.title || book.name || book.bookName || fallback || '未命名书籍'
}

export function remoteBookAuthor(book = {}) {
  return book.author || book.bookAuthor || ''
}

export function remoteBookCover(book = {}) {
  return book.coverUrl || book.bookCover || book.cover || ''
}

export function remoteBookIntro(book = {}) {
  return book.intro || book.desc || book.description || ''
}

export function remoteBookKind(book = {}) {
  return book.kind || book.category || book.genre || ''
}

export function remoteBookWordCount(book = {}) {
  return book.wordCount || book.words || ''
}

export function remoteBookSourceName(book = {}, fallback = '') {
  return book.sourceName || book.bookSourceName || book.originName || book.origin || fallback || '未知书源'
}

export function remoteBookLatestChapter(book = {}) {
  return book.latestChapter || book.latestChapterTitle || book.lastChapter || ''
}

export function remoteBookKey(book = {}, fallbackSourceId = '') {
  return `${remoteBookSourceId(book, fallbackSourceId)}-${remoteBookUrl(book)}`
}

export function remoteBookCreatePayload(book = {}, categoryIds = [], options = {}) {
  return {
    title: remoteBookTitle(book),
    author: remoteBookAuthor(book),
    coverUrl: remoteBookCover(book),
    intro: remoteBookIntro(book),
    kind: remoteBookKind(book),
    wordCount: remoteBookWordCount(book),
    bookUrl: remoteBookUrl(book),
    sourceId: remoteBookSourceId(book, options.sourceId),
    sourceName: remoteBookSourceName(book, options.sourceName),
    categoryIds: normalizeCategoryIds(categoryIds),
  }
}

function normalizeCategoryIds(value) {
  const values = Array.isArray(value) ? value : [value]
  return [...new Set(values.map(Number).filter(id => Number.isInteger(id) && id > 0))]
}
