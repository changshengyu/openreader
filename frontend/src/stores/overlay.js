import { defineStore } from 'pinia'

export const useOverlayStore = defineStore('overlay', {
  state: () => ({
    bookInfoVisible: false,
    bookInfoBook: null,
    bookInfoOptions: {},
    bookEditVisible: false,
    bookEditBook: null,
    bookManageVisible: false,
    bookGroupVisible: false,
    bookGroupMode: 'manage',
    importBookVisible: false,
    bookmarkVisible: false,
    bookmarkBook: null,
    bookmarkFormVisible: false,
    bookmarkFormBook: null,
    bookmarkFormDraft: null,
    bookmarkFormMode: 'create',
    bookmarkFormResolve: null,
    searchBookContentVisible: false,
    searchBook: null,
    localStoreVisible: false,
    rssVisible: false,
    webdavVisible: false,
    userManageVisible: false,
    replaceRulesVisible: false,
    backupVisible: false,
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
    openBookEdit(book) {
      this.bookEditBook = book
      this.bookEditVisible = true
    },
    closeBookEdit() {
      this.bookEditVisible = false
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
    openBookmarkForm(book, draft = {}, options = {}) {
      if (this.bookmarkFormResolve) {
        this.finishBookmarkForm({ saved: false, reason: 'replaced' })
      }
      this.bookmarkFormBook = book || null
      this.bookmarkFormDraft = { ...draft }
      this.bookmarkFormMode = options.mode === 'edit' ? 'edit' : 'create'
      this.bookmarkFormVisible = true
      return new Promise(resolve => {
        this.bookmarkFormResolve = resolve
      })
    },
    finishBookmarkForm(result = { saved: false }) {
      const resolve = this.bookmarkFormResolve
      this.bookmarkFormResolve = null
      this.bookmarkFormVisible = false
      resolve?.(result)
    },
    clearBookmarkForm() {
      if (this.bookmarkFormVisible) return
      this.bookmarkFormBook = null
      this.bookmarkFormDraft = null
      this.bookmarkFormMode = 'create'
    },
    openSearchBookContent(book) {
      this.searchBook = book
      this.searchBookContentVisible = true
    },
    openReplaceRules() {
      this.replaceRulesVisible = true
    },
    openRSS() {
      this.rssVisible = true
    },
    openUserManage() {
      this.userManageVisible = true
    },
    openLocalStore() {
      this.localStoreVisible = true
    },
    openWebDAV() {
      this.webdavVisible = true
    },
    openBackup() {
      this.backupVisible = true
    },
  },
})
