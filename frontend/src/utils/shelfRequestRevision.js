export function createShelfRequestRevisionGate() {
  let scope = ''
  let requestRevision = 0
  let mutationRevision = 0

  function ensureScope(nextScope) {
    const normalized = String(nextScope || '')
    if (scope !== normalized) {
      scope = normalized
      requestRevision += 1
      mutationRevision += 1
    }
    return normalized
  }

  return {
    begin(nextScope) {
      const normalized = ensureScope(nextScope)
      requestRevision += 1
      return {
        scope: normalized,
        requestRevision,
        mutationRevision,
      }
    },
    canCommit(token, nextScope) {
      return Boolean(token)
        && token.scope === ensureScope(nextScope)
        && token.requestRevision === requestRevision
        && token.mutationRevision === mutationRevision
    },
    mutate(nextScope) {
      ensureScope(nextScope)
      mutationRevision += 1
      requestRevision += 1
    },
    reset(nextScope) {
      scope = String(nextScope || '')
      mutationRevision += 1
      requestRevision += 1
    },
  }
}
