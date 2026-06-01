import { ref } from 'vue'
import { useReaderStore } from '../stores/reader'
import { useBookshelfStore } from '../stores/bookshelf'
import { usePreferencesStore } from '../stores/preferences'

const connected = ref(false)
let socket
let reconnectTimer
let reconnectDelay = 1500
let manualDisconnect = false
const MAX_RECONNECT_DELAY = 15000

export function useSync() {
  const reader = useReaderStore()
  const bookshelf = useBookshelfStore()
  const preferences = usePreferencesStore()

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
      bookshelf.loadBooks({ force: true, all: true }).catch(() => {})
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
        const progress = reader.applyServerProgress(message.payload) || message.payload
        bookshelf.applyBookProgress(progress, { replace: true })
      }
      if (message.type === 'bookshelf_update') {
        bookshelf.loadBooks({ force: true, all: true })
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
    })
  }

  function disconnect() {
    manualDisconnect = true
    clearReconnectTimer()
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
}
