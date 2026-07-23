import { nextTick, watch } from 'vue'

export function useReaderTypographySync(options) {
  async function syncPosition() {
    const offset = options.getCurrentOffset()
    const restorePercent = options.getCurrentPercent()
    options.setRestoring(true)
    try {
      await nextTick()
      options.updateLayout()
      await options.restorePosition(offset, {
        restorePercent,
        saveAfterLoad: false,
      })
    } finally {
      options.setRestoring(false)
    }
    options.progressVersion.value += 1
    options.scheduleProgressSave(300)
  }

  if (options.watchPosition !== false) {
    watch(
      () => [
        options.reader.fontFamily,
        options.reader.chineseFont,
        options.reader.fontSize,
        options.reader.fontWeight,
        options.reader.lineHeight,
        options.reader.paragraphSpace,
        options.reader.columnWidth,
      ],
      syncPosition,
    )
  }

  watch(() => options.reader.customFontsMap, customFonts => {
    options.syncFonts(customFonts)
  }, { deep: true })

  return {
    syncPosition,
  }
}
