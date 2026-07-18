import { currentUserScope } from './authScope'

export function currentAuthenticatedIdentity() {
  const token = typeof localStorage === 'undefined'
    ? ''
    : String(localStorage.getItem('openreader_token') || '')
  return {
    scope: currentUserScope(),
    // This value stays only in the short-lived operation closure. It is never
    // placed in Pinia, localStorage, logs, events, or API payloads.
    token,
  }
}

export function createAuthenticatedOperationGuard({
  getIdentity = currentAuthenticatedIdentity,
} = {}) {
  let generation = 0
  const revisions = new Map()

  function begin(key = 'default') {
    const operationKey = String(key)
    const revision = Number(revisions.get(operationKey) || 0) + 1
    revisions.set(operationKey, revision)
    const identity = getIdentity()
    return {
      key: operationKey,
      revision,
      generation,
      scope: identity.scope,
      token: identity.token,
    }
  }

  function canCommit(operation) {
    if (!operation || operation.generation !== generation) return false
    if (revisions.get(operation.key) !== operation.revision) return false
    const identity = getIdentity()
    return identity.scope === operation.scope && identity.token === operation.token
  }

  function invalidate(key = 'default') {
    const operationKey = String(key)
    revisions.set(operationKey, Number(revisions.get(operationKey) || 0) + 1)
  }

  function reset() {
    generation += 1
    revisions.clear()
  }

  return { begin, canCommit, invalidate, reset }
}
