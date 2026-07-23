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

  function effectiveMode() {
    return options.getEffectiveMode?.() || options.reader.mode
  }

  function mobileInterface() {
    return Boolean(unref(options.isMobileReader))
  }

  function layoutState() {
    return [
      effectiveMode(),
      mobileInterface(),
      options.reader.fontFamily,
      options.reader.chineseFont,
      options.reader.fontSize,
      options.reader.fontWeight,
      options.reader.lineHeight,
      options.reader.paragraphSpace,
      options.reader.columnWidth,
    ]
  }

  let transitionGeneration = 0

  watch(
    layoutState,
    async (next, previous, onCleanup) => {
      if (
        Array.isArray(previous)
        && next.length === previous.length
        && next.every((value, index) => Object.is(value, previous[index]))
      ) return

      const generation = ++transitionGeneration
      let active = true
      onCleanup(() => {
        active = false
      })

      const fromMode = previous?.[0] || next[0]
      const fromMobile = Boolean(previous?.[1])
      const toMode = next[0]
      const toMobile = Boolean(next[1])
      const modeLayoutChanged = fromMode !== toMode || fromMobile !== toMobile
      const transition = {
        fromMode,
        toMode,
        fromMobile,
        toMobile,
      }
      const captured = options.capturePosition?.({
        mode: fromMode,
        mobile: fromMobile,
      }) || null
      const offset = captured
        ? Number(captured.offset || 0)
        : options.getCurrentOffset()
      const percent = captured
        ? Number(captured.percent)
        : options.getCurrentPercent?.()

      options.setRestoring?.(true)
      try {
        if (modeLayoutChanged) {
          options.activateCapturedPosition?.(captured)
          options.invalidateChapterWindow?.()
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
        }

        await nextTick()
        if (!active || generation !== transitionGeneration) return
        options.updateLayout()
        if (options.nextFrame) {
          await options.nextFrame()
          if (!active || generation !== transitionGeneration) return
          options.updateLayout()
        }

        let restored = false
        if (captured && options.restoreCapturedPosition) {
          restored = await options.restoreCapturedPosition(
            captured,
            transition,
            () => active && generation === transitionGeneration,
          ) !== false
        }
        if (!restored) {
          const fallbackOffset = captured && fromMode !== toMode ? 0 : offset
          await options.restorePosition(fallbackOffset || 0, {
            ...(Number.isFinite(percent) ? { restorePercent: percent } : {}),
            saveAfterLoad: false,
          })
        }
      } finally {
        if (active && generation === transitionGeneration) {
          options.setRestoring?.(false)
        }
      }
      if (!active || generation !== transitionGeneration) return
      if (options.progressVersion) options.progressVersion.value += 1
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
