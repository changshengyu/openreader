import { ref } from 'vue'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation.js'

export function useBookInfoAddToShelf(options) {
  const addingBookKey = ref('')
  const operations = options.operationGuard || createAuthenticatedOperationGuard({
    getIdentity: options.getAuthenticatedIdentity,
  })

  async function addRemoteBook(book, context = {}) {
    const operation = operations.begin('add-remote-book')
    const initialCategoryIds = normalizeCategoryIds(context.categoryIds)
    let categoryIds
    try {
      categoryIds = await options.selectCategories(initialCategoryIds)
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '选择分组失败')
      }
      return null
    }
    if (!operations.canCommit(operation)) return null
    if (categoryIds === null) return null

    const key = String(context.key || book?.id || book?.bookUrl || book?.url || '')
    addingBookKey.value = key
    try {
      const payload = options.buildPayload(
        book,
        normalizeCategoryIds(categoryIds),
        context,
      )
      const { data } = await options.createRemoteBook(payload)
      if (!operations.canCommit(operation)) return null
      options.upsertBook(data)
      options.onSuccess(`已加入书架：《${book?.title || book?.name || '未命名书籍'}》`)
      return data
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '加入书架失败')
      }
      return null
    } finally {
      if (operations.canCommit(operation)) addingBookKey.value = ''
    }
  }

  return {
    addingBookKey,
    addRemoteBook,
    resetOperations: operations.reset,
  }
}

function normalizeCategoryIds(categoryIds) {
  const values = Array.isArray(categoryIds) ? categoryIds : [categoryIds]
  return [...new Set(values.map(Number).filter(id => Number.isInteger(id) && id > 0))]
}
