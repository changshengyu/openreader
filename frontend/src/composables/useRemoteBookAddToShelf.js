import { ref } from 'vue'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation.js'

export function useRemoteBookAddToShelf(options) {
  const addingBookKey = ref('')
  let loadingRevision = 0
  const operations = options.operationGuard || createAuthenticatedOperationGuard({
    getIdentity: options.getAuthenticatedIdentity,
  })

  async function addRemoteBook(book, context = {}) {
    return persistRemoteBook(book, context, normalizeCategoryIds(context.categoryIds))
  }

  async function addRemoteBookWithCategories(book, context = {}) {
    const operation = operations.begin('choose-remote-book-categories')
    let categoryIds
    try {
      categoryIds = await options.selectCategories([])
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '选择分组失败')
      }
      return null
    }
    if (!operations.canCommit(operation) || categoryIds === null) return null
    return persistRemoteBook(book, context, normalizeCategoryIds(categoryIds))
  }

  async function persistRemoteBook(book, context, categoryIds) {
    const key = String(context.key || book?.id || book?.bookUrl || book?.url || '')
    const operation = operations.begin(`add-remote-book:${key}`)
    const revision = ++loadingRevision
    addingBookKey.value = key
    try {
      const payload = options.buildPayload(book, categoryIds, context)
      const { data } = await options.createRemoteBook(payload)
      if (!operations.canCommit(operation)) return null
      options.upsertBook(data)
      options.onSuccess('加入书架成功')
      return data
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '加入书架失败')
      }
      return null
    } finally {
      if (revision === loadingRevision) addingBookKey.value = ''
    }
  }

  function resetOperations() {
    loadingRevision += 1
    addingBookKey.value = ''
    operations.reset()
  }

  return {
    addingBookKey,
    addRemoteBook,
    addRemoteBookWithCategories,
    resetOperations,
  }
}

function normalizeCategoryIds(categoryIds) {
  const values = Array.isArray(categoryIds) ? categoryIds : [categoryIds]
  return [...new Set(values.map(Number).filter(id => Number.isInteger(id) && id > 0))]
}
