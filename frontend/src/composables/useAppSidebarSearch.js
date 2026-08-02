import { computed, ref, watch } from 'vue'
import {
  searchConcurrentLabel,
  searchConcurrentOptions,
} from '../utils/searchPreference.js'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation.js'
import {
  storedSearchType,
  visibleSearchGroupOptions,
  visibleSearchMode,
} from '../utils/indexSearchPresentation.js'

export function useAppSidebarSearch(options) {
  const quickSearch = ref('')
  const sources = ref([])
  const sourceOperations = createAuthenticatedOperationGuard(
    options.getAuthenticatedIdentity
      ? { getIdentity: options.getAuthenticatedIdentity }
      : undefined,
  )

  const searchType = computed({
    get: () => visibleSearchMode(options.preferences.search.searchType),
    set: value => {
      options.preferences.setSearchConfig({
        searchType: storedSearchType(value, searchGroup.value),
      })
      notifySearchConfigChange()
    },
  })
  const searchGroup = computed({
    get: () => options.preferences.search.group,
    set: value => {
      options.preferences.setSearchConfig({
        group: value,
        searchType: storedSearchType('multi', value),
      })
      notifySearchConfigChange()
    },
  })
  const sourceId = computed({
    get: () => options.preferences.search.sourceId,
    set: value => {
      options.preferences.setSearchConfig({ sourceId: value })
      notifySearchConfigChange()
    },
  })
  const concurrent = computed({
    get: () => options.preferences.search.concurrent,
    set: value => {
      options.preferences.setSearchConfig({ concurrent: value })
      notifySearchConfigChange()
    },
  })
  const concurrentOptions = computed(() => searchConcurrentOptions(concurrent.value))
  const enabledSources = computed(() => (
    sources.value.filter(source => source.enabled)
  ))
  const sourceGroups = computed(() => visibleSearchGroupOptions(sources.value))

  function currentStoredSearchType() {
    return storedSearchType(searchType.value, searchGroup.value)
  }

  function searchRouteQuery(keyword = '') {
    const query = {}
    if (keyword) query.q = keyword
    query.searchType = currentStoredSearchType()
    query.concurrent = concurrent.value
    if (query.searchType === 'group' && searchGroup.value) {
      query.group = searchGroup.value
    }
    if (query.searchType === 'single' && sourceId.value) {
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
    if (searchType.value === 'single' && !sourceId.value) {
      options.onWarning('请选择书源进行搜索')
      return
    }
    openSearchWorkspace(searchRouteQuery(keyword))
  }

  function notifySearchConfigChange() {
    if (typeof options.onSearchConfigChange !== 'function') return
    options.onSearchConfigChange(searchRouteQuery(quickSearch.value.trim()))
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
      options.afterNavigate?.()
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
    const operation = sourceOperations.begin('sources')
    try {
      const response = await options.cacheFirstRequest(
        () => options.listSources(),
        sourceCacheKey(),
        { validate: data => Array.isArray(data) },
      )
      if (!sourceOperations.canCommit(operation)) return false
      applySources(response.data)
      if (response.fromCache) refreshSourcesCache().catch(() => {})
      return true
    } catch {
      if (sourceOperations.canCommit(operation) && !sources.value.length) sources.value = []
      return false
    }
  }

  async function refreshSourcesCache() {
    const operation = sourceOperations.begin('sources')
    try {
      const response = await options.networkFirstRequest(
        () => options.listSources(),
        sourceCacheKey(),
        { validate: data => Array.isArray(data) },
      )
      if (!sourceOperations.canCommit(operation)) return false
      applySources(response.data)
      return true
    } catch (error) {
      if (!sourceOperations.canCommit(operation)) return false
      throw error
    }
  }

  function applySources(data) {
    sources.value = Array.isArray(data) ? data : []
    if (!sourceId.value && enabledSources.value.length) {
      options.preferences.setSearchConfig({ sourceId: enabledSources.value[0].id })
    }
  }

  function sourceCacheKey() {
    return `bookSourceList@source-owner-v1@${options.getUserScope()}`
  }

  function legacySourceCacheKey() {
    return `bookSourceList@${options.getUserScope()}`
  }

  async function handleSourcesUpdated() {
    const operation = sourceOperations.begin('update')
    await options.removeBrowserCache(sourceCacheKey())
    await options.removeBrowserCache(legacySourceCacheKey())
    if (!sourceOperations.canCommit(operation)) return
    await loadSources()
    if (!sourceOperations.canCommit(operation)) return
    await options.afterSourcesUpdated?.()
  }

  function dispose() {
    sourceOperations.reset()
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
    concurrentLabel: searchConcurrentLabel,
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
    dispose,
  }
}

function isSearchScene(route) {
  return route?.name === 'search' || (route?.name === 'home' && route?.query?.workspace === 'search')
}
