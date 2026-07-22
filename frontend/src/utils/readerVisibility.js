function finiteNumber(value) {
  const number = Number(value)
  return Number.isFinite(number) ? number : 0
}

function clampUnit(value) {
  return Math.max(0, Math.min(1, finiteNumber(value)))
}

export function readerViewportAnchorY(viewport) {
  const top = finiteNumber(viewport?.top)
  const height = Math.max(0, finiteNumber(viewport?.height))
  return top + Math.min(height * 0.32, 180)
}

export function selectVisibleReaderBlock(entries, viewport, inset = 8) {
  if (!viewport || !Array.isArray(entries) || !entries.length) return null
  const padding = Math.max(0, finiteNumber(inset))
  const visibleTop = finiteNumber(viewport.top) + padding
  const visibleBottom = finiteNumber(viewport.bottom) - padding
  const visibleLeft = finiteNumber(viewport.left) + padding
  const visibleRight = finiteNumber(viewport.right) - padding
  const anchorY = readerViewportAnchorY(viewport)
  const visible = entries.filter(({ rect }) => (
    rect
    && finiteNumber(rect.bottom) >= visibleTop
    && finiteNumber(rect.top) <= visibleBottom
    && finiteNumber(rect.right) >= visibleLeft
    && finiteNumber(rect.left) <= visibleRight
  ))
  if (!visible.length) return null
  const anchored = visible.find(({ rect }) => (
    finiteNumber(rect.top) <= anchorY && finiteNumber(rect.bottom) >= anchorY
  ))
  if (anchored) return anchored.node || null
  return [...visible]
    .sort((a, b) => (
      Math.abs(finiteNumber(a.rect.top) - anchorY)
      - Math.abs(finiteNumber(b.rect.top) - anchorY)
    ))[0]?.node || null
}

export function findVisibleReaderBlock(nodes, viewport, inset = 8, verticallyOrdered = true) {
  if (!viewport || !nodes?.length) return null
  const padding = Math.max(0, finiteNumber(inset))
  const visibleTop = finiteNumber(viewport.top) + padding
  const visibleBottom = finiteNumber(viewport.bottom) - padding
  const visibleLeft = finiteNumber(viewport.left) + padding
  const visibleRight = finiteNumber(viewport.right) - padding
  const anchorY = readerViewportAnchorY(viewport)
  let nearest = null
  let nearestDistance = Infinity

  for (const node of nodes) {
    const rect = node?.getBoundingClientRect?.()
    if (!rect) continue
    if (verticallyOrdered && finiteNumber(rect.top) > visibleBottom) break
    if (
      finiteNumber(rect.bottom) < visibleTop
      || finiteNumber(rect.right) < visibleLeft
      || finiteNumber(rect.left) > visibleRight
    ) continue
    if (finiteNumber(rect.top) <= anchorY && finiteNumber(rect.bottom) >= anchorY) {
      return node
    }
    const distance = Math.abs(finiteNumber(rect.top) - anchorY)
    if (distance < nearestDistance) {
      nearest = node
      nearestDistance = distance
    }
  }
  return nearest
}

export function selectTopVisibleReaderBlock(entries, viewport, topInset = 50, sideInset = 8) {
  if (!viewport || !Array.isArray(entries) || !entries.length) return null
  const boundary = finiteNumber(viewport.top) + Math.max(0, finiteNumber(topInset))
  const visibleLeft = finiteNumber(viewport.left) + Math.max(0, finiteNumber(sideInset))
  const visibleRight = finiteNumber(viewport.right) - Math.max(0, finiteNumber(sideInset))
  return entries.find(({ rect }) => (
    rect
    && finiteNumber(rect.bottom) > boundary
    && finiteNumber(rect.right) >= visibleLeft
    && finiteNumber(rect.left) <= visibleRight
  ))?.node || null
}

export function findTopVisibleReaderBlock(nodes, viewport, topInset = 50, sideInset = 8) {
  if (!viewport || !nodes?.length) return null
  const boundary = finiteNumber(viewport.top) + Math.max(0, finiteNumber(topInset))
  const visibleLeft = finiteNumber(viewport.left) + Math.max(0, finiteNumber(sideInset))
  const visibleRight = finiteNumber(viewport.right) - Math.max(0, finiteNumber(sideInset))
  for (const node of nodes) {
    const rect = node?.getBoundingClientRect?.()
    if (!rect) continue
    if (
      finiteNumber(rect.bottom) > boundary
      && finiteNumber(rect.right) >= visibleLeft
      && finiteNumber(rect.left) <= visibleRight
    ) return node
  }
  return null
}

export function readerBlockTextOffset({
  blockPosition,
  textLength,
  blockRect,
  viewport,
}) {
  const position = finiteNumber(blockPosition)
  if (!viewport || !blockRect) return Math.max(0, Math.round(position))
  const anchorY = readerViewportAnchorY(viewport)
  const height = Math.max(0, finiteNumber(blockRect.height))
  const ratio = height > 0
    ? clampUnit((anchorY - finiteNumber(blockRect.top)) / height)
    : 0
  const extra = Math.round(Math.max(0, finiteNumber(textLength)) * ratio)
  return Math.max(0, Math.round(position + extra))
}

export function readerScrollTextOffset({
  scrollTop,
  scrollHeight,
  clientHeight,
  textLength,
}) {
  const bottom = Math.max(finiteNumber(scrollHeight) - finiteNumber(clientHeight), 1)
  const percent = clampUnit(finiteNumber(scrollTop) / bottom)
  return percent > 0
    ? Math.round(percent * Math.max(finiteNumber(textLength), 1))
    : 0
}

export function readerTextProgress(offset, textLength) {
  return clampUnit(finiteNumber(offset) / Math.max(finiteNumber(textLength), 1))
}
