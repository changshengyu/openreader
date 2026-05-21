function toTime(value) {
  const time = new Date(value || 0).getTime()
  return Number.isFinite(time) ? time : 0
}

export function compareByShelfOrder(a, b) {
  const aProgressAt = toTime(a?.progress?.updatedAt)
  const bProgressAt = toTime(b?.progress?.updatedAt)
  if (aProgressAt !== bProgressAt) return bProgressAt - aProgressAt

  const aShelfAt = Math.max(toTime(a?.updatedAt), toTime(a?.createdAt))
  const bShelfAt = Math.max(toTime(b?.updatedAt), toTime(b?.createdAt))
  if (aShelfAt !== bShelfAt) return bShelfAt - aShelfAt
  return Number(b?.id || 0) - Number(a?.id || 0)
}

export function sortByShelfOrder(books) {
  const list = Array.isArray(books) ? books : []
  return [...list].sort(compareByShelfOrder)
}
