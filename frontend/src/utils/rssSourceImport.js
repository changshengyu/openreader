function normalizedURL(value) {
  return String(value || '').trim()
}

const CURRENT_TO_UPSTREAM_FIELDS = {
  title: 'sourceName',
  url: 'sourceUrl',
  icon: 'sourceIcon',
  group: 'sourceGroup',
  comment: 'sourceComment',
}

export function createDefaultRSSSource() {
  return {
    sourceName: '新增RSS源',
    sourceUrl: '',
    sourceIcon: '',
    sourceGroup: '',
    enabled: true,
    singleUrl: true,
    articleStyle: 0,
    ruleArticles: '',
    ruleTitle: '',
    rulePubDate: '',
    ruleImage: '',
    ruleLink: '',
    ruleContent: '',
    enableJs: true,
  }
}

export function toUpstreamRSSSource(source = {}) {
  const result = { ...source }
  for (const [current, upstream] of Object.entries(CURRENT_TO_UPSTREAM_FIELDS)) {
    if (result[upstream] === undefined && result[current] !== undefined) result[upstream] = result[current]
    delete result[current]
  }
  delete result.id
  delete result.userId
  delete result.createdAt
  delete result.updatedAt
  return result
}

export function normalizeRSSSourceImport(payload) {
  if (!Array.isArray(payload)) return []
  return payload.map((rawSource) => {
    const source = toUpstreamRSSSource(rawSource && typeof rawSource === 'object' ? rawSource : {})
    source.sourceName = String(source.sourceName || '').trim()
    source.sourceUrl = normalizedURL(source.sourceUrl)
    if (source.header === undefined && source.headerMap !== undefined) {
      source.header = typeof source.headerMap === 'string'
        ? source.headerMap
        : JSON.stringify(source.headerMap)
    }
    delete source.headerMap
    if (!Object.prototype.hasOwnProperty.call(source, 'singleUrl')) source.singleUrl = false
    return source
  })
}

export function rssSourceRiskTags(source = {}) {
  const sourceText = JSON.stringify(source)
  const tags = []
  if (sourceText.includes('@js:')) tags.push('@Javascript')
  if (sourceText.includes('webView:')) tags.push('@WebView')
  return tags
}

export function safeRSSImportIndexes(sources) {
  return (Array.isArray(sources) ? sources : [])
    .map((source, index) => (rssSourceRiskTags(source).length ? -1 : index))
    .filter(index => index >= 0)
}

export function parseRSSSortOptions(source = {}) {
  if (source.singleUrl) return []
  const sortURL = String(source.sortUrl || '').trim()
  if (!sortURL || sortURL.startsWith('@js:') || sortURL.startsWith('<js>')) return []
  return sortURL
    .replace(/\r\n/g, '\n')
    .split('\n')
    .filter(Boolean)
    .map((row) => {
      const separator = row.indexOf('::')
      if (separator < 0) return { name: row, url: undefined }
      return { name: row.slice(0, separator), url: row.slice(separator + 2) }
    })
}

export function planRSSSourceImport(importedSources, existingSources) {
  const existingByURL = new Map(
    (Array.isArray(existingSources) ? existingSources : [])
      .map(source => [normalizedURL(source?.url), source])
      .filter(([url]) => url),
  )
  const importedByURL = new Map()
  for (const source of Array.isArray(importedSources) ? importedSources : []) {
    const url = normalizedURL(source?.url)
    if (url) importedByURL.set(url, { ...source, url })
  }

  const creates = []
  const updates = []
  for (const [url, source] of importedByURL) {
    const existing = existingByURL.get(url)
    if (existing?.id) {
      updates.push({ id: existing.id, source })
    } else {
      creates.push(source)
    }
  }
  return { creates, updates }
}
