export function sourceCandidateSourceId(candidate = {}) {
  return candidate.sourceId || candidate.bookSourceId || candidate.id || ''
}

export function sourceCandidateBookUrl(candidate = {}) {
  return candidate.bookUrl || candidate.url || candidate.bookURL || candidate.bookSourceUrl || ''
}

export function sourceCandidateTitle(candidate = {}, fallback = '') {
  return candidate.title || candidate.name || candidate.bookName || fallback || '未命名书籍'
}

export function sourceCandidateAuthor(candidate = {}) {
  return candidate.author || candidate.bookAuthor || ''
}

export function sourceCandidateCover(candidate = {}) {
  return candidate.coverUrl || candidate.bookCover || candidate.cover || ''
}

export function sourceCandidateIntro(candidate = {}) {
  return candidate.intro || candidate.desc || candidate.description || ''
}

export function sourceCandidateKind(candidate = {}) {
  return candidate.kind || candidate.category || candidate.genre || ''
}

export function sourceCandidateWordCount(candidate = {}) {
  return candidate.wordCount || candidate.words || ''
}

export function sourceCandidateSourceName(candidate = {}) {
  return candidate.sourceName || candidate.bookSourceName || candidate.originName || candidate.origin || '未知书源'
}

export function sourceCandidateKey(candidate = {}) {
  return `${sourceCandidateSourceId(candidate)}-${sourceCandidateBookUrl(candidate)}`
}
