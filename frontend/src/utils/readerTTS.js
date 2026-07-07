export function normalizeTTSRate(value) {
  const numeric = Number(value)
  return Math.max(0.5, Math.min(2, Number.isFinite(numeric) ? numeric : 1))
}

export function normalizeTTSPitch(value) {
  const numeric = Number(value)
  return Math.max(0, Math.min(2, Number.isFinite(numeric) ? numeric : 1))
}

export function normalizeTTSSleepMinutes(value) {
  return Math.max(0, Math.min(180, Math.floor(Number(value) || 0)))
}

export function sortTTSVoices(voices) {
  return [...(Array.isArray(voices) ? voices : [])].sort((a, b) => {
    const aLang = String(a?.lang || '')
    const bLang = String(b?.lang || '')
    const aChinese = aLang.startsWith('zh-')
    const bChinese = bLang.startsWith('zh-')
    if (aChinese && !bChinese) return -1
    if (!aChinese && bChinese) return 1
    return aLang.localeCompare(bLang)
  })
}

export function readerTTSBarVisible({ requested, supported, chapterFormat, audio }) {
  return Boolean(requested && supported && chapterFormat !== 'epub' && !audio)
}

export function readerTTSProgressLabel({ playing, currentIndex, total }) {
  const paragraphTotal = Number(total) || 0
  if (!playing || paragraphTotal <= 0) return '段落 - / -'
  return `段落 ${Math.min(Number(currentIndex) + 1, paragraphTotal)} / ${paragraphTotal}`
}

export function readerTTSSleepExpired(endAt, now = Date.now()) {
  return Number(endAt) > 0 && Number(now) > Number(endAt)
}
