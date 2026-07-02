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
    return {
      index,
      id: chapterRow?.id || fallback.id,
      title: displayChapterTitle(title),
      content: String(text || ''),
      paragraphs,
      imageUrls: paragraphs
        .filter(item => item.type === 'image')
        .map(item => item.src),
    }
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
