import { nextTick, unref, watch } from 'vue'

export function readerEffectiveMode(
  mode,
  isEPUB,
  isAudio = false,
  isReadBarOpen = false,
  isOrdinaryImageComic = false,
  isAutoReading = false,
) {
  // reader-dev's isCarToon excludes CBZ. Ordinary image-comic chapters use
  // the non-slide page branch, while a CBZ keeps an explicitly selected flip
  // mode for its image-page navigation.
  if (isEPUB || isAudio || isOrdinaryImageComic) return 'page'
  // reader-dev disables slide reading while its read-aloud bar is open. Its
  // non-slide branch is the vertical page interaction, while scroll modes
  // remain native scrolling.
  if ((isReadBarOpen || isAutoReading) && mode === 'flip') return 'page'
  return mode
}

export function readerAutoReadingSupported({
  isEPUB = false,
  isAudio = false,
  isOrdinaryImageComic = false,
} = {}) {
  return !isEPUB && !isAudio && !isOrdinaryImageComic
}

export function readerTTSSupported({
  speechSupported = false,
  isEPUB = false,
  isAudio = false,
  isOrdinaryImageComic = false,
} = {}) {
  return Boolean(speechSupported) && !isEPUB && !isAudio && !isOrdinaryImageComic
}

export function useReaderMode(options) {
  function change(mode) {
    options.reader.setMode(mode)
  }

  watch(
    () => options.reader.mode,
    async () => {
      options.invalidateChapterWindow?.()
      const offset = options.getCurrentOffset()
      options.page.value = 0
      if (unref(options.isEPUB) || unref(options.isAudio)) {
        options.chapterBlocks.value = []
      } else if (unref(options.isContinuousScrollRead)) {
        await options.computeChapterWindow()
      } else {
        options.chapterBlocks.value = [
          options.makeChapterBlock(
            options.currentIndex.value,
            options.chapter.value,
            options.content.value,
            unref(options.cachedImages) || {},
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
