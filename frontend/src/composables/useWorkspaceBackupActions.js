import { ref } from 'vue'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation.js'

export function useWorkspaceBackupActions(options) {
  const operations = options.operationGuard || createAuthenticatedOperationGuard({
    getIdentity: options.getAuthenticatedIdentity,
  })
  const backupLoading = ref(false)
  const portableBackupLoading = ref(false)

  async function runBackup() {
    if (backupLoading.value) return false
    const operation = operations.begin('backup')
    const confirmed = await confirm(options.confirmBackup)
    if (!confirmed || !operations.canCommit(operation)) return false
    backupLoading.value = true
    try {
      const { data } = await options.triggerBackup()
      if (!operations.canCommit(operation)) return false
      options.onSuccess(`当前账户备份已保存：${data?.name || data?.path || 'backup.zip'}`)
      return true
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '保存当前账户备份失败')
      }
      return false
    } finally {
      if (operations.canCommit(operation)) backupLoading.value = false
    }
  }

  async function runPortableBackup() {
    if (portableBackupLoading.value) return false
    const operation = operations.begin('portable-backup')
    const confirmed = await confirm(options.confirmPortable)
    if (!confirmed || !operations.canCommit(operation)) return false
    portableBackupLoading.value = true
    try {
      const { data } = await options.triggerPortableBackup()
      if (!operations.canCommit(operation)) return false
      const legacyNotice = Number(data?.legacyAssets || 0) > 0
        ? `；另有 ${Number(data.legacyAssets)} 个旧版资源仅保留链接`
        : ''
      options.onSuccess(
        `完整可移植备份已保存：${data?.name || data?.path || 'portable_backup.zip'}（${data?.localBooks || 0} 本书，${data?.assets || 0} 个自定义资源）${legacyNotice}`,
      )
      return true
    } catch (error) {
      if (operations.canCommit(operation)) {
        options.onError(error, '保存完整可移植备份失败')
      }
      return false
    } finally {
      if (operations.canCommit(operation)) portableBackupLoading.value = false
    }
  }

  return {
    backupLoading,
    portableBackupLoading,
    runBackup,
    runPortableBackup,
    resetOperations: operations.reset,
  }
}

async function confirm(callback) {
  if (!callback) return true
  return callback().then(() => true, () => false)
}
