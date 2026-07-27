import { getBrowserCache, listBrowserCacheKeys, removeBrowserCache } from './browserCache.js'
import { currentUserScope } from './authScope.js'

const GROUPS = ['bookSourceList', 'rssSources', 'chapterList', 'chapterContent']

export async function currentBrowserLocalCacheStats(
  scope = currentUserScope(),
  adapters = {},
) {
  const listKeys = adapters.listKeys || listBrowserCacheKeys
  const getCache = adapters.getCache || getBrowserCache
  const keys = await listKeys('')
  const stats = emptyStats()
  await Promise.all(keys.map(async (key) => {
    const metadata = browserLocalCacheKeyMetadata(key, scope)
    if (!metadata.owned) return
    const value = await getCache(key)
    const size = estimateCacheValueSize(value)
    stats.total.files += 1
    stats.total.size += size
    const group = metadata.group
    if (group && stats.groups[group]) {
      stats.groups[group].files += 1
      stats.groups[group].size += size
    }
  }))
  return stats
}

export async function clearBrowserLocalCacheGroup(
  group,
  scope = currentUserScope(),
  adapters = {},
) {
  const target = String(group || '')
  if (!GROUPS.includes(target)) return 0
  const listKeys = adapters.listKeys || listBrowserCacheKeys
  const removeCache = adapters.removeCache || removeBrowserCache
  const keys = await listKeys('')
  const matched = keys.filter(
    key => browserLocalCacheKeyMetadata(key, scope).group === target,
  )
  await Promise.all(matched.map(key => removeCache(key)))
  return matched.length
}

export function browserLocalCacheKeyMetadata(key, scope) {
  const value = String(key || '').replace(/^localCache@/, '')
  const ownerScope = String(scope || '')
  if (!ownerScope) return { owned: false, group: '' }

  if (
    value === `bookSourceList@source-owner-v1@${ownerScope}` ||
    value === `bookSourceList@${ownerScope}`
  ) {
    return { owned: true, group: 'bookSourceList' }
  }
  if (value === `rssSources@${ownerScope}`) {
    return { owned: true, group: 'rssSources' }
  }
  if (value.startsWith(`reader@${ownerScope}@`)) {
    return {
      owned: true,
      group: value.startsWith(`reader@${ownerScope}@chapters:`)
        ? 'chapterList'
        : '',
    }
  }
  if (value.startsWith(`${ownerScope}@`)) {
    return {
      owned: true,
      group: value.includes('@chapterContent-') ? 'chapterContent' : '',
    }
  }
  if (
    value.startsWith('bookshelf@getBookshelf:') &&
    value.endsWith(`:${ownerScope}`)
  ) {
    return { owned: true, group: '' }
  }
  return { owned: false, group: '' }
}

function emptyStats() {
  return {
    total: { files: 0, size: 0 },
    groups: Object.fromEntries(GROUPS.map(group => [group, { files: 0, size: 0 }])),
  }
}

function estimateCacheValueSize(value) {
  if (!value) return 0
  try {
    return new Blob([JSON.stringify(value)]).size
  } catch {
    try {
      return JSON.stringify(value).length
    } catch {
      return 0
    }
  }
}
