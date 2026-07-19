import { bookCategoryIds } from './bookCategory.js'
import { isLocalBook } from './localBook.js'

function groupKey(value) {
  return String(value && typeof value === 'object' ? (value.key || '') : (value || '')).trim()
}

function categoryIdFromGroup(value) {
  const explicit = Number(value?.categoryId || 0)
  if (explicit > 0) return explicit
  const key = groupKey(value)
  if (!key) {
    const legacyId = Number(value?.id || 0)
    return Number.isFinite(legacyId) && legacyId > 0 ? legacyId : 0
  }
  if (!key.startsWith('category:')) return 0
  const parsed = Number(key.slice('category:'.length))
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0
}

export function filterBooksByBookGroup(books, group) {
  const rows = Array.isArray(books) ? books : []
  const key = groupKey(group)
  const categoryId = categoryIdFromGroup(group)
  if (!key && categoryId) {
    return rows.filter(book => bookCategoryIds(book).some(id => Number(id) === categoryId))
  }
  if (!key || key === 'builtin:all') return rows
  if (key === 'builtin:local') return rows.filter(isLocalBook)
  if (key === 'builtin:audio') return rows.filter(book => Number(book?.type) === 1)
  if (key === 'builtin:ungrouped') return rows.filter(book => bookCategoryIds(book).length === 0)
  if (!categoryId) return []
  return rows.filter(book => bookCategoryIds(book).some(id => Number(id) === categoryId))
}

export function bookGroupBookCount(group, books) {
  return filterBooksByBookGroup(books, group).length
}

export function visibleBookGroups(groups, books) {
  return (Array.isArray(groups) ? groups : [])
    .filter(group => group?.show !== false)
    .map(group => ({ ...group, count: bookGroupBookCount(group, books) }))
    .filter(group => group.count > 0)
    .sort((a, b) => Number(a.sortOrder || 0) - Number(b.sortOrder || 0) || groupKey(a).localeCompare(groupKey(b)))
}

export function resolveBookGroupSelection(groups, books, selectedKey) {
  const visible = visibleBookGroups(groups, books)
  const selected = groupKey(selectedKey)
  if (visible.some(group => groupKey(group) === selected)) return selected
  return groupKey(visible[0]) || 'builtin:all'
}
