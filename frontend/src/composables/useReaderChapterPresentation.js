import { unref } from 'vue'
import { simplized, traditionalized } from '../utils/chinese.js'
import { parseReaderContentBlocks } from '../utils/readerContent.js'

export function useReaderChapterPresentation(options) {
  function formatChineseText(text) {
    if (!text) return ''
    return unref(options.reader)?.chineseFont === '繁体'
      ? traditionalized(String(text))
      : simplized(String(text))
  }

  function makeParagraphs(value, heading = '') {
    return parseReaderContentBlocks(value, heading, formatChineseText)
  }

  function displayChapterTitle(title) {
    return formatChineseText(title || '')
  }

  function makeChapterBlock(index, chapterRow, text, cachedImages = {}) {
    const fallback = unref(options.chapters)?.[index] || {}
    const title = chapterRow?.title || fallback.title || `第 ${index + 1} 章`
    const parsedParagraphs = makeParagraphs(text, title)
    const paragraphs = mapCachedImageSources(parsedParagraphs, cachedImages)
    const isVolume = Boolean(chapterRow?.isVolume ?? fallback.isVolume)
    const isCBZ = isCBZBook(unref(options.book))
    const isComic = isCBZ || containsImageMarkup(text) || paragraphs.some(item => item.type === 'image')
    const block = {
      index,
      id: chapterRow?.id || fallback.id,
      title: displayChapterTitle(title),
      content: String(text || ''),
      isVolume,
      volumeText: isVolume
        ? paragraphs.filter(item => item.type === 'text').map(item => item.text).join('\n')
        : '',
      paragraphs,
      cachedImages: { ...(cachedImages || {}) },
      imageUrls: paragraphs
        .filter(item => item.type === 'image')
        .map(item => item.src),
    }
    if (isCBZ) {
      block.isCBZ = true
      block.hideTitle = true
    }
    if (isComic) {
      block.isComic = true
    }
    return block
  }

  function chapterBlockTextLength(block) {
    const paragraphs = Array.isArray(block?.paragraphs) ? block.paragraphs : []
    if (!paragraphs.length) return 0
    const last = paragraphs[paragraphs.length - 1]
    return Number(last.endPos || last.pos || 0)
  }

  return {
    chapterBlockTextLength,
    displayChapterTitle,
    formatChineseText,
    makeChapterBlock,
    makeParagraphs,
  }
}

function mapCachedImageSources(paragraphs, cachedImages) {
  if (!cachedImages || typeof cachedImages !== 'object') return paragraphs
  const normalized = new Map()
  for (const [source, capability] of Object.entries(cachedImages)) {
    const sourceURL = normalizedURL(source)
    const capabilityURL = normalizedChapterImageCapability(capability)
    if (sourceURL && capabilityURL) normalized.set(sourceURL, capabilityURL)
  }
  if (!normalized.size) return paragraphs
  return paragraphs.map(paragraph => {
    if (paragraph?.type !== 'image') return paragraph
    const originalSrc = normalizedURL(paragraph.src)
    const cachedSrc = normalized.get(originalSrc)
    if (!cachedSrc) return paragraph
    return {
      ...paragraph,
      originalSrc: paragraph.src,
      fallbackSrc: paragraph.src,
      src: cachedSrc,
    }
  })
}

function normalizedURL(value) {
  try {
    return new URL(String(value || ''), runtimeOrigin()).href
  } catch {
    return ''
  }
}

function normalizedChapterImageCapability(value) {
  try {
    const origin = new URL(runtimeOrigin())
    const parsed = new URL(String(value || ''), origin)
    if (parsed.origin !== origin.origin || parsed.username || parsed.password || parsed.search || parsed.hash) return ''
    if (!/^\/api\/chapter-image\/[^/]+$/.test(parsed.pathname)) return ''
    return parsed.href
  } catch {
    return ''
  }
}

function runtimeOrigin() {
  if (typeof window !== 'undefined' && window.location?.origin) return window.location.origin
  if (typeof globalThis.location !== 'undefined' && globalThis.location?.origin) return globalThis.location.origin
  return 'http://localhost'
}

function containsImageMarkup(value) {
  return /<img\b/i.test(String(value || ''))
}

export function isCBZBook(book) {
  const candidates = [
    book?.url,
    book?.bookUrl,
    book?.libraryPath,
    book?.originalFile,
  ]
  return candidates.some(value => String(value || '').toLowerCase().split(/[?#]/)[0].endsWith('.cbz'))
}
