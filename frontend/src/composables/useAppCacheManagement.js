import { computed, ref } from 'vue'

const EMPTY_BROWSER_STATS = {
  total: { files: 0, size: 0 },
  groups: {},
}
const GROUPS = [
  { group: 'bookSourceList', label: '书源缓存' },
  { group: 'rssSources', label: 'RSS源缓存' },
  { group: 'chapterList', label: '章节列表缓存', always: true },
  { group: 'chapterContent', label: '章节内容缓存', always: true },
]

function isCancelled(error) {
  return error === 'cancel' || error === 'close'
}

export function useAppCacheManagement(options) {
  const serverStats = ref({})
  const browserStats = ref(EMPTY_BROWSER_STATS)
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
      .filter(row => row.always || groupFiles(row.group) > 0)
      .map(row => ({
        key: `clear-${row.group}`,
        label: clearingBrowserGroup.value === row.group
          ? '清理中'
          : clearBrowserLabel(row.group, row.label),
        action: () => clearBrowser(row.group),
      }))
  ))

  async function loadStats() {
    loading.value = true
    const [serverResult, browserResult] = await Promise.allSettled([
      options.getServerStats(),
      options.getBrowserStats(),
    ])
    serverStats.value = serverResult.status === 'fulfilled'
      ? serverResult.value?.data || {}
      : {}
    browserStats.value = browserResult.status === 'fulfilled'
      ? browserResult.value || EMPTY_BROWSER_STATS
      : EMPTY_BROWSER_STATS
    loading.value = false
  }

  async function clearServer() {
    try {
      await options.confirm(
        '确定清理服务器章节缓存吗？清理后阅读时会重新加载远程章节内容。',
        '清理缓存',
        { type: 'warning' },
      )
      clearingServer.value = true
      const { data } = await options.clearServerCache()
      options.onSuccess(
        `已清理 ${data.clearedFiles || 0} 个文件，释放 ${formatSize(data.clearedSize || 0)}`,
      )
      await loadStats()
    } catch (error) {
      if (isCancelled(error)) return
      options.onError(error, '清理缓存失败')
    } finally {
      clearingServer.value = false
    }
  }

  async function clearBrowser(group) {
    const label = groupLabel(group)
    try {
      if (!groupFiles(group)) {
        options.onInfo(`${label}为空`)
        return
      }
      await options.confirm(
        `确定清理当前浏览器的${label}吗？清理后会在需要时重新加载。`,
        '清理浏览器缓存',
        { type: 'warning' },
      )
      clearingBrowserGroup.value = group
      const removed = await options.clearBrowserGroup(group)
      options.onSuccess(`已清理${label} ${removed} 项`)
      await loadStats()
    } catch (error) {
      if (isCancelled(error)) return
      options.onError(error, '清理浏览器缓存失败')
    } finally {
      clearingBrowserGroup.value = ''
    }
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
    group,
    groupFiles,
    clearBrowserLabel,
    groupLabel,
    formatSize,
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
