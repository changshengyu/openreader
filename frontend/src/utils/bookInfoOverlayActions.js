export const BOOK_INFO_ACTION_LABELS = Object.freeze({
  addToShelf: '加入书架',
  addAndRead: '加入并阅读',
  startRead: '开始阅读',
  viewExistingInfo: '查看详情',
  continueRead: '继续阅读',
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
