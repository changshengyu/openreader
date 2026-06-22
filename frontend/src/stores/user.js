import { defineStore } from 'pinia'
import { getMe, loginUser } from '../api/user'
import { useBookshelfStore } from './bookshelf'
import { usePreferencesStore } from './preferences'
import { useReaderStore } from './reader'

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
      this.token = data.token
      this.profile = data.user
      this.authDialogVisible = false
      this.authReason = ''
      localStorage.setItem('openreader_token', data.token)
    },
    async loadMe() {
      const { data } = await getMe()
      this.profile = data
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
