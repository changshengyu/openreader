import { ref } from 'vue'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation.js'

function isCancelled(error) {
  return error === 'cancel' || error === 'close'
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
    return operateBookCategory(category, true)
  }

  async function batchRemoveCategory(category) {
    return operateBookCategory(category, false)
  }

  async function operateBookCategory(category, isAdd) {
    const operationName = isAdd ? '添加' : '移除'
    if (!selectedBookIds.value.length) {
      options.onValidationError(`请选择需要${operationName}分组的书籍`)
      return
    }

    const operation = operations.begin(isAdd ? 'batch-add-category' : 'batch-remove-category')
    try {
      await options.confirm(
        isAdd
          ? `确认要将所选择的书籍添加到${category.name}分组吗?`
          : `确认要将所选择的书籍从${category.name}分组中移除吗?`,
        '提示',
        { type: 'warning' },
      )
      if (!operations.canCommit(operation)) return
      batchBusy.value = true
      await options.bookshelf.batchSetCategory(
        [...selectedBookIds.value],
        category.id,
        { action: isAdd ? 'category-add' : 'category-remove' },
      )
      if (!operations.canCommit(operation)) return
      options.onSuccess('操作成功')
      await reloadManagedBooks(operation)
    } catch (error) {
      if (isCancelled(error)) return
      if (operations.canCommit(operation)) {
        options.onError(error, '操作失败')
      }
    } finally {
      if (operations.canCommit(operation)) batchBusy.value = false
    }
  }

  async function batchDeleteBooks() {
    if (!selectedBookIds.value.length) {
      options.onValidationError('请选择需要删除的书籍')
      return
    }
    const operation = operations.begin('batch-delete-books')
    const ids = [...selectedBookIds.value]
    try {
      await options.confirm(
        '确认要删除所选择的书籍吗?',
        '提示',
        { type: 'warning' },
      )
      if (!operations.canCommit(operation)) return
      batchBusy.value = true
      await options.bookshelf.batchDeleteBooks(ids)
      if (!operations.canCommit(operation)) return
      selectedBookIds.value = []
      options.onSuccess('删除书籍成功')
      await reloadManagedBooks(operation)
    } catch (error) {
      if (isCancelled(error)) return
      if (operations.canCommit(operation)) {
        options.onError(error, '删除书籍失败')
      }
    } finally {
      if (operations.canCommit(operation)) batchBusy.value = false
    }
  }

  async function reloadManagedBooks(operation) {
    try {
      await options.reloadManagedBooks?.()
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '获取书架信息失败')
      }
    }
  }

  return {
    selectedBookIds,
    batchBusy,
    onManageSelectionChange,
    clearManagedSelection,
    pruneManagedSelection,
    batchAddCategory,
    batchRemoveCategory,
    batchDeleteBooks,
    resetOperations: operations.reset,
  }
}
