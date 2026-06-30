export function normalizeTTSSleepMinutes(value) {
  return Math.max(0, Math.min(180, Math.floor(Number(value) || 0)))
}

export function readerTTSProgressLabel({ playing, currentIndex, total }) {
  const paragraphTotal = Number(total) || 0
  if (!playing || paragraphTotal <= 0) return '段落 - / -'
  return `段落 ${Math.min(Number(currentIndex) + 1, paragraphTotal)} / ${paragraphTotal}`
}

export function readerTTSSleepExpired(endAt, now = Date.now()) {
  return Number(endAt) > 0 && Number(now) > Number(endAt)
}
