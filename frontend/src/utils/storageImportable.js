function extensionOf(path) {
  const name = String(path || '').trim().split('/').pop() || ''
  const index = name.lastIndexOf('.')
  return index > 0 ? name.slice(index).toLowerCase() : ''
}

export function isLocalStoreImportable(path) {
  return ['.txt', '.epub', '.umd', '.cbz'].includes(extensionOf(path))
}

export function isWebDAVImportable(path) {
  return ['.txt', '.epub', '.umd'].includes(extensionOf(path))
}
