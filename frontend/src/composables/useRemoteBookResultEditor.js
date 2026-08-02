import { ref } from 'vue'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation.js'
import {
  remoteBookCreatePayload,
  remoteBookKey,
  remoteBookSourceId,
  remoteBookSourceName,
} from '../utils/remoteBookResult.js'

export function useRemoteBookResultEditor(options) {
  const visible = ref(false)
  const content = ref('')
  const saving = ref(false)
  const currentBook = ref(null)
  const currentContext = ref({})
  let savingRevision = 0
  const operations = options.operationGuard || createAuthenticatedOperationGuard({
    getIdentity: options.getAuthenticatedIdentity,
  })

  function open(book, context = {}) {
    const sourceId = remoteBookSourceId(book, context.sourceId)
    const sourceName = remoteBookSourceName(book, context.sourceName)
    currentBook.value = book
    currentContext.value = { ...context, sourceId, sourceName }
    content.value = JSON.stringify({
      ...book,
      ...(sourceId ? { sourceId } : {}),
      ...(sourceName ? { sourceName } : {}),
    }, null, 4)
    visible.value = true
  }

  function close() {
    savingRevision += 1
    operations.invalidate(currentOperationKey())
    visible.value = false
    saving.value = false
  }

  async function save() {
    if (!visible.value || saving.value) return null
    let edited
    try {
      edited = JSON.parse(content.value)
    } catch {
      options.onError(null, '书籍信息必须是JSON格式')
      return null
    }

    const title = String(edited?.title || edited?.name || edited?.bookName || '').trim()
    const bookUrl = String(edited?.bookUrl || edited?.url || edited?.bookURL || '').trim()
    const sourceId = Number(edited?.sourceId || edited?.bookSourceId || 0)
    if (!title) {
      options.onError(null, '书籍名称不能为空')
      return null
    }
    if (!bookUrl) {
      options.onError(null, '书籍链接不能为空')
      return null
    }
    if (!Number.isInteger(sourceId) || sourceId <= 0) {
      options.onError(null, '书籍来源不能为空')
      return null
    }

    try {
      await options.confirm(
        '加入书架之后才能编辑书籍信息, 是否加入书架?',
        '提示',
        {
          confirmButtonText: '确定',
          cancelButtonText: '取消',
          type: 'warning',
        },
      )
    } catch {
      return null
    }

    const operation = operations.begin(currentOperationKey())
    const revision = ++savingRevision
    saving.value = true
    try {
      const payload = remoteBookCreatePayload({
        ...edited,
        title,
        bookUrl,
        sourceId,
      }, [], {
        sourceId,
        sourceName: currentContext.value.sourceName,
      })
      const { data } = await options.createRemoteBook(payload)
      if (!operations.canCommit(operation)) return null
      options.upsertBook(data)
      visible.value = false
      options.onSuccess('修改书籍成功')
      return data
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '保存书籍失败')
      }
      return null
    } finally {
      if (revision === savingRevision) saving.value = false
    }
  }

  function reset() {
    savingRevision += 1
    operations.reset()
    visible.value = false
    saving.value = false
    content.value = ''
    currentBook.value = null
    currentContext.value = {}
  }

  function currentOperationKey() {
    return `edit-remote-result:${remoteBookKey(currentBook.value || {}, currentContext.value.sourceId)}`
  }

  return {
    visible,
    content,
    saving,
    open,
    close,
    save,
    reset,
  }
}
