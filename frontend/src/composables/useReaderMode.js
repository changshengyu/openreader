import { nextTick, unref, watch } from 'vue'

export function readerEffectiveMode(mode, isEPUB) {
  return isEPUB ? 'page' : mode
}

export function useReaderMode(options) {
  function change(mode) {
    options.reader.setMode(mode)
  }

  watch(
    () => options.reader.mode,
    async () => {
      const offset = options.getCurrentOffset()
      options.page.value = 0
      if (unref(options.isEPUB)) {
        options.chapterBlocks.value = []
      } else if (unref(options.isContinuousScrollRead)) {
        options.chapterLoading.value = true
        try {
          await options.computeChapterWindow()
        } finally {
          options.chapterLoading.value = false
        }
      } else {
        options.chapterBlocks.value = [
          options.makeChapterBlock(
            options.currentIndex.value,
            options.chapter.value,
            options.content.value,
          ),
        ]
      }
      await nextTick()
      options.updateLayout()
      await options.restorePosition(offset, { saveAfterLoad: false })
      options.saveProgress()
    },
  )

  watch(
    options.isMobileReader,
    mobile => {
      if (!mobile && options.reader.mode === 'flip') {
        options.reader.setMode('page')
      }
    },
    { immediate: true },
  )

  return {
    change,
  }
}
