import { ref } from 'vue'

export function useBookInfoAddToShelf(options) {
  const addingBookKey = ref('')

  async function addRemoteBook(book, context = {}) {
    const initialCategoryIds = normalizeCategoryIds(context.categoryIds)
    let categoryIds
    try {
      categoryIds = await options.selectCategories(initialCategoryIds)
    } catch (error) {
      options.onError(error, '选择分组失败')
      return null
    }
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
      options.upsertBook(data)
      options.onSuccess(`已加入书架：《${book?.title || book?.name || '未命名书籍'}》`)
      return data
    } catch (error) {
      options.onError(error, '加入书架失败')
      return null
    } finally {
      addingBookKey.value = ''
    }
  }

  return {
    addingBookKey,
    addRemoteBook,
  }
}

function normalizeCategoryIds(categoryIds) {
  const values = Array.isArray(categoryIds) ? categoryIds : [categoryIds]
  return [...new Set(values.map(Number).filter(id => Number.isInteger(id) && id > 0))]
}
