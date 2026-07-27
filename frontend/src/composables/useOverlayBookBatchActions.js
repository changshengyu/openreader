import { ref } from 'vue'
import { bookCategoryIds } from '../utils/bookCategory.js'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation.js'

function isCancelled(error) {
  return error === 'cancel' || error === 'close'
}

function bookHasCategory(book, categoryId) {
  return bookCategoryIds(book).some(id => String(id) === String(categoryId))
}

export function useOverlayBookBatchActions(options) {
  const operations = options.operationGuard || createAuthenticatedOperationGuard({
    getIdentity: options.getAuthenticatedIdentity,
  })
  const selectedBookIds = ref([])
  const batchBusy = ref(false)

  function onManageSelectionChange(rows) {
    selectedBookIds.value = rows.map(row => row.id)
  }

  function toggleManagedBook(bookId, checked) {
    if (checked) {
      if (!selectedBookIds.value.includes(bookId)) {
        selectedBookIds.value.push(bookId)
      }
      return
    }
    selectedBookIds.value = selectedBookIds.value.filter(id => id !== bookId)
  }

  function selectAllManagedBooks() {
    selectedBookIds.value = options.getFilteredManagedBooks().map(book => book.id)
  }

  function clearManagedSelection() {
    selectedBookIds.value = []
  }

  function pruneManagedSelection(bookIds = []) {
    const available = new Set(
      (Array.isArray(bookIds) ? bookIds : [])
        .map(id => Number(id))
        .filter(id => Number.isInteger(id) && id > 0),
    )
    selectedBookIds.value = selectedBookIds.value.filter(id => available.has(Number(id)))
  }

  async function batchAddCategory(category) {
    if (!selectedBookIds.value.length) return
    const operation = operations.begin('batch-add-category')
    batchBusy.value = true
    try {
      await options.bookshelf.batchSetCategory(
        [...selectedBookIds.value],
        category.id,
        { action: 'category-add' },
      )
      if (!operations.canCommit(operation)) return
      options.onSuccess(`已添加到“${category.name}”分组`)
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '批量添加分组失败')
      }
    } finally {
      if (operations.canCommit(operation)) batchBusy.value = false
    }
  }

  async function batchRemoveCategory(category) {
    if (!selectedBookIds.value.length) return
    const targetIds = options.getManagedBooks()
      .filter(book => (
        selectedBookIds.value.includes(book.id) &&
        bookHasCategory(book, category.id)
      ))
      .map(book => book.id)
    if (!targetIds.length) {
      options.onInfo('选中书籍不在该分组中')
      return
    }
    const operation = operations.begin('batch-remove-category')
    batchBusy.value = true
    try {
      await options.bookshelf.batchSetCategory(
        targetIds,
        category.id,
        { action: 'category-remove' },
      )
      if (!operations.canCommit(operation)) return
      options.onSuccess(`已从“${category.name}”分组移除`)
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '批量移除分组失败')
      }
    } finally {
      if (operations.canCommit(operation)) batchBusy.value = false
    }
  }

  async function batchDeleteBooks() {
    if (!selectedBookIds.value.length) return
    const operation = operations.begin('batch-delete-books')
    const ids = [...selectedBookIds.value]
    try {
      await options.confirm(
        `确定删除选中的 ${ids.length} 本书吗？`,
        '批量删除',
        { type: 'warning' },
      )
      if (!operations.canCommit(operation)) return
      batchBusy.value = true
      await options.bookshelf.batchDeleteBooks(ids)
      if (!operations.canCommit(operation)) return
      selectedBookIds.value = []
      options.onSuccess('已批量删除')
    } catch (error) {
      if (isCancelled(error)) return
      if (operations.canCommit(operation)) {
        options.onError(error, '批量删除失败')
      }
    } finally {
      if (operations.canCommit(operation)) batchBusy.value = false
    }
  }

  return {
    selectedBookIds,
    batchBusy,
    onManageSelectionChange,
    toggleManagedBook,
    selectAllManagedBooks,
    clearManagedSelection,
    pruneManagedSelection,
    batchAddCategory,
    batchRemoveCategory,
    batchDeleteBooks,
    resetOperations: operations.reset,
  }
}
