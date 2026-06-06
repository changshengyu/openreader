import { ref } from 'vue'
import { useReaderStore } from '../stores/reader'
import { useBookshelfStore } from '../stores/bookshelf'
import { usePreferencesStore } from '../stores/preferences'
import { useUserStore } from '../stores/user'

const connected = ref(false)
let socket
let reconnectTimer
let bookshelfRefreshTimer
let replaceRulesUpdateTimer
let rssUpdateTimer
let bookmarksUpdateTimer
let bookshelfRefreshPending = { books: false, categories: false }
let rssUpdatePending = { sources: false, articles: false, payload: null }
let bookmarksUpdatePending = { bookIds: new Set(), payload: null }
let reconnectDelay = 1500
let manualDisconnect = false
const MAX_RECONNECT_DELAY = 15000

export function useSync() {
  const reader = useReaderStore()
  const bookshelf = useBookshelfStore()
  const preferences = usePreferencesStore()
  const userStore = useUserStore()

  function connect() {
    const token = localStorage.getItem('openreader_token')
    if (!token || socket) return
    manualDisconnect = false
    clearReconnectTimer()

    const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
    socket = new WebSocket(`${protocol}://${window.location.host}/ws/sync?token=${encodeURIComponent(token)}`)

    socket.addEventListener('open', () => {
      connected.value = true
      reconnectDelay = 1500
      Promise.all([
        bookshelf.loadCategories(),
        bookshelf.loadBooks({ all: true }),
      ]).catch(() => {})
    })
    socket.addEventListener('close', () => {
      connected.value = false
      socket = undefined
      scheduleReconnect()
    })
    socket.addEventListener('error', () => {
      socket?.close()
    })
    socket.addEventListener('message', (event) => {
      let message
      try {
        message = JSON.parse(event.data)
      } catch {
        return
      }
      if (message.type === 'progress_update') {
        if (message.payload?.clientId && message.payload.clientId === reader.ensureClientId()) return
        const progressPayload = stripProgressClientId(message.payload)
        const progress = reader.applyServerProgress(progressPayload) || progressPayload
        bookshelf.applyBookProgress(progress, { replace: true })
        dispatchWindowEvent('openreader:progress-updated', {
          progress,
          raw: message.payload,
        })
      }
      if (message.type === 'bookshelf_update') {
        if (Array.isArray(message.payload)) {
          message.payload.forEach(book => {
            if (book?.id) bookshelf.upsertBook(book)
          })
        } else if (message.payload?.id) {
          bookshelf.upsertBook(message.payload)
        } else {
          scheduleBookshelfRefresh({ books: true, categories: true })
        }
      }
      if (message.type === 'bookshelf_delete') {
        const ids = Array.isArray(message.payload?.ids)
          ? message.payload.ids
          : [message.payload?.id]
        ids.filter(Boolean).forEach(id => bookshelf.removeBookLocal(id))
      }
      if (message.type === 'category_update') {
        bookshelf.upsertCategory(message.payload)
      }
      if (message.type === 'category_delete') {
        bookshelf.removeCategoryLocal(message.payload?.id)
      }
      if (message.type === 'categories_update') {
        if (Array.isArray(message.payload)) {
          bookshelf.replaceCategories(message.payload)
        } else {
          scheduleBookshelfRefresh({ books: false, categories: true })
        }
      }
      if (message.type === 'settings_update' && message.payload?.key === 'reader') {
        reader.loadReaderSettings().catch(() => {})
      }
      if (message.type === 'settings_update' && message.payload?.key === 'all') {
        reader.loadReaderSettings().catch(() => {})
        preferences.loadPreferences().catch(() => {})
      }
      if (message.type === 'settings_update' && ['shelf', 'search'].includes(message.payload?.key)) {
        preferences.loadPreference(message.payload.key).catch(() => {})
      }
      if (message.type === 'sources_update') {
        dispatchWindowEvent('openreader:sources-update', message.payload)
      }
      if (message.type === 'replace_rules_update') {
        scheduleReplaceRulesUpdate(message.payload)
      }
      if (message.type === 'rss_update') {
        scheduleRSSUpdate(message.payload)
      }
      if (message.type === 'bookmarks_update') {
        scheduleBookmarksUpdate(message.payload)
      }
      if (message.type === 'users_update') {
        handleUsersUpdate(message.payload)
      }
    })
  }

  function disconnect() {
    manualDisconnect = true
    clearReconnectTimer()
    clearBookshelfRefreshTimer()
    clearReplaceRulesUpdateTimer()
    clearRSSUpdateTimer()
    clearBookmarksUpdateTimer()
    socket?.close()
    socket = undefined
    connected.value = false
  }

  function send(type, payload) {
    if (socket?.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify({ type, payload }))
    }
  }

  return { connected, connect, disconnect, send }

  function scheduleReconnect() {
    if (manualDisconnect || reconnectTimer) return
    if (!localStorage.getItem('openreader_token')) return
    reconnectTimer = window.setTimeout(() => {
      reconnectTimer = undefined
      connect()
      reconnectDelay = Math.min(MAX_RECONNECT_DELAY, reconnectDelay * 1.7)
    }, reconnectDelay)
  }

  function clearReconnectTimer() {
    if (!reconnectTimer) return
    window.clearTimeout(reconnectTimer)
    reconnectTimer = undefined
  }

  function scheduleBookshelfRefresh(options = {}) {
    const refreshBooks = options.books !== false
    const refreshCategories = options.categories !== false
    bookshelfRefreshPending.books = bookshelfRefreshPending.books || refreshBooks
    bookshelfRefreshPending.categories = bookshelfRefreshPending.categories || refreshCategories
    if (bookshelfRefreshTimer) return
    bookshelfRefreshTimer = window.setTimeout(() => {
      bookshelfRefreshTimer = undefined
      const pending = bookshelfRefreshPending
      bookshelfRefreshPending = { books: false, categories: false }
      const jobs = []
      if (pending.categories) jobs.push(bookshelf.loadCategories({ force: true }))
      if (pending.books) jobs.push(bookshelf.loadBooks({ force: true, all: true }))
      Promise.all(jobs).catch(() => {})
    }, 500)
  }

  function clearBookshelfRefreshTimer() {
    if (!bookshelfRefreshTimer) return
    window.clearTimeout(bookshelfRefreshTimer)
    bookshelfRefreshTimer = undefined
    bookshelfRefreshPending = { books: false, categories: false }
  }

  function dispatchWindowEvent(name, detail) {
    if (typeof window === 'undefined') return
    window.dispatchEvent(new CustomEvent(name, { detail }))
  }

  function scheduleReplaceRulesUpdate(detail) {
    clearReplaceRulesUpdateTimer()
    replaceRulesUpdateTimer = window.setTimeout(() => {
      replaceRulesUpdateTimer = undefined
      dispatchWindowEvent('openreader:replace-rules-updated', detail)
    }, 500)
  }

  function clearReplaceRulesUpdateTimer() {
    if (!replaceRulesUpdateTimer) return
    window.clearTimeout(replaceRulesUpdateTimer)
    replaceRulesUpdateTimer = undefined
  }

  function scheduleRSSUpdate(detail = {}) {
    const kind = detail?.kind || ''
    rssUpdatePending.sources = rssUpdatePending.sources || kind.startsWith('source-')
    rssUpdatePending.articles = rssUpdatePending.articles || kind.startsWith('article-') || kind === 'source-refresh' || kind === 'source-delete'
    rssUpdatePending.payload = detail
    if (rssUpdateTimer) return
    rssUpdateTimer = window.setTimeout(() => {
      rssUpdateTimer = undefined
      const pending = rssUpdatePending
      rssUpdatePending = { sources: false, articles: false, payload: null }
      dispatchWindowEvent('openreader:rss-updated', pending)
    }, 500)
  }

  function clearRSSUpdateTimer() {
    if (!rssUpdateTimer) return
    window.clearTimeout(rssUpdateTimer)
    rssUpdateTimer = undefined
    rssUpdatePending = { sources: false, articles: false, payload: null }
  }

  function scheduleBookmarksUpdate(detail = {}) {
    const bookId = Number(detail?.bookId || 0)
    if (bookId) bookmarksUpdatePending.bookIds.add(bookId)
    bookmarksUpdatePending.payload = detail
    if (bookmarksUpdateTimer) return
    bookmarksUpdateTimer = window.setTimeout(() => {
      bookmarksUpdateTimer = undefined
      const pending = {
        bookIds: [...bookmarksUpdatePending.bookIds],
        payload: bookmarksUpdatePending.payload,
      }
      bookmarksUpdatePending = { bookIds: new Set(), payload: null }
      dispatchWindowEvent('openreader:bookmarks-updated', pending)
    }, 500)
  }

  function clearBookmarksUpdateTimer() {
    if (!bookmarksUpdateTimer) return
    window.clearTimeout(bookmarksUpdateTimer)
    bookmarksUpdateTimer = undefined
    bookmarksUpdatePending = { bookIds: new Set(), payload: null }
  }

  function handleUsersUpdate(detail = {}) {
    const userIds = Array.isArray(detail?.userIds) ? detail.userIds.map(Number).filter(Boolean) : []
    const currentId = Number(userStore.profile?.id || 0)
    dispatchWindowEvent('openreader:users-updated', detail)
    if (!currentId || (userIds.length && !userIds.includes(currentId))) return
    userStore.loadMe().catch(() => {
      if (['delete', 'cleanup'].includes(detail?.kind)) userStore.logout()
    })
  }

  function stripProgressClientId(progress = {}) {
    if (!progress || typeof progress !== 'object') return progress
    const { clientId, ...rest } = progress
    return rest
  }
}
