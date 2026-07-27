import { computed, reactive, ref } from 'vue'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation.js'

export function useOverlayUserManagement(options) {
  const operations = options.operationGuard || createAuthenticatedOperationGuard({
    getIdentity: options.getAuthenticatedIdentity,
  })
  const users = ref([])
  const usersLoading = ref(false)
  const deletingUsers = ref(false)
  const resettingSources = ref(false)
  const defaultingSourceUserId = ref(null)
  const creatingUser = ref(false)
  const createDialogVisible = ref(false)
  const selectedUserIds = ref([])
  const draft = reactive({
    username: '',
    password: '',
    canEditSources: true,
    canAccessStore: true,
    canAccessWebdav: true,
  })
  const scheduleTimeout = options.setTimeout || globalThis.setTimeout
  const cancelTimeout = options.clearTimeout || globalThis.clearTimeout
  let refreshTimer
  let managerRequest = 0

  function isMutable(user) {
    return user.role !== 'admin' && user.id !== options.getCurrentUserId()
  }

  function isDeletable(user) {
    return isMutable(user)
  }

  function isSelectable(user) {
    return Number(user?.id || 0) > 0
  }

  const selectedDeletableUserIds = computed(() => selectedUserIds.value.filter(id => (
    users.value.some(user => user.id === id && isDeletable(user))
  )))

  async function load() {
    const request = ++managerRequest
    const operation = operations.begin('load')
    usersLoading.value = true
    try {
      if (!options.userStore.profile) await options.userStore.loadMe()
      if (!operations.canCommit(operation)) return
      const { data } = await options.listUsers()
      if (request !== managerRequest || !operations.canCommit(operation)) return
      users.value = data || []
      selectedUserIds.value = selectedUserIds.value.filter(id => (
        users.value.some(user => user.id === id && isSelectable(user))
      ))
    } catch (error) {
      if (request !== managerRequest || !operations.canCommit(operation)) return
      options.onError(error, '加载用户失败')
    } finally {
      if (request === managerRequest && operations.canCommit(operation)) {
        usersLoading.value = false
      }
    }
  }

  function clearRefresh() {
    if (!refreshTimer) return
    cancelTimeout(refreshTimer)
    refreshTimer = undefined
  }

  function resetManager() {
    operations.reset()
    managerRequest += 1
    clearRefresh()
    users.value = []
    selectedUserIds.value = []
    usersLoading.value = false
    deletingUsers.value = false
    resettingSources.value = false
    defaultingSourceUserId.value = null
  }

  function scheduleRefresh() {
    clearRefresh()
    refreshTimer = scheduleTimeout(async () => {
      refreshTimer = undefined
      await load()
    }, 250)
  }

  function handleUpdated() {
    if (!options.isActive()) return
    scheduleRefresh()
  }

  function changeSelection(rows) {
    selectedUserIds.value = rows.filter(isSelectable).map(user => user.id)
  }

  function toggleSelection(id, checked) {
    const user = users.value.find(item => item.id === id)
    if (!user || !isSelectable(user)) return
    if (checked) {
      if (!selectedUserIds.value.includes(id)) selectedUserIds.value.push(id)
      return
    }
    selectedUserIds.value = selectedUserIds.value.filter(item => item !== id)
  }

  function openCreateDialog() {
    Object.assign(draft, {
      username: '',
      password: '',
      canEditSources: true,
      canAccessStore: true,
      canAccessWebdav: true,
    })
    createDialogVisible.value = true
  }

  async function create() {
    const username = draft.username.trim()
    if (!/^[A-Za-z0-9]{5,}$/.test(username) || username.toLowerCase() === 'default' || draft.password.length < 8) {
      options.onWarning('用户名至少 5 位且只能包含字母或数字，密码至少 8 位')
      return
    }
    const operation = operations.begin('create')
    creatingUser.value = true
    try {
      await options.createUser({
        username,
        password: draft.password,
        canEditSources: draft.canEditSources,
        canAccessStore: draft.canAccessStore,
        canAccessWebdav: draft.canAccessWebdav,
      })
      if (!operations.canCommit(operation)) return
      options.onSuccess('新增用户成功')
      createDialogVisible.value = false
      await load()
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '新增用户失败')
      }
    } finally {
      if (operations.canCommit(operation)) creatingUser.value = false
    }
  }

  async function resetPassword(row) {
    const operation = operations.begin('reset-password')
    try {
      const result = await options.prompt(
        '',
        `重置 ${row.username} 的密码`,
        {
          confirmButtonText: '确定',
          cancelButtonText: '取消',
          inputType: 'password',
          inputValidator(value) {
            if (!value || value.length < 8) return '密码至少 8 位'
            return true
          },
        },
      )
      if (!operations.canCommit(operation)) return
      await options.resetUserPassword(row.id, { password: result.value })
      if (!operations.canCommit(operation)) return
      options.onSuccess('重置密码成功')
    } catch (error) {
      if (error === 'cancel' || error === 'close') return
      if (operations.canCommit(operation)) {
        options.onError(error, '重置密码失败')
      }
    }
  }

  async function removeSelected() {
    const ids = [...selectedDeletableUserIds.value]
    if (!ids.length) {
      options.onWarning('请选择需要删除的用户')
      return
    }
    const operation = operations.begin('delete-users')
    deletingUsers.value = true
    try {
      await options.confirm(
        `确认要删除所选择的 ${ids.length} 个用户吗？该用户空间内的书架、进度、书签和设置也会删除。`,
        '批量删除用户',
        { type: 'warning' },
      )
      if (!operations.canCommit(operation)) return
      const { data } = await options.deleteUsers(ids)
      if (!operations.canCommit(operation)) return
      selectedUserIds.value = []
      options.onSuccess(`删除用户成功：${data.deleted || ids.length} 个`)
      await load()
    } catch (error) {
      if (error === 'cancel' || error === 'close') return
      if (operations.canCommit(operation)) {
        options.onError(error, '删除用户失败')
      }
    } finally {
      if (operations.canCommit(operation)) deletingUsers.value = false
    }
  }

  async function setDefaultSources(row) {
    if (!isSelectable(row)) return
    const operation = operations.begin(`set-default-sources:${row.id}`)
    defaultingSourceUserId.value = row.id
    try {
      await options.confirm(
        `确认要将用户${row.username}的书源设为默认书源（新用户有效）吗?`,
        '提示',
        {
          confirmButtonText: '确定',
          cancelButtonText: '取消',
          type: 'warning',
        },
      )
      if (!operations.canCommit(operation)) return
      await options.setUserSourcesAsDefault(row.id)
      if (!operations.canCommit(operation)) return
      options.onSuccess('设置成功')
      await load()
    } catch (error) {
      if (error === 'cancel' || error === 'close') return
      if (operations.canCommit(operation)) {
        options.onError(error, '设置失败')
      }
    } finally {
      if (operations.canCommit(operation) && defaultingSourceUserId.value === row.id) {
        defaultingSourceUserId.value = null
      }
    }
  }

  async function resetSelectedSources() {
    const ids = selectedUserIds.value.filter(id => (
      users.value.some(user => user.id === id && isSelectable(user))
    ))
    if (!ids.length) {
      options.onWarning('请选择需要删除书源的用户')
      return
    }
    const operation = operations.begin('reset-user-sources')
    resettingSources.value = true
    try {
      await options.confirm(
        '确认要删除所选择的用户书源吗?',
        '提示',
        {
          confirmButtonText: '确定',
          cancelButtonText: '取消',
          type: 'warning',
        },
      )
      if (!operations.canCommit(operation)) return
      await options.resetUserSources(ids)
      if (!operations.canCommit(operation)) return
      selectedUserIds.value = []
      options.onSuccess('操作成功')
      await load()
    } catch (error) {
      if (error === 'cancel' || error === 'close') return
      if (operations.canCommit(operation)) {
        options.onError(error, '操作失败')
      }
    } finally {
      if (operations.canCommit(operation)) resettingSources.value = false
    }
  }

  async function updatePermission(row) {
    const operation = operations.begin(`update-permission:${row.id}`)
    try {
      await options.updateUser(row.id, {
        canEditSources: row.canEditSources,
        canAccessStore: row.canAccessStore,
        canAccessWebdav: row.canAccessWebdav,
        bookLimit: row.bookLimit,
        sourceLimit: row.sourceLimit,
      })
      if (!operations.canCommit(operation)) return
      options.onSuccess('用户权限已更新')
    } catch (error) {
      if (!operations.canCommit(operation)) return
      options.onError(error, '更新用户失败')
      await load()
    }
  }

  return {
    users,
    usersLoading,
    deletingUsers,
    resettingSources,
    defaultingSourceUserId,
    creatingUser,
    createDialogVisible,
    selectedUserIds,
    selectedDeletableUserIds,
    draft,
    load,
    resetManager,
    handleUpdated,
    clearRefresh,
    isSelectable,
    isDeletable,
    isMutable,
    changeSelection,
    toggleSelection,
    openCreateDialog,
    create,
    resetPassword,
    removeSelected,
    setDefaultSources,
    resetSelectedSources,
    updatePermission,
    resetOperations: operations.reset,
  }
}
