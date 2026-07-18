import { defineStore } from 'pinia'
import { getMe, loginUser } from '../api/user'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation'
import { useBookshelfStore } from './bookshelf'
import { usePreferencesStore } from './preferences'
import { useReaderStore } from './reader'

const profileOperations = createAuthenticatedOperationGuard()

export const useUserStore = defineStore('user', {
  state: () => ({
    token: localStorage.getItem('openreader_token') || '',
    profile: null,
    authDialogVisible: false,
    authReason: '',
  }),
  actions: {
    async login(username, password, mode = 'login') {
      const { data } = await loginUser(mode, { username, password })
      profileOperations.reset()
      this.token = data.token
      this.profile = data.user
      this.authDialogVisible = false
      this.authReason = ''
      localStorage.setItem('openreader_token', data.token)
      if (typeof window !== 'undefined') delete window.__openreaderAuthRequired
    },
    async loadMe() {
      const operation = profileOperations.begin('profile')
      const { data } = await getMe()
      if (!profileOperations.canCommit(operation)) return null
      this.profile = data
      return data
    },
    logout() {
      this.clearSession()
      this.authDialogVisible = false
      this.authReason = ''
    },
    requireLogin(reason = 'session', rejectedToken = '') {
      if (rejectedToken && this.token && this.token !== rejectedToken) return
      this.clearSession()
      this.authReason = reason
      this.authDialogVisible = true
    },
    clearSession() {
      profileOperations.reset()
      this.token = ''
      this.profile = null
      localStorage.removeItem('openreader_token')
      useBookshelfStore().resetShelfState()
      usePreferencesStore().resetPreferenceState()
      const reader = useReaderStore()
      reader.resetReaderSettingsState()
      reader.ensureProgressScope()
    },
  },
})
