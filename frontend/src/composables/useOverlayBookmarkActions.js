import {
  bookmarkReaderQuery,
  normalizeImportedBookmarks,
} from '../utils/bookmark.js'

export function useOverlayBookmarkActions(options) {
  function jump(bookmark) {
    const book = options.getBook()
    if (!book?.id) return
    options.closePanel()
    options.navigate({
      name: 'reader',
      params: { id: book.id },
      query: bookmarkReaderQuery(bookmark),
    })
  }

  async function removeMany(rows) {
    if (!Array.isArray(rows) || !rows.length) return
    try {
      await options.confirm(
        `确认要删除所选择的 ${rows.length} 条书签吗？`,
        '批量删除书签',
        { type: 'warning' },
      )
      await options.removeMany(rows)
      options.onSuccess('书签已删除')
    } catch (error) {
      if (error === 'cancel' || error === 'close') return
      options.onError(error, '批量删除书签失败')
    }
  }

  async function importRows(rows) {
    const book = options.getBook()
    if (!book?.id) return
    const payloads = normalizeImportedBookmarks(rows)
    if (!payloads.length) {
      options.onInvalidImport('书签文件没有可导入内容')
      return
    }
    try {
      await options.confirm(
        `确认要导入文件中的 ${payloads.length} 条书签到当前书籍吗？`,
        '导入书签',
        { type: 'info' },
      )
      const created = await options.importPayloads(payloads)
      options.onSuccess(`已导入 ${created.length} 条书签`)
    } catch (error) {
      if (error === 'cancel' || error === 'close') return
      options.onError(error, '导入书签失败')
    }
  }

  return {
    jump,
    removeMany,
    importRows,
  }
}
