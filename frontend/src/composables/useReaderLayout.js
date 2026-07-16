import { unref } from 'vue'
import {
  readerFlipPageLayout,
  readerVerticalPageLayout,
} from '../utils/readerPagination.js'

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
    const style = windowTarget.getComputedStyle(viewport)
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
    if (options.reader.mode === 'page') {
      const layout = readerVerticalPageLayout({
        scrollHeight: viewportElement.scrollHeight,
        clientHeight: viewportElement.clientHeight,
        scrollTop: viewportElement.scrollTop,
        pageHeight: options.getScrollStep(),
      })
      options.pageHeight.value = layout.pageHeight
      options.pageCount.value = layout.pageCount
      options.page.value = layout.page
      return
    }
    options.pageCount.value = 1
    options.page.value = 0
  }

  function resize() {
    options.windowWidth.value = options.getViewportWidth()
    update()
  }

  return {
    readableViewportSize,
    resize,
    update,
  }
}
