function normalizedURL(value) {
  return String(value || '').trim()
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
