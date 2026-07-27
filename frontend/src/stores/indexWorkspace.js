import { defineStore } from 'pinia'
import {
  DEFAULT_SEARCH,
  normalizeSearchConcurrent,
} from '../utils/searchPreference.js'

const RESULT_MODES = new Set(['search', 'explore'])

function freshContinuation() {
  return {
    page: 1,
    lastIndex: -1,
    hasMore: false,
    loading: false,
  }
}

function freshSearch() {
  return {
    keyword: '',
    mode: 'remote',
    searchType: DEFAULT_SEARCH.searchType,
    group: '',
    sourceId: '',
    concurrent: DEFAULT_SEARCH.concurrent,
  }
}

function freshExplore() {
  return {
    sourceId: '',
    sourceGroup: '',
    url: '',
    name: '',
    sourceName: '',
  }
}

function normalizedText(value) {
  return typeof value === 'string' ? value.trim() : ''
}

function normalizedPositivePage(value, fallback = 1) {
  const parsed = Number(value)
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback
}

function normalizedLastIndex(value, fallback = -1) {
  const parsed = Number(value)
  return Number.isInteger(parsed) ? parsed : fallback
}

function normalizedRows(rows) {
  return Array.isArray(rows) ? [...rows] : []
}

function suspendedWorkspaceIntent(state) {
  if (state.mode === 'search') {
    return {
      mode: 'search',
      search: { ...state.search },
    }
  }
  if (state.mode === 'explore') {
    return {
      mode: 'explore',
      explore: { ...state.explore },
    }
  }
  return { mode: 'shelf' }
}

function normalizedExplore(intent = {}, fallback = freshExplore()) {
  return {
    sourceId: intent.sourceId ?? fallback.sourceId ?? '',
    sourceGroup: normalizedText(intent.sourceGroup ?? fallback.sourceGroup),
    url: normalizedText(intent.url ?? fallback.url),
    name: normalizedText(intent.name ?? fallback.name),
    sourceName: normalizedText(intent.sourceName ?? fallback.sourceName),
  }
}

function mergeContinuation(current, values = {}, { completed = false } = {}) {
  return {
    page: normalizedPositivePage(values.page, current.page),
    lastIndex: normalizedLastIndex(values.lastIndex, current.lastIndex),
    hasMore: values.hasMore === undefined ? current.hasMore : Boolean(values.hasMore),
    loading: completed ? false : (values.loading === undefined ? current.loading : Boolean(values.loading)),
  }
}

/**
 * State contract shared by the upstream-style Index workspace. The store deliberately
 * owns only scene state: network requests and legacy URL adaptation remain at the
 * view boundary while the P1 migration is in progress.
 */
export const useIndexWorkspaceStore = defineStore('index-workspace', {
  state: () => ({
    mode: 'shelf',
    resultRows: [],
    continuation: freshContinuation(),
    resultScrollTop: 0,
    searchRevision: 0,
    exploreRevision: 0,
    exploreChooserRevision: 0,
    exploreChooserPending: false,
    sessionGeneration: 0,
    suspendedSession: null,
    search: freshSearch(),
    explore: freshExplore(),
  }),
  getters: {
    showingResults: state => RESULT_MODES.has(state.mode),
    isSearchResult: state => state.mode === 'search',
    isExploreResult: state => state.mode === 'explore',
  },
  actions: {
    beginSearch(intent = {}) {
      this.mode = 'search'
      this.search = {
        keyword: normalizedText(intent.keyword),
        mode: intent.mode === 'local' ? 'local' : 'remote',
        searchType: normalizedText(intent.searchType) || DEFAULT_SEARCH.searchType,
        group: normalizedText(intent.group),
        sourceId: intent.sourceId ?? '',
        concurrent: normalizeSearchConcurrent(intent.concurrent),
      }
      this.clearResultState()
      this.searchRevision += 1
    },
    requestExplore(intent = {}) {
      this.explore = normalizedExplore(intent, this.explore)
      this.exploreChooserRevision += 1
      this.exploreChooserPending = true
    },
    beginExplore(intent = {}) {
      this.requestExplore(intent)
    },
    showExploreResults(rows, intent = {}) {
      this.mode = 'explore'
      this.exploreChooserPending = false
      this.explore = normalizedExplore(intent, this.explore)
      this.resultRows = normalizedRows(rows)
      this.continuation = {
        page: normalizedPositivePage(intent.page),
        lastIndex: -1,
        hasMore: Boolean(intent.hasMore),
        loading: false,
      }
      this.resultScrollTop = 0
    },
    replaceResultRows(rows, continuation = {}) {
      if (!this.showingResults) return
      this.resultRows = normalizedRows(rows)
      this.continuation = mergeContinuation(this.continuation, continuation, { completed: true })
    },
    appendResultRows(rows, continuation = {}) {
      if (!this.showingResults) return
      this.resultRows = [...this.resultRows, ...normalizedRows(rows)]
      this.continuation = mergeContinuation(this.continuation, continuation, { completed: true })
    },
    setResultLoading(loading) {
      if (!this.showingResults) return
      this.continuation = {
        ...this.continuation,
        loading: Boolean(loading),
      }
    },
    rememberResultScroll(value) {
      if (!this.showingResults) return
      const offset = Number(value)
      this.resultScrollTop = Number.isFinite(offset) && offset > 0 ? offset : 0
    },
    backToShelf() {
      this.mode = 'shelf'
      this.clearResultState()
    },
    suspendSessionState() {
      const suspended = this.suspendedSession || suspendedWorkspaceIntent(this)
      this.resetSessionState({ discardSuspended: false })
      this.suspendedSession = suspended
    },
    resumeSuspendedSession() {
      const suspended = this.suspendedSession
      this.suspendedSession = null
      if (suspended?.mode === 'search') {
        this.beginSearch(suspended.search)
        return 'search'
      }
      if (suspended?.mode === 'explore') {
        this.requestExplore(suspended.explore)
        return 'explore'
      }
      return 'shelf'
    },
    discardSuspendedSession() {
      this.suspendedSession = null
    },
    consumeExploreChooserRequest() {
      if (!this.exploreChooserPending) return false
      this.exploreChooserPending = false
      return true
    },
    resetSessionState({ discardSuspended = true } = {}) {
      this.sessionGeneration += 1
      this.mode = 'shelf'
      this.resultRows = []
      this.continuation = freshContinuation()
      this.resultScrollTop = 0
      this.searchRevision += 1
      this.exploreRevision += 1
      this.exploreChooserRevision = 0
      this.exploreChooserPending = false
      this.search = freshSearch()
      this.explore = freshExplore()
      if (discardSuspended) this.suspendedSession = null
    },
    clearResultState() {
      this.resultRows = []
      this.continuation = freshContinuation()
      this.resultScrollTop = 0
    },
  },
})
