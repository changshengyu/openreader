import { defineStore } from 'pinia'
import { getMe, loginUser } from '../api/user'
import { useBookshelfStore } from './bookshelf'
import { useReaderStore } from './reader'

export const useUserStore = defineStore('user', {
  state: () => ({
    token: localStorage.getItem('openreader_token') || '',
    profile: null,
  }),
  actions: {
    async login(username, password, mode = 'login') {
      const { data } = await loginUser(mode, { username, password })
      this.token = data.token
      this.profile = data.user
      localStorage.setItem('openreader_token', data.token)
    },
    async loadMe() {
      const { data } = await getMe()
      this.profile = data
    },
    logout() {
      this.token = ''
      this.profile = null
      localStorage.removeItem('openreader_token')
      useBookshelfStore().resetShelfState()
      useReaderStore().ensureProgressScope()
    },
  },
})
