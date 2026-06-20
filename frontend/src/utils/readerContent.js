export function parseReaderContentBlocks(value, heading = '', formatText = text => text) {
  let position = String(heading || '').length + 2
  const blocks = []
  for (const rawLine of String(value || '').split(/\n+/)) {
    const line = rawLine.trim()
    if (!line) continue
    const lineStart = position
    position += line.length + 2
    const imagePattern = /<img\b[^>]*>/gi
    let cursor = 0
    let match
    while ((match = imagePattern.exec(line)) !== null) {
      appendTextBlock(blocks, line.slice(cursor, match.index), lineStart + cursor, formatText)
      const image = parseImageTag(match[0])
      if (image) {
        blocks.push({
          type: 'image',
          src: image.src,
          alt: image.alt,
          text: '',
          pos: lineStart + match.index,
          endPos: lineStart + match.index + match[0].length,
        })
      }
      cursor = match.index + match[0].length
    }
    appendTextBlock(blocks, line.slice(cursor), lineStart + cursor, formatText)
  }
  return blocks
}

function appendTextBlock(blocks, value, pos, formatText) {
  const text = htmlText(value)
  if (!text) return
  blocks.push({
    type: 'text',
    text: formatText(text),
    pos,
    endPos: pos + value.length,
  })
}

function parseImageTag(value) {
  if (typeof DOMParser === 'undefined') return null
  const document = new DOMParser().parseFromString(value, 'text/html')
  const image = document.querySelector('img')
  if (!image) return null
  const source = image.getAttribute('src')
    || image.getAttribute('data-src')
    || image.getAttribute('data-original')
    || image.getAttribute('data-url')
  const src = safeImageURL(source)
  if (!src) return null
  return {
    src,
    alt: String(image.getAttribute('alt') || image.getAttribute('title') || '').trim(),
  }
}

function htmlText(value) {
  const source = String(value || '').trim()
  if (!source) return ''
  if (typeof DOMParser === 'undefined' || !/<[^>]+>/.test(source)) return source
  const document = new DOMParser().parseFromString(source, 'text/html')
  return String(document.body?.textContent || '').replace(/\s+/g, ' ').trim()
}

function safeImageURL(value) {
  const source = String(value || '').trim().replaceAll('__API_ROOT__', window.location.origin)
  if (!source) return ''
  try {
    const parsed = new URL(source, window.location.origin)
    if (!['http:', 'https:'].includes(parsed.protocol)) return ''
    return parsed.href
  } catch {
    return ''
  }
}
