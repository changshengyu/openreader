import { ref } from 'vue'
import { bookCategoryIds } from '../utils/bookCategory.js'

function isCancelled(error) {
  return error === 'cancel' || error === 'close'
}

function bookHasCategory(book, categoryId) {
  return bookCategoryIds(book).some(id => String(id) === String(categoryId))
}

export function useOverlayBookBatchActions(options) {
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

  async function batchAddCategory(category) {
    if (!selectedBookIds.value.length) return
    batchBusy.value = true
    try {
      await options.bookshelf.batchSetCategory(
        [...selectedBookIds.value],
        category.id,
        { action: 'category-add' },
      )
      options.onSuccess(`已添加到“${category.name}”分组`)
    } catch (error) {
      options.onError(error, '批量添加分组失败')
    } finally {
      batchBusy.value = false
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
    batchBusy.value = true
    try {
      await options.bookshelf.batchSetCategory(
        targetIds,
        category.id,
        { action: 'category-remove' },
      )
      options.onSuccess(`已从“${category.name}”分组移除`)
    } catch (error) {
      options.onError(error, '批量移除分组失败')
    } finally {
      batchBusy.value = false
    }
  }

  async function batchDeleteBooks() {
    if (!selectedBookIds.value.length) return
    try {
      await options.confirm(
        `确定删除选中的 ${selectedBookIds.value.length} 本书吗？`,
        '批量删除',
        { type: 'warning' },
      )
      batchBusy.value = true
      await options.bookshelf.batchDeleteBooks([...selectedBookIds.value])
      selectedBookIds.value = []
      options.onSuccess('已批量删除')
    } catch (error) {
      if (isCancelled(error)) return
      options.onError(error, '批量删除失败')
    } finally {
      batchBusy.value = false
    }
  }

  return {
    selectedBookIds,
    batchBusy,
    onManageSelectionChange,
    toggleManagedBook,
    selectAllManagedBooks,
    clearManagedSelection,
    batchAddCategory,
    batchRemoveCategory,
    batchDeleteBooks,
  }
}
