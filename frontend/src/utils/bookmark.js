export function normalizeImportedBookmarks(rows) {
  return (Array.isArray(rows) ? rows : [])
    .map(row => {
      const chapterIndex = Math.max(0, Math.floor(Number(row.chapterIndex ?? row.durChapterIndex ?? 0)))
      return {
        chapterIndex,
        offset: Math.max(0, Math.floor(Number(row.offset ?? 0))),
        percent: clampPercent(row.percent),
        title: String(row.title || row.chapterName || row.chapterTitle || `第 ${chapterIndex + 1} 章`).trim(),
        excerpt: String(row.excerpt || row.bookText || '').trim(),
        note: String(row.note || row.content || '').trim(),
      }
    })
    .filter(row => row.title || row.excerpt || row.note)
}

function clampPercent(value) {
  const percent = Number(value)
  return Number.isFinite(percent) ? Math.max(0, Math.min(1, percent)) : 0
}
