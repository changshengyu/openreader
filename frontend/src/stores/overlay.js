import { defineStore } from 'pinia'

export const useOverlayStore = defineStore('overlay', {
  state: () => ({
    bookInfoVisible: false,
    bookInfoBook: null,
    bookInfoOptions: {},
    bookManageVisible: false,
    bookGroupVisible: false,
    bookGroupMode: 'manage',
    importBookVisible: false,
    bookmarkVisible: false,
    bookmarkBook: null,
    searchBookContentVisible: false,
    searchBook: null,
  }),
  actions: {
    openBookInfo(book, options = {}) {
      this.bookInfoBook = book
      this.bookInfoOptions = options
      this.bookInfoVisible = true
    },
    closeBookInfo() {
      this.bookInfoVisible = false
    },
    openBookManage() {
      this.bookManageVisible = true
    },
    openBookGroup(mode = 'manage', book = null, options = {}) {
      if (book) {
        this.bookInfoBook = book
        this.bookInfoOptions = options
      }
      this.bookGroupMode = mode
      this.bookGroupVisible = true
    },
    openImportBook() {
      this.importBookVisible = true
    },
    openBookmark(book) {
      this.bookmarkBook = book
      this.bookmarkVisible = true
    },
    openSearchBookContent(book) {
      this.searchBook = book
      this.searchBookContentVisible = true
    },
    openReplaceRules(router) {
      router?.push?.({ name: 'settings', query: { panel: 'replace' } })
    },
    openRSS(router) {
      router?.push?.({ name: 'settings', query: { panel: 'rss' } })
    },
    openUserManage(router) {
      router?.push?.({ name: 'settings', query: { panel: 'admin' } })
    },
    openLocalStore(router) {
      router?.push?.({ name: 'local-store' })
    },
    openWebDAV(router) {
      router?.push?.({ name: 'settings', query: { panel: 'webdav' } })
    },
  },
})
