import { unref } from 'vue'
import { useAutoReading } from './useAutoReading.js'

export function useReaderAutoReading(options) {
  const createAutoReading = options.createAutoReading || useAutoReading

  async function advancePage() {
    const beforeChapter = unref(options.currentIndex)
    const beforePage = unref(options.page)
    await options.nextPage()
    return (
      beforeChapter !== unref(options.currentIndex) ||
      beforePage !== unref(options.page)
    )
  }

  function recordProgress() {
    options.progressVersion.value += 1
    options.saveProgress()
  }

  return createAutoReading({
    contentEl: options.contentEl,
    contentBody: options.contentBody,
    isVerticalRead: options.isVerticalRead,
    shouldPause: () => (
      unref(options.isOverlayOpen) ||
      unref(options.mobileChromeVisible)
    ),
    settings: () => ({
      method: options.reader.autoReadingMethod,
      pixel: options.reader.autoReadingPixel,
      interval: options.reader.autoReadingLineTime,
      fontSize: options.reader.fontSize,
      lineHeight: options.reader.lineHeight,
    }),
    currentVisibleParagraph: options.currentVisibleParagraph,
    scrollBehavior: options.scrollBehavior,
    advancePage,
    onProgress: recordProgress,
    onNotify: message => options.notify(message, 1200),
  })
}
