<template>
  <el-dialog
    v-model="overlay.userManageVisible"
    title="用户管理"
    width="min(1120px, calc(100vw - 48px))"
    :fullscreen="isMobile"
    class="global-user-dialog"
    destroy-on-close
    @open="loadUsers"
    @closed="resetManager"
  >
    <section class="user-overlay">
      <header class="file-overlay-head">
        <div>
          <strong>用户空间</strong>
          <span>管理员可调整书源、WebDAV、书仓权限和用户限制</span>
        </div>
        <div class="file-actions">
          <el-button size="small" type="primary" :icon="Edit" @click="openCreateUserDialog">新增</el-button>
          <el-button size="small" :icon="Refresh" :loading="usersLoading" @click="loadUsers">刷新</el-button>
        </div>
      </header>

      <el-table
        :data="users"
        stripe
        v-loading="usersLoading"
        class="desktop-user-table"
        @selection-change="onUserSelectionChange"
      >
        <el-table-column type="selection" width="44" :selectable="isUserDeletable" />
        <el-table-column prop="username" label="用户名" min-width="140" />
        <el-table-column prop="role" label="角色" width="90" />
        <el-table-column prop="lastActiveAt" label="最近活跃" min-width="150">
          <template #default="{ row }">
            {{ formatUserTime(row.lastActiveAt) }}
          </template>
        </el-table-column>
        <el-table-column prop="createdAt" label="注册时间" min-width="150">
          <template #default="{ row }">
            {{ formatUserTime(row.createdAt, '—') }}
          </template>
        </el-table-column>
        <el-table-column prop="bookCount" label="书籍" width="80" />
        <el-table-column prop="sourceCount" label="全局书源" width="100" />
        <el-table-column label="权限" min-width="300">
          <template #default="{ row }">
            <div v-if="isUserMutable(row)" class="permission-row">
              <el-switch v-model="row.canEditSources" size="small" active-text="书源" @change="updateUserPermission(row)" />
              <el-switch v-model="row.canAccessWebdav" size="small" active-text="WebDAV" @change="updateUserPermission(row)" />
              <el-switch v-model="row.canAccessStore" size="small" active-text="书仓" @change="updateUserPermission(row)" />
            </div>
            <span v-else class="protected-user-label">受保护账号</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="110" fixed="right">
          <template #default="{ row }">
            <el-button v-if="isUserMutable(row)" text @click="resetPassword(row)">重置密码</el-button>
            <span v-else class="protected-user-label">—</span>
          </template>
        </el-table-column>
      </el-table>

      <div v-if="users.length" v-loading="usersLoading" class="mobile-user-list">
        <article v-for="user in users" :key="user.id" class="mobile-user-card">
          <header>
            <el-checkbox
              :disabled="!isUserDeletable(user)"
              :model-value="selectedUserIds.includes(user.id)"
              @change="toggleUserSelection(user.id, $event)"
            />
            <div>
              <strong>{{ user.username }}</strong>
              <span>{{ user.role }} · 书籍 {{ user.bookCount || 0 }} · 全局书源 {{ user.sourceCount || 0 }}</span>
              <span>最近活跃：{{ formatUserTime(user.lastActiveAt) }} · 注册：{{ formatUserTime(user.createdAt, '—') }}</span>
            </div>
          </header>
          <div v-if="isUserMutable(user)" class="permission-row">
            <el-switch v-model="user.canEditSources" size="small" active-text="书源" @change="updateUserPermission(user)" />
            <el-switch v-model="user.canAccessWebdav" size="small" active-text="WebDAV" @change="updateUserPermission(user)" />
            <el-switch v-model="user.canAccessStore" size="small" active-text="书仓" @change="updateUserPermission(user)" />
            <el-button size="small" text @click="resetPassword(user)">重置密码</el-button>
          </div>
          <span v-else class="protected-user-label">受保护账号</span>
        </article>
      </div>

      <footer v-if="users.length" class="user-manage-footer">
        <span class="check-tip">已选择 {{ selectedUserIds.length }} 个</span>
        <el-button
          size="small"
          type="danger"
          :disabled="!selectedUserIds.length"
          :loading="deletingUsers"
          @click="deleteSelectedUsers"
        >
          批量删除
        </el-button>
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
      <el-button type="primary" :loading="creatingUser" @click="createManagedUser">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { onBeforeUnmount, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Edit, Refresh } from '@element-plus/icons-vue'
import * as adminApi from '../../api/admin'
import { useOverlayUserManagement } from '../../composables/useOverlayUserManagement'
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

const {
  users,
  usersLoading,
  deletingUsers,
  creatingUser,
  createDialogVisible: userCreateDialog,
  selectedUserIds,
  draft: userDraft,
  load: loadUsers,
  resetManager,
  handleUpdated: handleUsersUpdated,
  clearRefresh: clearUsersRefreshTimer,
  isDeletable: isUserDeletable,
  isMutable: isUserMutable,
  changeSelection: onUserSelectionChange,
  toggleSelection: toggleUserSelection,
  openCreateDialog: openCreateUserDialog,
  create: createManagedUser,
  resetPassword,
  removeSelected: deleteSelectedUsers,
  updatePermission: updateUserPermission,
} = useOverlayUserManagement({
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

function formatUserTime(value, emptyLabel = '未登录') {
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

.file-overlay-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.file-overlay-head > div:first-child {
  display: grid;
  gap: 2px;
}

.file-overlay-head span,
.check-tip,
.mobile-user-card span,
.protected-user-label {
  color: var(--app-text-muted);
  font-size: 12px;
}

.file-actions,
.permission-row,
.user-manage-footer {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.file-actions,
.user-manage-footer {
  justify-content: flex-end;
}

.permission-row {
  gap: 12px;
}

.mobile-user-list {
  display: none;
}

.mobile-user-card {
  display: grid;
  gap: 10px;
  padding: 10px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-radius-sm);
}

.mobile-user-card header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.mobile-user-card header > div {
  display: grid;
  min-width: 0;
  flex: 1;
  gap: 2px;
}

@media (max-width: 750px) {
  .file-overlay-head {
    align-items: flex-start;
    display: grid;
  }

  .file-actions {
    justify-content: flex-start;
  }

  .desktop-user-table {
    display: none;
  }

  .mobile-user-list {
    display: grid;
    gap: 10px;
  }
}
</style>
