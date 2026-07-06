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
          imageStyle: image.imageStyle,
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
  const html = inlineHTML(value, formatText)
  const block = {
    type: 'text',
    text: formatText(text),
    pos,
    endPos: pos + value.length,
  }
  if (html !== block.text) block.html = html
  blocks.push(block)
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
    imageStyle: normalizeImageStyle(image.getAttribute('data-image-style')),
  }
}

function normalizeImageStyle(value) {
  return String(value || '').trim().toUpperCase() === 'FULL' ? 'FULL' : ''
}

function htmlText(value) {
  const source = String(value || '').trim()
  if (!source) return ''
  if (typeof DOMParser === 'undefined' || !/<[^>]+>/.test(source)) {
    return stripHTML(source)
  }
  const document = new DOMParser().parseFromString(source, 'text/html')
  return String(document.body?.textContent || '').replace(/\s+/g, ' ').trim()
}

function inlineHTML(value, formatText = text => text) {
  const source = String(value || '').trim()
  if (!source || !/<[^>]+>/.test(source)) return formatText(stripHTML(source))
  if (typeof DOMParser === 'undefined' || typeof document === 'undefined') {
    return sanitizeInlineHTMLFallback(source, formatText)
  }
  const parsed = new DOMParser().parseFromString(source, 'text/html')
  const fragment = document.createElement('div')
  for (const child of Array.from(parsed.body.childNodes)) {
    appendSanitizedNode(fragment, child, formatText)
  }
  return fragment.innerHTML.trim()
}

const INLINE_TAGS = new Set([
  'B',
  'BR',
  'CODE',
  'DEL',
  'EM',
  'I',
  'MARK',
  'RP',
  'RT',
  'RUBY',
  'S',
  'SMALL',
  'SPAN',
  'STRONG',
  'SUB',
  'SUP',
  'U',
])

function appendSanitizedNode(parent, node, formatText) {
  if (node.nodeType === Node.TEXT_NODE) {
    parent.appendChild(document.createTextNode(formatText(node.textContent || '')))
    return
  }
  if (node.nodeType !== Node.ELEMENT_NODE) return
  if (node.tagName === 'IMG') return
  if (!INLINE_TAGS.has(node.tagName)) {
    for (const child of Array.from(node.childNodes)) {
      appendSanitizedNode(parent, child, formatText)
    }
    return
  }
  const element = document.createElement(node.tagName.toLowerCase())
  for (const child of Array.from(node.childNodes)) {
    appendSanitizedNode(element, child, formatText)
  }
  parent.appendChild(element)
}

function sanitizeInlineHTMLFallback(value, formatText) {
  const source = stripDangerousHTML(value)
    .replace(/<\s*img\b[^>]*>/gi, '')
  return source
    .split(/(<[^>]+>)/g)
    .map(part => {
      if (!part) return ''
      if (part.startsWith('<')) return sanitizeInlineTag(part)
      return escapeHTML(formatText(unescapeBasicEntities(part)))
    })
    .join('')
    .trim()
}

function sanitizeInlineTag(value) {
  const match = /^<\s*(\/)?\s*([a-z0-9]+)[^>]*?>$/i.exec(value)
  if (!match) return ''
  const closing = Boolean(match[1])
  const tag = match[2].toUpperCase()
  if (!INLINE_TAGS.has(tag)) return ''
  if (tag === 'BR') return '<br>'
  return closing ? `</${tag.toLowerCase()}>` : `<${tag.toLowerCase()}>`
}

function stripHTML(value) {
  return unescapeBasicEntities(stripDangerousHTML(String(value || '')).replace(/<[^>]+>/g, '')).replace(/\s+/g, ' ').trim()
}

function stripDangerousHTML(value) {
  return String(value || '')
    .replace(/<\s*(script|style|iframe|object|embed|svg|math)\b[^>]*>[\s\S]*?<\s*\/\s*\1\s*>/gi, '')
    .replace(/<\s*(script|style|iframe|object|embed|svg|math)\b[^>]*\/?\s*>/gi, '')
}

function escapeHTML(value) {
  return String(value || '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function unescapeBasicEntities(value) {
  return String(value || '')
    .replace(/&nbsp;/gi, ' ')
    .replace(/&lt;/gi, '<')
    .replace(/&gt;/gi, '>')
    .replace(/&quot;/gi, '"')
    .replace(/&#39;/gi, "'")
    .replace(/&amp;/gi, '&')
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
