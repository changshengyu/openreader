export function exploreSourceGroupOptions(sources = []) {
  const groups = []
  const seen = new Set()
  for (const source of Array.isArray(sources) ? sources : []) {
    const value = String(source?.group || '').trim()
    if (!value || seen.has(value)) continue
    seen.add(value)
    groups.push({ label: value, value })
  }
  groups.push({ label: '未分组', value: '未分组' })
  return groups
}

export function filteredExploreSources(sources = [], selectedGroup = '') {
  const rows = Array.isArray(sources) ? sources : []
  const group = String(selectedGroup || '').trim()
  if (!group) return rows
  return rows.filter(source => (String(source?.group || '').trim() || '未分组') === group)
}

export function toggledExploreGroup(currentGroup, nextGroup) {
  const current = String(currentGroup || '').trim()
  const next = String(nextGroup || '').trim()
  return current === next ? '' : next
}

export function expandedExploreSources(current = [], sourceId = '') {
  const values = Array.isArray(current) ? current.map(String) : []
  const id = String(sourceId || '').trim()
  return id ? [...new Set([...values, id])] : [...new Set(values)]
}
