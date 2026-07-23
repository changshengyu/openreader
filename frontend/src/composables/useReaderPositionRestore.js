import { nextTick, unref } from 'vue'
import {
  restoredReaderContinuousScrollTop,
  restoredReaderFlipPage,
  restoredReaderSingleChapterScrollTop,
} from '../utils/readerPosition.js'
import { readerElementScrollTop } from '../utils/readerScrollViewport.js'

export function useReaderPositionRestore(options) {
  function restoreByChapterPosition(position) {
    const body = unref(options.contentBody)
    if (!body || !Number.isFinite(position) || position <= 0) return false
    const activeChapter = body.querySelector(
      `.chapter-content[data-index="${unref(options.currentIndex)}"]`,
    ) || body
    const target = options.paragraphByChapterPosition(activeChapter, position)
    if (!target) return false
    options.jumpToParagraph(target, { save: false, flash: false })
    return true
  }

  function restoreContinuous(chapterOffset, restorePercent = null) {
    const viewport = unref(options.contentEl)
    const activeChapter = unref(options.contentBody)?.querySelector(
      `.chapter-content[data-index="${unref(options.currentIndex)}"]`,
    )
    if (!viewport || !activeChapter) return
    const chapterTop = readerElementScrollTop(viewport, activeChapter)
    const scrollTop = restoredReaderContinuousScrollTop({
      offset: chapterOffset,
      percent: restorePercent,
      chapterTop,
      chapterHeight: activeChapter.offsetHeight,
      clientHeight: viewport.clientHeight,
    })
    if (scrollTop !== null) {
      viewport.scrollTop = scrollTop
      return
    }
    if (chapterOffset > 0 && restoreByChapterPosition(chapterOffset)) return
    viewport.scrollTop = chapterTop
  }

  async function restore(offset = 0, restoreOptions = {}) {
    const restorePercent = Number(restoreOptions.restorePercent)
    const hasRestorePercent = Number.isFinite(restorePercent)
    await nextTick()
    await options.nextFrame()
    options.updateLayout()
    const chapterOffset = Number(offset || 0)
    if (options.reader.mode === 'flip') {
      options.page.value = restoredReaderFlipPage({
        offset: chapterOffset,
        percent: hasRestorePercent ? restorePercent : null,
        pageCount: unref(options.pageCount),
      })
      return
    }

    const viewport = unref(options.contentEl)
    if (!viewport) return
    if (unref(options.isContinuousScrollRead)) {
      restoreContinuous(chapterOffset, hasRestorePercent ? restorePercent : null)
      return
    }
    if (
      !hasRestorePercent
      && chapterOffset > 0
      && restoreByChapterPosition(chapterOffset)
    ) return

    const applyScroll = () => {
      const currentViewport = unref(options.contentEl)
      if (!currentViewport) return
      currentViewport.scrollTop = restoredReaderSingleChapterScrollTop({
        offset: chapterOffset,
        percent: hasRestorePercent ? restorePercent : null,
        scrollHeight: currentViewport.scrollHeight,
        clientHeight: currentViewport.clientHeight,
      })
    }
    applyScroll()
    await options.nextFrame()
    applyScroll()
  }

  return {
    restore,
    restoreByChapterPosition,
    restoreContinuous,
  }
}
