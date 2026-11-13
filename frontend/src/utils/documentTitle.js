// The only writer of the top-level document.title (browser tab, window switcher, history, bookmarks).
// The shell title is the product display name 闲读 (D1, user decision 2026-11-12) and must match the <title>
// in frontend/index.html; documentTitle.test.mjs and displayNameContract.test.mjs check it.
export const SHELL_DOCUMENT_TITLE = '闲读'
// reader-dev titles its standalone source editor <产品名>书源编辑器 (Reader书源编辑器).
export const SOURCE_DEBUG_DOCUMENT_TITLE = `${SHELL_DOCUMENT_TITLE}书源编辑器`

export function formatReaderDocumentTitle(bookName, chapterName) {
  const book = bookName === undefined || bookName === null ? '' : String(bookName)
  if (!book.trim()) return ''
  const chapter = chapterName === undefined || chapterName === null ? '' : String(chapterName).trim()
  return chapter ? `${book} - ${chapter}` : book
}

export function setDocumentTitle(title) {
  if (typeof document === 'undefined' || !title) return
  document.title = title
}

// The home workspace and the login page set the shell title when they mount (reader-dev Index mounted/activated),
// and source debug restores it when it unmounts.
export function restoreShellDocumentTitle() {
  setDocumentTitle(SHELL_DOCUMENT_TITLE)
}
