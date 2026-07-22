import { defineStore } from 'pinia'
import api from '../api/client'
import { currentUserScope } from '../utils/authScope'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation'
import { DEFAULT_SEARCH, sanitizeSearchPreference } from '../utils/searchPreference.js'

export {
  DEFAULT_SEARCH,
  SEARCH_CONCURRENT_OPTIONS,
  searchConcurrentLabel,
  searchConcurrentOptions,
  sanitizeSearchPreference,
} from '../utils/searchPreference.js'

const PREFERENCE_KEYS = ['shelf', 'search']
const SHELF_LAYOUT_VERSION = 2
const DEFAULT_SHELF = { view: 'grid', layoutVersion: SHELF_LAYOUT_VERSION, groupKey: 'builtin:all' }
const syncTimers = new Map()
const preferenceOperations = createAuthenticatedOperationGuard()

export const usePreferencesStore = defineStore('preferences', {
  state: () => ({
    preferenceScope: currentUserScope(),
    shelf: readLocalShelfPreference(),
    search: readLocalSearchPreference(),
    syncBaseUpdatedAt: {},
    syncing: {},
    syncError: {},
  }),
  persist: true,
  actions: {
    ensurePreferenceScope() {
      const scope = currentUserScope()
      if (!this.preferenceScope) {
        this.preferenceScope = scope
        return scope
      }
      if (this.preferenceScope !== scope) {
        this.resetPreferenceState(scope)
      }
      return scope
    },
    resetPreferenceState(scope = currentUserScope()) {
      clearPreferenceSyncTimers()
      preferenceOperations.reset()
      this.preferenceScope = scope
      this.shelf = { ...DEFAULT_SHELF }
      this.search = { ...DEFAULT_SEARCH }
      this.syncBaseUpdatedAt = {}
      this.syncing = {}
      this.syncError = {}
    },
    setShelfView(view) {
      this.ensurePreferenceScope()
      this.shelf = { ...this.shelf, layoutVersion: SHELF_LAYOUT_VERSION, view: view === 'list' ? 'list' : 'grid' }
      this.schedulePreferenceSync('shelf')
    },
    setShelfGroup(groupKey) {
      this.ensurePreferenceScope()
      this.shelf = { ...this.shelf, groupKey: normalizeBookGroupKey(groupKey) }
      this.schedulePreferenceSync('shelf')
    },
    setSearchConfig(config = {}) {
      this.ensurePreferenceScope()
      this.search = sanitizeSearchPreference({ ...this.search, ...config })
      this.schedulePreferenceSync('search')
    },
    applyPreference(key, value, updatedAt = '') {
      this.ensurePreferenceScope()
      if (key === 'shelf') this.shelf = sanitizeShelfPreference(value)
      if (key === 'search') this.search = sanitizeSearchPreference(value)
      if (updatedAt) this.syncBaseUpdatedAt[key] = updatedAt
      this.syncError[key] = ''
    },
    async loadPreferences(options = {}) {
      this.ensurePreferenceScope()
      return Promise.all(PREFERENCE_KEYS.map(key => this.loadPreference(key, options)))
    },
    async loadPreference(key, options = {}) {
      this.ensurePreferenceScope()
      if (!PREFERENCE_KEYS.includes(key) || !hasAuthToken()) return null
      const operation = preferenceOperations.begin(key)
      this.syncing[key] = false
      try {
        const { data } = await api.get(`/settings/${key}`)
        if (!preferenceOperations.canCommit(operation)) return null
        if (data?.value && typeof data.value === 'object') {
          this.applyPreference(key, data.value, data.updatedAt || '')
          return data.value
        }
        if (options.createIfMissing === false) {
          this.syncError[key] = '没有备份文件'
          return null
        }
        return await this.savePreference(key)
      } catch (err) {
        if (!preferenceOperations.canCommit(operation)) return null
        this.syncError[key] = readErrorMessage(err)
        return null
      }
    },
    schedulePreferenceSync(key) {
      this.ensurePreferenceScope()
      if (!PREFERENCE_KEYS.includes(key) || !hasAuthToken()) return
      this.syncError[key] = ''
      if (syncTimers.has(key)) clearTimeout(syncTimers.get(key))
      syncTimers.set(key, setTimeout(() => {
        syncTimers.delete(key)
        this.savePreference(key).catch(() => {})
      }, 700))
    },
    async savePreference(key, options = {}) {
      this.ensurePreferenceScope()
      if (!PREFERENCE_KEYS.includes(key) || !hasAuthToken()) return null
      if (syncTimers.has(key)) {
        clearTimeout(syncTimers.get(key))
        syncTimers.delete(key)
      }
      const operation = preferenceOperations.begin(key)
      this.syncing[key] = true
      this.syncError[key] = ''
      try {
        const { data, headers } = await api.put(`/settings/${key}`, {
          value: preferencePayload(this, key),
          baseUpdatedAt: this.syncBaseUpdatedAt[key] || '',
          ...(options.force === true ? { force: true } : {}),
        })
        if (!preferenceOperations.canCommit(operation)) return null
        if (data?.value && headers?.['x-openreader-setting-conflict']) {
          this.applyPreference(key, data.value, data.updatedAt || '')
          return data.value
        }
        if (data?.updatedAt) this.syncBaseUpdatedAt[key] = data.updatedAt
        return data?.value || preferencePayload(this, key)
      } catch (err) {
        if (!preferenceOperations.canCommit(operation)) return null
        this.syncError[key] = readErrorMessage(err)
        return null
      } finally {
        if (preferenceOperations.canCommit(operation)) this.syncing[key] = false
      }
    },
  },
})

function clearPreferenceSyncTimers() {
  for (const timer of syncTimers.values()) {
    clearTimeout(timer)
  }
  syncTimers.clear()
}

function preferencePayload(state, key) {
  if (key === 'shelf') return sanitizeShelfPreference(state.shelf)
  if (key === 'search') return sanitizeSearchPreference(state.search)
  return {}
}

function sanitizeShelfPreference(value = {}) {
  const migrated = Number(value.layoutVersion || 0) < SHELF_LAYOUT_VERSION
  return {
    ...DEFAULT_SHELF,
    view: !migrated && value.view === 'list' ? 'list' : 'grid',
    groupKey: normalizeBookGroupKey(value.groupKey),
  }
}

function normalizeBookGroupKey(value) {
  const key = String(value || '').trim()
  if (/^builtin:(all|local|audio|ungrouped)$/.test(key)) return key
  if (/^category:[1-9]\d*$/.test(key)) return key
  return 'builtin:all'
}

function readLocalShelfPreference() {
  try {
    return sanitizeShelfPreference({ view: window.localStorage?.getItem('openreader_shelf_view') })
  } catch {
    return DEFAULT_SHELF
  }
}

function readLocalSearchPreference() {
  try {
    return sanitizeSearchPreference(JSON.parse(window.localStorage?.getItem('openreader_sidebar_search') || '{}'))
  } catch {
    return DEFAULT_SEARCH
  }
}

function hasAuthToken() {
  return typeof localStorage !== 'undefined' && Boolean(localStorage.getItem('openreader_token'))
}

function readErrorMessage(err) {
  return err?.response?.data?.error || err?.message || '同步失败'
}
