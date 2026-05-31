import { ref } from 'vue'
import { useReaderStore } from '../stores/reader'
import { useBookshelfStore } from '../stores/bookshelf'

const connected = ref(false)
let socket

export function useSync() {
  const reader = useReaderStore()
  const bookshelf = useBookshelfStore()

  function connect() {
    const token = localStorage.getItem('openreader_token')
    if (!token || socket) return

    const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws'
    socket = new WebSocket(`${protocol}://${window.location.host}/ws/sync?token=${encodeURIComponent(token)}`)

    socket.addEventListener('open', () => {
      connected.value = true
    })
    socket.addEventListener('close', () => {
      connected.value = false
      socket = undefined
    })
    socket.addEventListener('message', (event) => {
      const message = JSON.parse(event.data)
      if (message.type === 'progress_update') {
        reader.applyProgress(message.payload)
        bookshelf.applyBookProgress(message.payload)
      }
      if (message.type === 'bookshelf_update') {
        bookshelf.loadBooks({ force: true, all: true })
      }
    })
  }

  function disconnect() {
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
}
