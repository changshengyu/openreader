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
  const groupManageTableRef = ref(null)
  let sortable

  const groupSetRows = computed(() => (
    options.bookshelf.categories.map(category => ({
      ...category,
      id: String(category.id),
      description: `${groupBookCount(category)} 本`,
    }))
  ))

  const groupManageRows = computed(() => {
    const groupByKey = new Map(
      options.bookshelf.bookGroups.map(group => [String(group.key), group]),
    )
    const rows = []
    for (const key of groupOrderDraftKeys.value) {
      const group = groupByKey.get(String(key))
      if (group) rows.push(group)
    }
    for (const group of options.bookshelf.bookGroups) {
      if (!groupOrderDraftKeys.value.includes(String(group.key))) rows.push(group)
    }
    return rows
  })

  const isGroupOrderDirty = computed(() => (
    groupManageRows.value.map(group => String(group.key)).join(',') !==
    options.bookshelf.bookGroups.map(group => String(group.key)).join(',')
  ))

  function groupBookCount(group) {
    return bookGroupBookCount(group, options.getManagedBooks())
  }

  function displayBookGroupName(group) {
    if (group?.kind !== 'builtin') return group?.name || ''
    return `${group.name}(${group.defaultName})`
  }

  function prepareOpen(mode = options.overlay.bookGroupMode) {
    if (mode === 'set') {
      selectedCategoryIds.value = bookCategoryIds(options.overlay.bookInfoBook)
        .map(id => String(id))
      return
    }
    resetGroupOrderDraft()
  }

  function isBookGroupSelected(category) {
    return selectedCategoryIds.value.includes(String(category.id))
  }

  function toggleBookGroupSelection(category) {
    const id = String(category.id)
    if (!id) return
    if (selectedCategoryIds.value.includes(id)) {
      selectedCategoryIds.value = selectedCategoryIds.value.filter(item => item !== id)
      return
    }
    selectedCategoryIds.value = [...selectedCategoryIds.value, id]
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
        options.onWarning('请选择书籍分组')
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
      options.onSuccess('分组已设置')
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '设置分组失败')
      }
    } finally {
      if (operations.canCommit(operation)) settingCategorySaving.value = false
    }
  }

  async function createCategory() {
    const operation = operations.begin('create-group')
    try {
      const { value } = await options.prompt('输入分组名称', '添加分组', {
        inputValidator: value => !!value?.trim() || '分组名称不能为空',
      })
      if (!operations.canCommit(operation)) return
      const name = value.trim()
      if (!name) return
      await options.bookshelf.addCategory({ name })
      if (!operations.canCommit(operation)) return
      resetGroupOrderDraft()
      options.onSuccess('分组已创建')
    } catch (error) {
      if (isCancelled(error)) return
      if (operations.canCommit(operation)) {
        options.onError(error, '创建分组失败')
      }
    }
  }

  async function renameGroup(category) {
    const operation = operations.begin(`rename-group:${category?.key || category?.id || ''}`)
    try {
      const { value } = await options.prompt('输入新的分组名称', '重命名分组', {
        inputValue: category.name,
        inputValidator: value => !!value?.trim() || '分组名称不能为空',
      })
      if (!operations.canCommit(operation)) return
      const name = value.trim()
      if (!name || name === category.name) return
      if (category.kind === 'builtin') {
        await options.bookshelf.updateBuiltInBookGroup(category.semantic, { name })
      } else {
        await options.bookshelf.renameCategory(category.categoryId || category.id, { name })
      }
      if (!operations.canCommit(operation)) return
      resetGroupOrderDraft()
      options.onSuccess('分组已重命名')
    } catch (error) {
      if (isCancelled(error)) return
      if (operations.canCommit(operation)) {
        options.onError(error, '重命名失败')
      }
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
      options.onSuccess(show ? '分组已显示' : '分组已隐藏')
    } catch (error) {
      if (!operations.canCommit(operation)) return
      await options.bookshelf.loadBookGroups({ force: true }).catch(() => {})
      if (!operations.canCommit(operation)) return
      options.onError(error, '修改分组显示状态失败')
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
        `确定删除分组“${category.name}”吗？`,
        '删除分组',
        { type: 'warning' },
      )
      if (!operations.canCommit(operation)) return
      await options.bookshelf.removeCategory(category.categoryId || category.id)
      if (!operations.canCommit(operation)) return
      resetGroupOrderDraft()
      options.onSuccess('分组已删除')
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
    const keys = groupManageRows.value.map(group => String(group.key))
    const [moved] = keys.splice(oldIndex, 1)
    if (!moved) return
    keys.splice(newIndex, 0, moved)
    groupOrderDraftKeys.value = keys
  }

  async function handleBookGroupOpened() {
    if (options.overlay.bookGroupMode !== 'manage') return
    const operation = operations.begin('open-group-manager')
    await options.nextFrame()
    if (!operations.canCommit(operation)) return
    destroyGroupSortable()
    const tableBody = groupManageTableRef.value?.$el
      ?.querySelector('.el-table__body-wrapper tbody')
    if (!tableBody) return
    sortable = options.createSortable(tableBody, {
      handle: '.group-drag-handle',
      animation: 150,
      forceFallback: true,
      fallbackTolerance: 4,
      onEnd: ({ oldIndex, newIndex }) => moveGroupOrder(oldIndex, newIndex),
    })
  }

  function destroyGroupSortable() {
    sortable?.destroy()
    sortable = null
  }

  async function handleModeChange(mode) {
    destroyGroupSortable()
    prepareOpen(mode)
    if (mode === 'manage' && options.overlay.bookGroupVisible) {
      await handleBookGroupOpened()
    }
  }

  async function saveGroupOrderDraft() {
    if (!isGroupOrderDirty.value) return
    const operation = operations.begin('save-group-order')
    const orderedKeys = groupManageRows.value.map(item => item.key)
    groupOrderSaving.value = true
    try {
      await options.bookshelf.reorderBookGroupKeys(orderedKeys)
      if (!operations.canCommit(operation)) return
      resetGroupOrderDraft()
      options.onSuccess('分组排序已更新')
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '分组排序失败')
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
    groupManageTableRef,
    groupSetRows,
    groupManageRows,
    isGroupOrderDirty,
    groupBookCount,
    displayBookGroupName,
    prepareOpen,
    isBookGroupSelected,
    toggleBookGroupSelection,
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
