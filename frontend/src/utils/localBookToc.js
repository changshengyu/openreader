// reader-dev PopCatalog.vue 110–117 / Index.vue 3013–3020 option names.
export const epubTocRuleOptions = [
  { label: '根据 Spin 获取章节，使用 Toc 补充章节名', value: 'spin+toc' },
  { label: '根据 Spin 获取章节，强制使用 Toc 章节名', value: 'spin<toc' },
  { label: '根据 Spin 获取章节', value: 'spin' },
  { label: '根据 Toc 获取章节，使用 Spin 补充章节名', value: 'toc+spin' },
  { label: '根据 Toc 获取章节，强制使用 Spin 章节名', value: 'toc<spin' },
  { label: '根据 Toc 获取章节', value: 'toc' },
]

export function localBookFileName(book = {}) {
  return String(book.originalFile || book.libraryPath || book.title || '').toLowerCase()
}

export function isTextLocalBook(book = {}) {
  return Number(book.sourceId || 0) <= 0 && /\.(txt|text|md)$/.test(localBookFileName(book))
}

export function isEPUBLocalBook(book = {}) {
  return Number(book.sourceId || 0) <= 0 && /\.epub$/.test(localBookFileName(book))
}

export function isTextLocalPath(path = '') {
  return /\.(txt|text|md)$/i.test(String(path))
}

export function isEPUBLocalPath(path = '') {
  return /\.epub$/i.test(String(path))
}

export function isDirectImportableLocalPath(path = '') {
  return /\.(txt|epub|umd|cbz)$/i.test(String(path))
}
