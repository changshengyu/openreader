import { defineStore } from 'pinia'
import { getMe, loginUser } from '../api/user'
import { createAuthenticatedOperationGuard } from '../utils/authenticatedOperation'
import { currentUserScope } from '../utils/authScope'
import { useBookshelfStore } from './bookshelf'
import { useOverlayStore } from './overlay'
import { usePreferencesStore } from './preferences'
import { useReaderStore } from './reader'
import { useIndexWorkspaceStore } from './indexWorkspace'
import { cancelAllBookManagementCacheJobs } from '../composables/useOverlayBookItemActions'

const profileOperations = createAuthenticatedOperationGuard()

export const useUserStore = defineStore('user', {
  state: () => ({
    token: localStorage.getItem('openreader_token') || '',
    profile: null,
    authDialogVisible: false,
    authReason: '',
    sessionGeneration: 0,
    readerSessionBlocked: false,
    invalidatedScope: '',
  }),
  actions: {
    async login(username, password, mode = 'login') {
      const { data } = await loginUser(mode, { username, password })
      const previousScope = this.invalidatedScope
      profileOperations.reset()
      this.token = data.token
      this.profile = data.user
      this.authDialogVisible = false
      this.authReason = ''
      localStorage.setItem('openreader_token', data.token)
      const currentScope = currentUserScope()
      const sameAuthenticatedScope = Boolean(previousScope && previousScope === currentScope)
      const workspace = useIndexWorkspaceStore()
      if (sameAuthenticatedScope) {
        workspace.resumeSuspendedSession()
      } else {
        workspace.discardSuspendedSession()
        workspace.resetSessionState()
      }
      this.sessionGeneration += 1
      if (typeof window !== 'undefined') delete window.__openreaderAuthRequired
      return {
        previousScope,
        currentScope,
        sameAuthenticatedScope,
      }
    },
    async loadMe() {
      const operation = profileOperations.begin('profile')
      const { data } = await getMe()
      if (!profileOperations.canCommit(operation)) return null
      this.profile = data
      return data
    },
    logout() {
      this.clearSession({ suspendWorkspace: false })
      this.authDialogVisible = false
      this.authReason = ''
    },
    requireLogin(reason = 'session', rejectedToken = '') {
      if (rejectedToken && this.token && this.token !== rejectedToken) return
      if (rejectedToken && !this.token) {
        const pendingToken = typeof window === 'undefined'
          ? ''
          : window.__openreaderAuthRequired?.rejectedToken
        if (pendingToken !== rejectedToken || this.authDialogVisible) return
      }
      if (reason === 'session' && this.authDialogVisible && this.readerSessionBlocked) return
      this.clearSession({ suspendWorkspace: true, tokenHint: rejectedToken })
      this.authReason = reason
      this.authDialogVisible = true
    },
    clearSession({ suspendWorkspace = true, tokenHint = '' } = {}) {
      let scope = currentUserScope(this.token)
      if (scope === 'anonymous') scope = currentUserScope(tokenHint)
      if (scope === 'anonymous') scope = currentUserScope()
      if (scope !== 'anonymous') this.invalidatedScope = scope
      this.readerSessionBlocked = true
      this.sessionGeneration += 1
      const workspace = useIndexWorkspaceStore()
      if (suspendWorkspace) {
        workspace.suspendSessionState()
      } else {
        workspace.resetSessionState()
      }
      dispatchSessionInvalidated({
        generation: this.sessionGeneration,
      })
      profileOperations.reset()
      cancelAllBookManagementCacheJobs()
      this.token = ''
      this.profile = null
      localStorage.removeItem('openreader_token')
      useOverlayStore().resetSessionState()
      useBookshelfStore().resetShelfState()
      usePreferencesStore().resetPreferenceState()
      const reader = useReaderStore()
      reader.resetReaderSettingsState()
      reader.ensureProgressScope()
    },
    completeReauthentication() {
      this.readerSessionBlocked = false
      this.invalidatedScope = ''
    },
  },
})

function dispatchSessionInvalidated(detail) {
  if (typeof window === 'undefined') return
  window.dispatchEvent(new CustomEvent('openreader:session-invalidated', { detail }))
}
