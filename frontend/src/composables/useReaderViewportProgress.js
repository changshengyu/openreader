import { nextTick } from 'vue'
import { readerFlipChapterPercent } from '../utils/readerPagination.js'
import { restoredReaderScrollTop } from '../utils/readerScrollAnchor.js'
import {
  findTopVisibleReaderBlock,
  findVisibleReaderBlock,
  readerBlockTextOffset,
  readerScrollTextOffset,
  readerTextProgress,
} from '../utils/readerVisibility.js'

export function useReaderViewportProgress(options) {
  function isEPUB() {
    return Boolean(options.isEPUB?.value)
  }

  function visibleParagraphIn(viewportTarget, mode = options.getMode()) {
    const viewport = viewportTarget?.getBoundingClientRect?.()
    const paragraphs = [...(
      options.contentBody.value?.querySelectorAll('h3[data-pos], [data-reader-block]') || []
    )]
    if (!viewport || !paragraphs.length) return null
    return findVisibleReaderBlock(paragraphs, viewport, 8, mode !== 'flip')
  }

  function currentVisibleParagraph() {
    return visibleParagraphIn(options.contentEl.value, options.getMode())
  }

  function captureReaderLayoutPosition({
    viewport = options.contentEl.value,
    mode = options.getMode(),
  } = {}) {
    if (!viewport) return null
    if (isEPUB()) {
      const bottom = Math.max(viewport.scrollHeight - viewport.clientHeight, 1)
      const offset = Math.max(0, Math.round(Number(viewport.scrollTop) || 0))
      return {
        chapterIndex: options.currentIndex.value,
        offset,
        percent: Math.max(0, Math.min(1, offset / bottom)),
      }
    }

    const paragraph = visibleParagraphIn(viewport, mode)
    if (!paragraph) {
      const offset = mode === 'flip'
        ? Math.max(0, Math.floor(options.page.value || 0))
        : readerScrollTextOffset({
            scrollTop: viewport.scrollTop,
            scrollHeight: viewport.scrollHeight,
            clientHeight: viewport.clientHeight,
            textLength: options.chapterTextLength.value,
          })
      const percent = mode === 'flip'
        ? readerFlipChapterPercent(options.page.value, options.pageCount.value)
        : readerTextProgress(offset, options.chapterTextLength.value)
      return {
        chapterIndex: options.currentIndex.value,
        offset,
        percent,
      }
    }

    const chapterEl = paragraph.closest?.('.chapter-content')
    const chapterIndex = Number(chapterEl?.dataset?.index)
    const safeChapterIndex = Number.isInteger(chapterIndex)
      ? chapterIndex
      : options.currentIndex.value
    const block = options.displayedChapterBlocks.value.find(item => item.index === safeChapterIndex)
      || options.chapterBlocks.value.find(item => item.index === safeChapterIndex)
      || (
        safeChapterIndex === options.currentIndex.value
          ? options.makeChapterBlock(
              options.currentIndex.value,
              options.chapter.value,
              options.content.value,
              options.cachedImages?.value || {},
            )
          : null
      )
    const paragraphPos = Number(paragraph.dataset?.pos)
    const viewportRect = viewport.getBoundingClientRect?.()
    const paragraphRect = paragraph.getBoundingClientRect?.()
    const offset = Number.isFinite(paragraphPos)
      ? readerBlockTextOffset({
          blockPosition: paragraphPos,
          textLength: paragraph.textContent?.length || 0,
          blockRect: paragraphRect,
          viewport: viewportRect,
        })
      : 0
    const textLength = Math.max(options.chapterBlockTextLength(block), 1)
    const chapterNodes = [...(
      chapterEl?.querySelectorAll?.('h3[data-pos], [data-reader-block]') || []
    )]

    return {
      chapterIndex: safeChapterIndex,
      paragraph,
      paragraphPos: Number.isFinite(paragraphPos) ? paragraphPos : null,
      paragraphIndex: Math.max(0, chapterNodes.indexOf(paragraph)),
      paragraphTag: String(paragraph.tagName || '').toLowerCase(),
      offset: mode === 'flip' ? Math.max(0, Math.floor(options.page.value || 0)) : offset,
      percent: mode === 'flip'
        ? readerFlipChapterPercent(options.page.value, options.pageCount.value)
        : readerTextProgress(offset, textLength),
    }
  }

  function currentProgressElement() {
    if (!options.isContinuousScrollRead.value) return currentVisibleParagraph()
    const viewport = options.contentEl.value?.getBoundingClientRect()
    const elements = [...(
      options.contentBody.value?.querySelectorAll('h3[data-pos], [data-reader-block]') || []
    )]
    if (!viewport || !elements.length) return null
    return findTopVisibleReaderBlock(elements, viewport, options.continuousTopInset ?? 50)
  }

  function visibleParagraphOffset(paragraph, paragraphPos) {
    if (Number(paragraphPos) <= 0) return 0
    const viewport = options.contentEl.value?.getBoundingClientRect()
    return readerBlockTextOffset({
      blockPosition: paragraphPos,
      textLength: paragraph.textContent?.length || 0,
      blockRect: viewport ? paragraph.getBoundingClientRect() : null,
      viewport,
    })
  }

  function visibleChapterProgressSnapshot() {
    if (!options.contentEl.value || !options.contentBody.value) return null
    const paragraph = currentProgressElement()
    if (!paragraph) return null
    const chapterEl = paragraph.closest?.('.chapter-content')
    const chapterIndex = Number(chapterEl?.dataset?.index)
    if (!Number.isInteger(chapterIndex)) return null
    const block = options.displayedChapterBlocks.value.find(item => item.index === chapterIndex)
      || options.chapterBlocks.value.find(item => item.index === chapterIndex)
      || (
        chapterIndex === options.currentIndex.value
          ? options.makeChapterBlock(
              options.currentIndex.value,
              options.chapter.value,
              options.content.value,
              options.cachedImages?.value || {},
            )
          : null
      )
    const paragraphPos = Number(paragraph.dataset?.pos)
    const offset = Number.isFinite(paragraphPos)
      ? visibleParagraphOffset(paragraph, paragraphPos)
      : 0
    const textLength = Math.max(options.chapterBlockTextLength(block), 1)
    return {
      chapterIndex,
      chapter: options.chapters.value[chapterIndex]
        || (block?.id ? { id: block.id, title: block.title, index: chapterIndex } : null),
      offset,
      chapterPercent: readerTextProgress(offset, textLength),
    }
  }

  function activeChapterElement() {
    const paragraph = currentProgressElement()
    const chapterEl = paragraph?.closest?.('.chapter-content')
    if (chapterEl) return chapterEl
    return options.contentBody.value
      ?.querySelector(`.chapter-content[data-index="${options.currentIndex.value}"]`)
      || null
  }

  function currentChapterPosition() {
    if (isEPUB()) {
      return Math.max(0, Number(options.contentEl.value?.scrollTop) || 0)
    }
    const snapshot = visibleChapterProgressSnapshot()
    if (snapshot) return snapshot.offset
    const el = options.contentEl.value
    if (!el) return 0
    const activeChapter = activeChapterElement()
    const heading = activeChapter?.querySelector('h3') || options.contentBody.value?.querySelector('h3')
    const viewport = el.getBoundingClientRect()
    const headingRect = heading?.getBoundingClientRect()
    if (headingRect && headingRect.bottom >= viewport.top && headingRect.top <= viewport.bottom) return 0
    const paragraph = currentProgressElement()
    const paragraphPos = Number(paragraph?.dataset?.pos)
    if (Number.isFinite(paragraphPos)) {
      return readerBlockTextOffset({
        blockPosition: paragraphPos,
        textLength: paragraph.textContent?.length || 0,
        blockRect: paragraph.getBoundingClientRect(),
        viewport,
      })
    }
    return readerScrollTextOffset({
      scrollTop: el.scrollTop,
      scrollHeight: el.scrollHeight,
      clientHeight: el.clientHeight,
      textLength: options.chapterTextLength.value,
    })
  }

  function currentChapterPercent() {
    options.progressVersion.value
    if (isEPUB()) {
      const el = options.contentEl.value
      if (!el) return 0
      const bottom = Math.max(el.scrollHeight - el.clientHeight, 1)
      return Math.max(0, Math.min(1, (Number(el.scrollTop) || 0) / bottom))
    }
    if (options.getMode() === 'flip') {
      return readerFlipChapterPercent(options.page.value, options.pageCount.value)
    }
    const snapshot = visibleChapterProgressSnapshot()
    if (snapshot) return snapshot.chapterPercent
    const el = options.contentEl.value
    if (!el) return 0
    const textLength = Math.max(options.chapterTextLength.value, 1)
    const position = currentChapterPosition()
    if (position > 0 || options.isContinuousScrollRead.value) {
      return readerTextProgress(position, textLength)
    }
    const bottom = Math.max(el.scrollHeight - el.clientHeight, 1)
    const scrollTop = Number(el.scrollTop || 0)
    if (scrollTop > 0) return scrollTop / bottom
    return position / textLength
  }

  function currentOffset() {
    if (isEPUB()) {
      return Math.max(0, Math.round(Number(options.contentEl.value?.scrollTop) || 0))
    }
    if (options.getMode() === 'flip') {
      return Math.max(0, Math.floor(options.page.value || 0))
    }
    const snapshot = visibleChapterProgressSnapshot()
    if (snapshot) return snapshot.offset
    return currentChapterPosition()
  }

  function captureReaderScrollAnchor() {
    if (!options.isContinuousScrollRead.value || !options.contentEl.value) return null
    const paragraph = currentProgressElement()
    const chapterEl = paragraph?.closest?.('.chapter-content')
    const chapterIndex = Number(chapterEl?.dataset?.index)
    const paragraphPos = Number(paragraph?.dataset?.pos)
    if (!paragraph || !Number.isInteger(chapterIndex) || !Number.isFinite(paragraphPos)) return null
    const viewport = options.contentEl.value.getBoundingClientRect()
    return {
      chapterIndex,
      paragraphPos,
      viewportOffset: paragraph.getBoundingClientRect().top - viewport.top,
    }
  }

  async function restoreReaderScrollAnchor(anchor) {
    if (!anchor || !options.contentEl.value || !options.contentBody.value) return
    await nextTick()
    await options.nextFrame()
    const chapterEl = options.contentBody.value
      .querySelector(`.chapter-content[data-index="${anchor.chapterIndex}"]`)
    const paragraph = chapterEl
      ?.querySelector(`[data-pos="${anchor.paragraphPos}"]`)
    if (!paragraph || !options.contentEl.value) return
    const viewport = options.contentEl.value.getBoundingClientRect()
    const currentOffset = paragraph.getBoundingClientRect().top - viewport.top
    const maxScroll = Math.max(
      0,
      options.contentEl.value.scrollHeight - options.contentEl.value.clientHeight,
    )
    options.contentEl.value.scrollTop = restoredReaderScrollTop({
      scrollTop: options.contentEl.value.scrollTop,
      previousOffset: anchor.viewportOffset,
      currentOffset,
      maxScroll,
    })
  }

  return {
    activeChapterElement,
    captureReaderLayoutPosition,
    captureReaderScrollAnchor,
    currentChapterPercent,
    currentChapterPosition,
    currentProgressElement,
    currentOffset,
    currentVisibleParagraph,
    restoreReaderScrollAnchor,
    visibleChapterProgressSnapshot,
  }
}
