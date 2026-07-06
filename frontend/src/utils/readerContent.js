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
  const image = parseImageAttributes(value)
  if (!image) return null
  const source = image.src
    || image['data-src']
    || image['data-original']
    || image['data-url']
  const src = safeImageURL(source)
  if (!src) return null
  return {
    src,
    alt: String(image.alt || image.title || '').trim(),
    imageStyle: normalizeImageStyle(image['data-image-style']),
  }
}

function parseImageAttributes(value) {
  if (typeof DOMParser !== 'undefined') {
    const document = new DOMParser().parseFromString(value, 'text/html')
    const image = document.querySelector('img')
    if (!image) return null
    return {
      src: image.getAttribute('src') || '',
      'data-src': image.getAttribute('data-src') || '',
      'data-original': image.getAttribute('data-original') || '',
      'data-url': image.getAttribute('data-url') || '',
      alt: image.getAttribute('alt') || '',
      title: image.getAttribute('title') || '',
      'data-image-style': image.getAttribute('data-image-style') || '',
    }
  }
  const tag = String(value || '').match(/<\s*img\b[^>]*>/i)?.[0]
  if (!tag) return null
  const attributes = {}
  const attrPattern = /([:@A-Za-z0-9_-]+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>`]+)))?/g
  let match
  while ((match = attrPattern.exec(tag)) !== null) {
    const name = String(match[1] || '').toLowerCase()
    if (!name || name === 'img') continue
    attributes[name] = match[2] ?? match[3] ?? match[4] ?? ''
  }
  return attributes
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
  const origin = runtimeOrigin()
  const source = String(value || '').trim().replaceAll('__API_ROOT__', origin)
  if (!source) return ''
  try {
    const parsed = new URL(source, origin)
    if (!['http:', 'https:'].includes(parsed.protocol)) return ''
    return parsed.href
  } catch {
    return ''
  }
}

function runtimeOrigin() {
  if (typeof window !== 'undefined' && window.location?.origin) {
    return window.location.origin
  }
  if (typeof globalThis.location !== 'undefined' && globalThis.location?.origin) {
    return globalThis.location.origin
  }
  return 'http://localhost'
}
