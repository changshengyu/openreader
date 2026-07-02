import { onBeforeUnmount, onMounted } from 'vue'

export function createReaderPageLifecycle(options) {
  const windowTarget = options.windowTarget ?? window
  const documentTarget = options.documentTarget ?? document

  function registerListeners() {
    windowTarget.addEventListener('resize', options.onResize)
    windowTarget.addEventListener('wheel', options.onWheel, { passive: false })
    windowTarget.addEventListener('pagehide', options.onPageHide)
    documentTarget.addEventListener('visibilitychange', options.onVisibilityChange)
    windowTarget.addEventListener('openreader:progress-updated', options.onProgressUpdated)
    windowTarget.addEventListener('openreader:reader-book-data-updated', options.onBookDataUpdated)
    windowTarget.addEventListener('openreader:replace-rules-updated', options.onReplaceRulesUpdated)
    windowTarget.addEventListener('openreader:bookmarks-updated', options.onBookmarksUpdated)
  }

  function unregisterListeners() {
    windowTarget.removeEventListener('resize', options.onResize)
    windowTarget.removeEventListener('wheel', options.onWheel)
    windowTarget.removeEventListener('pagehide', options.onPageHide)
    documentTarget.removeEventListener('visibilitychange', options.onVisibilityChange)
    windowTarget.removeEventListener('openreader:progress-updated', options.onProgressUpdated)
    windowTarget.removeEventListener('openreader:reader-book-data-updated', options.onBookDataUpdated)
    windowTarget.removeEventListener('openreader:replace-rules-updated', options.onReplaceRulesUpdated)
    windowTarget.removeEventListener('openreader:bookmarks-updated', options.onBookmarksUpdated)
  }

  async function mount() {
    options.reader.normalizeSettings()
    options.syncFonts(options.reader.customFontsMap)
    try {
      await options.loadBook()
    } catch (error) {
      options.onBookLoadError(error)
    }
    registerListeners()
    options.customBg.value = options.reader.customBgColor
    options.sliderLineHeight.value = options.reader.lineHeight
  }

  function unmount() {
    options.cancelProgressSave()
    options.clearChapterLoadingTimer()
    options.stopAutoReading()
    options.saveProgress({ force: true, background: true })
    unregisterListeners()
  }

  return {
    mount,
    unmount,
  }
}

export function useReaderPageLifecycle(options) {
  const lifecycle = createReaderPageLifecycle(options)
  onMounted(lifecycle.mount)
  onBeforeUnmount(lifecycle.unmount)
  return lifecycle
}
