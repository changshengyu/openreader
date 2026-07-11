import { reactive, ref } from 'vue'

export function useOverlayUserManagement(options) {
  const users = ref([])
  const usersLoading = ref(false)
  const cleanupLoading = ref(false)
  const deletingUsers = ref(false)
  const creatingUser = ref(false)
  const createDialogVisible = ref(false)
  const selectedUserIds = ref([])
  const draft = reactive({
    username: '',
    password: '',
    role: 'user',
    canEditSources: true,
    canAccessStore: true,
  })
  const scheduleTimeout = options.setTimeout || globalThis.setTimeout
  const cancelTimeout = options.clearTimeout || globalThis.clearTimeout
  let refreshTimer
  let managerRequest = 0

  function isDeletable(user) {
    return user.role !== 'admin' && user.id !== options.getCurrentUserId()
  }

  async function load() {
    const request = ++managerRequest
    usersLoading.value = true
    try {
      if (!options.userStore.profile) await options.userStore.loadMe()
      const { data } = await options.listUsers()
      if (request !== managerRequest) return
      users.value = data || []
      selectedUserIds.value = selectedUserIds.value.filter(id => (
        users.value.some(user => user.id === id && isDeletable(user))
      ))
    } catch (error) {
      if (request !== managerRequest) return
      options.onError(error, '加载用户失败')
    } finally {
      if (request === managerRequest) usersLoading.value = false
    }
  }

  function clearRefresh() {
    if (!refreshTimer) return
    cancelTimeout(refreshTimer)
    refreshTimer = undefined
  }

  function resetManager() {
    managerRequest += 1
    clearRefresh()
    users.value = []
    selectedUserIds.value = []
    usersLoading.value = false
    cleanupLoading.value = false
    deletingUsers.value = false
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
    selectedUserIds.value = rows.filter(isDeletable).map(user => user.id)
  }

  function toggleSelection(id, checked) {
    const user = users.value.find(item => item.id === id)
    if (!user || !isDeletable(user)) return
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
      role: 'user',
      canEditSources: true,
      canAccessStore: true,
    })
    createDialogVisible.value = true
  }

  async function create() {
    const username = draft.username.trim()
    if (username.length < 3 || draft.password.length < 6) {
      options.onWarning('用户名至少 3 位，密码至少 6 位')
      return
    }
    creatingUser.value = true
    try {
      await options.createUser({
        username,
        password: draft.password,
        role: draft.role,
        canEditSources: draft.canEditSources,
        canAccessStore: draft.canAccessStore,
      })
      options.onSuccess('新增用户成功')
      createDialogVisible.value = false
      await load()
    } catch (error) {
      options.onError(error, '新增用户失败')
    } finally {
      creatingUser.value = false
    }
  }

  async function resetPassword(row) {
    try {
      const result = await options.prompt(
        '',
        `重置 ${row.username} 的密码`,
        {
          confirmButtonText: '确定',
          cancelButtonText: '取消',
          inputType: 'password',
          inputValidator(value) {
            if (!value || value.length < 6) return '密码至少 6 位'
            return true
          },
        },
      )
      await options.resetUserPassword(row.id, { password: result.value })
      options.onSuccess('重置密码成功')
    } catch (error) {
      if (error === 'cancel' || error === 'close') return
      options.onError(error, '重置密码失败')
    }
  }

  async function removeSelected() {
    const ids = [...selectedUserIds.value]
    if (!ids.length) {
      options.onWarning('请选择需要删除的用户')
      return
    }
    deletingUsers.value = true
    try {
      await options.confirm(
        `确认要删除所选择的 ${ids.length} 个用户吗？该用户空间内的书架、进度、书签和设置也会删除。`,
        '批量删除用户',
        { type: 'warning' },
      )
      const { data } = await options.deleteUsers(ids)
      selectedUserIds.value = []
      options.onSuccess(`删除用户成功：${data.deleted || ids.length} 个`)
      await load()
    } catch (error) {
      if (error === 'cancel' || error === 'close') return
      options.onError(error, '删除用户失败')
    } finally {
      deletingUsers.value = false
    }
  }

  async function updatePermission(row) {
    try {
      await options.updateUser(row.id, {
        canEditSources: row.canEditSources,
        canAccessStore: row.canAccessStore,
        bookLimit: row.bookLimit,
        sourceLimit: row.sourceLimit,
      })
      options.onSuccess('用户权限已更新')
    } catch (error) {
      options.onError(error, '更新用户失败')
      await load()
    }
  }

  async function cleanupInactive() {
    cleanupLoading.value = true
    try {
      await options.confirm(
        '确定清理不活跃用户吗？',
        '清理用户',
        { type: 'warning' },
      )
      await options.cleanupInactiveUsers()
      options.onSuccess('清理完成')
      await load()
    } catch (error) {
      if (error !== 'cancel' && error !== 'close') {
        options.onError(error, '清理用户失败')
      }
    } finally {
      cleanupLoading.value = false
    }
  }

  return {
    users,
    usersLoading,
    cleanupLoading,
    deletingUsers,
    creatingUser,
    createDialogVisible,
    selectedUserIds,
    draft,
    load,
    resetManager,
    handleUpdated,
    clearRefresh,
    isDeletable,
    changeSelection,
    toggleSelection,
    openCreateDialog,
    create,
    resetPassword,
    removeSelected,
    updatePermission,
    cleanupInactive,
  }
}
