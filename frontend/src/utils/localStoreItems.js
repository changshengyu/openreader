export const LOCAL_STORE_INITIAL_ITEM_LIMIT = 101

export function shouldFilterLocalStoreItems(keyword) {
  return String(keyword || '').trim().length > 2
}

export function filterLocalStoreItems(items, options = {}) {
  const keyword = String(options.keyword || '').trim().toLowerCase()
  const extension = String(options.extension || '')
  return (Array.isArray(items) ? items : []).filter(item => {
    if (extension && !item.isDir && item.extension !== extension) return false
    if (extension && item.isDir) return true
    if (!shouldFilterLocalStoreItems(keyword)) return true
    return `${item.name || ''} ${item.path || ''}`.toLowerCase().includes(keyword)
  })
}

export function limitLocalStoreItems(items, limit = 100) {
  const rows = Array.isArray(items) ? items : []
  const size = Math.max(1, Math.floor(Number(limit) || 1))
  return rows.slice(0, size)
}

export function visibleLocalStoreItems(items, expanded = false) {
  const rows = Array.isArray(items) ? items : []
  return expanded ? rows : limitLocalStoreItems(rows, LOCAL_STORE_INITIAL_ITEM_LIMIT)
}
