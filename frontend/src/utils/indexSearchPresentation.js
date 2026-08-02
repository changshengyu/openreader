export function visibleSearchMode(searchType) {
  return searchType === 'single' ? 'single' : 'multi'
}

export function storedSearchType(mode, group = '') {
  if (mode === 'single') return 'single'
  return String(group || '').trim() ? 'group' : 'all'
}

export function visibleSearchGroupOptions(sources = []) {
  const enabledSources = Array.isArray(sources)
    ? sources.filter(source => source?.enabled)
    : []
  const groups = new Map()
  for (const source of enabledSources) {
    const group = String(source?.group || '').trim()
    if (!group) continue
    groups.set(group, Number(groups.get(group) || 0) + 1)
  }
  return [
    { label: '全部分组', value: '', count: enabledSources.length },
    ...[...groups.entries()].map(([label, count]) => ({ label, value: label, count })),
  ]
}
