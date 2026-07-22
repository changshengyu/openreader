import { defineStore } from 'pinia'
import { normalizeDeletedBookIds } from '../utils/bookDeletion.js'

export const useOverlayStore = defineStore('overlay', {
  state: () => ({
    bookInfoVisible: false,
    bookInfoBook: null,
    bookInfoOptions: {},
    bookAddCategoryVisible: false,
    bookAddCategoryIds: [],
    bookAddCategoryResolve: null,
    bookEditVisible: false,
    bookEditBook: null,
    bookManageVisible: false,
    bookGroupVisible: false,
    bookGroupMode: 'manage',
    importBookVisible: false,
    storageImportVisible: false,
    storageImportRequest: null,
    storageImportRequestSerial: 0,
    sourceManageVisible: false,
    sourceManageIntent: 'manage',
    bookmarkVisible: false,
    bookmarkBook: null,
    bookmarkCreateDraft: null,
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
    replaceRuleEditorDraft: null,
    replaceRuleEditorRequest: 0,
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
    selectBookAddCategories(initialCategoryIds = []) {
      if (this.bookAddCategoryResolve) {
        this.finishBookAddCategories()
      }
      this.bookAddCategoryIds = normalizeCategoryIds(initialCategoryIds)
      this.bookAddCategoryVisible = true
      return new Promise(resolve => {
        this.bookAddCategoryResolve = resolve
      })
    },
    finishBookAddCategories(categoryIds = null) {
      const resolve = this.bookAddCategoryResolve
      this.bookAddCategoryResolve = null
      this.bookAddCategoryVisible = false
      this.bookAddCategoryIds = []
      resolve?.(categoryIds === null ? null : normalizeCategoryIds(categoryIds))
    },
    openBookEdit(book) {
      const bookId = Number(book?.id)
      if (!Number.isInteger(bookId) || bookId <= 0) return false
      this.bookEditBook = book
      this.bookEditVisible = true
      return true
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
    openStorageImport(source, paths) {
      const normalizedSource = ['local-store', 'webdav'].includes(source) ? source : ''
      const normalizedPaths = Array.isArray(paths)
        ? [...new Set(paths.map(path => String(path || '').trim()).filter(Boolean))]
        : []
      if (!normalizedSource || !normalizedPaths.length) return
      this.storageImportRequestSerial += 1
      this.storageImportRequest = {
        requestId: this.storageImportRequestSerial,
        source: normalizedSource,
        paths: normalizedPaths,
      }
      this.storageImportVisible = true
    },
    closeStorageImport() {
      this.storageImportVisible = false
      this.storageImportRequest = null
    },
    openSourceManage(intent = 'manage') {
      this.sourceManageIntent = normalizeSourceManageIntent(intent)
      this.sourceManageVisible = true
    },
    closeSourceManage() {
      this.sourceManageVisible = false
      this.sourceManageIntent = 'manage'
    },
    openBookmark(book, options = {}) {
      this.bookmarkBook = book || null
      this.bookmarkCreateDraft = options.createDraft && typeof options.createDraft === 'object'
        ? { ...options.createDraft }
        : null
      this.bookmarkVisible = true
    },
    clearBookmark() {
      if (this.bookmarkVisible) return
      this.bookmarkBook = null
      this.bookmarkCreateDraft = null
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
    openReplaceRuleEditor(draft = {}) {
      this.replaceRuleEditorDraft = { ...draft }
      this.replaceRuleEditorRequest += 1
    },
    clearReplaceRuleEditor() {
      this.replaceRuleEditorDraft = null
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
    reconcileDeletedBooks(bookIds) {
      const deleted = new Set(normalizeDeletedBookIds(bookIds))
      if (!deleted.size) return false
      const targets = book => deleted.has(Number(book?.id))
      let matched = false

      if (targets(this.bookEditBook)) {
        matched = true
        this.bookEditVisible = false
        this.bookEditBook = null
      }
      if (targets(this.bookmarkBook)) {
        matched = true
        this.bookmarkVisible = false
        this.bookmarkBook = null
        this.bookmarkCreateDraft = null
      }
      if (targets(this.bookmarkFormBook)) {
        matched = true
        this.finishBookmarkForm({ saved: false, reason: 'book-deleted' })
        this.clearBookmarkForm()
      }
      if (targets(this.searchBook)) {
        matched = true
        this.searchBookContentVisible = false
        this.searchBook = null
      }
      if (targets(this.bookInfoBook)) {
        matched = true
        if (this.bookGroupMode === 'set') this.bookGroupVisible = false
        this.bookInfoVisible = false
        this.bookInfoBook = null
        this.bookInfoOptions = {}
      }
      return matched
    },
  },
})

function normalizeSourceManageIntent(intent) {
  return ['manage', 'import', 'remote', 'health', 'debug'].includes(intent)
    ? intent
    : 'manage'
}

function normalizeCategoryIds(categoryIds) {
  const values = Array.isArray(categoryIds) ? categoryIds : [categoryIds]
  return [...new Set(values.map(Number).filter(id => Number.isInteger(id) && id > 0))]
}
