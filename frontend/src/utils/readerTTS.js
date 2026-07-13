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

export function readerTTSBarVisible({ requested, supported, chapterFormat, audio, comic = false }) {
  return Boolean(requested && supported && chapterFormat !== 'epub' && !audio && !comic)
}

export function readerTTSParagraphText(element) {
  return String(element?.innerText || element?.textContent || '').trim()
}

export function readerTTSParagraphElements(root) {
  if (!root?.querySelectorAll) return []
  return Array.from(root.querySelectorAll('h3,p'))
    .filter(element => readerTTSParagraphText(element))
}

export function readerTTSCurrentParagraphIndex(elements, options = {}) {
  const list = Array.isArray(elements) ? elements : []
  if (!list.length) return -1
  const activeIndex = list.findIndex(element => (
    element?.classList?.contains?.('reading') ||
    element?.classList?.contains?.('tts-active')
  ))
  if (activeIndex >= 0) return activeIndex
  const topOffset = Number(options.topOffset ?? 50)
  const slide = options.slide === true
  const visibleIndex = list.findIndex((element) => {
    const rect = element?.getBoundingClientRect?.()
    if (!rect) return false
    return slide ? rect.right > 0 : rect.bottom > topOffset
  })
  return visibleIndex >= 0 ? visibleIndex : 0
}

export function readerTTSProgressLabel({ playing, currentIndex, total }) {
  const paragraphTotal = Number(total) || 0
  if (!playing || paragraphTotal <= 0) return '段落 - / -'
  return `段落 ${Math.min(Number(currentIndex) + 1, paragraphTotal)} / ${paragraphTotal}`
}

export function readerTTSSleepExpired(endAt, now = Date.now()) {
  return Number(endAt) > 0 && Number(now) > Number(endAt)
}
