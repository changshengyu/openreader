import { ref } from 'vue'

export function useOverlayBackups(options) {
  const backups = ref([])
  const backupLoading = ref(false)
  const listLoading = ref(false)
  const restoreLoading = ref(false)

  async function load() {
    listLoading.value = true
    try {
      const { data } = await options.listBackups()
      backups.value = data || []
    } catch (error) {
      options.onError(error, '加载备份列表失败')
    } finally {
      listLoading.value = false
    }
  }

  async function run() {
    if (options.confirmBeforeRun) {
      const confirmed = await options.confirmBeforeRun().then(() => true, () => false)
      if (!confirmed) return
    }
    backupLoading.value = true
    try {
      const { data } = await options.triggerBackup()
      options.onSuccess(
        `备份已保存到 WebDAV：${data.name || data.path || 'backup.zip'}`,
      )
      await load()
    } catch (error) {
      options.onError(error, '保存备份失败')
    } finally {
      backupLoading.value = false
    }
  }

  async function download(row) {
    try {
      const response = await options.downloadBackup(row.name)
      options.saveBlob(response.data, row.name)
    } catch (error) {
      options.onError(error, '下载备份失败')
    }
  }

  async function restore(data) {
    const file = data.raw
    if (!file) return
    if (options.confirmBeforeRestore) {
      const confirmed = await options.confirmBeforeRestore().then(() => true, () => false)
      if (!confirmed) return
    }
    restoreLoading.value = true
    try {
      const form = options.createFormData()
      form.append('file', file)
      const { data: result } = await options.restoreBackup(form)
      options.onSuccess(
        `恢复完成：书源 ${result.sources || 0}，书籍 ${result.books || 0}，进度 ${result.progress || 0}`,
      )
      await options.applyRestoreResult(result)
    } catch (error) {
      options.onError(error, '恢复备份失败')
    } finally {
      restoreLoading.value = false
    }
  }

  return {
    backups,
    backupLoading,
    listLoading,
    restoreLoading,
    load,
    run,
    download,
    restore,
  }
}
