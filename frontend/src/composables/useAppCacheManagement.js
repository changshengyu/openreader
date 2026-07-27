import { computed, ref } from 'vue'
import {
  createAuthenticatedOperationGuard,
  currentAuthenticatedIdentity,
} from '../utils/authenticatedOperation.js'

const EMPTY_BROWSER_STATS = {
  total: { files: 0, size: 0 },
  groups: {},
}
const GROUPS = [
  { group: 'bookSourceList', label: '书源缓存' },
  { group: 'rssSources', label: 'RSS源缓存' },
  { group: 'chapterList', label: '章节列表缓存' },
  { group: 'chapterContent', label: '章节内容缓存' },
]

function isCancelled(error) {
  return error === 'cancel' || error === 'close'
}

export function useAppCacheManagement(options) {
  const operationGuard = createAuthenticatedOperationGuard({
    getIdentity: options.getIdentity || currentAuthenticatedIdentity,
  })
  const serverStats = ref({})
  const browserStats = ref(emptyBrowserStats())
  const loading = ref(false)
  const clearingServer = ref(false)
  const clearingBrowserGroup = ref('')

  const sectionTitle = computed(() => {
    const size = Number(serverStats.value?.size || 0) +
      Number(browserStats.value?.total?.size || 0)
    return size ? `本地缓存 ${formatSize(size)}` : '本地缓存'
  })
  const clearServerLabel = computed(() => {
    const size = Number(serverStats.value?.size || 0)
    return size
      ? `清空服务器缓存 ${formatSize(size)}`
      : '清空服务器缓存'
  })
  const browserNavItems = computed(() => (
    GROUPS
      .map(row => ({
        key: `clear-${row.group}`,
        label: clearingBrowserGroup.value === row.group
          ? '清理中'
          : clearBrowserLabel(row.group, row.label),
        action: () => clearBrowser(row.group),
      }))
  ))

  async function loadStats() {
    const operation = operationGuard.begin('stats')
    loading.value = true
    const [serverResult, browserResult] = await Promise.allSettled([
      Promise.resolve().then(() => options.getServerStats()),
      Promise.resolve().then(() => options.getBrowserStats(operation.scope)),
    ])
    if (!operationGuard.canCommit(operation)) return false
    serverStats.value = serverResult.status === 'fulfilled'
      ? serverResult.value?.data || {}
      : {}
    browserStats.value = browserResult.status === 'fulfilled'
      ? browserResult.value || emptyBrowserStats()
      : emptyBrowserStats()
    loading.value = false
    return true
  }

  async function clearServer() {
    const operation = operationGuard.begin('clear-server')
    try {
      await options.confirm(
        '确定清理服务器章节缓存吗？清理后阅读时会重新加载远程章节内容。',
        '清理缓存',
        { type: 'warning' },
      )
      if (!operationGuard.canCommit(operation)) return
      invalidateStats()
      clearingServer.value = true
      const { data } = await options.clearServerCache()
      if (!operationGuard.canCommit(operation)) return
      options.onSuccess(
        `已清理 ${data.clearedFiles || 0} 个文件，释放 ${formatSize(data.clearedSize || 0)}`,
      )
      await loadStats()
    } catch (error) {
      if (isCancelled(error)) return
      if (operationGuard.canCommit(operation)) {
        options.onError(error, '清理缓存失败')
      }
    } finally {
      if (operationGuard.canCommit(operation)) clearingServer.value = false
    }
  }

  async function clearBrowser(group) {
    const operation = operationGuard.begin('clear-browser')
    const label = groupLabel(group)
    try {
      if (!groupFiles(group)) {
        if (operationGuard.canCommit(operation)) options.onInfo(`${label}为空`)
        return
      }
      await options.confirm(
        `确定清理当前浏览器的${label}吗？清理后会在需要时重新加载。`,
        '清理浏览器缓存',
        { type: 'warning' },
      )
      if (!operationGuard.canCommit(operation)) return
      invalidateStats()
      clearingBrowserGroup.value = group
      const removed = await options.clearBrowserGroup(group, operation.scope)
      if (!operationGuard.canCommit(operation)) return
      options.onSuccess(`已清理${label} ${removed} 项`)
      await loadStats()
    } catch (error) {
      if (isCancelled(error)) return
      if (operationGuard.canCommit(operation)) {
        options.onError(error, '清理浏览器缓存失败')
      }
    } finally {
      if (operationGuard.canCommit(operation)) clearingBrowserGroup.value = ''
    }
  }

  function invalidateStats() {
    operationGuard.invalidate('stats')
    loading.value = false
  }

  function resetScope() {
    operationGuard.reset()
    serverStats.value = {}
    browserStats.value = emptyBrowserStats()
    loading.value = false
    clearingServer.value = false
    clearingBrowserGroup.value = ''
  }

  function group(groupName) {
    return browserStats.value?.groups?.[groupName] || { files: 0, size: 0 }
  }

  function groupFiles(groupName) {
    return Number(group(groupName).files || 0)
  }

  function clearBrowserLabel(groupName, label) {
    const size = Number(group(groupName).size || 0)
    return size ? `清空${label} ${formatSize(size)}` : `清空${label}`
  }

  function groupLabel(groupName) {
    return GROUPS.find(row => row.group === groupName)?.label || '缓存'
  }

  return {
    serverStats,
    browserStats,
    loading,
    clearingServer,
    clearingBrowserGroup,
    sectionTitle,
    clearServerLabel,
    browserNavItems,
    loadStats,
    clearServer,
    clearBrowser,
    resetScope,
    group,
    groupFiles,
    clearBrowserLabel,
    groupLabel,
    formatSize,
  }
}

function emptyBrowserStats() {
  return {
    total: { ...EMPTY_BROWSER_STATS.total },
    groups: {},
  }
}

export function formatSize(bytes) {
  const value = Number(bytes || 0)
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  if (value < 1024 * 1024 * 1024) {
    return `${(value / 1024 / 1024).toFixed(1)} MB`
  }
  return `${(value / 1024 / 1024 / 1024).toFixed(2)} GB`
}
