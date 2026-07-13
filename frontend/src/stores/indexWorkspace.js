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
    beginExplore(intent = {}) {
      this.mode = 'explore'
      this.explore = {
        sourceId: intent.sourceId ?? '',
        sourceGroup: normalizedText(intent.sourceGroup),
        url: normalizedText(intent.url),
        name: normalizedText(intent.name),
      }
      this.clearResultState()
      this.exploreRevision += 1
    },
    showExploreResults(rows, intent = {}) {
      this.mode = 'explore'
      this.explore = {
        sourceId: intent.sourceId ?? '',
        sourceGroup: normalizedText(intent.sourceGroup),
        url: normalizedText(intent.url),
        name: normalizedText(intent.name),
      }
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
    clearResultState() {
      this.resultRows = []
      this.continuation = freshContinuation()
      this.resultScrollTop = 0
    },
  },
})
