import { unref } from 'vue'
import { useKeyboard } from './useKeyboard.js'
import { READER_CHAPTER_END_OFFSET } from '../utils/readerPosition.js'

export function useReaderKeyboard(options) {
  const registerKeyboard = options.registerKeyboard || useKeyboard
  const isAudio = () => Boolean(unref(options.isAudio))

  registerKeyboard({
    onPageUp: () => {
      if (!isAudio()) options.previousPage()
    },
    onPageDown: () => {
      if (!isAudio()) options.nextPage()
    },
    onArrowLeft: () => {
      if (isAudio()) return
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
      if (isAudio()) return
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
      if (isAudio()) return
      options.mobileChromeVisible.value = false
      if (options.reader.mode === 'page' || unref(options.isScrollRead)) {
        options.previousPage()
      }
    },
    onArrowDown: () => {
      if (isAudio()) return
      options.mobileChromeVisible.value = false
      if (options.reader.mode === 'page' || unref(options.isScrollRead)) {
        options.nextPage()
      }
    },
    onHome: () => {
      if (!isAudio()) options.scrollToTop()
    },
    onEnd: () => {
      if (!isAudio()) options.scrollToBottom()
    },
    onSpace: () => {
      if (!isAudio()) options.nextPage()
    },
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
