import { unref } from 'vue'
import {
  readerFlipPageLayout,
  readerVerticalCurrentPage,
  readerVerticalPageCount,
} from '../utils/readerPagination.js'

// reader-dev computePages measures the rendered Content element, not the scroller with its
// bars and bottom spacing; continuous modes span every rendered chapter.
function chapterContentHeight(body) {
  const blocks = body.querySelectorAll?.('.chapter-content') || []
  if (!blocks.length) return Number(body.scrollHeight) || 0
  const first = blocks[0].getBoundingClientRect()
  const last = blocks[blocks.length - 1].getBoundingClientRect()
  return Math.max(0, last.bottom - first.top)
}

export function useReaderLayout(options) {
  const windowTarget = options.windowTarget ?? window

  function readableViewportSize() {
    const viewport = unref(options.contentEl)
    if (!viewport) {
      return {
        width: windowTarget.innerWidth,
        height: windowTarget.innerHeight,
      }
    }
    const style = windowTarget.getComputedStyle(
      unref(options.contentElement) || viewport,
    )
    const horizontalPadding = (
      parseFloat(style.paddingLeft || '0')
      + parseFloat(style.paddingRight || '0')
    )
    const verticalPadding = (
      parseFloat(style.paddingTop || '0')
      + parseFloat(style.paddingBottom || '0')
    )
    return {
      width: Math.max(1, viewport.clientWidth - horizontalPadding),
      height: Math.max(1, viewport.clientHeight - verticalPadding),
    }
  }

  function update() {
    const viewportElement = unref(options.contentEl)
    const body = unref(options.contentBody)
    if (!viewportElement || !body) return
    const viewport = readableViewportSize()
    if (options.reader.mode === 'flip') {
      const layout = readerFlipPageLayout({
        viewportWidth: viewport.width,
        pageStride: Math.max(1, viewport.width - 16),
        viewportHeight: viewport.height,
        scrollWidth: body.scrollWidth,
        currentPage: options.page.value,
      })
      options.pageWidth.value = layout.pageWidth
      options.pageHeight.value = layout.pageHeight
      options.pageCount.value = layout.pageCount
      options.page.value = layout.page
      return
    }
    if (['page', 'scroll', 'scroll2'].includes(options.reader.mode)) {
      const typography = {
        fontSize: options.reader.fontSize,
        lineHeight: options.reader.lineHeight,
        paragraphSpace: options.reader.paragraphSpace,
      }
      const viewportHeight = windowTarget.innerHeight || viewportElement.clientHeight
      const pageCount = readerVerticalPageCount({
        contentHeight: chapterContentHeight(body),
        viewportHeight,
        ...typography,
      })
      options.pageHeight.value = options.getScrollStep()
      options.pageCount.value = pageCount
      options.page.value = readerVerticalCurrentPage({
        scrollTop: viewportElement.scrollTop,
        viewportHeight,
        pageCount,
        ...typography,
      }) - 1
      return
    }
    options.pageCount.value = 1
    options.page.value = 0
  }

  function resize() {
    const previousWidth = Number(options.windowWidth.value) || 0
    const nextWidth = Number(options.getViewportWidth()) || windowTarget.innerWidth
    options.windowWidth.value = nextWidth
    if (
      options.shouldIgnoreHeightOnlyResize?.()
      && Math.abs(nextWidth - previousWidth) < 1
    ) return
    update()
  }

  return {
    readableViewportSize,
    resize,
    update,
  }
}
