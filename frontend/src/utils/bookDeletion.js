export const BOOKS_DELETED_EVENT = 'openreader:books-deleted'

export function normalizeDeletedBookIds(value) {
  const source = Array.isArray(value)
    ? value
    : Array.isArray(value?.ids)
      ? value.ids
      : [value?.id ?? value]
  return [...new Set(source
    .map(id => Number(id))
    .filter(id => Number.isInteger(id) && id > 0))]
}

export function deletedBookIdsFromEvent(event) {
  return normalizeDeletedBookIds(event?.detail?.ids ?? event?.detail ?? event)
}

export function dispatchBooksDeleted(ids, target = globalThis.window) {
  const normalized = normalizeDeletedBookIds(ids)
  if (!normalized.length || !target?.dispatchEvent || typeof CustomEvent !== 'function') {
    return normalized
  }
  target.dispatchEvent(new CustomEvent(BOOKS_DELETED_EVENT, {
    detail: { ids: normalized },
  }))
  return normalized
}
