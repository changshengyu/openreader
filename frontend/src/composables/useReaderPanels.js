import { unref } from 'vue'
import { buildReaderBookInfoActions } from '../utils/bookInfoOverlayActions.js'

export function useReaderPanels(options) {
  function closeBookInfo() {
    options.closeBookInfo()
  }

  function openSettings() {
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
    options.cacheVisible.value = true
  }

  async function goShelf() {
    options.saveProgress({ force: true, background: true })
    await options.navigate({ name: 'home' })
  }

  function openSource() {
    if (!unref(options.isRemoteBook)) return
    options.sourceVisible.value = true
  }

  function openBookmarks() {
    options.bookmarkVisible.value = true
  }

  function openReplaceRules() {
    options.settingsVisible.value = false
    options.openReplaceRulesOverlay()
  }

  function openContentSearch() {
    options.searchVisible.value = true
    options.defer(() => options.focusContentSearch())
  }

  function openInfoToc() {
    closeBookInfo()
    options.openToc()
  }

  function openInfoBookmarks() {
    closeBookInfo()
    openBookmarks()
  }

  function openInfoSearch() {
    closeBookInfo()
    openContentSearch()
  }

  function openInfoSources() {
    if (!unref(options.isRemoteBook)) return
    closeBookInfo()
    options.sourceVisible.value = true
  }

  function openInfoSettings() {
    closeBookInfo()
    openSettings()
  }

  async function openInfoGroup() {
    const currentBook = unref(options.book)
    if (!currentBook) return
    closeBookInfo()
    try {
      await options.ensureCategoriesLoaded()
    } catch {
      // 分组弹层仍可打开，失败提示由保存时处理。
    }
    options.openBookGroup('set', currentBook, {
      categoryName: options.getCategoryName(currentBook),
      progress: unref(options.bookProgress),
      statusLabel: `阅读中 · ${unref(options.bookProgressLabel)}`,
      statusType: 'success',
    })
  }

  function openBookInfo() {
    const currentBook = unref(options.book)
    if (!currentBook) return
    const hasRemoteSource = unref(options.isRemoteBook)
    const actions = buildReaderBookInfoActions({
      hasRemoteSource,
      openToc: openInfoToc,
      openBookmarks: openInfoBookmarks,
      openContentSearch: openInfoSearch,
      openSource: openInfoSources,
      openGroup: openInfoGroup,
      refreshCatalog: options.refreshCatalog,
      openCache,
      clearCache: options.clearCache,
      openSettings: openInfoSettings,
    })
    options.openBookInfoOverlay(currentBook, {
      statusLabel: `阅读中 · ${unref(options.bookProgressLabel)}`,
      statusType: 'success',
      progress: unref(options.bookProgress),
      actions,
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
