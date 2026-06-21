export function bookCategoryIds(book) {
  const ids = Array.isArray(book?.categoryIds) ? book.categoryIds : []
  const values = ids
    .map(id => Number(id))
    .filter(id => Number.isFinite(id) && id > 0)
  if (values.length === 0 && book?.categoryId) {
    const id = Number(book.categoryId)
    if (Number.isFinite(id) && id > 0) values.push(id)
  }
  return [...new Set(values)]
}

export function bookCategoryName(bookOrId, categories = [], fallback = '未分组') {
  const ids = typeof bookOrId === 'object'
    ? bookCategoryIds(bookOrId)
    : (bookOrId ? [Number(bookOrId)] : [])
  if (!ids.length) return fallback
  const names = ids
    .map(id => categories.find(category => Number(category.id) === Number(id))?.name)
    .filter(Boolean)
  return names.length ? names.join('、') : fallback
}

export function createBookCategoryNameResolver(getCategories, fallback = '未分组') {
  return bookOrId => bookCategoryName(bookOrId, getCategories() || [], fallback)
}
