export function readerChapterCacheTargets({
  chapterCount,
  currentIndex,
  count,
  cachedMap,
}) {
  const start = Math.max(0, Number(currentIndex) + 1)
  const total = Math.max(0, Number(chapterCount) || 0)
  if (start >= total) return []
  const end = count === true
    ? total
    : Math.min(total, start + Math.max(0, Number(count) || 0))
  const targets = []
  for (let index = start; index < end; index += 1) {
    if (!cachedMap?.[index]) targets.push(index)
  }
  return targets
}

export function readerChapterCacheStatus(finished, total) {
  return `正在缓存章节 ${Math.max(0, Number(finished) || 0)}/${Math.max(0, Number(total) || 0)}`
}
