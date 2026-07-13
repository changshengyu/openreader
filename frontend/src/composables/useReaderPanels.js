import { unref } from 'vue'

export function useReaderPanels(options) {
  function openSettings() {
    if (options.settingsVisible.value) {
      options.settingsVisible.value = false
      return
    }
    options.customBg.value = options.getCustomBgColor()
    options.sliderLineHeight.value = options.getLineHeight()
    options.settingsVisible.value = true
  }

  function showClickZone() {
    options.settingsVisible.value = false
    options.clickZoneVisible.value = true
  }

  function openCache() {
    if (!unref(options.isRemoteBook)) return
    options.refreshBrowserCachedChapters()
    options.cacheVisible.value = !options.cacheVisible.value
  }

  async function goShelf() {
    options.saveProgress({ force: true, background: true })
    await options.navigate({ name: 'home' })
  }

  function openSource() {
    options.sourceVisible.value = true
  }

  function openBookmarks() {
    const currentBook = unref(options.book)
    if (!currentBook) return
    options.openBookmarksOverlay(currentBook)
  }

  function openReplaceRules() {
    options.settingsVisible.value = false
    options.openReplaceRulesOverlay()
  }

  function openContentSearch() {
    const currentBook = unref(options.book)
    if (!currentBook) return
    options.openContentSearchOverlay(currentBook)
  }

  function openBookInfo() {
    const currentBook = unref(options.book)
    if (!currentBook) return
    options.openBookInfoOverlay(currentBook, {
      statusLabel: `阅读中 · ${unref(options.bookProgressLabel)}`,
      statusType: 'success',
      progress: unref(options.bookProgress),
    })
  }

  return {
    goShelf,
    openBookInfo,
    openBookmarks,
    openCache,
    openContentSearch,
    openReplaceRules,
    openSettings,
    openSource,
    showClickZone,
  }
}
