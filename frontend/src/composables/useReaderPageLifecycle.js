import { onBeforeUnmount, onMounted } from 'vue'

export function createReaderPageLifecycle(options) {
  const windowTarget = options.windowTarget ?? window
  const documentTarget = options.documentTarget ?? document
  let sessionInvalidated = false

  function handlePageHide(event) {
    if (!sessionInvalidated) options.onPageHide(event)
  }

  function handleVisibilityChange(event) {
    if (!sessionInvalidated) options.onVisibilityChange(event)
  }

  function handleSessionInvalidated(event) {
    if (sessionInvalidated) return
    sessionInvalidated = true
    options.onSessionInvalidated?.(event)
  }

  function registerListeners() {
    windowTarget.addEventListener('resize', options.onResize)
    windowTarget.addEventListener('wheel', options.onWheel, { passive: false })
    windowTarget.addEventListener('scroll', options.onScroll, { passive: true })
    windowTarget.addEventListener('pagehide', handlePageHide)
    windowTarget.addEventListener('openreader:session-invalidated', handleSessionInvalidated)
    documentTarget.addEventListener('visibilitychange', handleVisibilityChange)
    windowTarget.addEventListener('openreader:progress-updated', options.onProgressUpdated)
    windowTarget.addEventListener('openreader:reader-book-data-updated', options.onBookDataUpdated)
    windowTarget.addEventListener('openreader:replace-rules-updated', options.onReplaceRulesUpdated)
    windowTarget.addEventListener('openreader:bookmarks-updated', options.onBookmarksUpdated)
    windowTarget.addEventListener('openreader:books-deleted', options.onBooksDeleted)
  }

  function unregisterListeners() {
    windowTarget.removeEventListener('resize', options.onResize)
    windowTarget.removeEventListener('wheel', options.onWheel)
    windowTarget.removeEventListener('scroll', options.onScroll)
    windowTarget.removeEventListener('pagehide', handlePageHide)
    windowTarget.removeEventListener('openreader:session-invalidated', handleSessionInvalidated)
    documentTarget.removeEventListener('visibilitychange', handleVisibilityChange)
    windowTarget.removeEventListener('openreader:progress-updated', options.onProgressUpdated)
    windowTarget.removeEventListener('openreader:reader-book-data-updated', options.onBookDataUpdated)
    windowTarget.removeEventListener('openreader:replace-rules-updated', options.onReplaceRulesUpdated)
    windowTarget.removeEventListener('openreader:bookmarks-updated', options.onBookmarksUpdated)
    windowTarget.removeEventListener('openreader:books-deleted', options.onBooksDeleted)
  }

  async function mount() {
    options.reader.normalizeSettings()
    options.syncFonts(options.reader.customFontsMap)
    registerListeners()
    try {
      await options.loadBook()
    } catch (error) {
      options.onBookLoadError(error)
    }
    options.customBg.value = options.reader.customBgColor
    options.sliderLineHeight.value = options.reader.lineHeight
  }

  function unmount() {
    options.cancelProgressSave()
    options.clearChapterLoadingTimer()
    options.stopAutoReading()
    if (!sessionInvalidated) options.saveProgress({ force: true, background: true })
    unregisterListeners()
    options.onUnmount?.()
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
