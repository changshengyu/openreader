import { getBrowserCache, listBrowserCacheKeys, removeBrowserCache } from './browserCache'
import { currentUserScope } from './authScope'

const GROUPS = ['bookSourceList', 'rssSources', 'chapterList', 'chapterContent']

export async function currentBrowserLocalCacheStats() {
  const keys = await listBrowserCacheKeys('')
  const stats = emptyStats()
  await Promise.all(keys.map(async (key) => {
    const value = await getBrowserCache(key)
    const size = estimateCacheValueSize(value)
    stats.total.files += 1
    stats.total.size += size
    const group = cacheGroupForKey(key)
    if (group && stats.groups[group]) {
      stats.groups[group].files += 1
      stats.groups[group].size += size
    }
  }))
  return stats
}

export async function clearBrowserLocalCacheGroup(group) {
  const target = String(group || '')
  if (!GROUPS.includes(target)) return 0
  const keys = await listBrowserCacheKeys('')
  const matched = keys.filter(key => cacheGroupForKey(key) === target)
  await Promise.all(matched.map(key => removeBrowserCache(key)))
  return matched.length
}

function emptyStats() {
  return {
    total: { files: 0, size: 0 },
    groups: Object.fromEntries(GROUPS.map(group => [group, { files: 0, size: 0 }])),
  }
}

function cacheGroupForKey(key) {
  const value = String(key || '')
  const unprefixed = value.replace(/^localCache@/, '')
  if (unprefixed.includes('bookSourceList')) return 'bookSourceList'
  if (unprefixed.includes('rssSources')) return 'rssSources'
  if (isCurrentUserChapterListKey(unprefixed)) return 'chapterList'
  if (isCurrentUserChapterContentKey(unprefixed)) return 'chapterContent'
  return ''
}

function isCurrentUserChapterListKey(key) {
  if (key.includes('chapterList')) return true
  if (!key.includes('@chapters:')) return false
  return belongsToCurrentUserOrLegacy(key)
}

function isCurrentUserChapterContentKey(key) {
  if (!key.includes('@chapterContent-')) return false
  return belongsToCurrentUserOrLegacy(key)
}

function belongsToCurrentUserOrLegacy(key) {
  const scope = currentUserScope()
  if (key.startsWith(`reader@${scope}@`)) return true
  if (key.startsWith(`${scope}@`)) return true
  if (/^(reader@)?(user:[^@]+|anonymous)@/.test(key)) return false
  return true
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
