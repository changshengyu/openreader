import { nextTick, unref } from 'vue'
import { bookContentSearchParagraphIndex } from '../utils/readerBookSearch.js'
import { findReaderBookmarkParagraph } from '../utils/readerBookmarkContext.js'
import { readerElementScrollTop } from '../utils/readerScrollViewport.js'

export function useReaderSearchNavigation(options) {
  function paragraphScope() {
    return options.contentBody.value
      ?.querySelector(`.chapter-content[data-index="${options.currentIndex.value}"]`)
      || options.contentBody.value
      || null
  }

  function jumpToLine(index) {
    const lineEl = paragraphScope()?.querySelectorAll('p')?.[index]
    if (!lineEl) return false
    jumpToParagraph(lineEl)
    return true
  }

  function jumpToFirstSearchMatch() {
    const keyword = String(unref(options.keyword) || '').trim()
    if (!keyword) return false
    return jumpToMatch({ query: keyword, resultCountWithinChapter: 0 })
  }

  function jumpToMatch(result) {
    const routeQuery = options.getRouteQuery?.() || {}
    const keyword = String(
      result?.query || unref(options.keyword) || routeQuery.q || '',
    ).trim()
    if (!keyword) return false
    const paragraphs = [...(paragraphScope()?.querySelectorAll('p') || [])]
    if (!paragraphs.length) return false
    const targetIndex = Number.isInteger(result?.resultCountWithinChapter)
      ? result.resultCountWithinChapter
      : Number(result?.resultCountWithinChapter ?? routeQuery.match ?? 0)
    const paragraphIndex = bookContentSearchParagraphIndex(
      paragraphs.map(paragraph => paragraph.textContent || ''),
      keyword,
      targetIndex,
    )
    if (paragraphIndex < 0) return false
    jumpToParagraph(paragraphs[paragraphIndex])
    return true
  }

  function jumpToBookmarkContext(context) {
    const text = String(context || '').trim()
    if (!text || options.canMatchBookmark?.() === false) return false
    const paragraphs = [...(paragraphScope()?.querySelectorAll('h3, p') || [])]
    const match = findReaderBookmarkParagraph({
      selectedText: text,
      paragraphs,
      minSimilarity: 0.6,
    })
    if (!match) return false
    jumpToParagraph(paragraphs[match.index])
    return true
  }

  function jumpToParagraph(lineEl, jumpOptions = {}) {
    if (!lineEl) return
    options.closeDrawer?.()
    const chapterEl = lineEl.closest?.('.chapter-content')
    const chapterIndex = Number(chapterEl?.dataset?.index)
    if (Number.isInteger(chapterIndex) && chapterIndex !== options.currentIndex.value) {
      const block = options.chapterBlocks.value.find(item => item.index === chapterIndex)
      options.currentIndex.value = chapterIndex
      options.chapter.value = options.chapters.value[chapterIndex]
        || (block?.id ? { id: block.id, title: block.title, index: chapterIndex } : options.chapter.value)
      options.content.value = block?.content || options.content.value
    }
    if (options.getMode() === 'flip') {
      const stride = Math.max(options.pageWidth.value, 1)
      const lineRect = lineEl.getBoundingClientRect?.()
      const viewportRect = options.contentEl.value?.getBoundingClientRect?.()
      const renderedPosition = Number.isFinite(lineRect?.left) && Number.isFinite(viewportRect?.left)
        ? options.page.value * stride + lineRect.left - viewportRect.left
        : Number.NaN
      const paragraphPage = Number.isFinite(renderedPosition)
        ? Math.round(renderedPosition / stride)
        : Math.floor(Number(lineEl.offsetLeft || 0) / stride)
      options.page.value = Math.min(
        options.pageCount.value - 1,
        Math.max(0, paragraphPage),
      )
    } else if (options.contentEl.value) {
      options.contentEl.value.scrollTop = Math.max(
        0,
        readerElementScrollTop(options.contentEl.value, lineEl) - 80,
      )
    }
    if (jumpOptions.flash !== false) options.flashParagraph?.(lineEl)
    if (jumpOptions.save !== false) options.saveProgress?.()
  }

  async function jumpToResult(result) {
    options.closeDrawer?.()
    const targetIndex = Number(result?.chapterIndex || 0)
    const resultPercent = Number(result?.percent)
    const restorePercent = Number.isFinite(resultPercent) ? resultPercent : null
    if (targetIndex !== options.currentIndex.value) {
      await options.navigate({
        chapter: targetIndex,
        percent: restorePercent ?? undefined,
      })
    }
    await options.loadChapter(targetIndex, {
      restorePercent,
      saveAfterLoad: true,
    })
    await nextTick()
    if (jumpToMatch(result)) return
    if (Number.isInteger(result?.lineIndex)) {
      jumpToLine(result.lineIndex)
    } else {
      jumpToFirstSearchMatch()
    }
  }

  async function jumpToRouteLine() {
    const routeQuery = options.getRouteQuery?.() || {}
    const bookmarkContext = String(routeQuery.bookmark || '').trim()
    if (bookmarkContext) {
      await nextTick()
      if (jumpToBookmarkContext(bookmarkContext)) return
      if (options.canMatchBookmark?.() !== false) options.onBookmarkNotFound?.()
    }
    if (routeQuery.q !== undefined && routeQuery.match !== undefined) {
      await nextTick()
      if (jumpToMatch({
        query: routeQuery.q,
        resultCountWithinChapter: Number(routeQuery.match),
        lineIndex: Number(routeQuery.line),
      })) {
        return
      }
    }
    if (routeQuery.line === undefined) return
    const index = Number(routeQuery.line)
    if (!Number.isFinite(index)) return
    await nextTick()
    jumpToLine(Math.max(0, Math.floor(index)))
  }

  return {
    jumpToFirstSearchMatch,
    jumpToLine,
    jumpToMatch,
    jumpToBookmarkContext,
    jumpToParagraph,
    jumpToResult,
    jumpToRouteLine,
  }
}
