import { computed, ref } from 'vue'
import { bookCategoryIds } from '../utils/bookCategory.js'
import { bookGroupBookCount } from '../utils/bookGroups.js'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation.js'

function isCancelled(error) {
  return error === 'cancel' || error === 'close'
}

export function useOverlayBookGroups(options) {
  const operations = options.operationGuard || createAuthenticatedOperationGuard({
    getIdentity: options.getAuthenticatedIdentity,
  })
  const selectedCategoryIds = ref([])
  const settingCategorySaving = ref(false)
  const visibilitySavingId = ref(null)
  const groupOrderDraftKeys = ref([])
  const groupOrderSaving = ref(false)
  const groupTableRef = ref(null)
  let sortable
  let syncingTableSelection = false

  const groupSetRows = computed(() => (
    options.bookshelf.categories.map(category => ({
      ...category,
      id: String(category.id),
    }))
  ))

  // Sortable owns the visible tbody order until save, exactly like reader-dev.
  // Re-projecting table data from the draft here would move the same row twice:
  // once in Sortable's DOM and once in Vue's keyed render.
  const groupManageRows = computed(() => options.bookshelf.bookGroups)

  const groupRows = computed(() => (
    options.overlay.bookGroupMode === 'set'
      ? groupSetRows.value
      : groupManageRows.value
  ))

  const isGroupOrderDirty = computed(() => (
    groupOrderDraftKeys.value.join(',') !==
    options.bookshelf.bookGroups.map(group => String(group.key)).join(',')
  ))

  function groupBookCount(group) {
    return bookGroupBookCount(group, options.getManagedBooks())
  }

  function displayBookGroupName(group) {
    if (group?.kind !== 'builtin') return group?.name || ''
    return `${group.name}(${group.defaultName})`
  }

  async function prepareOpen(mode = options.overlay.bookGroupMode) {
    if (mode === 'set') {
      selectedCategoryIds.value = bookCategoryIds(options.overlay.bookInfoBook)
        .map(id => String(id))
      await syncBookGroupTableSelection()
      return
    }
    resetGroupOrderDraft()
  }

  async function syncBookGroupTableSelection() {
    const table = groupTableRef.value
    if (!table) return
    await options.nextFrame()
    syncingTableSelection = true
    try {
      table.clearSelection?.()
      const selected = new Set(selectedCategoryIds.value.map(String))
      groupSetRows.value.forEach(row => {
        if (selected.has(String(row.id))) table.toggleRowSelection?.(row, true)
      })
    } finally {
      syncingTableSelection = false
    }
  }

  function handleBookGroupSelectionChange(rows) {
    if (syncingTableSelection) return
    selectedCategoryIds.value = (Array.isArray(rows) ? rows : [])
      .map(row => String(row?.id || ''))
      .filter(Boolean)
  }

  async function saveBookGroupSetting() {
    const book = options.overlay.bookInfoBook
    if (!book?.id) return
    const operation = operations.begin('set-book-groups')
    settingCategorySaving.value = true
    try {
      const categoryIds = selectedCategoryIds.value
        .map(id => Number(id))
        .filter(Boolean)
      if (!categoryIds.length) {
        options.onError(null, '请选择书籍分组')
        return
      }
      const { data } = await options.updateBookCategory(book.id, categoryIds)
      if (!operations.canCommit(operation)) return
      options.bookshelf.upsertBook(data)
      options.overlay.bookInfoBook = data
      options.emitBookInfoUpdated(data)
      options.overlay.bookInfoOptions = {
        ...options.overlay.bookInfoOptions,
        categoryName: options.categoryName(data),
        progress: options.getBookProgress(data)?.percent || 0,
      }
      options.overlay.bookGroupVisible = false
      options.onSuccess('设置成功')
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '设置失败')
      }
    } finally {
      if (operations.canCommit(operation)) settingCategorySaving.value = false
    }
  }

  async function createCategory() {
    const operation = operations.begin('create-group')
    try {
      const { value } = await options.prompt('', '添加分组', {
        inputValue: '',
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        inputValidator: value => !!value?.trim() || '分组名不能为空',
      })
      if (!operations.canCommit(operation)) return
      const name = value.trim()
      if (!name) return
      await options.bookshelf.addCategory({ name })
      if (!operations.canCommit(operation)) return
      resetGroupOrderDraft()
      await clearSetModeSelectionAfterGroupMutation()
      await refreshVisibleGroupTable()
      options.onSuccess('添加成功')
    } catch (error) {
      if (isCancelled(error)) return
      if (operations.canCommit(operation)) {
        options.onError(error, '添加失败')
      }
    }
  }

  async function renameGroup(category) {
    const operation = operations.begin(`rename-group:${category?.key || category?.id || ''}`)
    try {
      const { value } = await options.prompt('', '编辑分组', {
        inputValue: category.name,
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        inputValidator: value => !!value?.trim() || '分组名不能为空',
      })
      if (!operations.canCommit(operation)) return
      const name = value.trim()
      if (!name) return
      if (category.kind === 'builtin') {
        await options.bookshelf.updateBuiltInBookGroup(category.semantic, { name })
      } else {
        await options.bookshelf.renameCategory(category.categoryId || category.id, { name })
      }
      if (!operations.canCommit(operation)) return
      await options.bookshelf.loadBookGroups({ force: true })
      if (!operations.canCommit(operation)) return
      resetGroupOrderDraft()
      await clearSetModeSelectionAfterGroupMutation()
      await refreshVisibleGroupTable()
      options.onSuccess('修改成功')
    } catch (error) {
      if (isCancelled(error)) return
      if (operations.canCommit(operation)) {
        options.onError(error, '修改失败')
      }
    }
  }

  async function clearSetModeSelectionAfterGroupMutation() {
    if (options.overlay.bookGroupMode !== 'set') return
    selectedCategoryIds.value = []
    const table = groupTableRef.value
    if (!table) return
    await options.nextFrame()
    syncingTableSelection = true
    try {
      table.clearSelection?.()
    } finally {
      syncingTableSelection = false
    }
  }

  async function toggleGroupVisibility(category, show) {
    const operation = operations.begin(`toggle-group:${category?.key || category?.id || ''}`)
    visibilitySavingId.value = category.key || category.id
    try {
      if (category.kind === 'builtin') {
        await options.bookshelf.updateBuiltInBookGroup(category.semantic, { show })
      } else {
        await options.bookshelf.setCategoryVisible(category.categoryId || category.id, show)
      }
      if (!operations.canCommit(operation)) return
      await options.bookshelf.loadBookGroups({ force: true })
      if (!operations.canCommit(operation)) return
      resetGroupOrderDraft()
      await refreshVisibleGroupTable()
      options.onSuccess('修改成功')
    } catch (error) {
      if (!operations.canCommit(operation)) return
      await options.bookshelf.loadBookGroups({ force: true }).catch(() => {})
      if (!operations.canCommit(operation)) return
      options.onError(error, '修改失败')
    } finally {
      if (operations.canCommit(operation)) visibilitySavingId.value = null
    }
  }

  async function deleteGroup(category) {
    if (category.kind !== 'category') {
      options.onWarning('内置分组不能删除')
      return
    }
    if (groupBookCount(category) > 0) {
      options.onWarning('分组内还有书籍，清空后才能删除')
      return
    }
    const operation = operations.begin(`delete-group:${category?.id || ''}`)
    try {
      await options.confirm(
        '确认要删除该分组吗?',
        '提示',
        {
          confirmButtonText: '确定',
          cancelButtonText: '取消',
          type: 'warning',
        },
      )
      if (!operations.canCommit(operation)) return
      await options.bookshelf.removeCategory(category.categoryId || category.id)
      if (!operations.canCommit(operation)) return
      resetGroupOrderDraft()
      await refreshVisibleGroupTable()
      options.onSuccess('删除分组成功')
    } catch (error) {
      if (isCancelled(error)) return
      if (operations.canCommit(operation)) {
        options.onError(error, '删除分组失败')
      }
    }
  }

  function resetGroupOrderDraft() {
    groupOrderDraftKeys.value = options.bookshelf.bookGroups
      .map(group => String(group.key))
  }

  function moveGroupOrder(oldIndex, newIndex) {
    if (
      oldIndex == null ||
      newIndex == null ||
      oldIndex === newIndex
    ) return
    const keys = groupOrderDraftKeys.value.length
      ? [...groupOrderDraftKeys.value]
      : groupManageRows.value.map(group => String(group.key))
    const [moved] = keys.splice(oldIndex, 1)
    if (!moved) return
    keys.splice(newIndex, 0, moved)
    groupOrderDraftKeys.value = keys
  }

  async function handleBookGroupOpened() {
    const operation = operations.begin('open-group-manager')
    await options.nextFrame()
    if (!operations.canCommit(operation)) return
    destroyGroupSortable()
    groupTableRef.value?.doLayout?.()
    const tableBody = groupTableRef.value?.$el
      ?.querySelector('.el-table__body-wrapper tbody')
    if (!tableBody) return
    sortable = options.createSortable(tableBody, {
      handle: '.group-drag-icon',
      setData: dataTransfer => dataTransfer.setData('Text', ''),
      onEnd: ({ oldIndex, newIndex }) => moveGroupOrder(oldIndex, newIndex),
    })
    sortable?.option?.('disabled', options.overlay.bookGroupMode === 'set')
  }

  function destroyGroupSortable() {
    sortable?.destroy()
    sortable = null
  }

  async function refreshVisibleGroupTable() {
    if (!options.overlay.bookGroupVisible || !groupTableRef.value) return
    await handleBookGroupOpened()
  }

  async function handleModeChange(mode) {
    sortable?.option?.('disabled', mode === 'set')
    await prepareOpen(mode)
  }

  async function saveGroupOrderDraft() {
    if (!isGroupOrderDirty.value) return
    const operation = operations.begin('save-group-order')
    const orderedKeys = [...groupOrderDraftKeys.value]
    groupOrderSaving.value = true
    try {
      await options.bookshelf.reorderBookGroupKeys(orderedKeys)
      if (!operations.canCommit(operation)) return
      await options.bookshelf.loadBookGroups({ force: true })
      if (!operations.canCommit(operation)) return
      resetGroupOrderDraft()
      await refreshVisibleGroupTable()
      options.onSuccess('保存成功')
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '保存失败')
      }
    } finally {
      if (operations.canCommit(operation)) groupOrderSaving.value = false
    }
  }

  return {
    selectedCategoryIds,
    settingCategorySaving,
    visibilitySavingId,
    groupOrderDraftKeys,
    groupOrderSaving,
    groupTableRef,
    groupSetRows,
    groupManageRows,
    groupRows,
    isGroupOrderDirty,
    groupBookCount,
    displayBookGroupName,
    prepareOpen,
    handleBookGroupSelectionChange,
    saveBookGroupSetting,
    createCategory,
    renameGroup,
    toggleGroupVisibility,
    deleteGroup,
    resetGroupOrderDraft,
    moveGroupOrder,
    handleBookGroupOpened,
    destroyGroupSortable,
    handleModeChange,
    saveGroupOrderDraft,
    resetOperations: operations.reset,
  }
}
