export const BOOK_INFO_ACTION_LABELS = Object.freeze({
  addToShelf: '加入书架',
  addAndRead: '加入并阅读',
  startRead: '开始阅读',
  viewExistingInfo: '查看详情',
  continueRead: '继续阅读',
  toc: '目录',
  bookmarks: '书签',
  contentSearch: '搜正文',
  source: '书源',
  group: '分组',
  refreshCatalog: '刷新目录',
  cacheChapters: '缓存章节',
  clearCache: '清缓存',
  settings: '设置',
})

function action(label, handler, options = {}) {
  return {
    label,
    handler,
    ...(options.type ? { type: options.type } : {}),
    ...(options.plain ? { plain: true } : {}),
    ...(options.loading ? { loading: true } : {}),
    ...(options.disabled ? { disabled: true } : {}),
  }
}

export function buildBookInfoReadActions({ read, label = BOOK_INFO_ACTION_LABELS.continueRead } = {}) {
  return [
    action(label, read, { type: 'primary' }),
  ]
}

export function buildBookInfoStartReadActions({ read } = {}) {
  return buildBookInfoReadActions({
    read,
    label: BOOK_INFO_ACTION_LABELS.startRead,
  })
}

export function buildSearchExistingBookActions({ openInfo, read } = {}) {
  return [
    action(BOOK_INFO_ACTION_LABELS.viewExistingInfo, openInfo, { plain: true }),
    action(BOOK_INFO_ACTION_LABELS.continueRead, read, { type: 'primary' }),
  ]
}

export function buildSearchAddBookActions({ add, addAndRead, loading = false } = {}) {
  return [
    action(BOOK_INFO_ACTION_LABELS.addToShelf, add, { plain: true, loading }),
    action(BOOK_INFO_ACTION_LABELS.addAndRead, addAndRead, { type: 'primary', loading }),
  ]
}

export function buildReaderBookInfoActions({
  hasRemoteSource = false,
  openToc,
  openBookmarks,
  openContentSearch,
  openSource,
  openGroup,
  refreshCatalog,
  openCache,
  clearCache,
  openSettings,
} = {}) {
  return [
    action(BOOK_INFO_ACTION_LABELS.toc, openToc, { plain: true }),
    action(BOOK_INFO_ACTION_LABELS.bookmarks, openBookmarks, { plain: true }),
    action(BOOK_INFO_ACTION_LABELS.contentSearch, openContentSearch, { plain: true }),
    hasRemoteSource ? action(BOOK_INFO_ACTION_LABELS.source, openSource, { plain: true }) : null,
    action(BOOK_INFO_ACTION_LABELS.group, openGroup, { plain: true }),
    hasRemoteSource ? action(BOOK_INFO_ACTION_LABELS.refreshCatalog, refreshCatalog, { plain: true }) : null,
    hasRemoteSource ? action(BOOK_INFO_ACTION_LABELS.cacheChapters, openCache, { plain: true }) : null,
    hasRemoteSource ? action(BOOK_INFO_ACTION_LABELS.clearCache, clearCache, { plain: true }) : null,
    action(BOOK_INFO_ACTION_LABELS.settings, openSettings, { plain: true }),
  ].filter(Boolean)
}
