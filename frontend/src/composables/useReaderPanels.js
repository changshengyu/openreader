import { unref } from 'vue'

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

  async function goBookDetail() {
    options.saveProgress({ force: true, background: true })
    await options.navigate({
      name: 'book-detail',
      params: { id: unref(options.bookId) },
    })
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
    const actions = [
      { label: '目录', plain: true, handler: openInfoToc },
      { label: '书签', plain: true, handler: openInfoBookmarks },
      { label: '搜正文', plain: true, handler: openInfoSearch },
      hasRemoteSource ? { label: '书源', plain: true, handler: openInfoSources } : null,
      { label: '分组', plain: true, handler: openInfoGroup },
      hasRemoteSource ? { label: '刷新目录', plain: true, handler: options.refreshCatalog } : null,
      hasRemoteSource ? { label: '缓存章节', plain: true, handler: openCache } : null,
      hasRemoteSource ? { label: '清缓存', plain: true, handler: options.clearCache } : null,
      { label: '设置', plain: true, handler: openInfoSettings },
      {
        label: '完整详情',
        type: 'primary',
        handler: () => {
          options.closeBookInfo()
          goBookDetail()
        },
      },
    ].filter(Boolean)
    options.openBookInfoOverlay(currentBook, {
      statusLabel: `阅读中 · ${unref(options.bookProgressLabel)}`,
      statusType: 'success',
      progress: unref(options.bookProgress),
      actions,
    })
  }

  return {
    goBookDetail,
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
