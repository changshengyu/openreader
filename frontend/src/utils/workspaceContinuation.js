import {
  remoteBookAuthor,
  remoteBookSourceId,
  remoteBookTitle,
  remoteBookUrl,
} from './remoteBookResult.js'

// reader-dev uses bookUrl as the visible multi-source result identity. A broken
// source can omit that URL, though, so keep those rows distinct with a stable
// source/title/author fallback instead of collapsing every empty URL together.
export function remoteSearchResultKey(book = {}, fallbackSourceId = '') {
  const url = String(remoteBookUrl(book) || '').trim()
  if (url) return `url:${url}`
  const sourceId = String(remoteBookSourceId(book, fallbackSourceId) || 'unknown').trim() || 'unknown'
  const title = String(remoteBookTitle(book) || '').trim()
  const author = String(remoteBookAuthor(book) || '').trim()
  return `fallback:${sourceId}|${title}|${author}`
}

export function mergeRemoteSearchResults(existing = [], incoming = [], fallbackSourceId = '') {
  const rows = Array.isArray(existing) ? [...existing] : []
  const seen = new Set(rows.map(item => remoteSearchResultKey(item, fallbackSourceId)))
  let added = 0
  for (const item of Array.isArray(incoming) ? incoming : []) {
    const key = remoteSearchResultKey(item, fallbackSourceId)
    if (seen.has(key)) continue
    seen.add(key)
    rows.push(item)
    added += 1
  }
  return { rows, added }
}

export function createAsyncRequestGate() {
  let current = 0
  return {
    begin() {
      current += 1
      return current
    },
    invalidate() {
      current += 1
    },
    isCurrent(token) {
      return token === current
    },
  }
}

export function captureWorkspaceRequest(workspace, mode) {
  const revisionKey = mode === 'explore' ? 'exploreRevision' : 'searchRevision'
  return {
    mode,
    revision: Number(workspace?.[revisionKey] || 0),
  }
}

export function isWorkspaceRequestCurrent(workspace, stamp) {
  if (!workspace || !stamp || workspace.mode !== stamp.mode) return false
  const revisionKey = stamp.mode === 'explore' ? 'exploreRevision' : 'searchRevision'
  return Number(workspace[revisionKey] || 0) === stamp.revision
}
