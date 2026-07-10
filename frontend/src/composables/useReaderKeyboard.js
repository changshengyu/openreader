import { unref } from 'vue'
import { useKeyboard } from './useKeyboard.js'
import { READER_CHAPTER_END_OFFSET } from '../utils/readerPosition.js'

export function useReaderKeyboard(options) {
  const registerKeyboard = options.registerKeyboard || useKeyboard
  const isAudio = () => Boolean(unref(options.isAudio))
  const primaryPanelOpen = () => Boolean(unref(options.primaryPanelOpen))

  registerKeyboard({
    onPageUp: () => {
      if (!isAudio() && !primaryPanelOpen()) options.previousPage()
    },
    onPageDown: () => {
      if (!isAudio() && !primaryPanelOpen()) options.nextPage()
    },
    onArrowLeft: () => {
      if (isAudio() || primaryPanelOpen()) return
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
      if (isAudio() || primaryPanelOpen()) return
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
      if (isAudio() || primaryPanelOpen()) return
      options.mobileChromeVisible.value = false
      if (options.reader.mode === 'page' || unref(options.isScrollRead)) {
        options.previousPage()
      }
    },
    onArrowDown: () => {
      if (isAudio() || primaryPanelOpen()) return
      options.mobileChromeVisible.value = false
      if (options.reader.mode === 'page' || unref(options.isScrollRead)) {
        options.nextPage()
      }
    },
    onHome: () => {
      if (!isAudio() && !primaryPanelOpen()) options.scrollToTop()
    },
    onEnd: () => {
      if (!isAudio() && !primaryPanelOpen()) options.scrollToBottom()
    },
    onSpace: () => {
      if (!isAudio() && !primaryPanelOpen()) options.nextPage()
    },
    onEscape: () => {
      if (primaryPanelOpen()) return
      options.mobileChromeVisible.value = false
      options.goShelf()
    },
  })
}
