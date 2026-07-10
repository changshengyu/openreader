import { computed, ref, watch } from 'vue'

export function useAppSidebarSearch(options) {
  const quickSearch = ref('')
  const sources = ref([])
  const concurrentOptions = [8, 16, 32, 60]

  const searchType = computed({
    get: () => options.preferences.search.searchType,
    set: value => options.preferences.setSearchConfig({ searchType: value }),
  })
  const searchGroup = computed({
    get: () => options.preferences.search.group,
    set: value => options.preferences.setSearchConfig({ group: value }),
  })
  const sourceId = computed({
    get: () => options.preferences.search.sourceId,
    set: value => options.preferences.setSearchConfig({ sourceId: value }),
  })
  const concurrent = computed({
    get: () => options.preferences.search.concurrent,
    set: value => options.preferences.setSearchConfig({ concurrent: value }),
  })
  const enabledSources = computed(() => (
    sources.value.filter(source => source.enabled)
  ))
  const sourceGroups = computed(() => {
    const groups = new Map()
    for (const source of enabledSources.value) {
      const name = source.group || '默认分组'
      groups.set(name, (groups.get(name) || 0) + 1)
    }
    return [...groups.entries()].map(([label, count]) => ({
      label,
      value: label,
      count,
    }))
  })

  function searchRouteQuery(keyword = '') {
    const query = {}
    if (keyword) query.q = keyword
    query.searchType = searchType.value
    query.concurrent = concurrent.value
    if (searchType.value === 'group' && searchGroup.value) {
      query.group = searchGroup.value
    }
    if (searchType.value === 'single' && sourceId.value) {
      query.sourceId = sourceId.value
    }
    return query
  }

  function localSearchRouteQuery(keyword = quickSearch.value.trim()) {
    const query = { mode: 'local' }
    if (keyword) query.q = keyword
    return query
  }

  function goSearch() {
    const keyword = quickSearch.value.trim()
    if (!keyword) {
      options.onWarning('请输入关键词进行搜索')
      return
    }
    openSearchWorkspace(searchRouteQuery(keyword))
  }

  function goSearchRoute(mode = 'remote') {
    const keyword = quickSearch.value.trim()
    const query = mode === 'local'
      ? localSearchRouteQuery(keyword)
      : searchRouteQuery(keyword)
    openSearchWorkspace(query)
  }

  function openSearchWorkspace(query) {
    if (typeof options.onWorkspaceSearch === 'function') {
      options.onWorkspaceSearch(query)
      return
    }
    options.router.push({ name: 'search', query })
    options.afterNavigate?.()
  }

  function clearSearchQuery() {
    if (
      !isSearchScene(options.route) ||
      options.route.query.q === undefined
    ) {
      return
    }
    const { q, ...query } = options.route.query
    options.router.replace({ name: options.route.name === 'home' ? 'home' : 'search', query })
  }

  async function loadSources() {
    try {
      const response = await options.cacheFirstRequest(
        () => options.listSources(),
        sourceCacheKey(),
        { validate: data => Array.isArray(data) },
      )
      applySources(response.data)
      if (response.fromCache) refreshSourcesCache().catch(() => {})
    } catch {
      sources.value = []
    }
  }

  async function refreshSourcesCache() {
    const response = await options.networkFirstRequest(
      () => options.listSources(),
      sourceCacheKey(),
      { validate: data => Array.isArray(data) },
    )
    applySources(response.data)
  }

  function applySources(data) {
    sources.value = Array.isArray(data) ? data : []
    if (!searchGroup.value && sourceGroups.value.length) {
      searchGroup.value = sourceGroups.value[0].value
    }
    if (!sourceId.value && enabledSources.value.length) {
      sourceId.value = enabledSources.value[0].id
    }
  }

  function sourceCacheKey() {
    return `bookSourceList@${options.getUserScope()}`
  }

  async function handleSourcesUpdated() {
    await options.removeBrowserCache(sourceCacheKey())
    await loadSources()
    await options.afterSourcesUpdated?.()
  }

  watch(
    () => [options.route.name, options.route.query.q],
    ([name, value]) => {
      if (name === 'search' || (name === 'home' && options.route.query.workspace === 'search')) {
        quickSearch.value = typeof value === 'string' ? value : ''
      } else if (name !== 'home') {
        quickSearch.value = ''
      }
    },
    { immediate: true },
  )

  return {
    quickSearch,
    sources,
    concurrentOptions,
    searchType,
    searchGroup,
    sourceId,
    concurrent,
    enabledSources,
    sourceGroups,
    searchRouteQuery,
    localSearchRouteQuery,
    goSearch,
    goSearchRoute,
    openSearchWorkspace,
    clearSearchQuery,
    loadSources,
    refreshSourcesCache,
    applySources,
    sourceCacheKey,
    handleSourcesUpdated,
  }
}

function isSearchScene(route) {
  return route?.name === 'search' || (route?.name === 'home' && route?.query?.workspace === 'search')
}
