import { unref } from 'vue'
import { useKeyboard } from './useKeyboard.js'
import { READER_CHAPTER_END_OFFSET } from '../utils/readerPosition.js'

export function useReaderKeyboard(options) {
  const registerKeyboard = options.registerKeyboard || useKeyboard

  registerKeyboard({
    onPageUp: () => options.previousPage(),
    onPageDown: () => options.nextPage(),
    onArrowLeft: () => {
      options.mobileChromeVisible.value = false
      if (options.reader.mode === 'flip') {
        options.previousPage()
      } else if (unref(options.currentIndex) > 0) {
        options.goChapter(
          unref(options.currentIndex) - 1,
          READER_CHAPTER_END_OFFSET,
        )
      }
    },
    onArrowRight: () => {
      options.mobileChromeVisible.value = false
      if (options.reader.mode === 'flip') {
        options.nextPage()
      } else if (
        unref(options.currentIndex) <
        (unref(options.chapters)?.length || 0) - 1
      ) {
        options.goChapter(unref(options.currentIndex) + 1)
      }
    },
    onArrowUp: () => {
      options.mobileChromeVisible.value = false
      if (options.reader.mode === 'page' || unref(options.isScrollRead)) {
        options.previousPage()
      }
    },
    onArrowDown: () => {
      options.mobileChromeVisible.value = false
      if (options.reader.mode === 'page' || unref(options.isScrollRead)) {
        options.nextPage()
      }
    },
    onHome: () => options.scrollToTop(),
    onEnd: () => options.scrollToBottom(),
    onSpace: () => options.nextPage(),
    onEscape: () => {
      if (options.tocVisible.value || options.settingsVisible.value) {
        options.tocVisible.value = false
        options.settingsVisible.value = false
      } else {
        options.mobileChromeVisible.value = false
        options.goShelf()
      }
    },
  })
}
