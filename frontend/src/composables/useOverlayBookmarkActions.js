import {
  bookmarkReaderQuery,
  normalizeImportedBookmarks,
} from '../utils/bookmark.js'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation.js'

export function useOverlayBookmarkActions(options) {
  const operations = options.operationGuard || createAuthenticatedOperationGuard({
    getIdentity: options.getAuthenticatedIdentity,
  })
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
    if (!Array.isArray(rows) || !rows.length) {
      options.onInvalidSelection?.('请选择需要删除的书签')
      return
    }
    const operation = operations.begin('remove-many')
    try {
      await options.confirm(
        '确认要删除所选择的书签吗?',
        '提示',
        {
          confirmButtonText: '确定',
          cancelButtonText: '取消',
          type: 'warning',
        },
      )
      if (!operations.canCommit(operation)) return
      await options.removeMany(rows)
      if (!operations.canCommit(operation)) return
      options.onSuccess('删除书签成功')
    } catch (error) {
      if (error === 'cancel' || error === 'close') return
      if (operations.canCommit(operation)) {
        options.onError(error, '删除书签失败')
      }
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
    const operation = operations.begin('import')
    try {
      await options.confirm(
        `确认要导入文件中的${payloads.length}条书签吗?`,
        '提示',
        {
          confirmButtonText: '确定',
          cancelButtonText: '取消',
          type: 'warning',
        },
      )
      if (!operations.canCommit(operation)) return
      await options.importPayloads(payloads)
      if (!operations.canCommit(operation)) return
      options.onSuccess('导入书签成功')
    } catch (error) {
      if (error === 'cancel' || error === 'close') return
      if (operations.canCommit(operation)) {
        options.onError(error, '导入书签失败')
      }
    }
  }

  return {
    jump,
    removeMany,
    importRows,
    resetOperations: operations.reset,
  }
}
