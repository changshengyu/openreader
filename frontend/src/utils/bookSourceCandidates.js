import { sourceCandidateKey } from './sourceCandidate.js'

export function mergeBookSourceCandidates(existing, incoming) {
  const merged = Array.isArray(existing) ? [...existing] : []
  const seen = new Set(merged.map(item => sourceCandidateKey(item)))
  for (const item of incoming || []) {
    const key = sourceCandidateKey(item)
    if (seen.has(key)) continue
    seen.add(key)
    merged.push(item)
  }
  return merged
}

export function buildBookSourceGroups(rows) {
  const counts = new Map()
  for (const item of rows || []) {
    if (item?.enabled === false) continue
    const group = String(item?.group || '').trim()
    if (!group) continue
    counts.set(group, (counts.get(group) || 0) + 1)
  }
  return [...counts.entries()]
    .sort(([a], [b]) => a.localeCompare(b, 'zh-CN'))
    .map(([value, count]) => ({ value, label: value, count }))
}

export function nextBookSourcePage(data, rowCount, currentOffset, limit = 10) {
  return {
    offset: Number.isInteger(data?.nextOffset) ? data.nextOffset : Number(currentOffset || 0) + limit,
    hasMore: typeof data?.hasMore === 'boolean' ? data.hasMore : Number(rowCount || 0) >= limit,
  }
}
