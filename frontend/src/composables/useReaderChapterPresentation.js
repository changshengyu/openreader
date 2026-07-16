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

  function makeChapterBlock(index, chapterRow, text) {
    const fallback = unref(options.chapters)?.[index] || {}
    const title = chapterRow?.title || fallback.title || `第 ${index + 1} 章`
    const paragraphs = makeParagraphs(text, title)
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
