import { ref } from 'vue'

export function useWorkspaceBackupActions(options) {
  const backupLoading = ref(false)
  const portableBackupLoading = ref(false)

  async function runBackup() {
    if (backupLoading.value) return false
    const confirmed = await confirm(options.confirmBackup)
    if (!confirmed) return false
    backupLoading.value = true
    try {
      const { data } = await options.triggerBackup()
      options.onSuccess(`当前账户备份已保存：${data?.name || data?.path || 'backup.zip'}`)
      return true
    } catch (error) {
      options.onError(error, '保存当前账户备份失败')
      return false
    } finally {
      backupLoading.value = false
    }
  }

  async function runPortableBackup() {
    if (portableBackupLoading.value) return false
    const confirmed = await confirm(options.confirmPortable)
    if (!confirmed) return false
    portableBackupLoading.value = true
    try {
      const { data } = await options.triggerPortableBackup()
      options.onSuccess(`完整本地书备份已保存：${data?.name || data?.path || 'portable_backup.zip'}（${data?.localBooks || 0} 本）`)
      return true
    } catch (error) {
      options.onError(error, '保存完整本地书备份失败')
      return false
    } finally {
      portableBackupLoading.value = false
    }
  }

  return {
    backupLoading,
    portableBackupLoading,
    runBackup,
    runPortableBackup,
  }
}

async function confirm(callback) {
  if (!callback) return true
  return callback().then(() => true, () => false)
}
