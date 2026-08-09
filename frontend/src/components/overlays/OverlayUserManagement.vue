<template>
  <el-dialog
    v-model="overlay.userManageVisible"
    width="min(1120px, calc(100vw - 48px))"
    :fullscreen="isMobile"
    class="global-user-dialog"
    destroy-on-close
    @open="loadUsers"
    @closed="resetManager"
  >
    <template #header>
      <div class="user-dialog-title">
        <span class="el-dialog__title">用户管理</span>
        <el-button text type="primary" @click="openCreateUserDialog">新增</el-button>
      </div>
    </template>
    <section class="user-overlay">
      <el-table
        :data="users"
        :height="isMobile ? 'calc(100dvh - 160px)' : 'min(620px, calc(100vh - 250px))'"
        v-loading="usersLoading"
        class="user-manage-table"
        @selection-change="onUserSelectionChange"
      >
        <el-table-column type="selection" width="44" :selectable="isUserSelectable" :fixed="isMobile" />
        <el-table-column prop="username" label="用户名" min-width="120" :fixed="isMobile" />
        <el-table-column prop="lastLoginAt" label="上次登录" min-width="150">
          <template #default="{ row }">
            {{ formatUserTime(row.lastLoginAt) }}
          </template>
        </el-table-column>
        <el-table-column prop="createdAt" label="注册时间" min-width="150">
          <template #default="{ row }">
            {{ formatUserTime(row.createdAt, '—') }}
          </template>
        </el-table-column>
        <el-table-column prop="canAccessWebdav" label="WebDAV" min-width="90">
          <template #default="{ row }">
            <el-switch
              v-if="isUserMutable(row)"
              v-model="row.canAccessWebdav"
              size="small"
              active-text="WebDAV"
              :loading="isPermissionUpdating(row, 'canAccessWebdav')"
              @change="value => updateUserPermission(row, 'canAccessWebdav', value)"
            />
          </template>
        </el-table-column>
        <el-table-column prop="canAccessStore" label="书仓" min-width="80">
          <template #default="{ row }">
            <el-switch
              v-if="isUserMutable(row)"
              v-model="row.canAccessStore"
              size="small"
              active-text="书仓"
              :loading="isPermissionUpdating(row, 'canAccessStore')"
              @change="value => updateUserPermission(row, 'canAccessStore', value)"
            />
          </template>
        </el-table-column>
        <el-table-column prop="canEditSources" label="书源编辑" min-width="100">
          <template #default="{ row }">
            <el-switch
              v-if="isUserMutable(row)"
              v-model="row.canEditSources"
              size="small"
              active-text="书源"
              :loading="isPermissionUpdating(row, 'canEditSources')"
              @change="value => updateUserPermission(row, 'canEditSources', value)"
            />
            <span v-else class="protected-user-label">受保护账号</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="170">
          <template #default="{ row }">
            <el-button v-if="isUserMutable(row)" text @click="resetPassword(row)">重置密码</el-button>
            <el-button
              text
              :loading="defaultingSourceUserId === row.id"
              @click="setDefaultSources(row)"
            >
              设为默认书源
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <footer v-if="users.length" class="user-manage-footer">
        <el-button
          size="medium"
          type="primary"
          :loading="deletingUsers"
          @click="deleteSelectedUsers"
        >
          批量删除
        </el-button>
        <el-button
          size="medium"
          type="primary"
          :loading="resettingSources"
          @click="resetSelectedSources"
        >
          删除用户书源
        </el-button>
        <span class="check-tip">已选择 {{ selectedUserIds.length }} 个</span>
        <el-button class="cancel-button" size="medium" @click="closeUserManager">取消</el-button>
      </footer>
      <el-empty v-if="!usersLoading && !users.length" description="暂无用户，或当前账号无管理员权限" />
    </section>
  </el-dialog>

  <el-dialog
    v-model="userCreateDialog"
    title="新增用户"
    width="420px"
    :fullscreen="isMobile"
  >
    <el-form label-position="top">
      <el-form-item label="用户名">
        <el-input v-model="userDraft.username" autocomplete="on" />
      </el-form-item>
      <el-form-item label="密码">
        <el-input v-model="userDraft.password" type="password" show-password autocomplete="new-password" />
      </el-form-item>
      <el-form-item label="权限">
        <div class="permission-row">
          <el-switch v-model="userDraft.canEditSources" active-text="书源" />
          <el-switch v-model="userDraft.canAccessWebdav" active-text="WebDAV" />
          <el-switch v-model="userDraft.canAccessStore" active-text="书仓" />
        </div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="userCreateDialog = false">取消</el-button>
      <el-button type="primary" :loading="creatingUser" @click="createManagedUser">确定</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { onBeforeUnmount, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import * as adminApi from '../../api/admin'
import { useOverlayUserManagement } from '../../composables/useOverlayUserManagement'
import { useAuthenticatedOperationGuard } from '../../composables/useAuthenticatedOperationGuard'
import { useOverlayStore } from '../../stores/overlay'
import { useUserStore } from '../../stores/user'

defineProps({
  isMobile: {
    type: Boolean,
    default: false,
  },
})

const overlay = useOverlayStore()
const userStore = useUserStore()
const operations = useAuthenticatedOperationGuard()

const {
  users,
  usersLoading,
  deletingUsers,
  resettingSources,
  defaultingSourceUserId,
  creatingUser,
  createDialogVisible: userCreateDialog,
  selectedUserIds,
  selectedDeletableUserIds,
  draft: userDraft,
  load: loadUsers,
  resetManager,
  handleUpdated: handleUsersUpdated,
  clearRefresh: clearUsersRefreshTimer,
  isSelectable: isUserSelectable,
  isMutable: isUserMutable,
  isPermissionUpdating,
  changeSelection: onUserSelectionChange,
  openCreateDialog: openCreateUserDialog,
  create: createManagedUser,
  resetPassword,
  removeSelected: deleteSelectedUsers,
  setDefaultSources,
  resetSelectedSources,
  updatePermission: updateUserPermission,
} = useOverlayUserManagement({
  operationGuard: operations,
  userStore,
  getCurrentUserId: () => userStore.profile?.id || null,
  isActive: () => overlay.userManageVisible,
  ...adminApi,
  prompt: (...args) => ElMessageBox.prompt(...args),
  confirm: (...args) => ElMessageBox.confirm(...args),
  onSuccess: message => ElMessage.success(message),
  onWarning: message => ElMessage.warning(message),
  onError: (error, fallback) => ElMessage.error(readError(error, fallback)),
})

onMounted(() => {
  window.addEventListener('openreader:users-updated', handleUsersUpdated)
})

onBeforeUnmount(() => {
  window.removeEventListener('openreader:users-updated', handleUsersUpdated)
  clearUsersRefreshTimer()
})

function readError(error, fallback) {
  return error?.response?.data?.error?.message ||
    error?.response?.data?.error ||
    fallback
}

function closeUserManager() {
  overlay.userManageVisible = false
}

function formatUserTime(value, emptyLabel = '') {
  if (!value) return emptyLabel
  const date = new Date(value)
  if (Number.isNaN(date.getTime()) || date.getFullYear() < 2000) return emptyLabel
  return date.toLocaleString('zh-CN', {
    hour12: false,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}
</script>

<style scoped>
.user-overlay {
  display: grid;
  gap: 12px;
}

.user-dialog-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.check-tip,
.protected-user-label {
  color: var(--app-text-muted);
  font-size: 12px;
}

.permission-row,
.user-manage-footer {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.permission-row {
  gap: 12px;
}

.cancel-button {
  margin-left: auto;
}

@media (max-width: 750px) {
  .user-overlay {
    gap: 8px;
  }
}
</style>
