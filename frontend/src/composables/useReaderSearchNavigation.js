import { nextTick, unref } from 'vue'
import { bookContentSearchParagraphIndex } from '../utils/readerBookSearch.js'

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
      options.page.value = Math.min(
        options.pageCount.value - 1,
        Math.floor(lineEl.offsetLeft / Math.max(options.pageWidth.value, 1)),
      )
    } else if (options.contentEl.value) {
      options.contentEl.value.scrollTop = Math.max(0, lineEl.offsetTop - 80)
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
    jumpToParagraph,
    jumpToResult,
    jumpToRouteLine,
  }
}
