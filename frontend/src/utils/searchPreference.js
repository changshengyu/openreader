export const SEARCH_CONCURRENT_OPTIONS = Object.freeze([12, 18, 24, 30, 36, 42, 48, 54, 60])
export const LEGACY_SEARCH_CONCURRENT_OPTIONS = Object.freeze([8, 16, 32])
export const DEFAULT_SEARCH = Object.freeze({
  searchType: 'all',
  group: '',
  sourceId: '',
  concurrent: 24,
})

export function sanitizeSearchPreference(value = {}) {
  const searchType = ['all', 'group', 'single'].includes(value.searchType)
    ? value.searchType
    : DEFAULT_SEARCH.searchType
  return {
    ...DEFAULT_SEARCH,
    searchType,
    group: typeof value.group === 'string' ? value.group : '',
    sourceId: value.sourceId === undefined || value.sourceId === null ? '' : value.sourceId,
    concurrent: normalizeSearchConcurrent(value.concurrent),
  }
}

export function normalizeSearchConcurrent(value, fallback = DEFAULT_SEARCH.concurrent) {
  const parsed = Number(value)
  if (!Number.isInteger(parsed)) return fallback
  if (SEARCH_CONCURRENT_OPTIONS.includes(parsed)) return parsed
  if (LEGACY_SEARCH_CONCURRENT_OPTIONS.includes(parsed)) return parsed
  return fallback
}

export function searchConcurrentOptions(currentValue) {
  const normalized = normalizeSearchConcurrent(currentValue)
  if (!LEGACY_SEARCH_CONCURRENT_OPTIONS.includes(normalized)) {
    return [...SEARCH_CONCURRENT_OPTIONS]
  }
  return [normalized, ...SEARCH_CONCURRENT_OPTIONS]
}

export function isLegacySearchConcurrent(value) {
  return LEGACY_SEARCH_CONCURRENT_OPTIONS.includes(Number(value))
}

export function searchConcurrentLabel(value) {
  const suffix = isLegacySearchConcurrent(value) ? '（旧配置）' : ''
  return `${value}并发线程${suffix}`
}
