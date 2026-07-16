export const epubTocRuleOptions = [
  { label: '按 Spine 排序，Toc 补充标题', value: 'spin+toc' },
  { label: '按 Spine 排序，强制使用 Toc 标题', value: 'spin<toc' },
  { label: '仅按 Spine 解析', value: 'spin' },
  { label: '按 Toc 排序，Spine 补充标题', value: 'toc+spin' },
  { label: '按 Toc 排序，强制使用 Spine 标题', value: 'toc<spin' },
  { label: '仅按 Toc 解析', value: 'toc' },
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
