function requestKey(query = {}) {
  return [
    query.rootVisible ? '1' : '0',
    query.listVisible ? '1' : '0',
    String(query.sourceId || ''),
    String(query.sort || ''),
    String(query.filter || ''),
    String(query.page || 1),
  ].join('\u001f')
}

export function createRSSArticleRequestGate() {
  let generation = 0

  return {
    begin(query) {
      generation += 1
      return {
        generation,
        key: requestKey(query),
      }
    },
    invalidate() {
      generation += 1
    },
    isCurrent(request, query) {
      return !!request
        && request.generation === generation
        && request.key === requestKey(query)
    },
  }
}
