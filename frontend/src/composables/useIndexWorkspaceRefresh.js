import { ref } from 'vue'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation.js'

const REFRESH_JOBS = [
  ['书架', 'refreshShelf'],
  ['书源', 'refreshSources'],
  ['用户配置', 'refreshPreferences'],
  ['阅读设置', 'refreshReaderSettings'],
  ['RSS、替换规则和书签', 'refreshOverlays'],
  ['缓存统计', 'refreshCacheStats'],
]

export function useIndexWorkspaceRefresh(options) {
  const operations = createAuthenticatedOperationGuard({
    getIdentity: options.getIdentity,
  })
  const loading = ref(false)

  async function refresh() {
    const operation = operations.begin('refresh')
    loading.value = true
    try {
      const results = await Promise.allSettled(
        REFRESH_JOBS.map(([, key]) => Promise.resolve().then(() => options[key]?.())),
      )
      if (!operations.canCommit(operation)) return false

      const failures = results
        .map((result, index) => ({ result, label: REFRESH_JOBS[index][0] }))
        .filter(row => row.result.status === 'rejected')
      const shelfFailure = failures.find(row => row.label === '书架')
      if (shelfFailure) {
        options.onError?.(shelfFailure.result.reason, '刷新缓存失败')
        return false
      }
      if (failures.length) {
        options.onWarning?.(`工作台已刷新，部分数据刷新失败：${failures.map(row => row.label).join('、')}`)
        return true
      }
      options.onSuccess?.('工作台缓存已刷新')
      return true
    } finally {
      if (operations.canCommit(operation)) loading.value = false
    }
  }

  function resetScope() {
    operations.reset()
    loading.value = false
  }

  return {
    loading,
    refresh,
    resetScope,
  }
}
